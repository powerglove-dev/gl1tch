import Foundation

// EventRouter maps BUSD events to notifications and maintains menu state.
// It tracks the last 3 completed pipelines and any pending clarification.
final class EventRouter {
    private let notifier: NotificationManager
    var onMenuNeedsUpdate: (() -> Void)?

    // Menu state — updated on main thread.
    private(set) var recentPipelines: [String] = []
    private(set) var pendingClarification: String?

    init(notifier: NotificationManager) {
        self.notifier = notifier
        notifier.onAction = { [weak self] actionID, userInfo in
            self?.handleAction(actionID: actionID, userInfo: userInfo)
        }
    }

    func handle(event: String, payload: [String: Any]) {
        switch event {

        case "brain.alert.raised":
            // gl1tch's brain raised an alert (collector deltas, model
            // unreachable, …). Severity controls whether we treat it as
            // a normal notification or a "needs attention" one.
            let title = stringValue(payload, keys: ["title"])
            let subtitle = stringValue(payload, keys: ["subtitle", "detail"])
            let severity = stringValue(payload, keys: ["severity"])
            let category = severity == "error"
                ? NotificationManager.categoryPipelineFailed
                : NotificationManager.categoryDefault
            notifier.post(
                title: title.isEmpty ? "gl1tch · brain" : title,
                subtitle: subtitle,
                categoryID: category
            )

        case "pipeline.run.completed":
            let name = stringValue(payload, keys: ["pipeline", "name"])
            trackPipeline(name: name)
            notifier.post(
                title: "Pipeline done",
                subtitle: name,
                categoryID: NotificationManager.categoryPipelineDone,
                userInfo: name.isEmpty ? [:] : ["pipeline_name": name]
            )

        case "pipeline.run.failed":
            let name = stringValue(payload, keys: ["pipeline", "name"])
            trackPipeline(name: name)
            notifier.post(
                title: "Pipeline failed",
                subtitle: name,
                categoryID: NotificationManager.categoryPipelineFailed,
                userInfo: name.isEmpty ? [:] : ["pipeline_name": name]
            )

        case "pipeline.step.failed":
            let name = stringValue(payload, keys: ["step", "name"])
            notifier.post(title: "Step failed", subtitle: name, categoryID: NotificationManager.categoryDefault)

        case "agent.run.completed":
            let name = stringValue(payload, keys: ["agent", "name"])
            notifier.post(title: "Agent done", subtitle: name, categoryID: NotificationManager.categoryDefault)

        case "agent.run.failed":
            let name = stringValue(payload, keys: ["agent", "name"])
            notifier.post(title: "Agent failed", subtitle: name, categoryID: NotificationManager.categoryDefault)

        case "agent.run.clarification":
            let question = stringValue(payload, keys: ["question", "prompt", "message"])
            pendingClarification = question.isEmpty ? "gl1tch needs input" : question
            notifier.post(
                title: "gl1tch needs input",
                subtitle: question,
                categoryID: NotificationManager.categoryClarification
            )
            onMenuNeedsUpdate?()

        case "cron.job.started":
            let name = stringValue(payload, keys: ["job", "name"])
            notifier.post(title: "Cron started", subtitle: name, categoryID: NotificationManager.categoryDefault)

        case "cron.job.completed":
            let name = stringValue(payload, keys: ["job", "name"])
            notifier.post(title: "Cron done", subtitle: name, categoryID: NotificationManager.categoryDefault)

        case "game.achievement.unlocked":
            let name = stringValue(payload, keys: ["achievement", "name"])
            notifier.post(title: "Achievement!", subtitle: name, categoryID: NotificationManager.categoryDefault)

        case "game.bounty.completed":
            let name = stringValue(payload, keys: ["bounty", "name"])
            notifier.post(title: "Bounty complete", subtitle: name, categoryID: NotificationManager.categoryDefault)

        default:
            break // unknown event — ignore
        }
    }

    // MARK: — Action handling

    // Called when user taps a notification action button.
    // Forwarded from NotificationManager.onAction.
    private func handleAction(actionID: String, userInfo: [String: String]) {
        switch actionID {
        case NotificationManager.actionOpenGlitch:
            pendingClarification = nil
            onMenuNeedsUpdate?()

        default:
            break
        }
        // Note: RERUN action is handled by AppDelegate via the menu item
        // to avoid circular dependency. The notification's userInfo carries
        // pipeline_name so AppDelegate can publish pipeline.rerun.requested.
    }

    // MARK: — State updates

    private func trackPipeline(name: String) {
        guard !name.isEmpty else { return }
        recentPipelines.removeAll { $0 == name }
        recentPipelines.insert(name, at: 0)
        if recentPipelines.count > 3 {
            recentPipelines = Array(recentPipelines.prefix(3))
        }
        onMenuNeedsUpdate?()
    }

    // MARK: — Helpers

    private func stringValue(_ payload: [String: Any], keys: [String]) -> String {
        for key in keys {
            if let v = payload[key] as? String, !v.isEmpty {
                return v
            }
        }
        return ""
    }
}
