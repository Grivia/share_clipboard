package hair.zhy.clipboardassistant.data

import android.os.Build
import hair.zhy.clipboardassistant.BuildConfig
import hair.zhy.clipboardassistant.crypto.ClipboardCrypto
import hair.zhy.clipboardassistant.data.local.SecretStore
import hair.zhy.clipboardassistant.data.local.StateStore
import hair.zhy.clipboardassistant.data.model.AuthRequest
import hair.zhy.clipboardassistant.data.model.ClipEvent
import hair.zhy.clipboardassistant.data.model.DeviceInput
import hair.zhy.clipboardassistant.data.model.DeviceModel
import hair.zhy.clipboardassistant.data.model.PersistedState
import hair.zhy.clipboardassistant.data.model.SecretState
import hair.zhy.clipboardassistant.data.remote.ApiClient
import hair.zhy.clipboardassistant.data.remote.ApiException
import hair.zhy.clipboardassistant.platform.ClipboardController
import hair.zhy.clipboardassistant.platform.NotificationController
import hair.zhy.clipboardassistant.sync.SyncScheduler
import java.net.URI
import java.security.MessageDigest
import java.util.Base64
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.WebSocket

data class RepositoryState(
    val initialized: Boolean = false,
    val authenticated: Boolean = false,
    val connected: Boolean = false,
    val busy: Boolean = false,
    val status: String = "正在准备",
    val error: String? = null,
    val serverUrl: String = "https://zhy.hair/fastcopy",
    val account: String = "",
    val syncEnabled: Boolean = true,
    val pendingCount: Int = 0,
    val latestText: String? = null,
    val latestOrigin: String? = null,
    val devices: List<DeviceModel> = emptyList(),
)

class ClipboardRepository(
    private val stateStore: StateStore,
    private val secretStore: SecretStore,
    private val apiFactory: (String) -> ApiClient,
    private val clipboard: ClipboardController,
    private val notifications: NotificationController,
    private val scheduler: SyncScheduler,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val ready = CompletableDeferred<Unit>()
    private val operationMutex = Mutex()
    private val authMutex = Mutex()
    private val _state = MutableStateFlow(RepositoryState())
    val state: StateFlow<RepositoryState> = _state.asStateFlow()

    private lateinit var persisted: PersistedState
    private var secrets: SecretState? = null
    private var socket: WebSocket? = null
    private var reconnectJob: Job? = null
    private var foreground = false
    private var lastAppliedSeq = 0L
    private var lastLocalDigest: ByteArray? = null

    init {
        scope.launch { initialize() }
    }

    private suspend fun initialize() {
        persisted = stateStore.load()
        lastAppliedSeq = persisted.lastAppliedSeq
        secrets = secretStore.load()?.takeIf { it.keyVersion == 1 }
        val authenticated = hasSession()
        val latestText = if (authenticated) decryptLatestOrNull() else null
        _state.value = RepositoryState(
            initialized = true,
            authenticated = authenticated,
            status = if (authenticated) "等待连接" else "尚未登录",
            serverUrl = persisted.serverUrl,
            account = persisted.account,
            syncEnabled = persisted.syncEnabled,
            pendingCount = persisted.pendingUploads.size,
            latestText = latestText,
            latestOrigin = persisted.latestRemote?.originName,
        )
        if (authenticated && persisted.syncEnabled) scheduler.schedule()
        ready.complete(Unit)
    }

    suspend fun authenticate(serverUrl: String, account: String, password: String) {
        ready.await()
        update { it.copy(busy = true, error = null, status = "正在登录") }
        runCatching {
            val server = normalizeServer(serverUrl)
            val normalizedAccount = account.trim()
            require(normalizedAccount.isNotEmpty()) { "请输入账号" }
            require(password.length >= 4) { "密码至少需要 4 个字符" }
            val response = apiFactory(server).authenticate(
                AuthRequest(
                    account = normalizedAccount,
                    password = password,
                    device = DeviceInput(
                        installationId = persisted.installationId,
                        reportedName = "${Build.MANUFACTURER} ${Build.MODEL}".trim().take(64),
                        platform = "android",
                        osVersion = Build.VERSION.RELEASE,
                        appVersion = BuildConfig.VERSION_NAME,
                    ),
                ),
            )
            val key = ClipboardCrypto.deriveKey(response.user.account, password)
            secrets = SecretState(
                accessToken = response.tokens.accessToken,
                refreshToken = response.tokens.refreshToken,
                sharedKey = Base64.getEncoder().encodeToString(key),
            ).also(secretStore::save)
            persisted = persisted.copy(
                serverUrl = server,
                account = response.user.account,
                userId = response.user.id,
                deviceId = response.device.id,
                lastSeq = 0,
                lastAppliedSeq = 0,
                pendingUploads = emptyList(),
                latestRemote = null,
            )
            stateStore.save(persisted)
            scheduler.schedule()
            update {
                it.copy(
                    authenticated = true,
                    busy = false,
                    status = "正在同步",
                    serverUrl = server,
                    account = response.user.account,
                    pendingCount = 0,
                    latestText = null,
                    latestOrigin = null,
                )
            }
            if (foreground && persisted.syncEnabled) startForegroundSync()
            refreshDevices()
            sync(writeClipboard = foreground, notify = false)
        }.onFailure(::setFailure)
        update { it.copy(busy = false) }
    }

    fun setForeground(isForeground: Boolean) {
        scope.launch {
            ready.await()
            foreground = isForeground
            if (!hasSession() || !persisted.syncEnabled) return@launch
            if (isForeground) {
                startForegroundSync()
                applyPersistedRemote()
                sync(writeClipboard = true, notify = false)
            } else {
                clipboard.stop()
                closeSocket()
            }
        }
    }

    fun sendCurrentClipboard() {
        scope.launch {
            ready.await()
            val text = clipboard.readText()
            if (text == null) setFailure(IllegalStateException("当前剪贴板没有文本"))
            else queueLocalText(text, force = true)
        }
    }

    fun copyLatest() {
        val text = _state.value.latestText ?: return
        clipboard.writeRemote(text)
        scope.launch {
            persisted.latestRemote?.seq?.let { sequence ->
                lastAppliedSeq = sequence
                persisted = persisted.copy(lastAppliedSeq = sequence)
                stateStore.save(persisted)
            }
            update { it.copy(status = "已复制到本机") }
        }
    }

    fun refreshNow() {
        scope.launch {
            ready.await()
            update { it.copy(busy = true, error = null, status = "正在同步") }
            runCatching {
                flushPending()
                sync(writeClipboard = foreground, notify = false)
                refreshDevices()
            }.onFailure(::setFailure)
            update { it.copy(busy = false) }
        }
    }

    suspend fun backgroundSync(): Boolean {
        ready.await()
        if (!hasSession() || !persisted.syncEnabled) return true
        return runCatching {
            flushPending()
            sync(writeClipboard = false, notify = true)
        }.isSuccess
    }

    fun setSyncEnabled(enabled: Boolean) {
        scope.launch {
            ready.await()
            persisted = persisted.copy(syncEnabled = enabled)
            stateStore.save(persisted)
            update { it.copy(syncEnabled = enabled, status = if (enabled) "正在连接" else "同步已暂停") }
            if (enabled && hasSession()) {
                scheduler.schedule()
                if (foreground) startForegroundSync()
            } else {
                scheduler.cancel()
                clipboard.stop()
                closeSocket()
            }
        }
    }

    fun refreshDevicesAsync() {
        scope.launch { runCatching { refreshDevices() }.onFailure(::setFailure) }
    }

    fun revokeDevice(device: DeviceModel) {
        if (device.current || device.revokedAt != null) return
        scope.launch {
            runCatching {
                authorized { api, token -> api.revoke(token, device.id) }
                refreshDevices()
            }.onFailure(::setFailure)
        }
    }

    fun logout() {
        scope.launch {
            ready.await()
            runCatching {
                if (hasSession()) authorized { api, token -> api.logout(token) }
            }
            clipboard.stop()
            closeSocket()
            scheduler.cancel()
            secretStore.clear()
            secrets = null
            persisted = persisted.copy(
                userId = null,
                deviceId = null,
                lastSeq = 0,
                lastAppliedSeq = 0,
                pendingUploads = emptyList(),
                latestRemote = null,
            )
            stateStore.save(persisted)
            _state.value = RepositoryState(
                initialized = true,
                status = "尚未登录",
                serverUrl = persisted.serverUrl,
                account = persisted.account,
                syncEnabled = persisted.syncEnabled,
            )
        }
    }

    private fun startForegroundSync() {
        clipboard.start { text -> scope.launch { queueLocalText(text, force = false) } }
        connectSocket()
    }

    private fun connectSocket() {
        if (socket != null || !foreground || !hasSession() || !persisted.syncEnabled) return
        val accessToken = secrets?.accessToken ?: return
        socket = apiFactory(persisted.serverUrl).webSocket(
            accessToken = accessToken,
            onConnected = { update { it.copy(connected = true, status = "已连接", error = null) } },
            onEvent = { event ->
                if (event.type == "clip.created") scope.launch { sync(writeClipboard = foreground, notify = false) }
                if (event.type.startsWith("device.")) scope.launch { refreshDevices() }
            },
            onDisconnected = {
                socket = null
                update { state -> state.copy(connected = false, status = "等待重连") }
                scheduleReconnect()
            },
        )
    }

    private fun scheduleReconnect() {
        reconnectJob?.cancel()
        if (!foreground || !persisted.syncEnabled || !hasSession()) return
        reconnectJob = scope.launch {
            delay(5_000)
            runCatching { sync(writeClipboard = foreground, notify = false) }
                .onFailure(::setFailure)
            connectSocket()
        }
    }

    private fun closeSocket() {
        reconnectJob?.cancel()
        reconnectJob = null
        socket?.close(1000, "background")
        socket = null
        update { it.copy(connected = false) }
    }

    private suspend fun queueLocalText(text: String, force: Boolean) {
        if (!hasSession() || !persisted.syncEnabled) return
        val digest = MessageDigest.getInstance("SHA-256").digest(text.encodeToByteArray())
        if (!force && lastLocalDigest?.contentEquals(digest) == true) return
        lastLocalDigest = digest
        runCatching {
            val key = sharedKey()
            val upload = ClipboardCrypto.encrypt(text, key)
            persisted = persisted.copy(pendingUploads = (persisted.pendingUploads + upload).takeLast(100))
            stateStore.save(persisted)
            update { it.copy(pendingCount = persisted.pendingUploads.size, status = "正在发送", error = null) }
            flushPending()
        }.onFailure(::setFailure)
    }

    private suspend fun flushPending() = operationMutex.withLock {
        while (persisted.pendingUploads.isNotEmpty()) {
            val upload = persisted.pendingUploads.first()
            try {
                authorized { api, token -> api.upload(token, upload) }
            } catch (error: ApiException) {
                if (error.code != "CLIENT_EVENT_ID_REUSED") throw error
            }
            persisted = persisted.copy(pendingUploads = persisted.pendingUploads.drop(1))
            stateStore.save(persisted)
            update { it.copy(pendingCount = persisted.pendingUploads.size, status = "剪贴板已发送") }
        }
    }

    private suspend fun sync(writeClipboard: Boolean, notify: Boolean) = operationMutex.withLock {
        if (!hasSession()) return@withLock
        var cursor = persisted.lastSeq
        var newestRemote: ClipEvent? = null
        while (true) {
            val response = authorized { api, token -> api.clips(token, cursor) }
            for (event in response.clips) {
                if (event.originDeviceId != persisted.deviceId) newestRemote = event
                if (event.seq > cursor) cursor = event.seq
            }
            if (response.clips.size < 200) break
        }
        val newestText = newestRemote?.let { ClipboardCrypto.decrypt(it, sharedKey()) }
        if (cursor > persisted.lastSeq) {
            authorized { api, token -> api.acknowledge(token, cursor) }
            persisted = persisted.copy(lastSeq = cursor, latestRemote = newestRemote ?: persisted.latestRemote)
            stateStore.save(persisted)
        }
        if (newestRemote != null && newestText != null) {
            update { it.copy(latestText = newestText, latestOrigin = newestRemote.originName) }
            if (writeClipboard) {
                clipboard.writeRemote(newestText)
                lastAppliedSeq = newestRemote.seq
                persisted = persisted.copy(lastAppliedSeq = newestRemote.seq)
                stateStore.save(persisted)
            } else if (notify) {
                notifications.notifyRemoteClip(newestRemote.originName)
            }
        }
        update { it.copy(status = if (_state.value.connected) "已连接" else "同步完成", error = null) }
    }

    private suspend fun applyPersistedRemote() {
        val event = persisted.latestRemote ?: return
        if (event.seq <= lastAppliedSeq) return
        val text = runCatching { ClipboardCrypto.decrypt(event, sharedKey()) }.getOrNull() ?: return
        clipboard.writeRemote(text)
        lastAppliedSeq = event.seq
        persisted = persisted.copy(lastAppliedSeq = event.seq)
        stateStore.save(persisted)
        update { it.copy(latestText = text, latestOrigin = event.originName, status = "已复制到本机") }
    }

    private suspend fun refreshDevices() {
        if (!hasSession()) return
        val devices = authorized { api, token -> api.devices(token).devices }
        update { it.copy(devices = devices) }
    }

    private suspend fun <T> authorized(operation: suspend (ApiClient, String) -> T): T {
        val current = secrets ?: throw IllegalStateException("请重新登录")
        val api = apiFactory(persisted.serverUrl)
        try {
            return operation(api, current.accessToken)
        } catch (error: ApiException) {
            if (!error.unauthorized) throw error
        }
        return authMutex.withLock {
            val latest = secrets ?: throw IllegalStateException("请重新登录")
            if (latest.accessToken != current.accessToken) {
                return@withLock operation(api, latest.accessToken)
            }
            val refreshed = api.refresh(latest.refreshToken).tokens
            val replacement = latest.copy(
                accessToken = refreshed.accessToken,
                refreshToken = refreshed.refreshToken,
            )
            secrets = replacement
            secretStore.save(replacement)
            operation(api, replacement.accessToken)
        }
    }

    private fun hasSession(): Boolean =
        secrets != null && persisted.userId != null && persisted.deviceId != null

    private fun sharedKey(): ByteArray {
        val encoded = secrets?.sharedKey ?: throw IllegalStateException("本地密钥不存在")
        return Base64.getDecoder().decode(encoded)
    }

    private fun decryptLatestOrNull(): String? {
        val event = persisted.latestRemote ?: return null
        return runCatching { ClipboardCrypto.decrypt(event, sharedKey()) }.getOrNull()
    }

    private fun normalizeServer(value: String): String {
        val trimmed = value.trim().trimEnd('/')
        val uri = runCatching { URI(trimmed) }.getOrNull()
            ?: throw IllegalArgumentException("服务端地址无效")
        require(uri.scheme == "https" || uri.scheme == "http") { "服务端地址无效" }
        require(!uri.host.isNullOrBlank()) { "服务端地址无效" }
        if (uri.scheme == "http") {
            require(uri.host in setOf("localhost", "127.0.0.1", "10.0.2.2")) { "远程服务端必须使用 HTTPS" }
        }
        return trimmed
    }

    private fun setFailure(error: Throwable) {
        update { it.copy(error = userMessage(error), status = "操作失败", busy = false) }
    }

    private fun userMessage(error: Throwable): String = when (error) {
        is ApiException -> when (error.code) {
            "INVALID_CREDENTIALS" -> "账号或密码不正确"
            "REGISTRATION_LIMIT_REACHED" -> "服务端已达到账号上限"
            "RATE_LIMITED" -> "尝试次数过多，请稍后再试"
            else -> error.message
        }
        else -> error.message ?: "操作失败"
    }

    private inline fun update(transform: (RepositoryState) -> RepositoryState) {
        _state.update(transform)
    }
}
