import CoreML
import Foundation

/// The outcome of validating a model.
public enum ValidationResult: Equatable {
    case valid
    case invalid(String)
}

/// Checks that a downloaded, sha256-verified model actually loads and runs on
/// this device before it becomes the active model. If validation fails the
/// previous (last-known-good) model stays active and the server is told
/// MODEL_LOAD_FAILED. Supply your own to check what your app needs.
public protocol ModelValidator {
    func validate(fileURL: URL, model: ActiveModel) -> ValidationResult
}

/// Accepts every model.
public struct NoOpModelValidator: ModelValidator {
    public init() {}
    public func validate(fileURL: URL, model: ActiveModel) -> ValidationResult { .valid }
}

/// Compiles the model with Core ML, loads it, and runs one prediction on
/// zero-filled inputs when every input is a multi-array.
public struct CoreMLModelValidator: ModelValidator {
    public init() {}

    public func validate(fileURL: URL, model: ActiveModel) -> ValidationResult {
        do {
            // Core ML compiles from a .mlmodel path; give the cached blob that extension.
            let source = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".mlmodel")
            try FileManager.default.copyItem(at: fileURL, to: source)
            defer { try? FileManager.default.removeItem(at: source) }
            let compiled = try MLModel.compileModel(at: source)
            defer { try? FileManager.default.removeItem(at: compiled) }
            let mlModel = try MLModel(contentsOf: compiled)
            let inputs = mlModel.modelDescription.inputDescriptionsByName
            var features: [String: MLFeatureValue] = [:]
            for (name, desc) in inputs {
                guard desc.type == .multiArray, let constraint = desc.multiArrayConstraint else {
                    return .valid // non-array inputs: loading succeeded, prediction needs app data
                }
                let array = try MLMultiArray(shape: constraint.shape, dataType: constraint.dataType)
                for i in 0..<array.count { array[i] = 0 }
                features[name] = MLFeatureValue(multiArray: array)
            }
            _ = try mlModel.prediction(from: MLDictionaryFeatureProvider(dictionary: features))
            return .valid
        } catch {
            return .invalid("Core ML: \(error.localizedDescription)")
        }
    }
}

/// The default validator: TFLite files (which iOS cannot run without the app's
/// own runtime) are accepted; everything else is validated with Core ML.
public enum ModelValidators {
    public static func `default`() -> ModelValidator { DefaultValidator() }

    static func isTFLite(_ url: URL) -> Bool {
        guard let handle = try? FileHandle(forReadingFrom: url) else { return false }
        defer { try? handle.close() }
        let header = handle.readData(ofLength: 8)
        return header.count == 8 && String(data: header.subdata(in: 4..<8), encoding: .ascii) == "TFL3"
    }

    private struct DefaultValidator: ModelValidator {
        func validate(fileURL: URL, model: ActiveModel) -> ValidationResult {
            ModelValidators.isTFLite(fileURL) ? .valid : CoreMLModelValidator().validate(fileURL: fileURL, model: model)
        }
    }
}
