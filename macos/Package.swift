// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "FastCopyMac",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "FastCopyMac", targets: ["FastCopyMac"])
    ],
    targets: [
        .executableTarget(
            name: "FastCopyMac",
            path: "Sources/FastCopyMac",
            linkerSettings: [
                .linkedFramework("AppKit"),
                .linkedFramework("CryptoKit")
            ]
        ),
        .testTarget(
            name: "FastCopyMacTests",
            dependencies: ["FastCopyMac"],
            path: "Tests/FastCopyMacTests"
        )
    ]
)
