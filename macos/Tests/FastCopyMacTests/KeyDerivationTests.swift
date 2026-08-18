import XCTest
@testable import FastCopyMac

final class KeyDerivationTests: XCTestCase {
    func testProtocolVector() throws {
        let key = try KeyDerivation.derive(
            account: "alice",
            password: "correct horse battery staple"
        )
        XCTAssertEqual(key, "dpMRWwaHgnInWXwAZC2TxG3GuJZGNbWhYCGNP5T6I2g=")
    }
}
