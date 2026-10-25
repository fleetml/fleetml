import FleetMLSDK
import SwiftUI

@main
struct FleetMLSampleApp: App {
    // Created at launch, also when iOS launches the app in the background for a refresh.
    private let state: AppState

    init() {
        FleetML.registerBackgroundTask() // before the app finishes launching
        state = AppState()
    }

    var body: some Scene {
        WindowGroup {
            ContentView().environmentObject(state)
        }
    }
}

/// Owns the SDK for the life of the app and publishes what the screen shows.
/// Settings come from UserDefaults, so they can also be passed as launch
/// arguments (`-fleetmlHost localhost`), and from the environment:
/// `FLEETML_HOST`, `FLEETML_PORT` and `FLEETML_ENROLLMENT_KEY`, which
/// `SIMCTL_CHILD_` (simctl launch) and `DEVICECTL_CHILD_` (devicectl) pass
/// to the app without putting the key on a command line.
final class AppState: ObservableObject, FleetMLDelegate {
    struct Settings {
        var host: String
        var port: Int
        var enrollmentKey: String
    }

    @Published private(set) var log: [String] = []
    @Published private(set) var connected: Bool?
    @Published private(set) var deviceID: String?
    @Published private(set) var activeModels: [ActiveModel] = []
    @Published private(set) var result = ""

    private let defaults = UserDefaults.standard
    private let worker = DispatchQueue(label: "com.fleetml.sample.inference")

    init() {
        guard let s = Self.settings(saved: savedSettings, environment: ProcessInfo.processInfo.environment) else { return }
        save(s) // settings passed as launch arguments or in the environment are kept for later launches
        start(s)
    }

    var savedSettings: Settings? {
        guard let host = defaults.string(forKey: "fleetmlHost") else { return nil }
        let port = defaults.integer(forKey: "fleetmlPort")
        return Settings(host: host, port: port > 0 ? port : 19090, enrollmentKey: defaults.string(forKey: "fleetmlEnrollmentKey") ?? "")
    }

    /// The saved settings with what the environment sets on top; without saved
    /// settings, the environment's alone once it carries a key (host
    /// `localhost`, the simulator's Mac, unless `FLEETML_HOST` says otherwise).
    static func settings(saved: Settings?, environment env: [String: String]) -> Settings? {
        let key = env["FLEETML_ENROLLMENT_KEY"].flatMap { $0.isEmpty ? nil : $0 }
        guard var s = saved ?? key.map({ Settings(host: "localhost", port: 19090, enrollmentKey: $0) }) else { return nil }
        if let host = env["FLEETML_HOST"], !host.isEmpty { s.host = host }
        if let port = env["FLEETML_PORT"].flatMap(Int.init), port > 0 { s.port = port }
        if let key { s.enrollmentKey = key }
        return s
    }

    /// Saves the settings and (re)starts the SDK with them.
    func connect(_ s: Settings) {
        save(s)
        FleetML.stop()
        start(s)
    }

    private func save(_ s: Settings) {
        defaults.set(s.host, forKey: "fleetmlHost")
        defaults.set(s.port, forKey: "fleetmlPort")
        defaults.set(s.enrollmentKey, forKey: "fleetmlEnrollmentKey")
    }

    private func start(_ s: Settings) {
        note("Starting FleetML against \(s.host):\(s.port)")
        do {
            try FleetML.initialize(config: FleetMLConfig(serverHost: s.host, serverPort: s.port, enrollmentKey: s.enrollmentKey,
                                                         labels: ["app": "sample"]),
                                   delegate: self)
        } catch {
            note("FleetML failed to start: \(error.localizedDescription)")
        }
        refresh()
    }

    /// Classifies the sample flower with the active FleetML model (or the
    /// bundled one before any assignment) and reports inference telemetry.
    func classify() {
        worker.async { [self] in
            let active = FleetML.activeModel(named: "iris")
            let source = active.map { "\($0.modelName) \($0.version)" } ?? "bundled model"
            let text: String
            do {
                let r = try IrisClassifier.load(active?.fileURL).classify(IrisClassifier.sampleInput)
                if let active {
                    FleetML.telemetry?.reportInference(model: active, latencyMs: r.latencyMs, confidence: r.confidence, label: r.label)
                }
                note("Inference with \(source): \(r.label) (\(String(format: "%.2f", r.confidence))) in \(r.latencyMicros) µs")
                text = "\(r.label)  \(String(format: "%.1f", r.confidence * 100))%  \(r.latencyMicros) µs\n(\(source))"
            } catch {
                if let active { FleetML.telemetry?.reportInference(model: active, latencyMs: 0, error: error.localizedDescription) }
                note("Inference failed: \(error.localizedDescription)")
                text = "error: \(error.localizedDescription)"
            }
            DispatchQueue.main.async { self.result = text }
        }
    }

    private func refresh() {
        DispatchQueue.main.async {
            self.activeModels = FleetML.activeModels
            self.deviceID = FleetML.deviceID
        }
    }

    private func note(_ message: String) {
        let line = "\(Self.time.string(from: Date()))  \(message)"
        print("FleetMLSample: \(message)")
        DispatchQueue.main.async {
            self.log.insert(line, at: 0)
            if self.log.count > 200 { self.log.removeLast() }
        }
    }

    private static let time: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss"
        return f
    }()

    // MARK: FleetMLDelegate (called on a background queue)

    func fleetML(modelReady model: ActiveModel) {
        note("Model ready: \(model.modelName) \(model.version)")
        refresh()
        classify() // show what the new model predicts straight away
    }

    func fleetML(modelUnloaded modelID: String, modelName: String, variantID: String) {
        note("Model unloaded: \(modelName)")
        refresh()
    }

    func fleetML(modelLoadFailed modelName: String, version: String, reason: String) {
        note("Model failed: \(modelName) \(version): \(reason)")
        refresh()
    }

    func fleetML(registered deviceID: String) {
        note("Registered as \(deviceID)")
        refresh()
    }

    func fleetML(connectionChanged connected: Bool) {
        note(connected ? "Server reachable" : "Server unreachable (offline)")
        DispatchQueue.main.async { self.connected = connected }
        refresh()
    }

    func fleetML(connectionFailed error: Error) {
        note("Cannot register: \(error)")
    }
}
