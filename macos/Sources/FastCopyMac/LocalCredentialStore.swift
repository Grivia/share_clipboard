import CryptoKit
import Foundation

enum LocalCredentialStoreError: LocalizedError {
    case invalidKey
    case invalidCiphertext

    var errorDescription: String? {
        switch self {
        case .invalidKey:
            return "本地凭据加密密钥无效"
        case .invalidCiphertext:
            return "本地凭据密文无效"
        }
    }
}

struct LocalCredentialStore {
    private let directoryURL: URL
    private let encryptedFileURL: URL
    private let keyFileURL: URL
    private let legacyPlaintextFileURL: URL

    init(directoryURL: URL? = nil) {
        let directoryURL = directoryURL ?? Self.defaultDirectoryURL()
        self.directoryURL = directoryURL
        encryptedFileURL = directoryURL.appendingPathComponent("credentials.enc", isDirectory: false)
        keyFileURL = directoryURL.appendingPathComponent("credentials.key", isDirectory: false)
        legacyPlaintextFileURL = directoryURL.appendingPathComponent("credentials.json", isDirectory: false)
    }

    func string(for account: String) -> String? {
        (try? loadValues())?[account]
    }

    func set(_ value: String, for account: String) throws {
        try set([account: value])
    }

    func set(_ updates: [String: String]) throws {
        var stored = try loadValues()
        stored.merge(updates) { _, updated in updated }
        try save(stored)
    }

    func delete(_ account: String) {
        do {
            var stored = try loadValues()
            stored.removeValue(forKey: account)
            try save(stored)
        } catch {
            // Preserve unreadable data instead of replacing it with a partial store.
        }
    }

    private func loadValues() throws -> [String: String] {
        let fileManager = FileManager.default
        if fileManager.fileExists(atPath: encryptedFileURL.path) {
            let encryptedData = try Data(contentsOf: encryptedFileURL)
            let key = try loadExistingKey()
            let sealedBox: AES.GCM.SealedBox
            do {
                sealedBox = try AES.GCM.SealedBox(combined: encryptedData)
            } catch {
                throw LocalCredentialStoreError.invalidCiphertext
            }
            let plaintext = try AES.GCM.open(sealedBox, using: key)
            return try JSONDecoder().decode([String: String].self, from: plaintext)
        }

        guard fileManager.fileExists(atPath: legacyPlaintextFileURL.path) else {
            return [:]
        }

        let plaintext = try Data(contentsOf: legacyPlaintextFileURL)
        let values = try JSONDecoder().decode([String: String].self, from: plaintext)
        try save(values)
        try fileManager.removeItem(at: legacyPlaintextFileURL)
        return values
    }

    private func save(_ values: [String: String]) throws {
        let fileManager = FileManager.default
        try prepareDirectory()

        if values.isEmpty {
            try? fileManager.removeItem(at: encryptedFileURL)
            try? fileManager.removeItem(at: keyFileURL)
            try? fileManager.removeItem(at: legacyPlaintextFileURL)
            return
        }

        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let plaintext = try encoder.encode(values)
        let key = try loadOrCreateKey()
        let sealedBox = try AES.GCM.seal(plaintext, using: key)
        guard let combined = sealedBox.combined else {
            throw LocalCredentialStoreError.invalidCiphertext
        }
        try writeProtected(combined, to: encryptedFileURL)
    }

    private func loadExistingKey() throws -> SymmetricKey {
        let data = try Data(contentsOf: keyFileURL)
        guard data.count == 32 else {
            throw LocalCredentialStoreError.invalidKey
        }
        return SymmetricKey(data: data)
    }

    private func loadOrCreateKey() throws -> SymmetricKey {
        if FileManager.default.fileExists(atPath: keyFileURL.path) {
            return try loadExistingKey()
        }

        let key = SymmetricKey(size: .bits256)
        let data = key.withUnsafeBytes { Data($0) }
        try writeProtected(data, to: keyFileURL)
        return key
    }

    private func prepareDirectory() throws {
        let fileManager = FileManager.default
        try fileManager.createDirectory(
            at: directoryURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try fileManager.setAttributes(
            [.posixPermissions: 0o700],
            ofItemAtPath: directoryURL.path
        )
    }

    private func writeProtected(_ data: Data, to url: URL) throws {
        try data.write(to: url, options: .atomic)
        try FileManager.default.setAttributes(
            [.posixPermissions: 0o600],
            ofItemAtPath: url.path
        )
    }

    private static func defaultDirectoryURL() -> URL {
        let fileManager = FileManager.default
        let applicationSupport = fileManager.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first ?? fileManager.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support", isDirectory: true)
        return applicationSupport.appendingPathComponent("hair.zhy.fastcopy", isDirectory: true)
    }
}
