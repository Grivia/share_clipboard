import AppKit
import Foundation

@MainActor
final class ClipboardMonitor {
    private let pasteboard = NSPasteboard.general
    private var timer: Timer?
    private var changeCount = 0
    private var lastText: String?
    private var onChange: ((String) -> Void)?

    func start(onChange: @escaping (String) -> Void) {
        stop()
        self.onChange = onChange
        changeCount = pasteboard.changeCount
        lastText = pasteboard.string(forType: .string)
        let timer = Timer(timeInterval: 1.0, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.poll() }
        }
        timer.tolerance = 0.25
        RunLoop.main.add(timer, forMode: .common)
        self.timer = timer
    }

    func stop() {
        timer?.invalidate()
        timer = nil
        onChange = nil
    }

    func writeWithoutUploading(_ text: String) {
        lastText = text
        pasteboard.clearContents()
        pasteboard.setString(text, forType: .string)
        changeCount = pasteboard.changeCount
    }

    private func poll() {
        guard pasteboard.changeCount != changeCount else { return }
        changeCount = pasteboard.changeCount
        guard let text = pasteboard.string(forType: .string), text != lastText else { return }
        lastText = text
        onChange?(text)
    }
}
