import Foundation
import XCTest
@testable import FastCopyMac

final class LocalCredentialStoreTests: XCTestCase {
    func testPersistsEncryptedValuesWithoutKeychain() throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = LocalCredentialStore(directoryURL: directory)

        XCTAssertNil(store.string(for: "accessToken"))
        try store.set("access-value", for: "accessToken")
        try store.set("refresh-value", for: "refreshToken")

        let reloaded = LocalCredentialStore(directoryURL: directory)
        XCTAssertEqual(reloaded.string(for: "accessToken"), "access-value")
        XCTAssertEqual(reloaded.string(for: "refreshToken"), "refresh-value")

        let encryptedURL = directory.appendingPathComponent("credentials.enc")
        let keyURL = directory.appendingPathComponent("credentials.key")
        let encryptedData = try Data(contentsOf: encryptedURL)
        XCTAssertNil(encryptedData.range(of: Data("access-value".utf8)))
        XCTAssertNil(encryptedData.range(of: Data("refresh-value".utf8)))
        XCTAssertEqual(try permissions(of: encryptedURL), 0o600)
        XCTAssertEqual(try permissions(of: keyURL), 0o600)
        XCTAssertEqual(try permissions(of: directory), 0o700)

        reloaded.delete("accessToken")
        XCTAssertNil(reloaded.string(for: "accessToken"))
        XCTAssertEqual(reloaded.string(for: "refreshToken"), "refresh-value")
    }

    func testReencryptsWithFreshNonce() throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = LocalCredentialStore(directoryURL: directory)
        let encryptedURL = directory.appendingPathComponent("credentials.enc")

        try store.set("same-value", for: "accessToken")
        let first = try Data(contentsOf: encryptedURL)
        try store.set("same-value", for: "accessToken")
        let second = try Data(contentsOf: encryptedURL)

        XCTAssertNotEqual(first, second)
    }

    func testMigratesLocalPlaintextFileToEncryptedFiles() throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let plaintextURL = directory.appendingPathComponent("credentials.json")
        let plaintext = try JSONEncoder().encode([
            "accessToken": "legacy-access",
            "installationID": "legacy-installation"
        ])
        try plaintext.write(to: plaintextURL)

        let store = LocalCredentialStore(directoryURL: directory)
        XCTAssertEqual(store.string(for: "accessToken"), "legacy-access")
        XCTAssertEqual(store.string(for: "installationID"), "legacy-installation")
        XCTAssertFalse(FileManager.default.fileExists(atPath: plaintextURL.path))
        XCTAssertTrue(
            FileManager.default.fileExists(
                atPath: directory.appendingPathComponent("credentials.enc").path
            )
        )
        XCTAssertTrue(
            FileManager.default.fileExists(
                atPath: directory.appendingPathComponent("credentials.key").path
            )
        )
    }

    private func temporaryDirectory() -> URL {
        FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
    }

    private func permissions(of url: URL) throws -> Int {
        let attributes = try FileManager.default.attributesOfItem(atPath: url.path)
        return (attributes[.posixPermissions] as? NSNumber)?.intValue ?? -1
    }
}
