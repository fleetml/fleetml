import CoreML
import Foundation

/// Runs the Iris Core ML model: the FleetML-delivered file, or the copy
/// bundled with the app (compiled by Xcode) before any assignment.
struct IrisClassifier {
    struct Result {
        let label: String
        let confidence: Float
        let latencyMicros: Int
        var latencyMs: Int { max(1, latencyMicros / 1000) }
    }

    static let labels = ["setosa", "versicolor", "virginica"]
    /// A flower measurement (cm): sepal length, sepal width, petal length, petal width.
    static let sampleInput: [Float] = [6.7, 3.0, 5.2, 2.3]

    private static let lock = NSLock()
    private static var compiled: [URL: MLModel] = [:]

    let model: MLModel

    /// Loads (and caches) the model for a downloaded `.mlmodel` file, or the bundled one for nil.
    static func load(_ file: URL?) throws -> IrisClassifier {
        try lock.withLock {
            let key = file ?? URL(fileURLWithPath: "bundled")
            if let m = compiled[key] { return IrisClassifier(model: m) }
            let m: MLModel
            if let file {
                // Core ML compiles from a path ending in .mlmodel.
                let source = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".mlmodel")
                try FileManager.default.copyItem(at: file, to: source)
                defer { try? FileManager.default.removeItem(at: source) }
                m = try MLModel(contentsOf: try MLModel.compileModel(at: source))
            } else {
                guard let url = Bundle.main.url(forResource: "iris_v1", withExtension: "mlmodelc") else {
                    throw CocoaError(.fileNoSuchFile)
                }
                m = try MLModel(contentsOf: url)
            }
            compiled[key] = m
            return IrisClassifier(model: m)
        }
    }

    func classify(_ input: [Float]) throws -> Result {
        let array = try MLMultiArray(shape: [4], dataType: .float32)
        for (i, v) in input.enumerated() { array[i] = NSNumber(value: v) }
        let start = DispatchTime.now().uptimeNanoseconds
        let output = try model.prediction(from: MLDictionaryFeatureProvider(dictionary: ["measurements": array]))
        let micros = Int((DispatchTime.now().uptimeNanoseconds - start) / 1000)
        guard let probs = output.featureValue(for: "probabilities")?.multiArrayValue else {
            throw CocoaError(.coderValueNotFound)
        }
        let values = (0..<probs.count).map { probs[$0].floatValue }
        let best = values.indices.max { values[$0] < values[$1] } ?? 0
        return Result(label: Self.labels[best], confidence: values[best], latencyMicros: micros)
    }
}
