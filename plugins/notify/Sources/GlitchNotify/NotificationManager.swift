import Foundation
import UserNotifications

// NotificationManager wraps UNUserNotificationCenter.
// It registers action categories (for interactive notifications) and
// posts notifications. The delegate callback fires when users tap actions.
final class NotificationManager: NSObject, UNUserNotificationCenterDelegate {
    var onAction: ((String, [String: String]) -> Void)?

    // Notification category identifiers
    static let categoryPipelineDone  = "GL1TCH_PIPELINE_DONE"
    static let categoryPipelineFailed = "GL1TCH_PIPELINE_FAILED"
    static let categoryClarification = "GL1TCH_CLARIFICATION"
    static let categoryDefault       = "GL1TCH_DEFAULT"

    // Action identifiers
    static let actionRerun     = "RERUN"
    static let actionOpenGlitch = "OPEN_GLITCH"

    func setup() {
        let center = UNUserNotificationCenter.current()
        center.delegate = self

        // Request authorization
        center.requestAuthorization(options: [.alert, .sound]) { granted, error in
            if let error {
                print("glitch-notify: notification auth error: \(error)")
            }
        }

        // Register interactive categories
        let rerunAction = UNNotificationAction(
            identifier: Self.actionRerun,
            title: "Run Again",
            options: []
        )
        let openAction = UNNotificationAction(
            identifier: Self.actionOpenGlitch,
            title: "Open gl1tch",
            options: [.foreground]
        )

        let categories: Set<UNNotificationCategory> = [
            UNNotificationCategory(
                identifier: Self.categoryPipelineDone,
                actions: [rerunAction],
                intentIdentifiers: [],
                options: []
            ),
            UNNotificationCategory(
                identifier: Self.categoryPipelineFailed,
                actions: [rerunAction],
                intentIdentifiers: [],
                options: []
            ),
            UNNotificationCategory(
                identifier: Self.categoryClarification,
                actions: [openAction],
                intentIdentifiers: [],
                options: []
            ),
            UNNotificationCategory(
                identifier: Self.categoryDefault,
                actions: [],
                intentIdentifiers: [],
                options: []
            ),
        ]
        center.setNotificationCategories(categories)
    }

    func post(title: String, subtitle: String, categoryID: String, userInfo: [String: String] = [:]) {
        let content = UNMutableNotificationContent()
        content.title = title
        if !subtitle.isEmpty {
            content.subtitle = subtitle
        }
        content.categoryIdentifier = categoryID
        content.userInfo = userInfo

        let request = UNNotificationRequest(
            identifier: UUID().uuidString,
            content: content,
            trigger: nil // deliver immediately
        )
        UNUserNotificationCenter.current().add(request) { error in
            if let error {
                print("glitch-notify: post notification: \(error)")
            }
        }
    }

    // MARK: — UNUserNotificationCenterDelegate

    // Called when user taps a notification action button.
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let info = response.notification.request.content.userInfo as? [String: String] ?? [:]
        onAction?(response.actionIdentifier, info)
        completionHandler()
    }

    // Show notifications even when app is in foreground.
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .sound])
    }
}
