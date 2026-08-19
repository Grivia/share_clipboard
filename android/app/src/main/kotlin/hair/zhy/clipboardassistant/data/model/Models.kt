package hair.zhy.clipboardassistant.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class DeviceInput(
    @SerialName("installation_id") val installationId: String,
    @SerialName("reported_name") val reportedName: String,
    val platform: String,
    @SerialName("os_version") val osVersion: String,
    @SerialName("app_version") val appVersion: String,
)

@Serializable
data class AuthRequest(
    val account: String,
    val password: String,
    val device: DeviceInput,
)

@Serializable
data class UserModel(val id: String, val account: String)

@Serializable
data class SessionTokens(
    @SerialName("access_token") val accessToken: String,
    @SerialName("access_expires_at") val accessExpiresAt: String,
    @SerialName("refresh_token") val refreshToken: String,
    @SerialName("refresh_expires_at") val refreshExpiresAt: String,
)

@Serializable
data class DeviceModel(
    val id: String,
    @SerialName("reported_name") val reportedName: String,
    @SerialName("custom_name") val customName: String? = null,
    @SerialName("display_name") val displayName: String,
    val platform: String,
    @SerialName("os_version") val osVersion: String = "",
    @SerialName("app_version") val appVersion: String = "",
    val role: String = "member",
    @SerialName("first_login_at") val firstLoginAt: String,
    @SerialName("last_login_at") val lastLoginAt: String,
    @SerialName("last_seen_at") val lastSeenAt: String? = null,
    @SerialName("revoked_at") val revokedAt: String? = null,
    @SerialName("logged_in") val loggedIn: Boolean = false,
    val online: Boolean = false,
    val current: Boolean = false,
    @SerialName("can_revoke") val canRevoke: Boolean = false,
    @SerialName("can_change_role") val canChangeRole: Boolean = false,
)

@Serializable
data class UpdateDeviceRoleRequest(val role: String)

@Serializable
data class AuthResponse(
    val user: UserModel,
    val device: DeviceModel,
    val tokens: SessionTokens,
)

@Serializable
data class RefreshRequest(@SerialName("refresh_token") val refreshToken: String)

@Serializable
data class RefreshResponse(val tokens: SessionTokens)

@Serializable
data class DevicesResponse(val devices: List<DeviceModel>)

@Serializable
data class ClipUpload(
    @SerialName("client_event_id") val clientEventId: String,
    @SerialName("content_type") val contentType: String = "text/plain",
    val algorithm: String = "AES-256-GCM",
    val nonce: String,
    val ciphertext: String,
)

@Serializable
data class ClipEvent(
    @SerialName("event_id") val eventId: String,
    @SerialName("client_event_id") val clientEventId: String,
    val seq: Long,
    @SerialName("origin_device_id") val originDeviceId: String,
    @SerialName("origin_name") val originName: String,
    @SerialName("content_type") val contentType: String,
    val algorithm: String,
    val nonce: String,
    val ciphertext: String,
    @SerialName("created_at") val createdAt: String,
    @SerialName("expires_at") val expiresAt: String,
)

@Serializable
data class ClipCreateResponse(val event: ClipEvent, val status: String)

@Serializable
data class ClipsResponse(val clips: List<ClipEvent>)

@Serializable
data class AckRequest(val seq: Long)

@Serializable
data class ErrorEnvelope(val error: ApiErrorBody)

@Serializable
data class ApiErrorBody(val code: String, val message: String)

@Serializable
data class SocketEvent(val type: String)

@Serializable
data class SecretState(
    val accessToken: String,
    val refreshToken: String,
    val sharedKey: String,
    val keyVersion: Int = 1,
)

@Serializable
data class PersistedState(
    val serverUrl: String = "https://zhy.hair/fastcopy",
    val account: String = "",
    val installationId: String,
    val userId: String? = null,
    val deviceId: String? = null,
    val lastSeq: Long = 0,
    val lastAppliedSeq: Long = 0,
    val syncEnabled: Boolean = true,
    val pendingUploads: List<ClipUpload> = emptyList(),
    val latestRemote: ClipEvent? = null,
)
