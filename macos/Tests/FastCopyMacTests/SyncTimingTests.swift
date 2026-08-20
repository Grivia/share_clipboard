import XCTest
@testable import FastCopyMac

final class SyncTimingTests: XCTestCase {
    func testReconciliationIntervals() {
        XCTAssertEqual(SyncTiming.connectedReconciliationNanoseconds, 300_000_000_000)
        XCTAssertEqual(SyncTiming.disconnectedReconciliationNanoseconds, 60_000_000_000)
        XCTAssertEqual(SyncTiming.webSocketSessionCheckInterval, 30)
    }

    func testPendingUploadBackoff() {
        let expected: [UInt64] = [2, 5, 15, 30, 60, 60].map { $0 * 1_000_000_000 }
        let actual = (0..<expected.count).map { SyncTiming.pendingRetryNanoseconds(attempt: $0) }
        XCTAssertEqual(actual, expected)
    }

    func testOnlyUnauthorizedServerResponsesExpireSession() {
        XCTAssertTrue(APIClientError.server(status: 401, code: "SESSION_EXPIRED", message: "expired").isUnauthorized)
        XCTAssertFalse(APIClientError.server(status: 503, code: "UNAVAILABLE", message: "offline").isUnauthorized)
        XCTAssertFalse(APIClientError.invalidResponse.isUnauthorized)
    }

    func testPushedClipSequenceActions() {
        XCTAssertEqual(pushedClipAction(currentSeq: 7, incomingSeq: 7), .ignore)
        XCTAssertEqual(pushedClipAction(currentSeq: 7, incomingSeq: 6), .ignore)
        XCTAssertEqual(pushedClipAction(currentSeq: 7, incomingSeq: 8), .apply)
        XCTAssertEqual(pushedClipAction(currentSeq: 7, incomingSeq: 9), .reconcile)
    }

    func testWebSocketClipPayloadDecoding() throws {
        let json = #"{"type":"clip.created","data":{"event_id":"server-event","client_event_id":"client-event","seq":8,"origin_device_id":"device-a","origin_name":"Mac","content_type":"text/plain","algorithm":"AES-256-GCM","nonce":"nonce","ciphertext":"ciphertext","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-02T00:00:00Z"}}"#
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase

        let envelope = try decoder.decode(WebSocketEnvelope.self, from: Data(json.utf8))

        XCTAssertEqual(envelope.type, "clip.created")
        XCTAssertEqual(envelope.data?.seq, 8)
        XCTAssertEqual(envelope.data?.originDeviceId, "device-a")
    }
}
