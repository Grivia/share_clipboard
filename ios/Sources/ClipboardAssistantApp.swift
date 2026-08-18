import SwiftUI

@main
struct ClipboardAssistantApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(model)
                .onChange(of: scenePhase, initial: true) { _, phase in
                    model.setForeground(phase == .active)
                }
        }
    }
}
