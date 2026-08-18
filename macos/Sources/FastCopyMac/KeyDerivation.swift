import CommonCrypto
import CryptoKit
import Foundation

enum KeyDerivationError: LocalizedError {
    case failed(Int32)

    var errorDescription: String? {
        switch self {
        case .failed(let status):
            return "无法生成本地加密密钥（\(status)）"
        }
    }
}

struct KeyDerivation {
    static let version = 1
    static let iterations: UInt32 = 210_000

    static func derive(account: String, password: String) throws -> String {
        let saltSource = Data("fastcopy:key-salt:v1|\(account)".utf8)
        let salt = Data(SHA256.hash(data: saltSource))
        let passwordData = Data(password.utf8)
        var output = Data(count: 32)
        let outputLength = output.count

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
                        iterations,
                        outputBytes.bindMemory(to: UInt8.self).baseAddress,
                        outputLength
                    )
                }
            }
        }
        guard status == kCCSuccess else {
            throw KeyDerivationError.failed(status)
        }
        return output.base64EncodedString()
    }
}
