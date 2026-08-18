import AppKit
import SwiftUI

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }
}

@main
struct FastCopyMacApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel()

    var body: some Scene {
        MenuBarExtra {
            MenuContent()
                .environmentObject(model)
        } label: {
            Image(systemName: model.statusIcon)
                .accessibilityLabel("FastCopy")
                .task { await model.start() }
        }
        .menuBarExtraStyle(.menu)

        Window("FastCopy", id: "settings") {
            SettingsView()
                .environmentObject(model)
                .frame(minWidth: 440, minHeight: 390)
        }
        .defaultSize(width: 480, height: 440)
        .windowResizability(.contentMinSize)
    }
}

private struct MenuContent: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        Text(model.statusText)

        if model.isAuthenticated {
            Toggle("同步剪贴板", isOn: $model.syncEnabled)

            Button {
                Task { await model.refreshNow() }
            } label: {
                Label("立即同步", systemImage: "arrow.triangle.2.circlepath")
            }
        }

        Divider()

        Button {
            openWindow(id: "settings")
            NSApp.activate(ignoringOtherApps: true)
            if model.isAuthenticated {
                Task { await model.refreshDevices() }
            }
        } label: {
            Label(model.isAuthenticated ? "设备与设置" : "登录", systemImage: "gearshape")
        }

        Divider()

        Button {
            NSApp.terminate(nil)
        } label: {
            Label("退出 FastCopy", systemImage: "power")
        }
    }
}
