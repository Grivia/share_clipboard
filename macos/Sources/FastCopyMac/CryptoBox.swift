import CryptoKit
import Foundation

enum CryptoBoxError: LocalizedError {
    case invalidKey
    case invalidEnvelope
    case invalidText

    var errorDescription: String? {
        switch self {
        case .invalidKey:
            return "本地加密密钥无效，请重新登录"
        case .invalidEnvelope:
            return "收到的加密数据格式无效"
        case .invalidText:
            return "收到的数据不是有效的 UTF-8 文本"
        }
    }
}

struct CryptoBox {
    static func normalizedKey(_ value: String) throws -> String {
        let compact = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let data = Data(base64Encoded: compact), data.count == 32 else {
            throw CryptoBoxError.invalidKey
        }
        return data.base64EncodedString()
    }

    static func encrypt(_ text: String, keyBase64: String, clientEventId: String) throws -> ClipUpload {
        guard let keyData = Data(base64Encoded: keyBase64), keyData.count == 32 else {
            throw CryptoBoxError.invalidKey
        }
        let nonce = AES.GCM.Nonce()
        let aad = Data("fastcopy:v1|\(clientEventId)|text/plain".utf8)
        let sealed = try AES.GCM.seal(
            Data(text.utf8),
            using: SymmetricKey(data: keyData),
            nonce: nonce,
            authenticating: aad
        )
        let encrypted = sealed.ciphertext + sealed.tag
        return ClipUpload(
            clientEventId: clientEventId,
            contentType: "text/plain",
            algorithm: "AES-256-GCM",
            nonce: Data(nonce).base64EncodedString(),
            ciphertext: encrypted.base64EncodedString()
        )
    }

    static func decrypt(_ event: ClipEvent, keyBase64: String) throws -> String {
        guard let keyData = Data(base64Encoded: keyBase64), keyData.count == 32 else {
            throw CryptoBoxError.invalidKey
        }
        guard let nonceData = Data(base64Encoded: event.nonce), nonceData.count == 12,
              let encrypted = Data(base64Encoded: event.ciphertext), encrypted.count >= 16 else {
            throw CryptoBoxError.invalidEnvelope
        }
        let tagStart = encrypted.index(encrypted.endIndex, offsetBy: -16)
        let box = try AES.GCM.SealedBox(
            nonce: AES.GCM.Nonce(data: nonceData),
            ciphertext: encrypted[..<tagStart],
            tag: encrypted[tagStart...]
        )
        let aad = Data("fastcopy:v1|\(event.clientEventId)|text/plain".utf8)
        let plaintext = try AES.GCM.open(
            box,
            using: SymmetricKey(data: keyData),
            authenticating: aad
        )
        guard let text = String(data: plaintext, encoding: .utf8) else {
            throw CryptoBoxError.invalidText
        }
        return text
    }
}
