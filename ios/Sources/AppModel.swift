import Combine
import CryptoKit
import Foundation
import UIKit

@MainActor
final class AppModel: ObservableObject {
    @Published private(set) var initialized = false
    @Published private(set) var authenticated = false
    @Published private(set) var connected = false
    @Published private(set) var busy = false
    @Published private(set) var status = "正在准备"
    @Published private(set) var errorMessage: String?
    @Published private(set) var serverURL = "https://zhy.hair/fastcopy"
    @Published private(set) var account = ""
    @Published private(set) var syncEnabled = true
    @Published private(set) var pendingCount = 0
    @Published private(set) var latestText: String?
    @Published private(set) var latestOrigin: String?
    @Published private(set) var devices: [DeviceModel] = []

    private let stateStore = PersistentStateStore()
    private let secureStore = SecureStateStore()
    private var persisted: PersistedState
    private var secrets: SecretState?
    private var socket: WebSocketConnection?
    private var socketGeneration = 0
    private var reconnectTask: Task<Void, Never>?
    private var pasteboardObserver: NSObjectProtocol?
    private var foreground = false
    private var lastAppliedSequence: Int64 = 0
    private var ignoredPasteboardChange = -1
    private var lastLocalDigest: Data?
    private var flushing = false
    private var syncing = false
    private var resyncRequested = false
    private var pendingAPNsToken: (token: String, environment: String)?
    private var tokenRefreshTask: Task<SessionTokens, Error>?

    init() {
        persisted = stateStore.load()
        secrets = secureStore.load().flatMap { $0.keyVersion == ClipboardCrypto.keyVersion ? $0 : nil }
        serverURL = persisted.serverURL
        account = persisted.account
        syncEnabled = persisted.syncEnabled
        pendingCount = persisted.pendingUploads.count
        lastAppliedSequence = persisted.lastAppliedSeq
        authenticated = hasSession
        latestOrigin = persisted.latestRemote?.originName
        if hasSession, let event = persisted.latestRemote {
            latestText = try? ClipboardCrypto.decrypt(event, key: try sharedKey())
        }
        status = hasSession ? "等待连接" : "尚未登录"
        initialized = true
        bindPushCoordinator()
    }

    deinit {
        if let pasteboardObserver { NotificationCenter.default.removeObserver(pasteboardObserver) }
    }

    func authenticate(server: String, account inputAccount: String, password: String) async {
        guard !busy else { return }
        busy = true
        errorMessage = nil
        status = "正在登录"
        defer { busy = false }
        do {
            let normalizedServer = try normalizeServer(server)
            let normalizedAccount = inputAccount.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !normalizedAccount.isEmpty else { throw InputError("请输入账号") }
            guard password.count >= 4 else { throw InputError("密码至少需要 4 个字符") }
            let client = try makeAPIClient(server: normalizedServer)
            let response = try await client.authenticate(
                AuthRequest(
                    account: normalizedAccount,
                    password: password,
                    device: DeviceInput(
                        installationID: persisted.installationID,
                        reportedName: String(UIDevice.current.name.prefix(64)),
                        platform: "ios",
                        osVersion: UIDevice.current.systemVersion,
                        appVersion: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.1.0"
                    )
                )
            )
            let key = try ClipboardCrypto.deriveKey(account: response.user.account, password: password)
            let replacement = SecretState(
                accessToken: response.tokens.accessToken,
                refreshToken: response.tokens.refreshToken,
                sharedKey: key.base64EncodedString(),
                keyVersion: ClipboardCrypto.keyVersion
            )
            try secureStore.save(replacement)
            secrets = replacement
            persisted.serverURL = normalizedServer
            persisted.account = response.user.account
            persisted.userID = response.user.id
            persisted.deviceID = response.device.id
            persisted.lastSeq = 0
            persisted.lastAppliedSeq = 0
            persisted.pendingUploads = []
            persisted.latestRemote = nil
            saveState()
            serverURL = normalizedServer
            account = response.user.account
            pendingCount = 0
            latestText = nil
            latestOrigin = nil
            devices = []
            authenticated = true
            status = "正在同步"

            await PushCoordinator.shared.requestVisibleNotificationPermission()
            await uploadPushTokenIfPossible()
            if foreground, syncEnabled { startForegroundSync() }
            try await refreshDevices()
            _ = try await sync(writeClipboard: foreground)
        } catch {
            fail(error)
        }
    }

    func setForeground(_ isForeground: Bool) {
        foreground = isForeground
        guard hasSession, syncEnabled else { return }
        if isForeground {
            startForegroundSync()
            applyPersistedRemote()
            Task { [weak self] in
                guard let self else { return }
                do { _ = try await self.sync(writeClipboard: true) }
                catch { self.fail(error) }
            }
        } else {
            stopClipboardMonitoring()
            closeSocket()
        }
    }

    func sendCurrentClipboard() async {
        guard let text = UIPasteboard.general.string else {
            fail(InputError("当前剪贴板没有文本"))
            return
        }
        await queueLocalText(text, force: true)
    }

    func copyLatest() {
        guard let latestText else { return }
        writeClipboard(latestText)
        if let sequence = persisted.latestRemote?.seq {
            lastAppliedSequence = sequence
            persisted.lastAppliedSeq = sequence
            saveState()
        }
        status = "已复制到本机"
    }

    func refreshNow() async {
        guard !busy else { return }
        busy = true
        errorMessage = nil
        status = "正在同步"
        defer { busy = false }
        do {
            try await flushPending()
            _ = try await sync(writeClipboard: foreground)
            try await refreshDevices()
        } catch {
            fail(error)
        }
    }

    func refreshDevicesNow() async {
        do { try await refreshDevices() }
        catch { fail(error) }
    }

    func revoke(_ device: DeviceModel) async {
        guard device.canRevoke == true else { return }
        do {
            try await authorized { client, token in
                try await client.revoke(accessToken: token, deviceID: device.id)
            }
            try await refreshDevices()
        } catch {
            fail(error)
        }
    }

    func setRole(_ device: DeviceModel, role: String) async {
        guard device.canChangeRole == true, ["admin", "member"].contains(role) else { return }
        do {
            try await authorized { client, token in
                try await client.updateDeviceRole(accessToken: token, deviceID: device.id, role: role)
            }
            try await refreshDevices()
        } catch {
            fail(error)
        }
    }

    func setSyncEnabled(_ enabled: Bool) {
        syncEnabled = enabled
        persisted.syncEnabled = enabled
        saveState()
        if enabled, hasSession {
            status = "正在连接"
            if foreground { startForegroundSync() }
        } else {
            status = "同步已暂停"
            stopClipboardMonitoring()
            closeSocket()
        }
    }

    func logout() async {
        if hasSession {
            try? await authorized { client, token in
                try await client.deleteAPNsToken(accessToken: token)
            }
            try? await authorized { client, token in
                try await client.logout(accessToken: token)
            }
        }
        stopClipboardMonitoring()
        closeSocket()
        secureStore.clear()
        secrets = nil
        persisted.userID = nil
        persisted.deviceID = nil
        persisted.lastSeq = 0
        persisted.lastAppliedSeq = 0
        persisted.pendingUploads = []
        persisted.latestRemote = nil
        saveState()
        authenticated = false
        connected = false
        status = "尚未登录"
        errorMessage = nil
        pendingCount = 0
        latestText = nil
        latestOrigin = nil
        devices = []
    }

    func handleRemotePush() async -> Bool {
        guard hasSession, syncEnabled else { return false }
        do {
            return try await sync(writeClipboard: false)
        } catch {
            return false
        }
    }

    private func bindPushCoordinator() {
        let coordinator = PushCoordinator.shared
        coordinator.onToken = { [weak self] token, environment in
            guard let self else { return }
            self.pendingAPNsToken = (token, environment)
            Task { await self.uploadPushTokenIfPossible() }
        }
        coordinator.onRegistrationError = { [weak self] error in
            guard let self, self.authenticated else { return }
            self.errorMessage = "推送注册失败：\(error.localizedDescription)"
        }
        coordinator.onRemoteNotification = { [weak self] in
            guard let self else { return false }
            return await self.handleRemotePush()
        }
    }

    private func uploadPushTokenIfPossible() async {
        guard hasSession, let pendingAPNsToken else { return }
        do {
            try await authorized { client, accessToken in
                try await client.registerAPNsToken(
                    accessToken: accessToken,
                    token: pendingAPNsToken.token,
                    environment: pendingAPNsToken.environment
                )
            }
        } catch {
            errorMessage = "推送令牌上传失败：\(userMessage(error))"
        }
    }

    private func startForegroundSync() {
        startClipboardMonitoring()
        connectSocket()
    }

    private func startClipboardMonitoring() {
        guard pasteboardObserver == nil else { return }
        pasteboardObserver = NotificationCenter.default.addObserver(
            forName: UIPasteboard.changedNotification,
            object: UIPasteboard.general,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in await self?.clipboardChanged() }
        }
    }

    private func stopClipboardMonitoring() {
        guard let pasteboardObserver else { return }
        NotificationCenter.default.removeObserver(pasteboardObserver)
        self.pasteboardObserver = nil
    }

    private func clipboardChanged() async {
        let pasteboard = UIPasteboard.general
        guard pasteboard.changeCount != ignoredPasteboardChange,
              let text = pasteboard.string else { return }
        await queueLocalText(text, force: false)
    }

    private func queueLocalText(_ text: String, force: Bool) async {
        guard hasSession, syncEnabled else { return }
        let digest = Data(SHA256.hash(data: Data(text.utf8)))
        guard force || digest != lastLocalDigest else { return }
        lastLocalDigest = digest
        do {
            let upload = try ClipboardCrypto.encrypt(text, key: try sharedKey())
            persisted.pendingUploads = Array((persisted.pendingUploads + [upload]).suffix(100))
            saveState()
            pendingCount = persisted.pendingUploads.count
            status = "正在发送"
            errorMessage = nil
            try await flushPending()
        } catch {
            fail(error)
        }
    }

    private func flushPending() async throws {
        guard !flushing else { return }
        flushing = true
        defer { flushing = false }
        while let upload = persisted.pendingUploads.first {
            do {
                let _: ClipCreateResponse = try await authorized { client, token in
                    try await client.upload(accessToken: token, clip: upload)
                }
            } catch let error as AppAPIError where error.code == "CLIENT_EVENT_ID_REUSED" {
                // The server already accepted this idempotent event.
            }
            persisted.pendingUploads.removeFirst()
            saveState()
            pendingCount = persisted.pendingUploads.count
            status = "剪贴板已发送"
        }
    }

    private func sync(writeClipboard: Bool) async throws -> Bool {
        guard hasSession else { return false }
        if syncing {
            resyncRequested = true
            return false
        }
        syncing = true
        defer { syncing = false }
        var changed = false
        repeat {
            resyncRequested = false
            changed = try await syncPass(writeClipboard: writeClipboard) || changed
        } while resyncRequested
        return changed
    }

    private func syncPass(writeClipboard shouldWriteClipboard: Bool) async throws -> Bool {
        var cursor = persisted.lastSeq
        var newestRemote: ClipEvent?
        repeat {
            let response: ClipsResponse = try await authorized { client, token in
                try await client.clips(accessToken: token, after: cursor)
            }
            for event in response.clips {
                if event.originDeviceID != persisted.deviceID { newestRemote = event }
                cursor = max(cursor, event.seq)
            }
            if response.clips.count < 200 { break }
        } while true

        let newestText = try newestRemote.map { event in
            try ClipboardCrypto.decrypt(event, key: sharedKey())
        }
        if cursor > persisted.lastSeq {
            try await authorized { client, token in
                try await client.acknowledge(accessToken: token, sequence: cursor)
            }
            persisted.lastSeq = cursor
            if let newestRemote { persisted.latestRemote = newestRemote }
            saveState()
        }
        guard let newestRemote else {
            status = connected ? "已连接" : "同步完成"
            errorMessage = nil
            return false
        }
        guard let newestText else { return false }
        latestText = newestText
        latestOrigin = newestRemote.originName
        if shouldWriteClipboard, foreground {
            writeClipboard(newestText)
            lastAppliedSequence = newestRemote.seq
            persisted.lastAppliedSeq = newestRemote.seq
            saveState()
        }
        status = connected ? "已连接" : "同步完成"
        errorMessage = nil
        return true
    }

    private func handlePushedClip(_ event: ClipEvent) async throws {
        guard hasSession, syncEnabled else { return }
        if syncing {
            resyncRequested = true
            return
        }
        switch pushedClipAction(currentSeq: persisted.lastSeq, incomingSeq: event.seq) {
        case .ignore:
            return
        case .reconcile:
            _ = try await sync(writeClipboard: foreground)
            return
        case .apply:
            break
        }

        syncing = true
        defer {
            syncing = false
            if resyncRequested {
                resyncRequested = false
                Task { [weak self] in
                    guard let self else { return }
                    do { _ = try await self.sync(writeClipboard: self.foreground) }
                    catch { self.fail(error) }
                }
            }
        }

        let isRemote = event.originDeviceID != persisted.deviceID
        let text = isRemote ? try ClipboardCrypto.decrypt(event, key: sharedKey()) : nil
        persisted.lastSeq = event.seq
        if isRemote { persisted.latestRemote = event }
        saveState()
        try await authorized { client, token in
            try await client.acknowledge(accessToken: token, sequence: event.seq)
        }

        if let text {
            latestText = text
            latestOrigin = event.originName
            if foreground {
                writeClipboard(text)
                lastAppliedSequence = event.seq
                persisted.lastAppliedSeq = event.seq
                saveState()
            }
        }
        status = connected ? "已连接" : "同步完成"
        errorMessage = nil
    }

    private func applyPersistedRemote() {
        guard let event = persisted.latestRemote, event.seq > lastAppliedSequence,
              let key = try? sharedKey(),
              let text = try? ClipboardCrypto.decrypt(event, key: key) else { return }
        latestText = text
        latestOrigin = event.originName
        writeClipboard(text)
        lastAppliedSequence = event.seq
        persisted.lastAppliedSeq = event.seq
        saveState()
        status = "已复制到本机"
    }

    private func writeClipboard(_ text: String) {
        UIPasteboard.general.string = text
        ignoredPasteboardChange = UIPasteboard.general.changeCount
    }

    private func refreshDevices() async throws {
        guard hasSession else { return }
        let response: DevicesResponse = try await authorized { client, token in
            try await client.devices(accessToken: token)
        }
        devices = response.devices
    }

    private func connectSocket() {
        guard socket == nil, foreground, syncEnabled, hasSession,
              let accessToken = secrets?.accessToken,
              let client = try? makeAPIClient() else { return }
        socketGeneration += 1
        let generation = socketGeneration
        do {
            socket = try client.webSocket(
                accessToken: accessToken,
                onConnected: { [weak self] in
                    Task { @MainActor [weak self] in
                        guard let self, self.socketGeneration == generation else { return }
                        self.connected = true
                        self.status = "已连接"
                        self.errorMessage = nil
                    }
                },
                onEvent: { [weak self] event in
                    Task { @MainActor [weak self] in await self?.handleSocketEvent(event, generation: generation) }
                },
                onDisconnected: { [weak self] error in
                    Task { @MainActor [weak self] in self?.socketDisconnected(error, generation: generation) }
                }
            )
        } catch {
            socketDisconnected(error, generation: generation)
        }
    }

    private func handleSocketEvent(_ event: SocketEvent, generation: Int) async {
        guard generation == socketGeneration else { return }
        do {
            if event.type == "clip.created" {
                if let clip = event.data { try await handlePushedClip(clip) }
                else { _ = try await sync(writeClipboard: foreground) }
            }
            if event.type.hasPrefix("device.") { try await refreshDevices() }
        } catch {
            fail(error)
        }
    }

    private func socketDisconnected(_ error: Error?, generation: Int) {
        guard generation == socketGeneration else { return }
        socket = nil
        connected = false
        guard foreground, syncEnabled, hasSession else { return }
        status = "等待重连"
        reconnectTask?.cancel()
        reconnectTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(5))
            guard !Task.isCancelled, let self else { return }
            do { _ = try await self.sync(writeClipboard: self.foreground) }
            catch { self.errorMessage = self.userMessage(error) }
            self.connectSocket()
        }
    }

    private func closeSocket() {
        reconnectTask?.cancel()
        reconnectTask = nil
        socketGeneration += 1
        socket?.close()
        socket = nil
        connected = false
    }

    private func authorized<T>(_ operation: (APIClient, String) async throws -> T) async throws -> T {
        guard let current = secrets else { throw InputError("请重新登录") }
        let client = try makeAPIClient()
        do {
            return try await operation(client, current.accessToken)
        } catch let error as AppAPIError where error.unauthorized {
            let replacement = try await refreshedSecrets(after: current, client: client)
            return try await operation(client, replacement.accessToken)
        }
    }

    private func refreshedSecrets(after current: SecretState, client: APIClient) async throws -> SecretState {
        if let latest = secrets, latest.accessToken != current.accessToken { return latest }
        let task: Task<SessionTokens, Error>
        if let existing = tokenRefreshTask {
            task = existing
        } else {
            task = Task { try await client.refresh(current.refreshToken).tokens }
            tokenRefreshTask = task
        }
        do {
            let tokens = try await task.value
            if let latest = secrets, latest.accessToken != current.accessToken {
                tokenRefreshTask = nil
                return latest
            }
            let replacement = SecretState(
                accessToken: tokens.accessToken,
                refreshToken: tokens.refreshToken,
                sharedKey: current.sharedKey,
                keyVersion: current.keyVersion
            )
            try secureStore.save(replacement)
            secrets = replacement
            tokenRefreshTask = nil
            return replacement
        } catch {
            tokenRefreshTask = nil
            throw error
        }
    }

    private var hasSession: Bool {
        secrets != nil && persisted.userID != nil && persisted.deviceID != nil
    }

    private func sharedKey() throws -> Data {
        guard let value = secrets?.sharedKey, let key = Data(base64Encoded: value), key.count == 32 else {
            throw InputError("本地密钥不存在，请重新登录")
        }
        return key
    }

    private func makeAPIClient(server: String? = nil) throws -> APIClient {
        guard let url = URL(string: server ?? persisted.serverURL) else { throw InputError("服务端地址无效") }
        return APIClient(baseURL: url)
    }

    private func normalizeServer(_ value: String) throws -> String {
        let normalized = value.trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard let components = URLComponents(string: normalized),
              let scheme = components.scheme?.lowercased(),
              let host = components.host, !host.isEmpty,
              scheme == "https" || scheme == "http" else { throw InputError("服务端地址无效") }
        if scheme == "http", !["localhost", "127.0.0.1"].contains(host) {
            throw InputError("远程服务端必须使用 HTTPS")
        }
        return normalized
    }

    private func saveState() {
        stateStore.save(persisted)
    }

    private func fail(_ error: Error) {
        errorMessage = userMessage(error)
        status = "操作失败"
        busy = false
    }

    private func userMessage(_ error: Error) -> String {
        if let error = error as? AppAPIError {
            switch error.code {
            case "INVALID_CREDENTIALS": return "账号或密码不正确"
            case "REGISTRATION_LIMIT_REACHED": return "服务端已达到账号上限"
            case "RATE_LIMITED": return "尝试次数过多，请稍后再试"
            default: return error.message
            }
        }
        return error.localizedDescription
    }
}

private struct InputError: LocalizedError {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}
