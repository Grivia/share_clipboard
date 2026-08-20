import XCTest
@testable import ClipboardAssistant

final class ClipboardCryptoTests: XCTestCase {
    func testKeyDerivationMatchesProtocolVector() throws {
        let key = try ClipboardCrypto.deriveKey(
            account: "alice",
            password: "correct horse battery staple"
        )
        XCTAssertEqual(key.base64EncodedString(), "dpMRWwaHgnInWXwAZC2TxG3GuJZGNbWhYCGNP5T6I2g=")
    }

    func testEncryptionRoundTripsUnicodeText() throws {
        let key = try ClipboardCrypto.deriveKey(account: "测试", password: "安全 密码")
        let upload = try ClipboardCrypto.encrypt("跨设备文本\niOS", key: key)
        let event = ClipEvent(
            eventID: "server-event",
            clientEventID: upload.clientEventID,
            seq: 1,
            originDeviceID: "device-a",
            originName: "iPhone",
            contentType: upload.contentType,
            algorithm: upload.algorithm,
            nonce: upload.nonce,
            ciphertext: upload.ciphertext,
            createdAt: "2026-01-01T00:00:00Z",
            expiresAt: "2026-01-02T00:00:00Z"
        )
        XCTAssertEqual(try ClipboardCrypto.decrypt(event, key: key), "跨设备文本\niOS")
    }

    func testPushedClipSequenceActions() {
        XCTAssertEqual(pushedClipAction(currentSeq: 7, incomingSeq: 7), .ignore)
        XCTAssertEqual(pushedClipAction(currentSeq: 7, incomingSeq: 6), .ignore)
        XCTAssertEqual(pushedClipAction(currentSeq: 7, incomingSeq: 8), .apply)
        XCTAssertEqual(pushedClipAction(currentSeq: 7, incomingSeq: 9), .reconcile)
    }

    func testSocketEventDecodesClipPayload() throws {
        let json = #"{"type":"clip.created","data":{"event_id":"server-event","client_event_id":"client-event","seq":8,"origin_device_id":"device-a","origin_name":"iPhone","content_type":"text/plain","algorithm":"AES-256-GCM","nonce":"nonce","ciphertext":"ciphertext","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-02T00:00:00Z"}}"#

        let event = try JSONDecoder().decode(SocketEvent.self, from: Data(json.utf8))

        XCTAssertEqual(event.type, "clip.created")
        XCTAssertEqual(event.data?.seq, 8)
        XCTAssertEqual(event.data?.originDeviceID, "device-a")
    }
}
