import XCTest
@testable import FleetMLSDK

final class ValidatorTests: XCTestCase {
    private func model(_ url: URL) -> ActiveModel {
        ActiveModel(modelID: "m", modelName: "iris", version: "1", versionID: "v", variantID: "x", sha256: "", fileURL: url)
    }

    func testTheSampleCoreMLModelIsValid() {
        let url = irisModel("iris_v2.mlmodel")
        XCTAssertEqual(CoreMLModelValidator().validate(fileURL: url, model: model(url)), .valid)
        XCTAssertEqual(ModelValidators.default().validate(fileURL: url, model: model(url)), .valid)
    }

    func testACorruptModelIsInvalid() throws {
        let url = tempDir().appendingPathComponent("broken.bin")
        try Data("this is not a model".utf8).write(to: url)
        guard case .invalid(let reason) = ModelValidators.default().validate(fileURL: url, model: model(url)) else {
            return XCTFail("a corrupt file must not validate")
        }
        XCTAssertTrue(reason.hasPrefix("Core ML:"), reason)
    }

    func testTFLiteFilesAreRecognised() {
        XCTAssertTrue(ModelValidators.isTFLite(irisModel("iris_v1.tflite")))
        XCTAssertFalse(ModelValidators.isTFLite(irisModel("iris_v1.mlmodel")))
    }
}
