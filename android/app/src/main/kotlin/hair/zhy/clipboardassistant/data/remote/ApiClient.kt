package hair.zhy.clipboardassistant.data.remote

import hair.zhy.clipboardassistant.data.model.AckRequest
import hair.zhy.clipboardassistant.data.model.AuthRequest
import hair.zhy.clipboardassistant.data.model.AuthResponse
import hair.zhy.clipboardassistant.data.model.ClipCreateResponse
import hair.zhy.clipboardassistant.data.model.ClipUpload
import hair.zhy.clipboardassistant.data.model.ClipsResponse
import hair.zhy.clipboardassistant.data.model.DevicesResponse
import hair.zhy.clipboardassistant.data.model.ErrorEnvelope
import hair.zhy.clipboardassistant.data.model.RefreshRequest
import hair.zhy.clipboardassistant.data.model.RefreshResponse
import hair.zhy.clipboardassistant.data.model.SocketEvent
import hair.zhy.clipboardassistant.data.model.UpdateDeviceRoleRequest
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener

class ApiException(val status: Int, val code: String, override val message: String) : IOException(message) {
    val unauthorized: Boolean get() = status == 401
}

class ApiClient(
    private val baseUrl: String,
    private val http: OkHttpClient,
    private val json: Json,
) {
    private val jsonType = "application/json; charset=utf-8".toMediaType()

    suspend fun authenticate(request: AuthRequest): AuthResponse =
        call("POST", "/v1/auth/session", body = json.encodeToString(request))

    suspend fun refresh(refreshToken: String): RefreshResponse =
        call("POST", "/v1/auth/refresh", body = json.encodeToString(RefreshRequest(refreshToken)))

    suspend fun devices(accessToken: String): DevicesResponse =
        call("GET", "/v1/devices", accessToken)

    suspend fun upload(accessToken: String, upload: ClipUpload): ClipCreateResponse =
        call("POST", "/v1/clips", accessToken, json.encodeToString(upload))

    suspend fun clips(accessToken: String, afterSeq: Long): ClipsResponse =
        call("GET", "/v1/clips?after_seq=$afterSeq&limit=200", accessToken)

    suspend fun acknowledge(accessToken: String, seq: Long) {
        callWithoutResponse("POST", "/v1/sync/ack", accessToken, json.encodeToString(AckRequest(seq)))
    }

    suspend fun revoke(accessToken: String, deviceId: String) {
        callWithoutResponse("POST", "/v1/devices/$deviceId/revoke", accessToken)
    }

    suspend fun updateDeviceRole(accessToken: String, deviceId: String, role: String) {
        callWithoutResponse(
            "PATCH",
            "/v1/devices/$deviceId/role",
            accessToken,
            json.encodeToString(UpdateDeviceRoleRequest(role)),
        )
    }

    suspend fun logout(accessToken: String) {
        callWithoutResponse("POST", "/v1/auth/logout", accessToken)
    }

    fun webSocket(
        accessToken: String,
        onConnected: () -> Unit,
        onEvent: (SocketEvent) -> Unit,
        onDisconnected: (Throwable?) -> Unit,
    ): WebSocket {
        val wsBase = when {
            baseUrl.startsWith("https://") -> "wss://${baseUrl.removePrefix("https://")}"
            baseUrl.startsWith("http://") -> "ws://${baseUrl.removePrefix("http://")}"
            else -> baseUrl
        }
        val request = Request.Builder()
            .url("${wsBase.trimEnd('/')}/v1/events/ws")
            .header("Authorization", "Bearer $accessToken")
            .header("User-Agent", "ClipboardAssistantAndroid/0.1.1")
            .build()
        return http.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) = onConnected()

            override fun onMessage(webSocket: WebSocket, text: String) {
                runCatching { json.decodeFromString<SocketEvent>(text) }.getOrNull()?.let(onEvent)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) = onDisconnected(null)

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) = onDisconnected(t)
        })
    }

    private suspend inline fun <reified T> call(
        method: String,
        path: String,
        accessToken: String? = null,
        body: String? = null,
    ): T = withContext(Dispatchers.IO) {
        val response = http.newCall(request(method, path, accessToken, body)).execute()
        response.use {
            val responseBody = it.body.string()
            if (!it.isSuccessful) throw apiError(it.code, responseBody)
            json.decodeFromString<T>(responseBody)
        }
    }

    private suspend fun callWithoutResponse(
        method: String,
        path: String,
        accessToken: String? = null,
        body: String? = null,
    ) = withContext(Dispatchers.IO) {
        http.newCall(request(method, path, accessToken, body)).execute().use {
            val responseBody = it.body.string()
            if (!it.isSuccessful) throw apiError(it.code, responseBody)
        }
    }

    private fun request(method: String, path: String, accessToken: String?, body: String?): Request {
        val builder = Request.Builder()
            .url("${baseUrl.trimEnd('/')}$path")
            .header("Accept", "application/json")
            .header("User-Agent", "ClipboardAssistantAndroid/0.1.1")
        if (accessToken != null) builder.header("Authorization", "Bearer $accessToken")
        val requestBody = when {
            body != null -> body.toRequestBody(jsonType)
            method in setOf("POST", "PUT", "PATCH") -> ByteArray(0).toRequestBody(null)
            else -> null
        }
        return builder.method(method, requestBody).build()
    }

    private fun apiError(status: Int, body: String): ApiException {
        val error = runCatching { json.decodeFromString<ErrorEnvelope>(body).error }.getOrNull()
        return ApiException(status, error?.code ?: "HTTP_$status", error?.message ?: "服务器请求失败")
    }
}
