import Foundation

struct DeviceInput: Codable {
    let installationID: String
    let reportedName: String
    let platform: String
    let osVersion: String
    let appVersion: String

    enum CodingKeys: String, CodingKey {
        case installationID = "installation_id"
        case reportedName = "reported_name"
        case platform
        case osVersion = "os_version"
        case appVersion = "app_version"
    }
}

struct AuthRequest: Codable {
    let account: String
    let password: String
    let device: DeviceInput
}

struct UserModel: Codable {
    let id: String
    let account: String
}

struct SessionTokens: Codable {
    let accessToken: String
    let accessExpiresAt: String
    let refreshToken: String
    let refreshExpiresAt: String

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case accessExpiresAt = "access_expires_at"
        case refreshToken = "refresh_token"
        case refreshExpiresAt = "refresh_expires_at"
    }
}

struct DeviceModel: Codable, Identifiable, Hashable {
    let id: String
    let reportedName: String
    let customName: String?
    let displayName: String
    let platform: String
    let osVersion: String
    let appVersion: String
    let role: String?
    let firstLoginAt: String
    let lastLoginAt: String
    let lastSeenAt: String?
    let revokedAt: String?
    let loggedIn: Bool
    let online: Bool
    let current: Bool
    let canRevoke: Bool?
    let canChangeRole: Bool?

    var roleLabel: String {
        switch role {
        case "super_admin": return "超级管理员"
        case "admin": return "管理员"
        default: return "普通设备"
        }
    }

    enum CodingKeys: String, CodingKey {
        case id
        case reportedName = "reported_name"
        case customName = "custom_name"
        case displayName = "display_name"
        case platform
        case osVersion = "os_version"
        case appVersion = "app_version"
        case role
        case firstLoginAt = "first_login_at"
        case lastLoginAt = "last_login_at"
        case lastSeenAt = "last_seen_at"
        case revokedAt = "revoked_at"
        case loggedIn = "logged_in"
        case online
        case current
        case canRevoke = "can_revoke"
        case canChangeRole = "can_change_role"
    }
}

struct UpdateDeviceRoleRequest: Codable {
    let role: String
}

struct AuthResponse: Codable {
    let user: UserModel
    let device: DeviceModel
    let tokens: SessionTokens
}

struct RefreshRequest: Codable {
    let refreshToken: String

    enum CodingKeys: String, CodingKey {
        case refreshToken = "refresh_token"
    }
}

struct RefreshResponse: Codable { let tokens: SessionTokens }
struct DevicesResponse: Codable { let devices: [DeviceModel] }

struct ClipUpload: Codable, Equatable {
    let clientEventID: String
    let contentType: String
    let algorithm: String
    let nonce: String
    let ciphertext: String

    enum CodingKeys: String, CodingKey {
        case clientEventID = "client_event_id"
        case contentType = "content_type"
        case algorithm, nonce, ciphertext
    }
}

struct ClipEvent: Codable, Equatable {
    let eventID: String
    let clientEventID: String
    let seq: Int64
    let originDeviceID: String
    let originName: String
    let contentType: String
    let algorithm: String
    let nonce: String
    let ciphertext: String
    let createdAt: String
    let expiresAt: String

    enum CodingKeys: String, CodingKey {
        case eventID = "event_id"
        case clientEventID = "client_event_id"
        case seq
        case originDeviceID = "origin_device_id"
        case originName = "origin_name"
        case contentType = "content_type"
        case algorithm, nonce, ciphertext
        case createdAt = "created_at"
        case expiresAt = "expires_at"
    }
}

struct ClipCreateResponse: Codable { let event: ClipEvent; let status: String }
struct ClipsResponse: Codable { let clips: [ClipEvent] }
struct AckRequest: Codable { let seq: Int64 }
struct APNsTokenRequest: Codable { let token: String; let environment: String }

struct APIErrorEnvelope: Codable { let error: APIErrorBody }
struct APIErrorBody: Codable { let code: String; let message: String }
struct SocketEvent: Codable { let type: String }

struct SecretState: Codable {
    var accessToken: String
    var refreshToken: String
    let sharedKey: String
    let keyVersion: Int
}

struct PersistedState: Codable {
    var serverURL: String
    var account: String
    var installationID: String
    var userID: String?
    var deviceID: String?
    var lastSeq: Int64
    var lastAppliedSeq: Int64
    var syncEnabled: Bool
    var pendingUploads: [ClipUpload]
    var latestRemote: ClipEvent?

    static var initial: PersistedState {
        PersistedState(
            serverURL: "https://zhy.hair/fastcopy",
            account: "",
            installationID: UUID().uuidString.lowercased(),
            userID: nil,
            deviceID: nil,
            lastSeq: 0,
            lastAppliedSeq: 0,
            syncEnabled: true,
            pendingUploads: [],
            latestRemote: nil
        )
    }
}
