import UIKit
import UserNotifications

@MainActor
final class PushCoordinator: NSObject, UNUserNotificationCenterDelegate {
    static let shared = PushCoordinator()

    var onToken: ((String, String) -> Void)? {
        didSet { deliverTokenIfPossible() }
    }
    var onRegistrationError: ((Error) -> Void)?
    var onRemoteNotification: (() async -> Bool)?

    private var currentToken: String?

    var environment: String {
        #if DEBUG
        "sandbox"
        #else
        "production"
        #endif
    }

    func start() {
        UNUserNotificationCenter.current().delegate = self
        UIApplication.shared.registerForRemoteNotifications()
    }

    func requestVisibleNotificationPermission() async {
        _ = try? await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .badge, .sound])
        UIApplication.shared.registerForRemoteNotifications()
    }

    func received(deviceToken: Data) {
        currentToken = deviceToken.map { String(format: "%02x", $0) }.joined()
        deliverTokenIfPossible()
    }

    func failed(error: Error) {
        onRegistrationError?(error)
    }

    func handleRemoteNotification(completion: @escaping (UIBackgroundFetchResult) -> Void) {
        guard let onRemoteNotification else {
            completion(.noData)
            return
        }
        Task {
            let changed = await onRemoteNotification()
            completion(changed ? .newData : .noData)
        }
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .list, .sound])
    }

    private func deliverTokenIfPossible() {
        guard let currentToken else { return }
        onToken?(currentToken, environment)
    }
}

final class AppDelegate: NSObject, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        PushCoordinator.shared.start()
        return true
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        PushCoordinator.shared.received(deviceToken: deviceToken)
    }

    func application(_ application: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        PushCoordinator.shared.failed(error: error)
    }

    func application(
        _ application: UIApplication,
        didReceiveRemoteNotification userInfo: [AnyHashable: Any],
        fetchCompletionHandler completionHandler: @escaping (UIBackgroundFetchResult) -> Void
    ) {
        PushCoordinator.shared.handleRemoteNotification(completion: completionHandler)
    }
}
