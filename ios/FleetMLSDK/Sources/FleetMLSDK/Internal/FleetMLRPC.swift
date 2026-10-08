import Foundation
import GRPC
import NIOCore
import NIOPosix

/// Owns the gRPC channel and implements the device and telemetry RPCs.
final class FleetMLRPC: DeviceAPI, TelemetrySender {
    private let group = MultiThreadedEventLoopGroup(numberOfThreads: 1)
    private let channel: GRPCChannel

    init(config: FleetMLConfig) throws {
        let security: GRPCChannelPool.Configuration.TransportSecurity =
            config.useTLS ? .tls(.makeClientConfigurationBackedByNIOSSL()) : .plaintext
        channel = try GRPCChannelPool.with(target: .host(config.serverHost, port: config.serverPort),
                                           transportSecurity: security, eventLoopGroup: group)
    }

    private func options(token: String?) -> CallOptions {
        var o = CallOptions(timeLimit: .timeout(.seconds(30)))
        if let token { o.customMetadata.add(name: "authorization", value: "Bearer \(token)") }
        return o
    }

    private func mapped<T>(_ call: () async throws -> T) async throws -> T {
        do {
            return try await call()
        } catch let status as GRPCStatus where status.code == .unauthenticated {
            throw UnauthenticatedError()
        }
    }

    func register(_ request: Fleetml_Device_V1_RegisterRequest, token: String?) async throws -> Fleetml_Device_V1_RegisterResponse {
        try await Fleetml_Device_V1_DeviceServiceAsyncClient(channel: channel, defaultCallOptions: options(token: token)).register(request)
    }

    func heartbeat(token: String, request: Fleetml_Device_V1_HeartbeatRequest) async throws -> Fleetml_Device_V1_HeartbeatResponse {
        try await mapped { try await Fleetml_Device_V1_DeviceServiceAsyncClient(channel: channel, defaultCallOptions: options(token: token)).heartbeat(request) }
    }

    func refreshToken(token: String) async throws -> Fleetml_Device_V1_RefreshTokenResponse {
        try await mapped {
            try await Fleetml_Device_V1_DeviceServiceAsyncClient(channel: channel, defaultCallOptions: options(token: token))
                .refreshToken(Fleetml_Device_V1_RefreshTokenRequest())
        }
    }

    func send(deviceID: String, token: String, events: [Fleetml_Telemetry_V1_TelemetryEvent]) async throws -> Int {
        var batch = Fleetml_Telemetry_V1_TelemetryBatch()
        batch.deviceID = deviceID
        batch.events = events
        let ack = try await mapped {
            try await Fleetml_Telemetry_V1_TelemetryServiceAsyncClient(channel: channel, defaultCallOptions: options(token: token)).reportBatch(batch)
        }
        return Int(ack.acceptedCount)
    }

    func shutdown() {
        _ = channel.close()
        try? group.syncShutdownGracefully()
    }
}
