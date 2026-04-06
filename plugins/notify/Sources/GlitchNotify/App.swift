import SwiftUI
import AppKit

@main
struct GlitchNotifyApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    var body: some Scene {
        // No window — menu bar only.
        Settings { EmptyView() }
    }
}

// AppDelegate wires up the status item and notification delegate.
// SwiftUI's MenuBarExtra would work but NSStatusItem gives us a dynamic
// menu we can mutate in response to BUSD events.
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var statusItem: NSStatusItem?
    private let client = BUSDClient()
    private let notifier = NotificationManager()
    private let router: EventRouter

    override init() {
        self.router = EventRouter(notifier: notifier)
        super.init()
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory) // no dock icon
        notifier.setup()
        setupStatusItem()
        client.onEvent = { [weak self] event, payload in
            self?.router.handle(event: event, payload: payload)
        }
        client.onConnectionChange = { [weak self] connected in
            self?.updateStatus(connected: connected)
        }
        router.onMenuNeedsUpdate = { [weak self] in
            self?.rebuildMenu()
        }
        // Wire notification actions back to BUSD (e.g. "Run Again" tapped).
        notifier.onAction = { [weak self] actionID, userInfo in
            guard let self else { return }
            switch actionID {
            case NotificationManager.actionRerun:
                if let name = userInfo["pipeline_name"], !name.isEmpty {
                    self.client.publish(event: "pipeline.rerun.requested", payload: ["name": name])
                }
            case NotificationManager.actionOpenGlitch:
                self.openGlitch()
            default:
                break
            }
            self.rebuildMenu()
        }
        client.start()
    }

    // MARK: — Status item

    private func setupStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem?.button {
            button.image = loadIcon()
            button.image?.isTemplate = true
            button.toolTip = "gl1tch"
        }
        rebuildMenu()
    }

    private func loadIcon() -> NSImage? {
        // When running as a bundled .app, Resources/icon.png is placed there by Makefile.
        if let url = Bundle.main.url(forResource: "icon", withExtension: "png") {
            return NSImage(contentsOf: url)
        }
        return NSImage(systemSymbolName: "circle.fill", accessibilityDescription: "gl1tch")
    }

    private func updateStatus(connected: Bool) {
        rebuildMenu()
    }

    func rebuildMenu() {
        let menu = NSMenu()

        let status = client.isConnected ? "● Connected" : "○ Disconnected"
        let statusItem = NSMenuItem(title: status, action: nil, keyEquivalent: "")
        statusItem.isEnabled = false
        menu.addItem(statusItem)

        menu.addItem(.separator())

        // Recent pipelines submenu
        let recent = router.recentPipelines
        if !recent.isEmpty {
            let recentsMenu = NSMenu()
            for name in recent {
                let item = NSMenuItem(
                    title: "Run again: \(name)",
                    action: #selector(rerunPipeline(_:)),
                    keyEquivalent: ""
                )
                item.target = self
                item.representedObject = name
                recentsMenu.addItem(item)
            }
            let recentsItem = NSMenuItem(title: "Recent Pipelines", action: nil, keyEquivalent: "")
            recentsItem.submenu = recentsMenu
            menu.addItem(recentsItem)
            menu.addItem(.separator())
        }

        // Clarification pending
        if let question = router.pendingClarification {
            let clarifyItem = NSMenuItem(
                title: "⚡ Respond: \(truncate(question, 40))",
                action: #selector(openGlitch),
                keyEquivalent: ""
            )
            clarifyItem.target = self
            menu.addItem(clarifyItem)
            menu.addItem(.separator())
        }

        let quitItem = NSMenuItem(title: "Quit", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        menu.addItem(quitItem)

        self.statusItem?.menu = menu
    }

    @objc private func rerunPipeline(_ sender: NSMenuItem) {
        guard let name = sender.representedObject as? String else { return }
        client.publish(event: "pipeline.rerun.requested", payload: ["name": name])
    }

    @objc private func openGlitch() {
        // Bring glitch tmux session to front via AppleScript.
        let script = """
        tell application "Terminal" to activate
        do shell script "tmux switch-client -t glitch 2>/dev/null || true"
        """
        if let appleScript = NSAppleScript(source: script) {
            appleScript.executeAndReturnError(nil)
        }
    }

    private func truncate(_ s: String, _ n: Int) -> String {
        guard s.count > n else { return s }
        return String(s.prefix(n)) + "…"
    }
}
