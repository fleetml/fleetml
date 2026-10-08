// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "FleetMLSDK",
    platforms: [.iOS(.v15), .macOS(.v12)],
    products: [
        .library(name: "FleetMLSDK", targets: ["FleetMLSDK"]),
    ],
    dependencies: [
        // grpc-swift 1.x: the line that supports iOS 15 (2.x needs iOS 18).
        .package(url: "https://github.com/grpc/grpc-swift.git", exact: "1.27.6"),
        .package(url: "https://github.com/apple/swift-protobuf.git", from: "1.28.0"),
    ],
    targets: [
        .target(
            name: "FleetMLSDK",
            dependencies: [
                .product(name: "GRPC", package: "grpc-swift"),
                .product(name: "SwiftProtobuf", package: "swift-protobuf"),
            ]
        ),
        .testTarget(name: "FleetMLSDKTests", dependencies: ["FleetMLSDK"]),
    ]
)
