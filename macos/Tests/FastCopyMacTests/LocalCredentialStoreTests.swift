import Foundation
import XCTest
@testable import FastCopyMac

final class LocalCredentialStoreTests: XCTestCase {
    func testPersistsValuesWithoutKeychain() throws {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let fileURL = directory.appendingPathComponent("credentials.json")
        let store = LocalCredentialStore(fileURL: fileURL)

        XCTAssertNil(store.string(for: "accessToken"))
        try store.set("access-value", for: "accessToken")
        try store.set("refresh-value", for: "refreshToken")

        let reloaded = LocalCredentialStore(fileURL: fileURL)
        XCTAssertEqual(reloaded.string(for: "accessToken"), "access-value")
        XCTAssertEqual(reloaded.string(for: "refreshToken"), "refresh-value")

        let attributes = try FileManager.default.attributesOfItem(atPath: fileURL.path)
        XCTAssertEqual((attributes[.posixPermissions] as? NSNumber)?.intValue, 0o600)

        reloaded.delete("accessToken")
        XCTAssertNil(reloaded.string(for: "accessToken"))
        XCTAssertEqual(reloaded.string(for: "refreshToken"), "refresh-value")
    }

    func testMergeDoesNotReplaceNewerLocalValues() throws {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = LocalCredentialStore(
            fileURL: directory.appendingPathComponent("credentials.json")
        )

        try store.set("current", for: "refreshToken")
        try store.merge(["refreshToken": "legacy", "installationID": "device-installation"])

        XCTAssertEqual(store.string(for: "refreshToken"), "current")
        XCTAssertEqual(store.string(for: "installationID"), "device-installation")
    }
}
