import Foundation

struct LocalCredentialStore {
    private let fileURL: URL

    init(fileURL: URL? = nil) {
        self.fileURL = fileURL ?? Self.defaultFileURL()
    }

    func string(for account: String) -> String? {
        values()[account]
    }

    func set(_ value: String, for account: String) throws {
        try set([account: value])
    }

    func set(_ updates: [String: String]) throws {
        var stored = values()
        stored.merge(updates) { _, updated in updated }
        try save(stored)
    }

    func merge(_ imported: [String: String]) throws {
        guard !imported.isEmpty else { return }
        var stored = values()
        stored.merge(imported) { current, _ in current }
        try save(stored)
    }

    func delete(_ account: String) {
        var stored = values()
        stored.removeValue(forKey: account)
        try? save(stored)
    }

    func containsValues() -> Bool {
        !values().isEmpty
    }

    private func values() -> [String: String] {
        guard let data = try? Data(contentsOf: fileURL) else { return [:] }
        return (try? JSONDecoder().decode([String: String].self, from: data)) ?? [:]
    }

    private func save(_ values: [String: String]) throws {
        let fileManager = FileManager.default
        let directory = fileURL.deletingLastPathComponent()
        try fileManager.createDirectory(
            at: directory,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: directory.path)

        if values.isEmpty {
            try? fileManager.removeItem(at: fileURL)
            return
        }

        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let data = try encoder.encode(values)
        try data.write(to: fileURL, options: .atomic)
        try fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: fileURL.path)
    }

    private static func defaultFileURL() -> URL {
        let fileManager = FileManager.default
        let applicationSupport = fileManager.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first ?? fileManager.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support", isDirectory: true)
        return applicationSupport
            .appendingPathComponent("hair.zhy.fastcopy", isDirectory: true)
            .appendingPathComponent("credentials.json", isDirectory: false)
    }
}
