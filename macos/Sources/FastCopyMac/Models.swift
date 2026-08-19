import Foundation

struct DeviceInput: Codable {
    let installationId: String
    let reportedName: String
    let platform: String
    let osVersion: String
    let appVersion: String
}

struct AuthRequest: Codable {
    let account: String
    let password: String
    let device: DeviceInput
}

struct User: Codable {
    let id: String
    let account: String
    let createdAt: String
}

struct Device: Codable, Identifiable {
    let id: String
    let installationId: String?
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
}

struct UpdateDeviceRoleRequest: Codable {
    let role: String
}

struct SessionTokens: Codable {
    let accessToken: String
    let accessExpiresAt: String
    let refreshToken: String
    let refreshExpiresAt: String
}

struct AuthResponse: Codable {
    let user: User
    let device: Device
    let tokens: SessionTokens
}

struct RefreshResponse: Codable {
    let tokens: SessionTokens
}

struct DevicesResponse: Codable {
    let devices: [Device]
}

struct ClipUpload: Codable, Equatable {
    let clientEventId: String
    let contentType: String
    let algorithm: String
    let nonce: String
    let ciphertext: String
}

struct ClipEvent: Codable {
    let eventId: String
    let clientEventId: String
    let seq: Int64
    let originDeviceId: String
    let originName: String
    let contentType: String
    let algorithm: String
    let nonce: String
    let ciphertext: String
    let createdAt: String
    let expiresAt: String
}

struct ClipCreateResponse: Codable {
    let event: ClipEvent
    let status: String
}

struct ClipsResponse: Codable {
    let clips: [ClipEvent]
}

struct APIErrorEnvelope: Codable {
    struct Detail: Codable {
        let code: String
        let message: String
    }

    let error: Detail
}

struct WebSocketEnvelope: Codable {
    let type: String
}
