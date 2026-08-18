import Foundation
import Security

struct PersistentStateStore {
    private let defaults = UserDefaults.standard
    private let key = "clipboardAssistant.state.v1"

    func load() -> PersistedState {
        guard let data = defaults.data(forKey: key),
              let state = try? JSONDecoder().decode(PersistedState.self, from: data) else {
            let state = PersistedState.initial
            save(state)
            return state
        }
        return state
    }

    func save(_ state: PersistedState) {
        guard let data = try? JSONEncoder().encode(state) else { return }
        defaults.set(data, forKey: key)
    }
}

struct SecureStateStore {
    private let service = "hair.zhy.fastcopy.ios"
    private let account = "session-v1"

    func load() -> SecretState? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess,
              let data = result as? Data else { return nil }
        return try? JSONDecoder().decode(SecretState.self, from: data)
    }

    func save(_ state: SecretState) throws {
        let data = try JSONEncoder().encode(state)
        let update = [kSecValueData as String: data]
        let status = SecItemUpdate(baseQuery as CFDictionary, update as CFDictionary)
        if status == errSecSuccess { return }
        guard status == errSecItemNotFound else { throw NSError(domain: NSOSStatusErrorDomain, code: Int(status)) }
        var item = baseQuery
        item[kSecValueData as String] = data
        item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let addStatus = SecItemAdd(item as CFDictionary, nil)
        guard addStatus == errSecSuccess else { throw NSError(domain: NSOSStatusErrorDomain, code: Int(addStatus)) }
    }

    func clear() {
        SecItemDelete(baseQuery as CFDictionary)
    }

    private var baseQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
    }
}
