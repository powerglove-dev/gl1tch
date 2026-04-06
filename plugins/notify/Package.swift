// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "GlitchNotify",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "GlitchNotify",
            path: "Sources/GlitchNotify",
            swiftSettings: [
                // Required for @main in an executable target (no main.swift)
                .unsafeFlags(["-parse-as-library"]),
            ]
        )
    ]
)
