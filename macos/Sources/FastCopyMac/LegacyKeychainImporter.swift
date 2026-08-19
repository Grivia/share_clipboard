import Foundation
import Security

enum LegacyKeychainImportError: LocalizedError {
    case unexpectedStatus(OSStatus)

    var errorDescription: String? {
        switch self {
        case .unexpectedStatus(let status):
            return "旧钥匙串凭据迁移失败（\(status)）"
        }
    }
}

struct LegacyKeychainImporter {
    private static let service = "hair.zhy.fastcopy"
    private static let migrationKey = "localCredentialMigration.v1"
    private static let supportedAccounts: Set<String> = [
        "accessToken",
        "refreshToken",
        "sharedKey",
        "installationID"
    ]

    static func migrateIfNeeded(
        to store: LocalCredentialStore,
        defaults: UserDefaults = .standard
    ) -> Error? {
        if defaults.bool(forKey: migrationKey) || store.containsValues() {
            defaults.set(true, forKey: migrationKey)
            return nil
        }

        do {
            try store.merge(readLegacyValues())
            defaults.set(true, forKey: migrationKey)
            return nil
        } catch {
            return error
        }
    }

    private static func readLegacyValues() throws -> [String: String] {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecReturnAttributes as String: true,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitAll
        ]
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound {
            return [:]
        }
        guard status == errSecSuccess else {
            throw LegacyKeychainImportError.unexpectedStatus(status)
        }

        let items: [[String: Any]]
        if let array = result as? [[String: Any]] {
            items = array
        } else if let item = result as? [String: Any] {
            items = [item]
        } else {
            return [:]
        }

        return items.reduce(into: [:]) { values, item in
            guard let account = item[kSecAttrAccount as String] as? String,
                  supportedAccounts.contains(account),
                  let data = item[kSecValueData as String] as? Data,
                  let value = String(data: data, encoding: .utf8) else {
                return
            }
            values[account] = value
        }
    }
}
