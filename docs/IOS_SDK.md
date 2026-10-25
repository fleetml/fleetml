# iOS SDK

The FleetML iOS SDK (a Swift package for iOS 15 and later, with its manifest at
the repository root and its code in `ios/FleetMLSDK`) puts an iPhone or iPad
app under FleetML's control: it registers with an enrollment key, receives
model assignments over gRPC, downloads and verifies them, validates each model
with Core ML before using it, keeps the last-known-good version, works offline,
and reports telemetry. It does not run your models: your app does, with Core ML
or anything else. It mirrors the [Android SDK](ANDROID_SDK.md) feature for
feature.

This guide starts with the sample app on the simulator and then on your
iPhone ([Get started](#get-started)), then follows the path to your own app
and to production: add the package, configure the app from build settings,
run it on your iPhone with a model published and deployed to it, then the
same on the simulator, and what production needs. The sample app and the
tests come last.

## Get started

The [sample app](#sample-app) on the simulator, then on your iPhone, against
a server on your Mac: copy and paste, nothing to choose. Each step says what
you should see; if you see something else, stop there and look it up in
[Troubleshooting](ADVANCED.md#troubleshooting). Use one terminal and keep it
open until the end.

Before you start, once: Xcode with an iOS simulator (Xcode, Settings,
Components), [Homebrew](https://brew.sh) and Docker Desktop.

1. **Check the tools.** This installs [XcodeGen](https://github.com/yonaskolb/XcodeGen),
   which generates the sample app's Xcode project:

   <!-- golden: ios-tools device -->
   ```bash
   brew install -q xcodegen
   docker compose version && jq --version && xcodebuild -version && xcodegen --version
   ```

   You should see `Xcode` and `Version: `.

2. **Start FleetML.** The first start takes a few minutes, to download and
   build the images:

   <!-- golden: start -->
   ```bash
   git clone https://github.com/fleetml/fleetml.git && cd fleetml
   [ -f ~/.fleetml.env ] || (umask 077; printf "export JWT_SECRET='%s' SIGNING_MASTER_KEY='%s' FLEETML_ADMIN_KEY='%s' FLEETML_REST_PORT=%s FLEETML_GRPC_PORT=%s FLEETML_DASHBOARD_PORT=%s FLEETML_S3_PUBLIC_PORT=%s S3_PUBLIC_ENDPOINT=localhost:%s\n" "$(openssl rand -base64 32)" "$(openssl rand -base64 32)" "$(openssl rand -hex 32)" "${FLEETML_REST_PORT:-18080}" "${FLEETML_GRPC_PORT:-19090}" "${FLEETML_DASHBOARD_PORT:-80}" "${FLEETML_S3_PUBLIC_PORT:-19100}" "${FLEETML_S3_PUBLIC_PORT:-19100}" > ~/.fleetml.env)
   source ~/.fleetml.env
   docker compose -f deploy/docker-compose.prod.yml up -d --build
   until curl -sf localhost:${FLEETML_REST_PORT:-18080}/readyz; do sleep 2; done; echo
   curl -s -H "Authorization: Bearer $FLEETML_ADMIN_KEY" localhost:${FLEETML_REST_PORT:-18080}/api/v1/projects; echo
   echo "Dashboard: http://localhost:${FLEETML_DASHBOARD_PORT:-80}  (log in with FLEETML_ADMIN_KEY from ~/.fleetml.env)"
   ```

   You should see `{"db":"connected","nats":"connected","s3":"connected","status":"ok","version":"<version>"}`.
   The block saved the server's secrets in `~/.fleetml.env`: keep that file.

3. **Get the CLI**, checked against the release's checksums:

   <!-- golden: cli -->
   ```bash
   OS=$(uname -s | tr '[:upper:]' '[:lower:]'); ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
   curl -fsSLO "https://github.com/fleetml/fleetml/releases/latest/download/fleetml-$OS-$ARCH.tar.gz"
   curl -fsSLO https://github.com/fleetml/fleetml/releases/latest/download/SHA256SUMS
   grep " fleetml-$OS-$ARCH.tar.gz\$" SHA256SUMS | shasum -a 256 -c
   tar -xzf "fleetml-$OS-$ARCH.tar.gz" fleetml && mkdir -p ~/.local/bin && mv fleetml ~/.local/bin/
   rm "fleetml-$OS-$ARCH.tar.gz" SHA256SUMS
   export PATH="$HOME/.local/bin:$PATH"
   fleetml version
   ```

   You should see `fleetml-<os>-<arch>.tar.gz: OK` and `✓ fleetml v<version>`.

4. **Publish the sample model and deploy it**, the Core ML build of it. The
   project gets the time in its name, so running the block again makes a new
   one:

   <!-- golden: ios-publish -->
   ```bash
   export FLEETML_SERVER=http://localhost:${FLEETML_REST_PORT:-18080} FLEETML_API_KEY=$FLEETML_ADMIN_KEY
   P=$(fleetml project create --name "first-ios-$(date +%H%M%S)" -o json | jq -r .id)
   fleetml push models/iris/iris_v1.mlmodel --project $P --model iris --version 1.0.0 --framework COREML --quantization FP32 --metadata '{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}'
   V=$(fleetml ver list --model "$(fleetml model list --project $P -o json | jq -r '.[0].id')" -o json | jq -r '.[0].id')
   fleetml deploy start "$(fleetml deploy create --project $P --version $V -o json | jq -r .id)"
   export FLEETML_ENROLLMENT_KEY=$(fleetml enrollment-key create --project $P --name first-device -o json | jq -r .key)
   ```

   You should see `✓ Pushed models/iris/iris_v1.mlmodel → iris v1.0.0 (FP32)` and `✓ Deployment start → COMPLETE`.

5. **Put the simulator on it.** This builds the sample app, starts an iPhone
   simulator, installs the app and launches it with the enrollment key in
   its environment, then waits until the model is active (a few minutes the
   first time, for the build):

   <!-- golden: ios-simulator device -->
   ```bash
   (cd ios/SampleApp && xcodegen generate --quiet)
   SIM=$(xcrun simctl list devices available -j | jq -r '[.devices[][] | select(.name | startswith("iPhone"))][0].udid')
   xcodebuild -quiet -project ios/SampleApp/FleetMLSample.xcodeproj -scheme FleetMLSample -destination "platform=iOS Simulator,id=$SIM" -derivedDataPath ios/SampleApp/build build
   xcrun simctl boot $SIM 2>/dev/null; open -a Simulator
   xcrun simctl install $SIM ios/SampleApp/build/Build/Products/Debug-iphonesimulator/FleetMLSample.app
   SIMCTL_CHILD_FLEETML_ENROLLMENT_KEY=$FLEETML_ENROLLMENT_KEY xcrun simctl launch --terminate-running-process --stdout="$PWD/sample.log" $SIM com.fleetml.sample
   for i in $(seq 60); do grep 'Model ready' sample.log && break; sleep 2; done
   ```

   You should see `FleetMLSample: Model ready: iris 1.0.0`, and the app's
   screen **Active model: iris 1.0.0**. Tap **Classify sample flower** to
   run it.

6. **Put your iPhone on it.** Once, before: on the phone, Developer Mode
   (Settings, Privacy & Security, Developer Mode, then restart); connect it
   to the Mac with a cable once and tap **Trust**; in Xcode, your Apple ID
   (Settings, Accounts) and an Apple Development certificate (Manage
   Certificates, +). The phone and the Mac on the same Wi-Fi, the phone
   unlocked. The block points the server's downloads at the Mac's network
   address, which the phone reaches, then builds, installs and launches the
   app on the phone:

   <!-- golden: ios-iphone phone -->
   ```bash
   . ~/.fleetml.env && sed -i.bak "s/S3_PUBLIC_ENDPOINT=[^ ]*/S3_PUBLIC_ENDPOINT=$(ipconfig getifaddr en0):${FLEETML_S3_PUBLIC_PORT:-19100}/" ~/.fleetml.env && rm ~/.fleetml.env.bak
   source ~/.fleetml.env && docker compose -f deploy/docker-compose.prod.yml up -d
   TEAM=$(security find-certificate -c "Apple Development" -p | openssl x509 -noout -subject | grep -o 'OU=[A-Z0-9]*' | cut -d= -f2)
   PHONE=$(xcrun devicectl list devices --quiet --json-output /dev/stdout | jq -r '[.result.devices[] | select(.hardwareProperties.platform == "iOS" and .connectionProperties.pairingState == "paired")][0].identifier')
   xcodebuild -quiet -project ios/SampleApp/FleetMLSample.xcodeproj -scheme FleetMLSample -destination 'generic/platform=iOS' -derivedDataPath ios/SampleApp/build -allowProvisioningUpdates DEVELOPMENT_TEAM=$TEAM build
   xcrun devicectl device install app --device $PHONE ios/SampleApp/build/Build/Products/Debug-iphoneos/FleetMLSample.app
   DEVICECTL_CHILD_FLEETML_HOST=$(ipconfig getifaddr en0) DEVICECTL_CHILD_FLEETML_ENROLLMENT_KEY=$FLEETML_ENROLLMENT_KEY xcrun devicectl device process launch --device $PHONE --terminate-existing com.fleetml.sample
   for i in $(seq 60); do fleetml device list --project $P -o json | jq -e 'length == 2' > /dev/null && break; sleep 3; done; fleetml device list --project $P
   ```

   You should see `Launched application with com.fleetml.sample bundle identifier`
   and two devices in the list, the simulator and the iPhone. The first
   launch of the app on a phone stops at two prompts on the phone: trust the
   developer (Settings, General, VPN & Device Management) and allow Local
   Network access; after you tap them, run the `devicectl device process launch`
   line again. The phone's screen then shows **Active model: iris 1.0.0**.

7. **See it in the dashboard.** This puts the admin key on the clipboard (on
   a Mac; elsewhere it prints it):

   <!-- golden: dashboard manual -->
   ```bash
   printf %s "$FLEETML_ADMIN_KEY" | pbcopy 2>/dev/null || echo "$FLEETML_ADMIN_KEY"
   ```

   Open <http://localhost> and paste the key into the login box. You should see the
   Overview page; **Devices** lists the simulator (and the iPhone), and **Deployments** one
   `COMPLETE` deployment of iris 1.0.0.

8. **Clean up**: the app on the simulator and the phone, the build, the
   server and all its data, and the saved secrets:

   <!-- golden: ios-cleanup device cleanup -->
   ```bash
   xcrun simctl uninstall $SIM com.fleetml.sample; xcrun simctl shutdown $SIM
   [ -z "${PHONE:-}" ] || xcrun devicectl device uninstall app --device $PHONE com.fleetml.sample
   rm -rf ios/SampleApp/build ios/SampleApp/FleetMLSample.xcodeproj sample.log
   source ~/.fleetml.env && docker compose -f deploy/docker-compose.prod.yml down -v
   rm -f ~/.fleetml.env
   ```

   You should see `Network <stack>_default  Removed`.

Then your own app: [Install](#install) and [Configure and start](#configure-and-start),
and [Your own app on an iPhone](#your-own-app-on-an-iphone) for its own
project, signing and Info.plist keys.

## Install

It needs Xcode 15 or later (16 recommended; the package manifest is
swift-tools-version 5.9). The package comes from GitHub or from a clone of
this repository, the latter to work against changes that are not released yet:

| | From GitHub | From a clone |
|---|---|---|
| Xcode | **File → Add Package Dependencies**, paste `https://github.com/fleetml/fleetml`, Dependency Rule **Up to Next Major Version** from 0.9.1, add the **FleetMLSDK** library to the app target | **File → Add Package Dependencies → Add Local…**, pick the clone's root folder, add **FleetMLSDK** to the app target |
| `Package.swift` | `.package(url: "https://github.com/fleetml/fleetml", from: "0.9.1")` | `.package(path: "path/to/fleetml")` |
| XcodeGen `project.yml` | `url: https://github.com/fleetml/fleetml` with `from: 0.9.1` | `path: ../fleetml`, relative to `project.yml` |

In a `Package.swift`, the target depends on the **FleetMLSDK** product, and
`package:` names the dependency by its identity, which Swift Package Manager
takes from the URL or the folder, not from the manifest's own name: `fleetml`
for the GitHub URL, and the clone folder's name, lowercased, for a path
(`fleetml` for a plain `git clone`):

```swift
.product(name: "FleetMLSDK", package: "fleetml"),
```

With `package: "FleetMLSDK"` instead, the build stops with
`unknown package 'FleetMLSDK' in dependencies of target '<your target>'; valid packages are: 'fleetml'`.
With XcodeGen, the package's key in `project.yml` is a name of your choice,
and the target depends on it (`product: FleetMLSDK` can be left out when the
key is `FleetMLSDK` too); the [own app runbook](#your-own-app-on-an-iphone)
has a whole file:

```yaml
packages:
  FleetMLSDK:
    url: https://github.com/fleetml/fleetml
    from: 0.9.1        # or path: ../fleetml for a clone
targets:
  MyApp:
    dependencies:
      - package: FleetMLSDK
```

The v0.3.0 and earlier tags have no manifest at the repository root, so
0.4.0 is the first version Swift Package Manager can install by URL. The
package depends on grpc-swift, pinned to exactly 1.27.6 (the 1.x line, which
supports iOS 15; 2.x needs iOS 18), and on swift-protobuf from 1.28. An app
that already depends on grpc-swift must use that same version, or Swift
Package Manager cannot resolve the graph. macOS 12 is supported too, for tests
and tools.

## Configure and start

Initialize once, for example in your `App` or app delegate. The server's
address, the port, TLS and the enrollment key come from the app's
`Info.plist`, where the build puts them from an `.xcconfig` file you do not
commit, so moving between a phone, the simulator, a closed port for an offline
test and production is an edit of that file, not of the code. This object
starts the SDK, prints what it hears, and loads each model it delivers with
Core ML:

```swift
import CoreML
import FleetMLSDK

final class AppModel: FleetMLDelegate {
    private(set) var mlModel: MLModel?

    private static func setting(_ key: String) -> String {
        Bundle.main.object(forInfoDictionaryKey: key) as? String ?? ""
    }

    init() {
        do {
            try FleetML.initialize(
                config: FleetMLConfig(serverHost: Self.setting("FleetMLServerHost"),
                                      serverPort: Int(Self.setting("FleetMLServerPort")) ?? 443,
                                      enrollmentKey: Self.setting("FleetMLEnrollmentKey"),
                                      useTLS: Self.setting("FleetMLUseTLS") == "YES",
                                      labels: ["app": "myapp", "tier": "beta"]),
                delegate: self)
        } catch {
            print("FleetML did not start: \(error)")
        }
    }

    func fleetML(registered deviceID: String) {
        print("FleetML registered as \(deviceID)")
    }

    func fleetML(connectionChanged connected: Bool) {
        print("FleetML connected: \(connected)")
    }

    func fleetML(connectionFailed error: Error) {
        print("FleetML cannot register: \(error)")
    }

    func fleetML(modelReady model: ActiveModel) {
        print("FleetML model ready: \(model.modelName) \(model.version)")
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

The four values, in an `.xcconfig` file set as the target's configuration
file and kept out of git (`Secrets.xcconfig` below), here for an iPhone and a
server on the Mac:

```text
FLEETML_SERVER_HOST = <Mac LAN IP>
FLEETML_SERVER_PORT = 19090
FLEETML_USE_TLS = NO
FLEETML_ENROLLMENT_KEY = <enrollment key>
```

and four entries in the target's `Info.plist` that take them over:
`FleetMLServerHost` = `$(FLEETML_SERVER_HOST)`, `FleetMLServerPort` =
`$(FLEETML_SERVER_PORT)`, `FleetMLUseTLS` = `$(FLEETML_USE_TLS)` and
`FleetMLEnrollmentKey` = `$(FLEETML_ENROLLMENT_KEY)` (on the target's Info tab
as Custom iOS Target Properties, or under `info: properties:` with XcodeGen;
the `INFOPLIST_KEY_` build settings of a generated `Info.plist` take only
Apple's keys). The simulator uses `localhost` as the host, and production
your server's name, port 443 and `YES` ([Production](#production)). For a
quick test only, the values can be literals in the code, but the key then
sits in a file you commit.

`print` goes to Xcode's console, or to the terminal with
`simctl launch --console-pty` on the simulator and
`devicectl device process launch --console` on a phone. The SDK holds the
delegate weakly: keep the object alive for as long as the app runs, for
example as a property of your `App` or app delegate, or its callbacks stop.
Delivered files keep the server's name (`{variant}.bin`), and Core ML
compiles only from a path ending in `.mlmodel`, which is why
`fleetML(modelReady:)` copies the file to one first (the sample app's
`IrisClassifier` does the same).

Without the background task set up (optional, see [Battery and
data](#battery-and-data)), the SDK logs one info line each time the app goes
to the background, and at a launch that iOS starts in the background state
(as the first launch after `open -a Simulator` can be):
`Background refresh not scheduled (BGTaskSchedulerErrorDomain error 3): the app does not permit com.fleetml.sdk.refresh; add BGTaskSchedulerPermittedIdentifiers and call registerBackgroundTask(), see docs/IOS_SDK.md "Battery and data"`.
It is harmless: the SDK works in the foreground as described here (SDK 0.5.0
logged the same thing as an error).

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
| `useTLS` | `false` | TLS for gRPC (`true` with the server behind a TLS proxy, as in production) |
| `labels` | `[:]` | Labels deployments and experiments target (`target_selector`) |
| `heartbeatIntervalSeconds` | `30` | Foreground heartbeat interval, until the server sends its own (server default 60) |
| `backgroundHeartbeatIntervalSeconds` | `3600` | Earliest interval of the background refresh, until the server sends its own (at least 15 minutes) |
| `telemetryBatchSize`, `telemetryFlushIntervalSeconds` | `50`, `30` | Until the server's `DeviceConfig` overrides them (server defaults 100 and 30) |
| `maxCacheSizeMB` | `500` | Disk budget for models; the active and last-known-good models are never evicted |
| `downloadPolicy` | `.unmeteredOrCharging` | Which networks non-urgent downloads may use, until the server sends its own ([Battery and data](#battery-and-data)) |
| `autoStart` | `true` | Call `FleetML.start()` yourself if `false` |

The parameters after `serverHost` all have defaults and are labelled, in the
order of the table. `FleetML.stop()` stops background work so `initialize` can
be called again (for example with a new server address). `FleetML.reset()`
also deletes cached models and the registration.

### When it does not connect

If the SDK cannot register, `fleetML(connectionChanged:)` reports `false` and
`fleetML(connectionFailed:)` receives the error, on the first failure after
start and again whenever the error changes (registration is retried every
10 seconds). `connectionFailed` is for registration only: a device that has
registered before and cannot reach the server reports it with
`fleetML(connectionChanged: false)` and the log line `Heartbeat failed: ...`
(below).

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
lines). The runbooks below show how to read it from a terminal.

A first launch that registers and receives iris 1.0.0 logs these lines (time
stamps and the process column left out):

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

## Your own app on an iPhone

This runs an own app with the `AppModel` above on an iPhone, against a FleetML
server on your Mac (the [quickstart's](QUICKSTART.md#run-fleetml), or one a
colleague runs that the phone can reach), with
[XcodeGen](https://github.com/yonaskolb/XcodeGen) (`brew install xcodegen`),
`jq`, and the `fleetml` CLI on the `PATH` ([Get the
CLI](QUICKSTART.md#get-the-cli)). It is not verified on a phone by CI; the
maintainers run it by hand. [The same on the
simulator](#the-same-on-the-simulator) follows as a shorter variant.

The app's folder holds `project.yml`, `Secrets.xcconfig` and a `Sources`
folder with `AppModel.swift` (above, labels `["app": "myapp", ...]`) and an
`App` that keeps it alive:

```swift
import FleetMLSDK
import SwiftUI

@main
struct MyApp: App {
    private let model: AppModel

    init() {
        FleetML.registerBackgroundTask() // before launch finishes; needs step 4's background keys
        model = AppModel()
    }

    var body: some Scene { WindowGroup { Text("FleetML") } }
}
```

1. **The phone.** Turn on Developer Mode (Settings, Privacy & Security,
   Developer Mode) and restart it when asked. In Xcode, add your Apple ID
   (Settings, Accounts); a free one gives a personal team. Your team's ID is
   on developer.apple.com under Membership, in Xcode's Signing & Capabilities
   tab once a project uses the team, or, from a terminal, in your
   development certificate (once Xcode has made one):

   ```bash
   security find-certificate -c "Apple Development" -p | openssl x509 -noout -subject | grep -o 'OU=[A-Z0-9]*'
   ```

   The team ID is the 10 characters after `OU=`. The ID in parentheses that
   `security find-identity -v -p codesigning` prints is another one, and
   with it `xcodebuild` stops with `No Account for Team "<ID>"`. Put the
   phone and the Mac on the same Wi-Fi network.

   Everything from step 5 on needs the phone awake, unlocked and reachable,
   and some of it needs a hand on the phone once. Do these before you
   start, and the terminal steps can then run with the phone on the desk:

   - Developer Mode on, and the Mac trusted (connect the cable once and tap
     Trust).
   - **Connect via network** in Xcode's Devices and Simulators window
     (Shift-Command-2), if you want to work without the cable (step 5,
     Wirelessly).
   - Settings, Display & Brightness, Auto-Lock, Never, and the phone on its
     charger.
   - For a bundle ID the phone has not run before, the first launch in step
     5 stops at two prompts: trust the developer (Settings, General, VPN &
     Device Management) and allow Local Network access. After that,
     relaunches need no hand at the phone.

2. **The server, with its object store on the Mac's LAN address.** The phone
   reaches the server at the Mac's LAN address (`ipconfig getifaddr en0`,
   `en1` on some Macs), and every model download is a presigned URL for the
   server's `S3_PUBLIC_ENDPOINT`, so that must be the same address. Save it
   in the quickstart's `~/.fleetml.env` (so every later compose command
   keeps it) and restart the server, in the quickstart's clone:

   ```bash
   . ~/.fleetml.env && sed -i.bak "s/S3_PUBLIC_ENDPOINT=[^ ]*/S3_PUBLIC_ENDPOINT=$(ipconfig getifaddr en0):${FLEETML_S3_PUBLIC_PORT:-19100}/" ~/.fleetml.env && rm ~/.fleetml.env.bak
   source ~/.fleetml.env
   docker compose -f deploy/docker-compose.prod.yml up -d
   ```

   If someone else runs the server, ask them to start it this way. A phone
   that registers but never downloads: it is almost always the endpoint.

   Check the network path from the phone before the app: in Safari on the
   phone, `http://<Mac LAN IP>:18080/readyz` must show `"status":"ok"`, and
   `http://<Mac LAN IP>:19100` must answer at all (an XML error page from
   the object store is fine). If Safari cannot load them, it is the Wi-Fi
   (a guest network that isolates devices) or the Mac's firewall, not the
   app or the SDK.

3. **The models, the enrollment key and the deployment**, from the root of a
   clone (for the Core ML sample models; push your own `.mlmodel` instead),
   with the key your team gave you or the quickstart's `$FLEETML_ADMIN_KEY`
   (which key does what: [Android's list](ANDROID_SDK.md#publish-and-deploy-with-the-cli)
   applies here too). Project names are unique on a server:
   `project already exists: slug "myapp-ios"` means the project is there from an
   earlier run, so pick another name.

   ```bash
   export FLEETML_SERVER=http://localhost:18080 FLEETML_API_KEY=<the admin or project key your team gave you>
   REQ='{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}'
   P=$(fleetml project create --name myapp-ios -o json | jq -r .id)   # or P=<the project ID your team gave you>
   fleetml push models/iris/iris_v1.mlmodel --project $P --model iris --version 1.0.0 --framework COREML --quantization FP32 --metadata "$REQ"
   fleetml push models/iris/iris_v2.mlmodel --project $P --model iris --version 2.0.0 --framework COREML --quantization FP32 --metadata "$REQ"
   M=$(fleetml model list --project $P -o json | jq -r '.[] | select(.name == "iris") | .id')
   vid() { fleetml ver list --model $M -o json | jq -r --arg v "$1" '.[] | select(.version == $v) | .id'; }
   EK=$(fleetml enrollment-key create --project $P --name myapp -o json | jq -r .key)
   D1=$(fleetml deploy create --project $P --version $(vid 1.0.0) --target '{"app":"myapp"}' -o json | jq -r .id)
   fleetml deploy start $D1
   ```

   Each push prints `✓ Pushed ... (FP32)`; the deployment starts as
   `COMPLETE` (`✓ Deployment start → COMPLETE`): it waits for the device.

4. **`project.yml`**, in the app's folder: the package, the four settings
   from `Secrets.xcconfig`, what a phone needs in the `Info.plist`, and the
   signing team:

   ```yaml
   name: MyApp
   options:
     deploymentTarget:
       iOS: "15.0"
   packages:
     FleetMLSDK:
       url: https://github.com/fleetml/fleetml
       from: 0.9.1        # or path: ../fleetml for a clone
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
           FleetMLServerHost: $(FLEETML_SERVER_HOST)
           FleetMLServerPort: $(FLEETML_SERVER_PORT)
           FleetMLUseTLS: $(FLEETML_USE_TLS)
           FleetMLEnrollmentKey: $(FLEETML_ENROLLMENT_KEY)
           UILaunchScreen: {}
           UIRequiresFullScreen: true
           NSAppTransportSecurity:
             NSAllowsLocalNetworking: true
           NSLocalNetworkUsageDescription: Connects to the FleetML server on your network.
           BGTaskSchedulerPermittedIdentifiers: [com.fleetml.sdk.refresh]
           UIBackgroundModes: [fetch]
       settings:
         base:
           PRODUCT_BUNDLE_IDENTIFIER: com.example.myapp
           DEVELOPMENT_TEAM: <your team ID>
           CODE_SIGN_STYLE: Automatic
   ```

   - `UIRequiresFullScreen` keeps the build free of the warning
     `All interface orientations must be supported unless the app requires full screen`;
     list every orientation in `UISupportedInterfaceOrientations` instead if
     the app should run in iPad split view.
   - `NSAllowsLocalNetworking` lets the SDK download models over plain HTTP
     from the Mac, which App Transport Security blocks otherwise; it does not
     loosen ATS for any other host.
   - `NSLocalNetworkUsageDescription` is the reason iOS shows in the Local
     Network prompt on the first launch that connects to the Mac.
   - `BGTaskSchedulerPermittedIdentifiers` and `UIBackgroundModes: fetch`
     enable the background refresh, together with
     `FleetML.registerBackgroundTask()` before `FleetML.initialize` in the
     app's initializer, as in the `App` above ([Battery and
     data](#battery-and-data)). Leave out all three if you do not want it.
     With the keys but without the call, the SDK does not ask for the
     refresh and logs
     `Background refresh not scheduled: the app permits com.fleetml.sdk.refresh but has not called FleetML.registerBackgroundTask(); call it before the app finishes launching, see docs/IOS_SDK.md "Battery and data"`
     (SDK 0.6.0 and earlier submitted it, and iOS ended the app with
     `No launch handler registered for task with identifier com.fleetml.sdk.refresh`).

   XcodeGen writes `Info.plist` from `info:`; keep `Secrets.xcconfig` and
   the generated `Info.plist` and `.xcodeproj` out of git.

5. **Build and install over USB.** Connect the phone with a cable and trust
   the Mac on it when asked. `<device>` is the phone's name or identifier
   from `xcrun devicectl list devices`:

   ```bash
   (umask 077; printf 'FLEETML_SERVER_HOST = %s\nFLEETML_SERVER_PORT = 19090\nFLEETML_USE_TLS = NO\nFLEETML_ENROLLMENT_KEY = %s\n' "$(ipconfig getifaddr en0)" "$EK" > Secrets.xcconfig)
   xcodegen generate
   xcodebuild -project MyApp.xcodeproj -scheme MyApp -destination 'generic/platform=iOS' -derivedDataPath build -allowProvisioningUpdates ENABLE_PREVIEWS=NO build
   xcrun devicectl device install app --device <device> build/Build/Products/Debug-iphoneos/MyApp.app
   xcrun devicectl device process launch --device <device> --console com.example.myapp
   ```

   Command-line builds for a device fail at codesign with
   `errSecInternalComponent` on `__preview.dylib` unless `ENABLE_PREVIEWS=NO`
   is passed. `Secrets.xcconfig` holds the enrollment key, so the subshell
   with `umask 077` writes it readable only by you. The first launch of a
   personally signed app stops at a trust prompt: trust the developer in
   Settings, General, VPN & Device Management, then launch again. iOS then
   asks whether the app may find and connect to devices on the local
   network: allow it. After a rebuild with a free team, a launch can be
   refused once more with an error about an invalid code signature,
   inadequate entitlements or a profile that "has not been explicitly
   trusted", although the developer is trusted: the new build came with a
   new 7-day profile, and the same launch line works a few minutes later.

   **Wirelessly:** once the phone has been connected by cable, Xcode's
   Devices and Simulators window (Shift-Command-2) has **Connect via
   network** for it; after that `xcrun devicectl list devices` lists it as
   available without the cable, and the same `install` and `launch` lines
   work. A phone that is asleep or out of reach is listed as `unavailable`
   (or `disconnected` until the next command reaches it), and `install`
   fails with `Connection interrupted`, or with Xcode 26
   `CoreDeviceService was unable to locate a device matching the requested device identifier`
   (`com.apple.dt.CoreDeviceError error 1011`): wake and unlock it, or use
   the cable. A locked phone that is reachable still takes the `install`;
   only the `launch` needs it unlocked.

   **Keep the phone unlocked with the app in front from the launch to step
   8** (step 1's list: Auto-Lock, Never). iOS refuses
   to launch an app on a locked phone (`Unable to launch com.example.myapp
   because the device was not, or could not be, unlocked`), and an app that
   nobody looks at is suspended within about a minute, which stops the SDK's
   timers until it is in front again ([Battery and data](#battery-and-data)):
   an update then waits for the next launch, which the `launch` line gives.

6. **What it prints.** `--console` shows the app's `print` lines:
   `FleetML registered as <device ID>` (on the launch that registers; later
   launches start with the next two), `FleetML connected: true` and
   `FleetML model ready: iris 1.0.0`, which are enough to follow the steps
   below. The SDK's own lines (the five in [When it does not
   connect](#when-it-does-not-connect)) are in Console.app with the phone
   selected, without a password, or in a log archive from the phone, which
   `log collect` writes only as root (it asks for your administrator
   password; without `sudo` it stops with
   `Must be root to collect logs from attached device`). `log` is a built-in
   command in zsh, macOS's default shell, so call `/usr/bin/log` to read the
   archive:

   ```bash
   sudo log collect --device-name '<phone name>' --last 10m --output fleetml.logarchive
   /usr/bin/log show fleetml.logarchive --info --predicate 'subsystem == "com.fleetml.sdk"'
   ```

   `--device-name` takes the phone's name, as `xcrun devicectl list devices`
   lists it, not the identifier the step 5 lines accept (with one phone
   attached, `--device` alone picks it).

   When it does not get that far:

   - **It does not register**, `FleetML cannot register: ...` with a
     connection error, while the phone is online: the app has no Local
     Network permission (any request to the Mac fails with -1009, "The
     Internet connection appears to be offline"). Turn it on in Settings,
     Privacy & Security, Local Network; no relaunch is needed, the SDK
     registers on its next retry (within 10 seconds) and the model arrives
     with the next heartbeat.
   - **It registers but never downloads**, and the SDK logs
     `Download of iris 1.0.0 failed: Could not connect to the server.; retried on the next heartbeat`
     (-1004): nothing answers at the download address, so
     `S3_PUBLIC_ENDPOINT` is still `localhost:19100` (on a phone, the phone
     itself) or the Mac's firewall blocks 19100. The line is the one the
     simulator logs with the endpoint on a closed port.

   `fleetml deploy get $D1 | jq -c .devices` prints `{"loaded":1,"targeted":1}`
   one heartbeat after the model is active, about a minute later: the SDK
   heartbeats right after start, which brings the assignment, and then at the
   interval the server sends in its first answer, 60 seconds by default (the
   SDK's own 30 seconds apply only until that answer arrives).

7. **An update.** Roll out 2.0.0; the app gets it with its next heartbeat, 60
   seconds by default (`FleetML model ready: iris 2.0.0`), and keeps 1.0.0 as
   last-known-good:

   ```bash
   fleetml deploy start $(fleetml deploy create --project $P --version $(vid 2.0.0) --target '{"app":"myapp"}' -o json | jq -r .id)
   ```

   On a phone nobody is looking at, iOS suspends the app within a minute or
   two and the heartbeat waits with it: run step 5's `launch` line again
   after the rollout, and the update comes within seconds.

8. **Offline.** Turn on airplane mode (Wi-Fi off too, if it stays on), then
   swipe the app away and open it again from the home screen: the cached
   2.0.0 is ready at once (`fleetML(modelReady:)`), and
   `fleetML(connectionChanged: false)` follows within about ten seconds.
   Turn airplane mode off: the next heartbeat reconnects. Without touching
   the phone, the closed port from [the simulator's offline
   test](#the-same-on-the-simulator) works here too: `FLEETML_SERVER_PORT = 19999`
   in `Secrets.xcconfig`, then the `xcodebuild`, `install` and `launch`
   lines of step 5; the cached 2.0.0 is ready at once
   (`FleetML model ready: iris 2.0.0`). Set the port back and rebuild to
   reconnect.

9. **Background refresh** (with the three settings of step 4 and
   `FleetML.registerBackgroundTask()`). This is the one step that needs
   Xcode and someone at the phone; the steps before it run from a terminal.
   The simulator cannot run background
   tasks, so test it on the phone: run the app from Xcode, send it to the
   background, pause it in Xcode's debugger (the LLDB prompt needs the
   debugger attached, which a `devicectl` launch does not have), enter

   ```text
   e -l objc -- (void)[[BGTaskScheduler sharedScheduler] _simulateLaunchForTaskWithIdentifier:@"com.fleetml.sdk.refresh"]
   ```

   and continue: the SDK runs one heartbeat and one telemetry flush, as the
   real refresh would.

## The same on the simulator

The simulator shares the Mac's `localhost`, so the differences from the
phone are few:

- `FLEETML_SERVER_HOST = localhost` in `Secrets.xcconfig`, and the server
  needs no `S3_PUBLIC_ENDPOINT` change: its default, `localhost:19100`,
  reaches the Mac, and so does the LAN address from step 2, so one server
  serves the phone and the simulator together. Downloads from `localhost` need no App Transport Security
  key and there is no Local Network prompt; the keys from step 4 are harmless
  on the simulator.
- No team, trust prompt or Developer Mode: Xcode signs simulator builds
  without a team.

Steps 3 and 4 as above, then build, install and launch on the first
available iPhone simulator (on a Mac with several, a simulator of your own,
`SIM=$(xcrun simctl create fleetml-test "iPhone 17 Pro")`, keeps the test
apart; `xcrun simctl delete fleetml-test` removes it):

```bash
(umask 077; printf 'FLEETML_SERVER_HOST = localhost\nFLEETML_SERVER_PORT = 19090\nFLEETML_USE_TLS = NO\nFLEETML_ENROLLMENT_KEY = %s\n' "$EK" > Secrets.xcconfig)
xcodegen generate
SIM=$(xcrun simctl list devices available -j | jq -r '[.devices[][] | select(.name | startswith("iPhone"))][0].udid')
xcodebuild -project MyApp.xcodeproj -scheme MyApp -destination "platform=iOS Simulator,id=$SIM" -derivedDataPath build build
xcrun simctl boot $SIM; open -a Simulator
xcrun simctl install $SIM build/Build/Products/Debug-iphonesimulator/MyApp.app
xcrun simctl launch --console-pty $SIM com.example.myapp
```

`simctl boot` reports an error if that simulator is already booted, which
does no harm. The console prints the app's three lines
(`FleetML registered as <device ID>`, `FleetML connected: true`,
`FleetML model ready: iris 1.0.0`). In a second terminal, this prints the
SDK's own lines, the five above (`--level debug` adds the heartbeats; without
`--level` only warnings appear):

```bash
xcrun simctl spawn booted log stream --level info --predicate 'subsystem == "com.fleetml.sdk"'
```

It first prints `getpwuid_r did not find a match for uid <your uid>` (the
simulator has no user of that ID) and the filter line; both are harmless.
`fleetml deploy get $D1 | jq -c .devices` turns to `{"loaded":1,"targeted":1}`
one heartbeat after the model is active, about a minute later as on the
phone (`--level debug` shows `Heartbeat: 1 assignment(s), next in 60s` at
start and the next heartbeat 60 seconds after it), and step 7 rolls out
2.0.0 the same way.

**Offline, without stopping a shared server:** set
`FLEETML_SERVER_PORT = 19999`, a port nothing listens on, in
`Secrets.xcconfig`, then build, install and launch again with the
`xcodebuild` and `simctl` lines. The cached 2.0.0 is ready at once
(`FleetML model ready: iris 2.0.0`), and `FleetML connected: false` follows
about ten seconds later, with the `Heartbeat failed` line above in the log.
Set the port back and rebuild to reconnect.

## Production

- **TLS.** FleetML does not terminate TLS: put the server behind a proxy that
  does and forwards gRPC over HTTP/2 (gRPC needs HTTP/2 end to end), see the
  [trust boundaries](ARCHITECTURE.md#trust-boundaries). The app then uses
  the server's host name, port 443 and TLS, which is a separate `.xcconfig`
  for the Release configuration (`configFiles: Release:` in `project.yml`):

  ```text
  FLEETML_SERVER_HOST = fleetml.example.com
  FLEETML_SERVER_PORT = 443
  FLEETML_USE_TLS = YES
  FLEETML_ENROLLMENT_KEY = <enrollment key>
  ```

- **Downloads over HTTPS.** The server's `S3_PUBLIC_ENDPOINT` must be an
  HTTPS address phones reach from the internet, since every download URL is
  signed for it ([CONFIG.md](CONFIG.md#network-and-storage)). Release builds
  then need no App Transport Security exception: `NSAllowsLocalNetworking`
  covers only local addresses, so keeping it does no harm, and the Local
  Network prompt appears only when the app connects to a local address.
- **The enrollment key** is in every installed app, so it is not a secret in
  the way a password is, but it lets a device join the project: keep it in
  the uncommitted `.xcconfig` or the CI's secret store, give each app or
  release channel its own, and revoke one that leaks. The key is needed only
  to register: revoking it stops new installs from registering, and devices
  that already registered keep their token.

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
  How to trigger the refresh on a phone is in step 9 of [Your own app on an
  iPhone](#your-own-app-on-an-iphone).
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
  at info level).

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

On the server, `fleetml telemetry events --device <device ID>` lists the
device's events (type, latency, confidence, error) once the SDK has flushed
them (30 seconds by default), and `fleetml telemetry summary <version ID>`
sums up a version's.

Events go into a persistent queue (bounded to 10,000; oldest dropped first):
they survive app termination and are kept while offline or before
registration. They are sent in batches every flush interval, with exponential
backoff (5 s up to 5 min) after a failure and an immediate flush when the
connection returns. For a model in an A/B experiment the event carries the
device's arm ID automatically. Feature values are binned on the device with
the edges from the server ([feature-level drift](ROLLOUTS.md#feature-level-drift));
only the counts are sent.

## Sample app

![Sample app](screenshots/ios-sample.png)

`ios/SampleApp` is the reference integration, generated with XcodeGen from
`project.yml`: a SwiftUI config screen (host, port, enrollment key), the SDK
owned by an app-wide object, the Iris Core ML model from `models/iris`
(Apache-2.0) bundled and used until the server assigns one, and inference
telemetry. Its `Info.plist` sets `NSAllowsLocalNetworking` and
`NSLocalNetworkUsageDescription` so model downloads from a server on the Mac
work over plain HTTP, on the simulator and on a phone; production servers
should serve presigned URLs over HTTPS. The config screen's host defaults to
`localhost`, which works on the simulator; on a phone, use the Mac's LAN
address. Unlike an own app, it takes its settings as launch arguments
(`-fleetmlHost`, `-fleetmlPort`, `-fleetmlEnrollmentKey`) or on its config
screen.

### Simulator runbook

This is how the SDK and sample app are verified, against the server from the
[quickstart](QUICKSTART.md#run-fleetml) (the production compose stack, with the
admin key in `$FLEETML_ADMIN_KEY`) and the `fleetml` CLI, from the root of a
clone. The simulator shares the Mac's `localhost`, so the server needs no
change. It needs Xcode and [XcodeGen](https://github.com/yonaskolb/XcodeGen)
(`brew install xcodegen`). Contributors on the dev stack (`make dev`) use the
key from `.env` instead.

1. Start the server with the quickstart's "Run FleetML" block (or
   `source ~/.fleetml.env` in a new shell).

2. Publish the Core ML sample models (with the requirements override from
   [Which variant a device gets](#which-variant-a-device-gets)), create an
   enrollment key and deploy 1.0.0 to the sample app's label (`app=sample`):

   ```bash
   export FLEETML_SERVER=http://localhost:18080 FLEETML_API_KEY=$FLEETML_ADMIN_KEY
   REQ='{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}'
   P=$(fleetml project create --name ios-demo -o json | jq -r .id)
   fleetml push models/iris/iris_v1.mlmodel --project $P --model iris --version 1.0.0 --framework COREML --quantization FP32 --metadata "$REQ"
   fleetml push models/iris/iris_v2.mlmodel --project $P --model iris --version 2.0.0 --framework COREML --quantization FP32 --metadata "$REQ"
   M=$(fleetml model list --project $P -o json | jq -r '.[] | select(.name == "iris") | .id')
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

   On a simulator where the sample app ran before, its saved settings stay,
   the port included, and the launch arguments replace only the host and the
   key: `xcrun simctl uninstall $SIM com.fleetml.sample` before the install
   starts it clean. The app registers, downloads iris 1.0.0, validates it
   with Core ML, shows **Active model: iris 1.0.0** and classifies the sample
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
   (`-fleetmlPort 19999` as one more launch argument).

### Physical device

The sample app runs on an iPhone too, over Wi-Fi, with the same server and
models, after steps 1 and 2 of [Your own app on an
iPhone](#your-own-app-on-an-iphone) (Developer Mode, Apple ID, the server
with `S3_PUBLIC_ENDPOINT` on the Mac's LAN address in `~/.fleetml.env`), and
with the phone unlocked and the app in front (step 5). Its `Info.plist` already has
the keys a phone needs. After `xcodegen generate`, open
`ios/SampleApp/FleetMLSample.xcodeproj` and, on the FleetMLSample target's
Signing & Capabilities tab, turn on **Automatically manage signing** with
your team; or pass the team on the command line as below, where `<team>` is
its ID and `<device>` the phone's name or identifier from
`xcrun devicectl list devices`. The config screen's default host,
`localhost`, is the phone itself, so on a phone always give the Mac's
address:

```bash
cd ios/SampleApp && xcodegen generate
xcodebuild -project FleetMLSample.xcodeproj -scheme FleetMLSample -destination 'generic/platform=iOS' -derivedDataPath build -allowProvisioningUpdates DEVELOPMENT_TEAM=<team> ENABLE_PREVIEWS=NO build
xcrun devicectl device install app --device <device> build/Build/Products/Debug-iphoneos/FleetMLSample.app
xcrun devicectl device process launch --device <device> --console com.fleetml.sample -- -fleetmlHost <Mac LAN IP> -fleetmlEnrollmentKey "$EK"
```

The trust prompt, the Local Network prompt and the error codes are as in
steps 5 and 6 of the own app's runbook.

### Tests

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

The live tests (`scripts/ios-e2e.sh` and `scripts/signing-e2e.sh ios`) stay
simulator-only, because Swift package tests cannot run on a physical device
(`xcodebuild` reports "Tool-hosted testing is unavailable on device
destinations") unless an app target hosts them. The download policy can be
exercised on a phone by hand instead: on battery and with Low Data Mode on for
the Wi-Fi network, a non-urgent deployment waits and an urgent one downloads.

### For contributors

Against the dev stack (`make dev`):
`FLEETML_API_KEY=<admin key> ./scripts/ios-e2e.sh` runs the live test, and
`FLEETML_API_KEY=<admin key> ./scripts/signing-e2e.sh ios` the signature
checks (tampered, and the rotation grace). `scripts/ios-e2e.sh` creates a
non-urgent deployment and runs `LiveServerTests` on the simulator: a device on
an expensive network and on battery registers, its model waits, then an urgent
deployment downloads and activates. Without the script's environment the live
test is skipped.

## Contact

- Anything about FleetML: [hello@fleetml.dev](mailto:hello@fleetml.dev).
- A security problem: [security@fleetml.dev](mailto:security@fleetml.dev), see [SECURITY.md](../SECURITY.md).
- Questions in public: [GitHub Discussions](https://github.com/fleetml/fleetml/discussions).
- Bugs: [GitHub Issues](https://github.com/fleetml/fleetml/issues/new/choose), with the FleetML version and the command or log line that shows the problem.
