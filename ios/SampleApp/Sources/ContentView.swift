import FleetMLSDK
import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var state: AppState
    @State private var host = "localhost"
    @State private var port = "19090"
    @State private var key = ""

    var body: some View {
        NavigationView {
            Form {
                Section("Server") {
                    TextField("Host", text: $host).textInputAutocapitalization(.never).disableAutocorrection(true)
                    TextField("gRPC port", text: $port).keyboardType(.numberPad)
                    SecureField("Enrollment key", text: $key)
                    Button("Save and connect") {
                        state.connect(.init(host: host.trimmingCharacters(in: .whitespaces), port: Int(port) ?? 19090,
                                            enrollmentKey: key.trimmingCharacters(in: .whitespaces)))
                    }
                }
                Section("Status") {
                    Text(status)
                    Text(activeModel).bold()
                }
                Section("Inference") {
                    Button("Classify sample flower") { state.classify() }
                    if !state.result.isEmpty { Text(state.result).font(.body.monospacedDigit()) }
                }
                Section("Log") {
                    ForEach(Array(state.log.prefix(30).enumerated()), id: \.offset) { _, line in
                        Text(line).font(.caption.monospaced())
                    }
                }
            }
            .navigationTitle("FleetML Sample")
        }
        .navigationViewStyle(.stack)
        .onAppear {
            if let s = state.current ?? state.savedSettings {
                host = s.host
                port = String(s.port)
                key = s.enrollmentKey
            }
        }
    }

    private var status: String {
        guard FleetML.isInitialized else { return "Not configured" }
        if state.deviceID == nil, let error = state.registrationError { return "Not registered · \(error)" }
        let reachability = state.connected.map { $0 ? "online" : "offline" } ?? "connecting"
        return "Device \(state.deviceID.map { String($0.prefix(8)) } ?? "(registering)") · \(reachability)"
    }

    private var activeModel: String {
        state.activeModels.isEmpty ? "Active model: bundled iris (no assignment yet)"
            : "Active model: " + state.activeModels.map { "\($0.modelName) \($0.version)" }.joined(separator: ", ")
    }
}
