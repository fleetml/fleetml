# iOS SDK

The FleetML iOS SDK (a Swift package for iOS 15 and later, with its manifest at
the repository root and its code in `ios/FleetMLSDK`) puts an iPhone or iPad
app under FleetML's control: it registers with an enrollment key, receives
model assignments over gRPC, downloads and verifies them, validates each model
with Core ML before using it, keeps the last-known-good version, works offline,
and reports telemetry. It does not run your models: your app does, with Core ML
or anything else. It mirrors the [Android SDK](ANDROID_SDK.md) feature for
feature.

![Sample app](screenshots/ios-sample.png)

## Install

It needs Xcode 15 or later (16 recommended; the package manifest is
swift-tools-version 5.9). In Xcode, choose **File → Add Package Dependencies**, paste
`https://github.com/fleetml/fleetml` and add the **FleetMLSDK** library to the
app target, with the Dependency Rule **Up to Next Major Version** from
0.5.0.

In a `Package.swift`:

```swift
.package(url: "https://github.com/fleetml/fleetml", from: "0.5.0"),
// target dependency:
.product(name: "FleetMLSDK", package: "fleetml"),
```

The v0.3.0 and earlier tags have no manifest at the repository root, so
0.4.0 is the first version Swift Package Manager can install by URL.

### From a clone

To work against local changes, add the package from a clone instead: in Xcode
**File → Add Package Dependencies → Add Local…** and pick the clone's root
folder, or in a `Package.swift`:

```swift
.package(path: "path/to/fleetml"),
// target dependency (the package is named after the clone's folder):
.product(name: "FleetMLSDK", package: "fleetml"),
```

With [XcodeGen](https://github.com/yonaskolb/XcodeGen), the clone is a local
package in `project.yml`, under a name of your choice, and the target depends
on its `FleetMLSDK` product (`product:` can be left out when the name is
`FleetMLSDK` too); the [Own app runbook](#own-app-runbook) has a whole file:

```yaml
packages:
  FleetMLSDK:
    path: ../fleetml
targets:
  MyApp:
    dependencies:
      - package: FleetMLSDK
        product: FleetMLSDK
```

`path` is relative to `project.yml` and points at the clone's root, where
the package manifest is. From GitHub instead, the package entry is
`url: https://github.com/fleetml/fleetml` with `from: 0.5.0`.

It depends on grpc-swift, pinned to exactly 1.27.6 (the 1.x line, which
supports iOS 15; 2.x needs iOS 18), and on swift-protobuf from 1.28. An app
that already depends on grpc-swift must use that same version, or Swift
Package Manager cannot resolve the graph. macOS 12 is supported too, for tests
and tools.

## Configure and start

Initialize once, for example in your `App` or app delegate. The enrollment
key comes from `fleetml enrollment-key create --project <project ID>`, which
prints it once. Keep it out of the source: the app reads it from its
`Info.plist`, where the build puts it from an `.xcconfig` file you do not
commit ([Your own app](#your-own-app), step 5, has the two settings). This
object starts the SDK and loads each model it delivers with Core ML:

```swift
import CoreML
import FleetMLSDK

final class AppModel: FleetMLDelegate {
    static let enrollmentKey = Bundle.main.object(forInfoDictionaryKey: "FleetMLEnrollmentKey") as? String ?? ""
    private(set) var mlModel: MLModel?

    init() {
        do {
            try FleetML.initialize(
                config: FleetMLConfig(serverHost: "localhost", enrollmentKey: Self.enrollmentKey,
                                      labels: ["app": "myapp", "tier": "beta"]),
                delegate: self)
        } catch {
            print("FleetML did not start: \(error)")
        }
    }

    func fleetML(modelReady model: ActiveModel) {
        let source = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".mlmodel")
        do {
            try FileManager.default.copyItem(at: model.fileURL, to: source)
            defer { try? FileManager.default.removeItem(at: source) }
            mlModel = try MLModel(contentsOf: try MLModel.compileModel(at: source))
        } catch {
            print("Core ML could not load \(model.modelName) \(model.version): \(error)")
        }
    }
}
```

For a quick test only, `static let enrollmentKey = "<enrollment key>"` works
too, but the key then sits in a file you commit.

Without the background task set up (it is optional, see
[Battery and data](#battery-and-data)), the SDK logs one info line each time
the app goes to the background, and at a launch that iOS starts in the
background state (as the first launch after `open -a Simulator` can be):
`Background refresh not scheduled (BGTaskSchedulerErrorDomain error 3): the app does not permit com.fleetml.sdk.refresh; add BGTaskSchedulerPermittedIdentifiers and call registerBackgroundTask(), see docs/IOS_SDK.md "Battery and data"`.
It is harmless: the SDK works in the foreground as described here (SDK 0.5.0
logged the same thing as an error).

`localhost` is the quickstart's server on the Mac, which the simulator
shares; a phone needs the Mac's LAN address ([Own app on a real
iPhone](#own-app-on-a-real-iphone)). Delivered files keep the server's name
(`{variant}.bin`), and Core ML compiles only from a path ending in `.mlmodel`,
which is why `fleetML(modelReady:)` copies the file to one first (the sample
app's `IrisClassifier` does the same).

The SDK holds the delegate weakly: keep the object alive for as long as the
app runs, for example as a property of your `App` or app delegate, or its
callbacks stop.

In production, behind a proxy that terminates TLS and forwards gRPC over
HTTP/2 (gRPC needs HTTP/2 end to end):
`FleetMLConfig(serverHost: "fleetml.example.com", serverPort: 443, enrollmentKey: Self.enrollmentKey, useTLS: true)`.

The delegate's methods all have empty default implementations, so implement
only what the app needs, with exactly these signatures: a method whose
parameter types differ still compiles, as a method of your own, and the SDK
never calls it. They are called on a background queue: switch to the main
queue before touching the UI.

| Method | When |
|---|---|
| `func fleetML(modelReady model: ActiveModel)` | A model was downloaded, verified and validated and is now active; also at start for models cached from an earlier run |
| `func fleetML(modelUnloaded modelID: String, modelName: String, variantID: String)` | The server no longer assigns the model |
| `func fleetML(modelLoadFailed modelName: String, version: String, reason: String)` | A new version could not be downloaded or failed validation; the last-known-good one stays |
| `func fleetML(registered deviceID: String)` | The device registered (or re-registered) |
| `func fleetML(connectionChanged connected: Bool)` | The server became reachable or unreachable |
| `func fleetML(connectionFailed error: Error)` | Registration failed, with the error ([When it does not connect](#when-it-does-not-connect)) |

`ActiveModel` has `modelName` and `version` (for example `iris` and `1.0.0`),
`fileURL` (the verified and validated file), `modelID`, `versionID` (telemetry
is attributed to it), `variantID` and `sha256`.

The intervals and the batch size below are what the SDK uses until it first
hears from the server; from the first registration on, the server's values
apply (their defaults are in [Battery and data](#battery-and-data)).

| `FleetMLConfig` field | Default | Purpose |
|---|---|---|
| `serverHost`, `serverPort` | –, `19090` | The FleetML gRPC endpoint |
| `enrollmentKey` | `""` | Per-project key for the first registration (`fleetml enrollment-key create`) |
| `useTLS` | `false` | TLS for gRPC (put the server behind a TLS proxy in production) |
| `labels` | `[:]` | Labels deployments and experiments target (`target_selector`) |
| `heartbeatIntervalSeconds` | `30` | Foreground heartbeat interval, until the server sends its own (server default 60) |
| `backgroundHeartbeatIntervalSeconds` | `3600` | Earliest interval of the background refresh, until the server sends its own (at least 15 minutes) |
| `telemetryBatchSize`, `telemetryFlushIntervalSeconds` | `50`, `30` | Until the server's `DeviceConfig` overrides them (server defaults 100 and 30) |
| `maxCacheSizeMB` | `500` | Disk budget for models; the active and last-known-good models are never evicted |
| `downloadPolicy` | `.unmeteredOrCharging` | Which networks non-urgent downloads may use, until the server sends its own ([Battery and data](#battery-and-data)) |
| `autoStart` | `true` | Call `FleetML.start()` yourself if `false` |

`FleetML.stop()` stops background work so `initialize` can be called again
(for example with a new server address). `FleetML.reset()` also deletes cached
models and the registration.

### When it does not connect

If the SDK cannot register, `fleetML(connectionChanged:)` reports `false` and
`fleetML(connectionFailed:)` receives the error, on the first failure after
start and again whenever the error changes (registration is retried every
10 seconds). `connectionFailed` is for registration only: a device that has
registered before and cannot reach the server reports it with
`fleetML(connectionChanged: false)` and the log line
`Heartbeat failed: ...` (below). Print the error while you set up the app:

```swift
func fleetML(connectionFailed error: Error) {
    print("FleetML cannot register: \(error)")
}
```

A wrong host or port, or a server that is not running, reads like
`deadlineExceeded (Connection errors: ... Connection refused)`; a wrong
enrollment key is refused by the server as `unauthenticated`. The SDK also
logs to the unified log with subsystem `com.fleetml.sdk` and category
`FleetML`: at info level the registration, each download (model, version,
bytes), each validation result and each activation, at warning level every
failure, and at debug level every heartbeat. Xcode's console shows it while the
app runs from Xcode; without a debugger, open Console.app on the Mac, select
the phone (or the simulator) in the sidebar, press Start, and search for
`subsystem:com.fleetml.sdk` (Action, Include Info Messages shows the info
lines). For the simulator, this prints it in a terminal (`--level debug` adds
the heartbeats; without `--level` only warnings appear):

```bash
xcrun simctl spawn booted log stream --level info --predicate 'subsystem == "com.fleetml.sdk"'
```

It first prints `getpwuid_r did not find a match for uid <your uid>` (the
simulator has no user of that ID) and the filter line; both are harmless.

A first launch that registers and receives iris 1.0.0 prints these lines
(time stamps and the process column left out):

```text
[com.fleetml.sdk:FleetML] Registered as device <device ID>
[com.fleetml.sdk:FleetML] Downloading iris 1.0.0 (555 bytes)
[com.fleetml.sdk:FleetML] Downloaded iris 1.0.0 (555 bytes, sha256 verified)
[com.fleetml.sdk:FleetML] Model iris 1.0.0 passed validation
[com.fleetml.sdk:FleetML] Model iris 1.0.0 is active
```

A registered app that cannot reach the server logs, at error level, about
ten seconds after launch (here the server's port was closed):

```text
[com.fleetml.sdk:FleetML] Heartbeat failed: The operation couldn’t be completed. (GRPC.GRPCConnectionPoolError error 1.)
```

At launch the delegate hears `fleetML(connectionChanged: true)` once the
first heartbeat is answered. SDK 0.5.0 also reported a `false` right before
it and logged `Heartbeat failed ... GRPCStatus error 1` at every launch; that
meant nothing (a heartbeat cancelled by the SDK itself) and is gone from
0.6.0.

### Your own app

What an own app needs, in one place:

1. **Labels that match the deployment.** A deployment reaches the app only if
   the app's `labels` contain its `--target`: `--target '{"app":"myapp"}'`
   reaches `labels: ["app": "myapp", "tier": "beta"]`.
2. **A Core ML version it can get.** Push the `.mlmodel` with
   `--framework COREML` and the requirements override in
   [Which variant a device gets](#which-variant-a-device-gets).
3. **On the simulator, nothing more.** It shares the Mac's `localhost`, so
   `serverHost: "localhost"` reaches the quickstart's server, and the
   downloads from `localhost:19100` need no App Transport Security keys.
4. **On a phone, two `Info.plist` keys and the server's object store on the
   Mac's LAN address**, as in [Own app on a real iPhone](#own-app-on-a-real-iphone).
   `S3_PUBLIC_ENDPOINT` is set by whoever starts the server: if that is not
   you, ask them to start the stack with it set to an address the phone
   reaches.
5. **The enrollment key out of the source.** Keep it in a build setting
   instead: a line `FLEETML_ENROLLMENT_KEY = <enrollment key>` in an
   `.xcconfig` file that is not committed, set as the target's configuration
   file, and an entry `FleetMLEnrollmentKey` with the value
   `$(FLEETML_ENROLLMENT_KEY)` on the target's Info tab (Custom iOS Target
   Properties), which keeps it in the app's `Info.plist` file; the
   `INFOPLIST_KEY_` build settings of a generated `Info.plist` take only
   Apple's keys. The app reads it with:

   ```swift
   static let enrollmentKey = Bundle.main.object(forInfoDictionaryKey: "FleetMLEnrollmentKey") as? String ?? ""
   ```

   The key is needed only to register. Revoking it stops new installs from
   registering; devices that already registered keep their token.
6. **An offline test without stopping a shared server.** Point the app at a
   port nothing listens on (`serverPort: 19999`) and launch it again: the
   cached model is ready at once, and `fleetML(connectionChanged: false)`
   follows within about ten seconds. On a phone, airplane mode does the same.

### Own app runbook

The [Simulator runbook](#simulator-runbook) below is for the sample app,
which takes its settings as launch arguments; an own app has them in its
code. This runs an own app with the `AppModel` above on the simulator,
against the quickstart's server, with the `fleetml` CLI on the `PATH` and
[XcodeGen](https://github.com/yonaskolb/XcodeGen). The app's folder holds
`project.yml`, `Secrets.xcconfig` and a `Sources` folder with the app's Swift
files: `AppModel` as above (labels `["app": "myapp", ...]`) and an `App` that
keeps it alive, for example:

```swift
import SwiftUI

@main
struct MyApp: App {
    private let model = AppModel()
    var body: some Scene { WindowGroup { Text("FleetML") } }
}
```

1. Publish the Core ML sample models from the clone's root, create an
   enrollment key and deploy 1.0.0 to the app's label (project names are
   unique on a server: `already exists: slug "myapp-ios"` means the project
   is there from an earlier run, so pick another name):

   ```bash
   export FLEETML_SERVER=http://localhost:18080 FLEETML_API_KEY=$FLEETML_ADMIN_KEY
   REQ='{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}'
   P=$(fleetml project create --name myapp-ios -o json | jq -r .id)
   fleetml push models/iris/iris_v1.mlmodel --project $P --model iris --version 1.0.0 --framework COREML --quantization FP32 --metadata "$REQ"
   fleetml push models/iris/iris_v2.mlmodel --project $P --model iris --version 2.0.0 --framework COREML --quantization FP32 --metadata "$REQ"
   M=$(fleetml model list --project $P -o json | jq -r '.[] | select(.name == "iris") | .id')
   vid() { fleetml ver list --model $M -o json | jq -r --arg v "$1" '.[] | select(.version == $v) | .id'; }
   EK=$(fleetml enrollment-key create --project $P --name myapp -o json | jq -r .key)
   D1=$(fleetml deploy create --project $P --version $(vid 1.0.0) --target '{"app":"myapp"}' -o json | jq -r .id)
   fleetml deploy start $D1
   ```

2. In the app's folder, `project.yml`, with the clone as a local package
   (`path` relative to this file; `url:` and `from:` for the released
   package) and the key from `Secrets.xcconfig` in the `Info.plist`:

   ```yaml
   name: MyApp
   options:
     deploymentTarget:
       iOS: "15.0"
   packages:
     FleetMLSDK:
       path: ../fleetml
   targets:
     MyApp:
       type: application
       platform: iOS
       sources: [Sources]
       dependencies:
         - package: FleetMLSDK
       configFiles:
         Debug: Secrets.xcconfig
         Release: Secrets.xcconfig
       info:
         path: Info.plist
         properties:
           FleetMLEnrollmentKey: $(FLEETML_ENROLLMENT_KEY)
           UILaunchScreen: {}
       settings:
         base:
           PRODUCT_BUNDLE_IDENTIFIER: com.example.myapp
   ```

   XcodeGen writes `Info.plist` from `info:`; keep `Secrets.xcconfig` and
   the generated `Info.plist` and `.xcodeproj` out of git.

3. Write the key into `Secrets.xcconfig`, generate the project, build, and
   install and launch the app on the first available iPhone simulator:

   ```bash
   printf 'FLEETML_ENROLLMENT_KEY = %s\n' "$EK" > Secrets.xcconfig
   xcodegen generate
   SIM=$(xcrun simctl list devices available -j | jq -r '[.devices[][] | select(.name | startswith("iPhone"))][0].udid')
   xcodebuild -project MyApp.xcodeproj -scheme MyApp -destination "platform=iOS Simulator,id=$SIM" -derivedDataPath build build
   xcrun simctl boot $SIM; open -a Simulator
   xcrun simctl install $SIM build/Build/Products/Debug-iphonesimulator/MyApp.app
   xcrun simctl launch --console-pty $SIM com.example.myapp
   ```

   In a second terminal, the `log stream` command of
   [When it does not connect](#when-it-does-not-connect) shows the
   registration, the download, the validation and `Model iris 1.0.0 is active`.
   `fleetml deploy get $D1 | jq -c .devices` prints `{"loaded":1,"targeted":1}`
   one heartbeat (60 seconds by default) after that; it counts what each
   device reported in its last heartbeat.

4. Roll out 2.0.0; the app gets it with its next heartbeat:

   ```bash
   fleetml deploy start $(fleetml deploy create --project $P --version $(vid 2.0.0) --target '{"app":"myapp"}' -o json | jq -r .id)
   ```

5. Offline test: change `FleetMLConfig(serverHost: "localhost", ...)` to
   `FleetMLConfig(serverHost: "localhost", serverPort: 19999, ...)`, rebuild
   with the `xcodebuild` line, install and launch again. The cached 2.0.0 is
   ready at once (`fleetML(modelReady:)`), and `fleetML(connectionChanged: false)`
   arrives about 10 seconds later. Change the port back and rebuild to
   reconnect.

## Enrollment and identity

The first registration needs a valid enrollment key; the device is enrolled
into that key's project and receives a device token (30 days by default),
stored in the Keychain (this device only, available after first unlock). The
device ID is stable per app and device: the fingerprint is a SHA-256 of the
bundle identifier and hardware properties (no identifierForVendor or
advertising identifier), so two FleetML apps on one phone are separate
devices. A device only ever receives deployments and experiments of its own
project. After registering, the SDK:

- re-registers only when it is new or its labels or SDK version changed (an
  app built without code signing, `CODE_SIGNING_ALLOWED=NO`, cannot keep the
  token in the simulator's Keychain and registers again, with the same device
  ID, at every launch; the SDK logs a warning then, and Xcode's default
  simulator builds are signed);
- refreshes the token before it expires (6 days before, or at half its lifetime
  for short-lived tokens);
- re-registers with the enrollment key if the server rejects the token.

## Models: download, verify, validate, activate

For each assignment in a heartbeat the SDK:

1. **Checks the signature** of the assignment with the project's signing
   keys (sent with registration and every heartbeat): an artifact whose
   signature does not verify is not downloaded, the current model stays, and
   `SIGNATURE_INVALID` is reported ([artifact signing](ROLLOUTS.md#artifact-signing)).
2. **Downloads** the model from the presigned URL, writing each chunk to a
   `.part` file as it arrives. An interrupted download resumes with an HTTP
   Range request. When the server offers a delta from the
   version the device has, only the delta is downloaded and applied
   ([delta updates](ROLLOUTS.md#delta-updates)).
3. **Verifies** the sha256 before moving the file into place.
4. **Validates** it with a `ModelValidator`.
5. **Activates** it only if validation passed: the model becomes active, the
   previously active version becomes last-known-good, the device reports the
   new version as loaded and `fleetML(modelReady:)` is called.

If validation fails, the active (last-known-good) model stays in place, the SDK
reports a `MODEL_LOAD_FAILED` event with the error (so a canary's health gate
can roll the release back), calls `fleetML(modelLoadFailed:version:reason:)`,
and does not retry that variant for an hour.

![A corrupt 3.0.0 is rejected and 2.0.0 stays active](screenshots/ios-last-known-good.png)

### Validators

`ModelValidators.default()` (used unless you pass `validator:` to
`initialize`) compiles the model with Core ML, loads it and, when every input
is a multi-array, runs one prediction on zeros. TensorFlow Lite files
(identifier `TFL3`) are accepted as they are, because iOS has no TFLite
runtime of its own. Write your own to check what matters to your app:

```swift
struct GoldenInputValidator: ModelValidator {
    func validate(fileURL: URL, model: ActiveModel) -> ValidationResult {
        runGoldenInput(fileURL) ? .valid : .invalid("golden input mismatch")
    }
}
try FleetML.initialize(config: config, delegate: self, validator: GoldenInputValidator())
```

### Which variant a device gets

iOS devices register with OS `IOS`, and the server prefers `COREML` variants
for them (then `TFLITE`, then `GGUF`). Requirements default by quantization
(for example FP32 needs a device score of 60 and 2 GB of RAM, INT8 a score of
20), and an iPhone scores 54 at most, so an FP32 Core ML model reaches it only
with a lower requirement in the version's metadata. `fleetml push` sets it on
the version it creates:

```bash
fleetml push model.mlmodel --project <project ID> --model iris --version 1.0.0 --framework COREML --quantization FP32 --metadata '{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}'
```

If the device registers but never gets an assignment, compare the version's
requirements with the device: `fleetml device profile <device ID>` prints its
score and RAM (`fleetml device list --project <project ID>` gives the ID). How
the server matches devices, labels and variants is in
[ROLLOUTS.md](ROLLOUTS.md#how-a-device-gets-a-model).

## Battery and data

| | In the foreground | In the background |
|---|---|---|
| Heartbeat | every `heartbeat_interval_seconds` (server default 60 s) | one per background refresh |
| Telemetry | batches of `telemetry_batch_size` (100), every `telemetry_flush_interval_seconds` (30 s) and when a batch fills | one flush when the app goes to the background, one per background refresh |
| Background refresh | not scheduled | a `BGAppRefreshTask` no earlier than `background_heartbeat_interval_seconds` (1 h, at least 15 min); iOS decides the exact time |
| Model downloads | per `download_policy` | per `download_policy` |

- **Foreground only timers.** The SDK stops its timers when the app enters the
  background and starts them when it returns. To get the background refresh,
  add `com.fleetml.sdk.refresh` (`FleetML.backgroundTaskIdentifier`) to
  `BGTaskSchedulerPermittedIdentifiers` in your `Info.plist`, enable the
  "Background fetch" mode (`UIBackgroundModes`: `fetch`), and call
  `FleetML.registerBackgroundTask()` and `FleetML.initialize` before the app
  finishes launching (the sample app does both in its `App` initializer).
  `FleetML.syncNow()` runs one heartbeat and one telemetry flush on demand.
- **Downloads.** `.unmeteredOrCharging` (the default): a network that is
  neither expensive (cellular, personal hotspot) nor constrained (Low Data
  Mode), or any network while the device charges. `.unmeteredOnly` and
  `.anyNetwork` are the alternatives. A waiting assignment is retried on the
  next heartbeat. Assignments of an
  [urgent deployment](ROLLOUTS.md#urgent-deployments-and-the-download-policy),
  and every rollback, download on any connected network.
- **Telemetry** is always batched; no event is sent on its own.
- The server sets all of these for every device with the `DEVICE_*`
  variables ([CONFIG.md](CONFIG.md#devices)); `FleetMLConfig` only provides
  the values used before the first response.
- The iOS Simulator cannot run background tasks (`BGTaskScheduler` reports
  it unavailable, and an app that permits the task logs
  `Background refresh not scheduled: background refresh is unavailable here (the simulator, or Background App Refresh is off in Settings)`
  at info level); test the background refresh on a device: run the app from
  Xcode, send it to the background, pause it in Xcode's debugger (the LLDB
  prompt needs the debugger attached, which a `devicectl` launch does not
  have), and enter
  `e -l objc -- (void)[[BGTaskScheduler sharedScheduler] _simulateLaunchForTaskWithIdentifier:@"com.fleetml.sdk.refresh"]`,
  then continue.

## Offline behaviour

Models activated in an earlier run are delivered through
`fleetML(modelReady:)` (and `FleetML.activeModels`) as soon as the SDK starts,
before the server is reached. Until the server answers, the app keeps using its
cached models; `fleetML(connectionChanged:)` reports reachability within about
ten seconds.

![Offline start with the server down](screenshots/ios-offline-start.png)

## Telemetry

```swift
if let model = FleetML.activeModel(named: "iris") {
    FleetML.telemetry?.reportInference(model: model, latencyMs: 3, confidence: 0.97, label: "virginica")
    // with input values, for feature-level drift (only features configured on the server are kept):
    FleetML.telemetry?.reportInference(model: model, latencyMs: 3, features: ["petal_length": [5.1]])
}
```

Events go into a persistent queue (bounded to 10,000; oldest dropped first):
they survive app termination and are kept while offline or before
registration. They are sent in batches every flush interval, with exponential
backoff (5 s up to 5 min) after a failure and an immediate flush when the
connection returns. For a model in an A/B experiment the event carries the
device's arm ID automatically. Feature values are binned on the device with
the edges from the server ([feature-level drift](ROLLOUTS.md#feature-level-drift));
only the counts are sent.

## Tests

```bash
make ios-test
```

runs the unit tests on macOS (`swift test` at the repository root). The same
suite runs on a simulator, from the root, with the first available iPhone
simulator (`xcrun simctl list devices available` lists them):

```bash
SIM=$(xcrun simctl list devices available -j | jq -r '[.devices[][] | select(.name | startswith("iPhone"))][0].udid')
xcodebuild test -scheme FleetMLSDK -destination "platform=iOS Simulator,id=$SIM"
```

For contributors, against the dev stack (`make dev`):
`FLEETML_API_KEY=<admin key> ./scripts/ios-e2e.sh` runs the live test, and
`FLEETML_API_KEY=<admin key> ./scripts/signing-e2e.sh ios` the signature
checks (tampered, and the rotation grace). `scripts/ios-e2e.sh` creates a
non-urgent deployment and runs `LiveServerTests` on the simulator: a device on
an expensive network and on battery registers, its model waits, then an urgent
deployment downloads and activates. Without the script's environment the live
test is skipped.

The package's deployment target is iOS 15. The suite, the live test and the
signing check have passed on the iOS 16.4 simulator (an iPhone 14) and on the
iOS 26 simulator; no iOS 15 runtime was tested. Older runtimes are installed
with `xcodebuild -downloadPlatform iOS -buildVersion 16.4`. The SDK was also
verified on a physical iPhone running iOS 26 (lifecycle, update, offline,
download policy, signing, background refresh), with the sample app as in
[Physical device](#physical-device).

The suite covers the telemetry queue and reporter, the model store, the
downloader (including an interrupted transfer resumed with Range), cache
eviction, the model manager (activation, last-known-good, UPDATE + UNLOAD,
restart), the token refresh policy, the heartbeat loop, connection errors
reaching the delegate, the Core ML validator on the real Iris model, the
battery and data policy, and signature checks against the shared vectors in
`internal/artifact/testdata`. `make proto-swift` regenerates the gRPC client
code from `proto/`.

## Simulator runbook

This is how the SDK and sample app are verified, against the server from the
[quickstart](QUICKSTART.md#run-fleetml) (the production compose stack, with the
admin key in `$FLEETML_ADMIN_KEY`) and the `fleetml` CLI, from the root of a
clone. The simulator shares the Mac's `localhost`, so the server needs no
change. It needs Xcode and [XcodeGen](https://github.com/yonaskolb/XcodeGen)
(`brew install xcodegen`). Contributors on the dev stack (`make dev`) use the
key from `.env` instead.

1. Start the server with the quickstart's "Run FleetML" block, and keep its
   shell: it holds `FLEETML_ADMIN_KEY`.

2. Publish the Core ML sample models (with the requirements override from
   [Which variant a device gets](#which-variant-a-device-gets)), create an
   enrollment key and deploy 1.0.0 to the sample app's label (`app=sample`):

   ```bash
   export FLEETML_SERVER=http://localhost:18080 FLEETML_API_KEY=$FLEETML_ADMIN_KEY
   REQ='{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}'
   P=$(fleetml project create --name ios-demo -o json | jq -r .id)
   fleetml push models/iris/iris_v1.mlmodel --project $P --model iris --version 1.0.0 --framework COREML --quantization FP32 --metadata "$REQ"
   fleetml push models/iris/iris_v2.mlmodel --project $P --model iris --version 2.0.0 --framework COREML --quantization FP32 --metadata "$REQ"
   M=$(fleetml model list --project $P -o json | jq -r '.[0].id')
   V1=$(fleetml ver list --model $M -o json | jq -r '.[] | select(.version == "1.0.0") | .id')
   V2=$(fleetml ver list --model $M -o json | jq -r '.[] | select(.version == "2.0.0") | .id')
   EK=$(fleetml enrollment-key create --project $P --name simulator -o json | jq -r .key)
   D=$(fleetml deploy create --project $P --version $V1 --target '{"app":"sample"}' -o json | jq -r .id)
   fleetml deploy start $D
   ```

   Each push prints `✓ Pushed ... (FP32)`, and the deployment starts as
   `COMPLETE` (`✓ Deployment start → COMPLETE`): it waits for the device.

3. Generate the Xcode project, build, install and launch the sample app on the
   first available iPhone simulator, with the settings as launch arguments
   (or, after `xcodegen generate`, open `ios/SampleApp/FleetMLSample.xcodeproj`,
   run it and type them on the config screen):

   ```bash
   cd ios/SampleApp && xcodegen generate
   SIM=$(xcrun simctl list devices available -j | jq -r '[.devices[][] | select(.name | startswith("iPhone"))][0].udid')
   xcodebuild -project FleetMLSample.xcodeproj -scheme FleetMLSample -destination "platform=iOS Simulator,id=$SIM" -derivedDataPath build build
   xcrun simctl boot $SIM; open -a Simulator
   xcrun simctl install $SIM build/Build/Products/Debug-iphonesimulator/FleetMLSample.app
   xcrun simctl launch --console-pty $SIM com.fleetml.sample -fleetmlHost localhost -fleetmlEnrollmentKey "$EK"
   ```

   `simctl boot` reports an error if that simulator is already booted, which
   does no harm. On a simulator where the sample app ran before, its saved
   settings stay, the port included, and the launch arguments replace only
   the host and the key: `xcrun simctl uninstall $SIM com.fleetml.sample`
   before the install starts it clean. The app registers, downloads iris 1.0.0, validates it with
   Core ML, shows **Active model: iris 1.0.0** and classifies the sample
   flower with it, reporting inference telemetry; the console prints the same
   steps (`FleetMLSample: Registered as ...`, `Model ready: iris 1.0.0`).
   `fleetml deploy create --project $P --version $V2 --target '{"app":"sample"}'`
   and `deploy start` update the app to 2.0.0 on its next heartbeat, 60 seconds
   by default (1.0.0 is kept as last-known-good). `fleetml deploy get $D | jq -c .devices`
   counts the devices whose last heartbeat reported a version, so `loaded`
   turns one heartbeat after the app shows the model, and it is counted each
   time you ask: once 2.0.0 replaces 1.0.0, `$D` counts down to
   `{"loaded":0,"targeted":1}`.

4. Offline start: stop the server
   (`docker compose -f deploy/docker-compose.prod.yml stop server`), then
   `xcrun simctl terminate $SIM com.fleetml.sample` and launch the app again.
   The cached model is active immediately and inference works; events are
   queued and sent once the server is back
   (`docker compose -f deploy/docker-compose.prod.yml start server`). On a
   server that others use, launch the app with a closed port instead
   (`-fleetmlPort 19999` as one more launch argument), as in
   [Your own app](#your-own-app).

### Physical device

The runbook works on an iPhone too, over Wi-Fi, with the same server and
models:

1. On the phone, turn on Developer Mode (Settings, Privacy & Security,
   Developer Mode) and restart it when asked.
2. Put the phone and the Mac on the same Wi-Fi network. The phone reaches the
   server at the Mac's LAN address (`ipconfig getifaddr en0`), so restart the
   stack with the object store on that address too, in the quickstart's shell,
   which makes the presigned download URLs reachable from the phone:

   ```bash
   S3_PUBLIC_ENDPOINT=<Mac LAN IP>:19100 docker compose -f deploy/docker-compose.prod.yml up -d
   ```

   A phone that registers but never downloads: it is almost always the
   endpoint.

3. In Xcode, add a free Apple ID (Settings, Accounts); after
   `xcodegen generate`, open `ios/SampleApp/FleetMLSample.xcodeproj`, and on
   the FleetMLSample target's Signing & Capabilities tab turn on
   **Automatically manage signing** with the personal team.
4. Build from the command line, then install and launch the app with the
   Mac's address and the enrollment key as launch arguments (the config
   screen's default host, `localhost`, is the phone itself, so on a phone
   always give the Mac's address). `<team>` is the personal team's ID, which
   Xcode puts in the target's `DEVELOPMENT_TEAM` setting, and `<device>` is
   the phone's name or identifier from `xcrun devicectl list devices`:

   ```bash
   cd ios/SampleApp && xcodegen generate
   xcodebuild -project FleetMLSample.xcodeproj -scheme FleetMLSample -destination 'generic/platform=iOS' -derivedDataPath build -allowProvisioningUpdates DEVELOPMENT_TEAM=<team> ENABLE_PREVIEWS=NO build
   xcrun devicectl device install app --device <device> build/Build/Products/Debug-iphoneos/FleetMLSample.app
   xcrun devicectl device process launch --device <device> --console com.fleetml.sample -- -fleetmlHost <Mac LAN IP> -fleetmlEnrollmentKey "$EK"
   ```

   Command-line builds for a device fail at codesign with
   `errSecInternalComponent` on `__preview.dylib` unless `ENABLE_PREVIEWS=NO`
   is passed. The first launch of a personally signed app stops at a trust
   prompt: trust the developer in Settings, General, VPN & Device Management,
   then launch again. iOS then asks whether the app may find and connect to
   devices on the local network: allow it ([Own app on a real iPhone](#own-app-on-a-real-iphone)).

### Own app on a real iPhone

The sample app has what a phone needs; an own app needs the same three things
before it can use a server on the Mac:

1. **App Transport Security.** The SDK downloads models with `URLSession`
   over plain HTTP from the Mac, which ATS blocks. Allow local networks only,
   in the app's `Info.plist`:

   ```xml
   <key>NSAppTransportSecurity</key>
   <dict>
       <key>NSAllowsLocalNetworking</key>
       <true/>
   </dict>
   ```

2. **Local Network permission.** Add a reason to the `Info.plist`; iOS shows
   it in the permission prompt on the first launch that connects to the Mac:

   ```xml
   <key>NSLocalNetworkUsageDescription</key>
   <string>Connects to the FleetML server on your network.</string>
   ```

   If the prompt was dismissed or denied, turn the app on in Settings,
   Privacy & Security, Local Network. Until then the SDK reports
   `fleetML(connectionChanged: false)`, `fleetML(connectionFailed:)` shows a
   connection error, and any `URLSession` request to the Mac's LAN address
   fails with -1009 ("The Internet connection appears to be offline"). Once
   allowed, no relaunch is needed: the SDK registers on its next retry (within
   10 seconds) and the first model arrives with the next heartbeat, 60 seconds
   later at most with the server's default.

3. **The server's object store on the Mac's LAN address:**
   `S3_PUBLIC_ENDPOINT=<Mac LAN IP>:19100` as in step 2 of
   [Physical device](#physical-device), and `serverHost` set to the same
   address. Registration goes over gRPC to port 19090, but every download
   uses a presigned URL for `S3_PUBLIC_ENDPOINT`: registers but never
   downloads, it is almost always the endpoint.

Release builds need neither exception: they talk to a server behind a TLS
proxy with a real host name, as in [Configure and start](#configure-and-start).

The live tests (`scripts/ios-e2e.sh` and `scripts/signing-e2e.sh ios`) stay
simulator-only, because Swift package tests cannot run on a physical device
(`xcodebuild` reports "Tool-hosted testing is unavailable on device
destinations") unless an app target hosts them. The download policy can be
exercised on a phone by hand instead: on battery and with Low Data Mode on for
the Wi-Fi network, a non-urgent deployment waits and an urgent one downloads.
The background refresh, which the simulator cannot run, is tested on a phone
with the debugger command in [Battery and data](#battery-and-data).

## Sample app

`ios/SampleApp` is the reference integration, generated with XcodeGen from
`project.yml`: a SwiftUI config screen (host, port, enrollment key), the SDK
owned by an app-wide object, the Iris Core ML model from `models/iris`
(Apache-2.0) bundled and used until the server assigns one, and inference
telemetry. Its `Info.plist` sets `NSAllowsLocalNetworking` and
`NSLocalNetworkUsageDescription` so model downloads from a server on the Mac
work over plain HTTP, on the simulator and on a phone; production servers
should serve presigned URLs over HTTPS. The config screen's host defaults to
`localhost`, which works on the simulator; on a phone, use the Mac's LAN
address.
