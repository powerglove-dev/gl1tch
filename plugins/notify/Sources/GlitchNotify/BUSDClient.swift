import Foundation
import Darwin

// BUSDClient connects to the gl1tch event bus (Unix domain socket) using
// POSIX BSD socket APIs — Network.framework doesn't reliably support
// AF_UNIX stream sockets on macOS without a TCP protocol stack.
//
// Reconnects automatically with exponential backoff (1s → 30s).
final class BUSDClient: @unchecked Sendable {
    var onEvent: ((String, [String: Any]) -> Void)?
    var onConnectionChange: ((Bool) -> Void)?

    private(set) var isConnected = false

    private let queue = DispatchQueue(label: "com.gl1tch.notify.busd", qos: .background)
    private var fd: Int32 = -1
    private var backoff: TimeInterval = 1
    private let backoffMax: TimeInterval = 30
    private var stopped = false

    // MARK: — Start / Stop

    func start() {
        stopped = false
        queue.async { self.connectLoop() }
    }

    func stop() {
        stopped = true
        closeSocket()
    }

    // MARK: — Publish

    func publish(event: String, payload: [String: Any]) {
        guard isConnected, fd >= 0 else { return }
        let frame: [String: Any] = ["action": "publish", "event": event, "payload": payload]
        guard let data = try? JSONSerialization.data(withJSONObject: frame),
              var line = String(data: data, encoding: .utf8) else { return }
        line += "\n"
        writeLine(line)
    }

    // MARK: — Connect loop

    private func connectLoop() {
        while !stopped {
            let path = socketPath()
            let newFD = openSocket(path: path)
            if newFD < 0 {
                Thread.sleep(forTimeInterval: backoff)
                backoff = min(backoff * 2, backoffMax)
                continue
            }

            fd = newFD
            backoff = 1
            isConnected = true
            DispatchQueue.main.async { self.onConnectionChange?(true) }

            // Send registration
            let reg = "{\"name\":\"glitch-notify\",\"subscribe\":[\"*\"]}\n"
            writeLine(reg)

            // Read loop
            readLoop()

            // Fell out of read loop — socket closed or error
            closeSocket()
            isConnected = false
            DispatchQueue.main.async { self.onConnectionChange?(false) }

            if !stopped {
                Thread.sleep(forTimeInterval: backoff)
                backoff = min(backoff * 2, backoffMax)
            }
        }
    }

    // MARK: — POSIX helpers

    private func openSocket(path: String) -> Int32 {
        let s = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        guard s >= 0 else { return -1 }

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)

        let pathBytes = path.utf8CString
        guard pathBytes.count <= MemoryLayout.size(ofValue: addr.sun_path) else {
            Darwin.close(s)
            return -1
        }
        withUnsafeMutablePointer(to: &addr.sun_path) { ptr in
            ptr.withMemoryRebound(to: CChar.self, capacity: pathBytes.count) { dst in
                _ = strcpy(dst, path)
            }
        }

        let len = socklen_t(MemoryLayout<UInt8>.size + MemoryLayout<UInt8>.size + path.utf8.count)
        let result = withUnsafePointer(to: &addr) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { sa in
                Darwin.connect(s, sa, len)
            }
        }

        if result < 0 {
            Darwin.close(s)
            return -1
        }
        return s
    }

    private func writeLine(_ line: String) {
        guard fd >= 0 else { return }
        line.withCString { ptr in
            let len = strlen(ptr)
            var written = 0
            while written < len {
                let n = Darwin.write(fd, ptr + written, len - written)
                if n <= 0 { break }
                written += n
            }
        }
    }

    private func readLoop() {
        var buffer = Data(count: 65_536)
        var lineBuffer = Data()
        let newline = UInt8(ascii: "\n")

        while !stopped && fd >= 0 {
            let n = buffer.withUnsafeMutableBytes { ptr in
                Darwin.read(fd, ptr.baseAddress!, ptr.count)
            }
            if n <= 0 { break }

            lineBuffer.append(buffer[0..<n])

            while let idx = lineBuffer.firstIndex(of: newline) {
                let lineData = lineBuffer[lineBuffer.startIndex..<idx]
                lineBuffer.removeSubrange(lineBuffer.startIndex...idx)
                processLine(lineData)
            }
        }
    }

    private func processLine(_ data: Data) {
        guard let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let event = json["event"] as? String,
              let payload = json["payload"] as? [String: Any]
        else { return }

        DispatchQueue.main.async {
            self.onEvent?(event, payload)
        }
    }

    private func closeSocket() {
        if fd >= 0 {
            Darwin.close(fd)
            fd = -1
        }
    }

    // MARK: — Socket path

    private func socketPath() -> String {
        if let dir = ProcessInfo.processInfo.environment["XDG_RUNTIME_DIR"] {
            return "\(dir)/glitch/bus.sock"
        }
        let cacheDir = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first!
        return cacheDir.appendingPathComponent("glitch/bus.sock").path
    }
}
