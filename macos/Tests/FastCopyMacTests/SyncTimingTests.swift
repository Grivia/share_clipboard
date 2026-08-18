import XCTest
@testable import FastCopyMac

final class SyncTimingTests: XCTestCase {
    func testReconciliationIntervals() {
        XCTAssertEqual(SyncTiming.connectedReconciliationNanoseconds, 300_000_000_000)
        XCTAssertEqual(SyncTiming.disconnectedReconciliationNanoseconds, 60_000_000_000)
    }

    func testPendingUploadBackoff() {
        let expected: [UInt64] = [2, 5, 15, 30, 60, 60].map { $0 * 1_000_000_000 }
        let actual = (0..<expected.count).map { SyncTiming.pendingRetryNanoseconds(attempt: $0) }
        XCTAssertEqual(actual, expected)
    }
}
