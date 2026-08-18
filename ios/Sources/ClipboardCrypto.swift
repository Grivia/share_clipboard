import CommonCrypto
import CryptoKit
import Foundation

enum ClipboardCryptoError: LocalizedError {
    case keyDerivation(Int32)
    case invalidEnvelope
    case textTooLarge

    var errorDescription: String? {
        switch self {
        case .keyDerivation: return "无法生成本地加密密钥"
        case .invalidEnvelope: return "剪贴板密文无效"
        case .textTooLarge: return "剪贴板文本超过 256 KiB"
        }
    }
}

enum ClipboardCrypto {
    static let keyVersion = 1
    static let maxPlaintextBytes = 256 * 1024 - 16

    static func deriveKey(account: String, password: String) throws -> Data {
        let salt = Data(SHA256.hash(data: Data("fastcopy:key-salt:v1|\(account)".utf8)))
        let passwordData = Data(password.utf8)
        var output = Data(count: 32)
        let outputCount = output.count
        let status = passwordData.withUnsafeBytes { passwordBytes in
            salt.withUnsafeBytes { saltBytes in
                output.withUnsafeMutableBytes { outputBytes in
                    CCKeyDerivationPBKDF(
                        CCPBKDFAlgorithm(kCCPBKDF2),
                        passwordBytes.bindMemory(to: Int8.self).baseAddress,
                        passwordData.count,
                        saltBytes.bindMemory(to: UInt8.self).baseAddress,
                        salt.count,
                        CCPseudoRandomAlgorithm(kCCPRFHmacAlgSHA256),
                        210_000,
                        outputBytes.bindMemory(to: UInt8.self).baseAddress,
                        outputCount
                    )
                }
            }
        }
        guard status == kCCSuccess else { throw ClipboardCryptoError.keyDerivation(status) }
        return output
    }

    static func encrypt(_ text: String, key: Data, eventID: String = UUID().uuidString.lowercased()) throws -> ClipUpload {
        let plaintext = Data(text.utf8)
        guard plaintext.count <= maxPlaintextBytes else { throw ClipboardCryptoError.textTooLarge }
        let nonce = AES.GCM.Nonce()
        let sealed = try AES.GCM.seal(
            plaintext,
            using: SymmetricKey(data: key),
            nonce: nonce,
            authenticating: additionalData(eventID: eventID)
        )
        return ClipUpload(
            clientEventID: eventID,
            contentType: "text/plain",
            algorithm: "AES-256-GCM",
            nonce: Data(nonce).base64EncodedString(),
            ciphertext: (sealed.ciphertext + sealed.tag).base64EncodedString()
        )
    }

    static func decrypt(_ event: ClipEvent, key: Data) throws -> String {
        guard event.contentType == "text/plain",
              event.algorithm == "AES-256-GCM",
              let nonceData = Data(base64Encoded: event.nonce),
              let encrypted = Data(base64Encoded: event.ciphertext),
              nonceData.count == 12,
              encrypted.count >= 16 else {
            throw ClipboardCryptoError.invalidEnvelope
        }
        let ciphertext = encrypted.dropLast(16)
        let tag = encrypted.suffix(16)
        let box = try AES.GCM.SealedBox(
            nonce: AES.GCM.Nonce(data: nonceData),
            ciphertext: ciphertext,
            tag: tag
        )
        let plaintext = try AES.GCM.open(
            box,
            using: SymmetricKey(data: key),
            authenticating: additionalData(eventID: event.clientEventID)
        )
        guard let text = String(data: plaintext, encoding: .utf8) else {
            throw ClipboardCryptoError.invalidEnvelope
        }
        return text
    }

    private static func additionalData(eventID: String) -> Data {
        Data("fastcopy:v1|\(eventID)|text/plain".utf8)
    }
}
