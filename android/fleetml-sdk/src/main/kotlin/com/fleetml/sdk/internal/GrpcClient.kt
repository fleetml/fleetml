package com.fleetml.sdk.internal

import com.fleetml.sdk.FleetMLConfig
import fleetml.device.v1.DeviceServiceGrpcKt as DeviceGrpc
import fleetml.telemetry.v1.Telemetry.TelemetryBatch
import fleetml.telemetry.v1.TelemetryServiceGrpcKt as TelemetryGrpc
import io.grpc.CallCredentials
import io.grpc.ManagedChannel
import io.grpc.Metadata
import io.grpc.okhttp.OkHttpChannelBuilder
import java.util.concurrent.Executor
import java.util.concurrent.TimeUnit

/**
 * Manages the gRPC channel and stubs for communicating with the FleetML server.
 */
internal class GrpcClient(private val config: FleetMLConfig) {

    val channel: ManagedChannel by lazy {
        val builder = OkHttpChannelBuilder
            .forAddress(config.serverHost, config.serverPort)
        if (!config.useTls) {
            builder.usePlaintext()
        }
        builder
            .keepAliveTime(30, TimeUnit.SECONDS)
            .keepAliveTimeout(10, TimeUnit.SECONDS)
            .build()
    }

    /** Unauthenticated stub — used for Register. */
    val deviceService: DeviceGrpc.DeviceServiceCoroutineStub by lazy {
        DeviceGrpc.DeviceServiceCoroutineStub(channel)
    }

    /** Authenticated stub — used for Heartbeat and other authenticated RPCs. */
    fun authenticatedStub(token: String): DeviceGrpc.DeviceServiceCoroutineStub {
        return deviceService.withCallCredentials(BearerToken(token))
    }

    /** Sends telemetry batches with the device token. */
    val telemetrySender = com.fleetml.sdk.TelemetrySender { deviceId, token, events ->
        TelemetryGrpc.TelemetryServiceCoroutineStub(channel)
            .withCallCredentials(BearerToken(token))
            .reportBatch(TelemetryBatch.newBuilder().setDeviceId(deviceId).addAllEvents(events).build())
            .acceptedCount
    }

    fun shutdown() {
        if (channel.isShutdown) return
        channel.shutdown()
        try {
            channel.awaitTermination(5, TimeUnit.SECONDS)
        } catch (_: InterruptedException) {
            channel.shutdownNow()
        }
    }
}

/**
 * gRPC CallCredentials that adds a Bearer token to the authorization header.
 */
internal class BearerToken(private val token: String) : CallCredentials() {
    override fun applyRequestMetadata(
        requestInfo: RequestInfo,
        executor: Executor,
        applier: MetadataApplier
    ) {
        executor.execute {
            val headers = Metadata()
            headers.put(
                Metadata.Key.of("authorization", Metadata.ASCII_STRING_MARSHALLER),
                "Bearer $token"
            )
            applier.apply(headers)
        }
    }
}
