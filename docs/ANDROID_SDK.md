# Android SDK

The FleetML Android SDK (`android/fleetml-sdk`, Kotlin, minSdk 26) puts a
device under FleetML's control: it registers with an enrollment key, receives
model assignments over gRPC, downloads and verifies them, validates each model
before using it, keeps the last-known-good version, works offline, and reports
telemetry. It does not run your models: your app does, with TFLite, ONNX
Runtime or anything else.

![Sample app](screenshots/android-sample.png)

## Install

The SDK is on Maven Central as `dev.fleetml:fleetml-sdk`. Add it to the app
with TensorFlow Lite next to it:

```kotlin
// app/build.gradle.kts
dependencies {
    implementation("dev.fleetml:fleetml-sdk:0.4.0")
    implementation("org.tensorflow:tensorflow-lite:2.16.1") // if you run TFLite models
}
```

The app's repositories must include `mavenCentral()`, as a new Android Studio
project's `settings.gradle.kts` does. The SDK needs minSdk 26 and is built
with compileSdk and targetSdk 34 and Java 8 bytecode, so the app needs
compileSdk 34 or later. TensorFlow Lite is not a dependency of the SDK: the app
brings the runtime it uses, and the default validator uses it when it is there
([Validators](#validators)).

The SDK brings these libraries with it: `protobuf-kotlin-lite` 3.25 (as an
`api` dependency: protobuf-lite messages appear in its public API),
`grpc-okhttp` and the gRPC stubs 1.62, `okhttp` 4.12, WorkManager 2.9,
`lifecycle-process`, `security-crypto` and BouncyCastle (`bcprov-jdk18on`).
An app that already uses the full `protobuf-java` gets duplicate-class errors
from Gradle, because `protobuf-javalite` and `protobuf-java` hold the same
classes; the usual fix is to exclude the lite runtime, since the full one also
contains the classes lite-generated code uses (not tested here with every
protobuf version):

```kotlin
implementation("dev.fleetml:fleetml-sdk:0.4.0") {
    exclude(group = "com.google.protobuf", module = "protobuf-javalite")
}
```

While the server runs on your own machine without TLS, the app talks plain
HTTP to it (gRPC on 19090, model downloads on 19100), which Android blocks by
default. Allow it for debug builds only, to `localhost` only, with a network
security config, the way the sample app does:

```xml
<!-- app/src/debug/res/xml/network_security_config.xml -->
<network-security-config>
    <domain-config cleartextTrafficPermitted="true">
        <domain includeSubdomains="false">localhost</domain>
        <domain includeSubdomains="false">10.0.2.2</domain>
    </domain-config>
</network-security-config>
```

```xml
<!-- app/src/debug/AndroidManifest.xml -->
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application android:networkSecurityConfig="@xml/network_security_config" />
</manifest>
```

`10.0.2.2` is the emulator's name for the host. A phone reaches a server on
your laptop in one of two ways:

- **Over USB, as `localhost`:** `serverHost = "localhost"` in the app, and the
  `adb reverse` lines in [Physical device](#physical-device) forward the
  phone's ports 19090 and 19100 to the laptop. The server needs no change.
- **Over Wi-Fi, at the laptop's LAN address:** `serverHost = "<LAN IP>"`, the
  same address as one more `<domain>` line in the network security config,
  and the server started with the object store on that address:
  `S3_PUBLIC_ENDPOINT=<LAN IP>:19100 docker compose -f deploy/docker-compose.prod.yml up -d`
  in the quickstart's shell.

Port 19100 matters as much as 19090: the server hands out model downloads as
presigned URLs signed for `S3_PUBLIC_ENDPOINT` (`localhost:19100` in the
quickstart's compose file), so a phone that cannot reach that address
registers and then fails every download ([CONFIG.md](CONFIG.md#network-and-storage)).
Release builds keep Android's default and need `useTls = true` with the
server behind a TLS proxy.

### From a clone

To work against changes that are not released yet, include the module from a
clone of this repository instead:

```kotlin
// settings.gradle.kts
include(":fleetml-sdk")
project(":fleetml-sdk").projectDir = file("path/to/fleetml/android/fleetml-sdk")

// app/build.gradle.kts
dependencies {
    implementation(project(":fleetml-sdk"))
    implementation("org.tensorflow:tensorflow-lite:2.16.1") // if you run TFLite models
}
```

Building the SDK from source on the command line needs JDK 17: the wrapper's
Gradle 8.7 with AGP 8.5 fails on a newer default JDK. Point `JAVA_HOME` at it,
for example `/opt/homebrew/opt/openjdk@17` on macOS with Homebrew or
`/usr/lib/jvm/java-17-openjdk-amd64` on Debian and Ubuntu
(`sudo apt install openjdk-17-jdk`). Android Studio runs Gradle with its own
JDK, set in its Gradle settings.

## Configure and start

Initialize once per process, in `Application.onCreate()`, so screen rotation
never re-initializes the SDK:

```kotlin
class MyApp : Application(), FleetMLCallback {
    override fun onCreate() {
        super.onCreate()
        FleetML.init(
            context = this,
            config = FleetMLConfig(
                serverHost = "fleetml.example.com",
                serverPort = 19090,
                enrollmentKey = "<enrollment key>",
                labels = mapOf("region" to "eu", "tier" to "beta"),
            ),
            callback = this,
        )
    }

    override fun onModelReady(model: ActiveModel) {
        // model.file is verified and validated: load it.
    }
}
```

The enrollment key comes from `fleetml enrollment-key create --project <project ID>`,
which prints it once. To keep it out of the source, put it in the build
instead: in `app/build.gradle.kts`, `buildFeatures { buildConfig = true }` and
`buildConfigField("String", "FLEETML_ENROLLMENT_KEY", "\"<enrollment key>\"")`
in `defaultConfig`, then `enrollmentKey = BuildConfig.FLEETML_ENROLLMENT_KEY`
(the sample app reads it from a Gradle property this way).

`FleetMLCallback` has five methods, each with an empty default, so implement
only what the app needs. They run on a background thread: switch to the main
thread before touching views.

| Callback | When |
|---|---|
| `onModelReady(model)` | A model was downloaded, verified and validated and is now active; also at start for models cached from an earlier run |
| `onModelUnload(modelId, modelName, variantId)` | The server no longer assigns the model |
| `onModelLoadFailed(modelName, version, reason)` | A new version could not be downloaded or failed validation; the last-known-good one stays |
| `onRegistered(deviceId)` | The device registered (or re-registered) |
| `onConnectionStatusChanged(connected)` | The server became reachable or unreachable |

**What to expect on the first run.** Within seconds of `FleetML.init`,
`onRegistered(deviceId)` fires and the device appears in
`fleetml device list --project <project ID>` and on the dashboard's Devices
page. `onModelReady` fires only once a deployment targets the app's labels
(`fleetml deploy create --target '{"region":"eu"}'` and `deploy start`), on
the next heartbeat. The SDK logs every step to logcat with the tag `FleetML`
(`adb logcat -s FleetML`): registration, downloads, validation, and at
warning level or above why registration or a heartbeat failed, such as an
unreachable host or a refused enrollment key.

| `FleetMLConfig` field | Default | Purpose |
|---|---|---|
| `serverHost`, `serverPort` | –, `19090` | The FleetML gRPC endpoint |
| `enrollmentKey` | `""` | Per-project key for the first registration (`fleetml enrollment-key create`) |
| `useTls` | `false` | TLS for gRPC (put the server behind a TLS proxy in production) |
| `labels` | `{}` | Labels deployments and experiments target (`target_selector`) |
| `heartbeatIntervalSeconds` | `30` | Foreground heartbeat interval, until the server sends its own |
| `backgroundHeartbeatIntervalSeconds` | `3600` | Background check interval, until the server sends its own (at least 15 minutes) |
| `telemetryBatchSize`, `telemetryFlushIntervalSeconds` | `50`, `30` | Until the server's `DeviceConfig` overrides them (server default 100; the SDK uses 50 until the first response) |
| `maxCacheSizeMb` | `500` | Disk budget for models; the active and last-known-good models are never evicted |
| `downloadPolicy` | `UNMETERED_OR_CHARGING` | Which networks non-urgent downloads may use, until the server sends its own ([Battery and data](#battery-and-data)) |
| `autoStart` | `true` | Call `FleetML.start()` yourself if `false` |

`FleetML.stop()` stops background work and clears the singleton, so `init` can
be called again (for example with a new server address). `FleetML.reset()` also
deletes cached models and the registration.

## Enrollment and identity

The first registration needs a valid enrollment key; the device is enrolled
into that key's project and receives a device token (30 days by default). The
device ID is stable per app and device: the fingerprint is a SHA-256 of the
app's package name and hardware properties (no ANDROID_ID, IMEI or other
resettable identifiers), so two FleetML apps on one phone are separate devices.
After that the SDK:

- re-registers only when it is new or its labels or SDK version changed;
- refreshes the token before it expires (6 days before, or at half its lifetime
  for short-lived tokens) with the `RefreshToken` RPC;
- re-registers with the enrollment key if the server rejects the token
  (revoked with `POST /api/v1/devices/{id}/revoke`, or expired).

## Models: download, verify, validate, activate

For each assignment in a heartbeat the SDK:

1. **Checks the signature** of the assignment with the project's signing
   keys (sent with registration and every heartbeat): an artifact whose
   signature does not verify is not downloaded, the current model stays, and
   `SIGNATURE_INVALID` is reported ([artifact signing](ROLLOUTS.md#artifact-signing)).
2. **Downloads** the model from the presigned URL, streaming to a `.part` file.
   An interrupted download resumes with an HTTP Range request. When the server offers a delta from the
   version the device has, only the delta is downloaded and applied
   ([delta updates](ROLLOUTS.md#delta-updates)).
3. **Verifies** the sha256 before moving the file into place.
4. **Validates** it with a `ModelValidator`.
5. **Activates** it only if validation passed: the model becomes active, the
   previously active version becomes last-known-good, the device reports the
   new version as loaded and `onModelReady(model)` is called.

If validation fails, the active (last-known-good) model stays in place, the SDK
reports a `MODEL_LOAD_FAILED` event with the error (so a canary's health gate
can roll the release back), calls `onModelLoadFailed`, and does not retry that
variant for an hour.

![A broken model is rejected and 2.0.1 stays active](screenshots/android-last-known-good.png)

### Validators

`ModelValidators.default()` (used unless you pass `validator = ...` to `init`):

- **TFLite** files (identifier `TFL3`): opens an `Interpreter` and runs one
  inference on zero inputs of the model's declared shapes, if
  `org.tensorflow:tensorflow-lite` is in the app.
- **Other files**: opens an ONNX Runtime session and runs one inference on zero
  float inputs, if `com.microsoft.onnxruntime:onnxruntime-android` is in the app.
- Otherwise the model is accepted (`NoOpModelValidator`).

Write your own to check what matters to your app:

```kotlin
FleetML.init(this, config, callback, validator = ModelValidator { file, model ->
    val ok = runGoldenInput(file)   // e.g. a known input with a known answer
    if (ok) ValidationResult.Valid else ValidationResult.Invalid("golden input mismatch")
})
```

### Which variant a device gets

The server picks one READY variant per assignment for each device, by runtime
(TFLite on Android) and by the variant's requirements. Requirements default by
quantization (for example FP32 needs a device score of 60 and 2 GB of RAM).
Override them for small models in the version's metadata:

```json
{"version": "1.0.0", "metadata": {"fleetml_requirements": {"min_ram_mb": 64, "min_score": 0}}}
```

`POST /api/v1/intelligence/preview` with a `device_id` and `deployment_id`
explains the choice for one device.

## Battery and data

| | In the foreground | In the background |
|---|---|---|
| Heartbeat | every `heartbeat_interval_seconds` (server default 60 s) | one per background check |
| Telemetry | batches of `telemetry_batch_size` (server default 100), every `telemetry_flush_interval_seconds` (30 s) and when a batch fills | one flush when the app goes to the background, one per background check |
| Background check | not scheduled | a WorkManager job every `background_heartbeat_interval_seconds` (1 h, at least 15 min), only with a network connection |
| Model downloads | per `download_policy` | per `download_policy` |

- **Foreground only timers.** The SDK follows the app's process lifecycle
  (`ProcessLifecycleOwner`): its timers stop when the app leaves the
  foreground and start again when it returns. Initialize FleetML in
  `Application.onCreate()` so the background job finds it when WorkManager
  starts your app. `FleetML.syncNow()` runs one heartbeat and one telemetry
  flush on demand.
- **Downloads.** `UNMETERED_OR_CHARGING` (the default): an unmetered network
  (Wi-Fi, Ethernet), or any network while the device charges.
  `UNMETERED_ONLY` and `ANY_NETWORK` are the alternatives. A waiting
  assignment is retried on the next heartbeat. Assignments of an
  [urgent deployment](ROLLOUTS.md#urgent-deployments-and-the-download-policy),
  and every rollback, skip the battery and metered rules, but they still need
  a connection: with no network at all, every download waits, urgent ones
  included.
- **Telemetry** is always batched; no event is sent on its own.
- The server sets all of these for every device with the `DEVICE_*`
  variables ([CONFIG.md](CONFIG.md#devices)); `FleetMLConfig` only provides
  the values used before the first response.

## Offline behaviour

Models activated in an earlier run are delivered through `onModelReady` (and
`FleetML.activeModels()`) as soon as `start()` runs, before the server is
reached. A registered device in the foreground starts heartbeating straight away; until the
server answers it keeps using its cached models, and `onConnectionStatusChanged`
reports reachability.

![Offline start in airplane mode](screenshots/android-offline-start.png)

## Telemetry

```kotlin
val model = FleetML.activeModel("iris")!!
FleetML.getTelemetryReporter().reportInference(model, latencyMs = 3, confidence = 0.97f, predictionLabel = "virginica")
// with input values, for feature-level drift (only features configured on the server are kept):
FleetML.getTelemetryReporter().reportInference(model, latencyMs = 3, features = mapOf("petal_length" to floatArrayOf(5.1f)))
```

`reportInference(modelVersionId, latencyMs, ...)` reports for a version ID
instead of an `ActiveModel` (for a model the app loaded itself), and
`reportModelStatus(modelVersionId, type, ...)` reports a lifecycle event
(`MODEL_LOAD`, `MODEL_LOAD_FAILED`, `OOM`, `ERROR`) of the app's own.

Events go into a persistent queue (bounded to 10,000; oldest dropped first):
they survive process death and are kept while offline or before registration.
They are sent in batches every flush interval, with exponential backoff (5 s up
to 5 min) after a failure and an immediate flush when the connection returns.
For a model in an A/B experiment the event carries the device's arm ID
automatically, so results are attributed per arm. Feature values are binned
on the device with the edges from the server
([feature-level drift](ROLLOUTS.md#feature-level-drift)); only the counts
are sent.

## Emulator runbook

This is how the SDK and sample app are verified, against the server from the
[quickstart](QUICKSTART.md#run-fleetml) (the production compose stack, with the
admin key in `$FLEETML_ADMIN_KEY`) and the `fleetml` CLI. It runs from the root
of a clone, since the sample app and models are in the repository.
Contributors on the dev stack (`make dev`) use the key from `.env` and port
19000 instead of 19100.

1. Start the server with the quickstart's "Run FleetML" block, and keep its
   shell: it holds `FLEETML_ADMIN_KEY`.

2. Start an emulator (API 26 or later), from Android Studio's Device Manager
   or `emulator -avd <name>`, and forward the object store's port, so the
   presigned download URLs (`localhost:19100`) work inside it:

   ```bash
   adb reverse tcp:19100 tcp:19100
   ```

   The emulator reaches the gRPC port as `10.0.2.2:19090`, which the sample app
   uses by default.

3. Publish the sample models, create an enrollment key and deploy 1.0.0 to the
   sample app's label (`app=sample`):

   ```bash
   export FLEETML_SERVER=http://localhost:18080 FLEETML_API_KEY=$FLEETML_ADMIN_KEY
   P=$(fleetml project create --name android-demo -o json | jq -r .id)
   fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
   fleetml push models/iris/iris_v2.tflite --project $P --model iris --version 2.0.0
   M=$(fleetml model list --project $P -o json | jq -r '.[0].id')
   V1=$(fleetml ver list --model $M -o json | jq -r '.[] | select(.version == "1.0.0") | .id')
   V2=$(fleetml ver list --model $M -o json | jq -r '.[] | select(.version == "2.0.0") | .id')
   EK=$(fleetml enrollment-key create --project $P --name emulator -o json | jq -r .key)
   D=$(fleetml deploy create --project $P --version $V1 --target '{"app":"sample"}' -o json | jq -r .id)
   fleetml deploy start $D
   ```

   Each push prints `✓ Pushed ...`, and the deployment starts as `COMPLETE`
   (`✓ Deployment start → COMPLETE`): it waits for the device.

4. Build and install the sample app with the key prefilled (or type it on the
   config screen), open it and tap **Save and connect**. A fresh clone has no
   `android/local.properties`, so Gradle on the command line finds the Android
   SDK through `ANDROID_HOME` (`$HOME/Library/Android/sdk` on macOS,
   `$HOME/Android/Sdk` on Linux, where Android Studio puts it), and it needs JDK
   17 ([From a clone](#from-a-clone)). Android Studio writes `local.properties`
   when it opens the project, which makes the `ANDROID_HOME` export
   unnecessary there:

   ```bash
   export ANDROID_HOME=$HOME/Library/Android/sdk JAVA_HOME=/opt/homebrew/opt/openjdk@17
   cd android
   ./gradlew :sample-app:installDebug -PfleetmlEnrollmentKey=$EK
   adb shell am start -n com.fleetml.sample/.MainActivity
   ```

   On an emulator where the sample app ran before, its saved settings win
   over the prefilled key: check the key on the screen, or
   `adb shell pm clear com.fleetml.sample` first.

   The app registers, downloads iris 1.0.0, validates it and shows
   **Active model: iris 1.0.0**; logcat (`adb logcat -s FleetML`) shows the
   registration and `Model iris 1.0.0 is active`. **Classify sample flower**
   runs it and reports inference telemetry. `fleetml deploy create --project $P
   --version $V2 --target '{"app":"sample"}'` and `deploy start` update the app
   to 2.0.0 (1.0.0 is kept as last-known-good).

5. Offline start: enable airplane mode
   (`adb shell cmd connectivity airplane-mode enable`, on API 30 and later;
   on API 26 to 29 use the quick settings tile), force-stop and reopen the
   app. The cached model is active immediately and inference works; events
   are queued and sent after `adb shell cmd connectivity airplane-mode disable`.
   On a phone connected over USB, airplane mode does not cut the ports
   forwarded with `adb reverse`: remove them with `adb reverse --remove-all`
   instead, force-stop and reopen the app, and set the reverses again
   afterwards ([Physical device](#physical-device)).

### Physical device

The runbook works the same on a phone connected over USB, with the same
server:

1. Turn on USB debugging in the phone's developer options, connect it and
   accept the authorisation prompt; `adb devices` then lists it as `device`.
2. Forward the REST, gRPC and model-file ports, so the phone reaches the
   laptop as `localhost`:

   ```bash
   adb reverse tcp:18080 tcp:18080
   adb reverse tcp:19090 tcp:19090
   adb reverse tcp:19100 tcp:19100
   ```

   On the dev stack (`make dev`), forward 19000 instead of 19100.

3. On the sample app's config screen, replace `10.0.2.2` with `localhost`; in
   an own app, set `serverHost = "localhost"`.

Everything else is the same as on the emulator, including the debug network
security config, which already allows cleartext to `localhost`. Without USB,
use the laptop's LAN address instead, as in [Install](#install).

### From Windows

With the server in WSL 2 ([On Windows](QUICKSTART.md#on-windows)), the
Android tools run on the Windows side, where Android Studio installs them:
`adb` and the emulator are in `%LOCALAPPDATA%\Android\Sdk` (`adb` in its
`platform-tools`), and an `adb` installed inside Ubuntu does not see the
Windows emulator. The CLI steps of the runbook (step 3) run in the Ubuntu
shell, and the enrollment key crosses by hand (`echo $EK` there). Steps 2, 4
and 5 run in PowerShell: build the sample app with
`.\gradlew.bat :sample-app:installDebug -PfleetmlEnrollmentKey=<key>` in the
`android` folder of a clone on the Windows side (Gradle is slow on a
`\\wsl.localhost` path), with `ANDROID_HOME` set to that SDK folder and
`JAVA_HOME` to a JDK 17, or open the folder in Android Studio. The emulator's
`10.0.2.2:19090` and `adb reverse tcp:19100 tcp:19100` reach the ports
Docker Desktop publishes on Windows. A phone over Wi-Fi reaches the laptop's
LAN address only if Windows Firewall lets in TCP 19090 and 19100, for example
with
`New-NetFirewallRule -DisplayName FleetML -Direction Inbound -Protocol TCP -LocalPort 19090,19100 -Action Allow`
in an administrator PowerShell. None of this is verified on Windows by CI.

## For contributors

### Instrumented test

`LiveServerTest` (in `fleetml-sdk/src/androidTest`) runs register → download →
validate → activate → inference telemetry against a running server and checks
through the REST API that the server stored the event. With the dev stack
(`make dev`) and an emulator running:

```bash
FLEETML_API_KEY=$(grep '^FLEETML_ADMIN_KEY=' .env | cut -d= -f2) ./scripts/android-e2e.sh
```

The script forwards ports 19000 and 19100, so it runs against the quickstart's
server too (`FLEETML_API_KEY=$FLEETML_ADMIN_KEY`). It needs `ANDROID_HOME` (or
`local.properties`) as in step 4, and runs Gradle with JDK 17 whatever
`JAVA_HOME` says: from `JAVA_HOME_17` if it is set, otherwise from
`/opt/homebrew/opt/openjdk@17`. The test connects to `10.0.2.2` unless told
otherwise; on a phone with the reverses set, pass the host through Gradle's
environment:

```bash
env 'ORG_GRADLE_PROJECT_android.testInstrumentationRunnerArguments.host=localhost' FLEETML_API_KEY=$FLEETML_API_KEY ./scripts/android-e2e.sh
```

`./scripts/signing-e2e.sh android` runs `SignatureLiveTest` twice against
the same server: a model whose stored signature was overwritten is rejected,
and one signed with the previous key is accepted during the rotation's grace
period and rejected after it.

They need a device or emulator and a running server, so they are not part of
the hosted CI; the unit tests (`./gradlew :fleetml-sdk:testDebugUnitTest`) are.
The test process has no activity, so the SDK counts as being in the
background and the test drives it with `FleetML.syncNow()`.

### Battery and data on the emulator or a phone

With the sample app connected and showing its model, send it to the
background (its heartbeats stop), find its WorkManager job (a line like
`JOB #u0aNNN/<id>`) and run the job once, which is one background check with
one heartbeat:

```bash
adb shell input keyevent KEYCODE_HOME
adb shell dumpsys jobscheduler | grep "com.fleetml.sample/androidx.work"
adb shell cmd jobscheduler run -f com.fleetml.sample <id>
```

Then put the device on battery and on a metered network:

```bash
adb shell dumpsys battery unplug && adb shell dumpsys battery set status 3
```

- On the emulator, `adb shell svc wifi disable` leaves only mobile data,
  which is metered.
- On a phone, keep Wi-Fi on and mark it metered with
  `adb shell cmd netpolicy set metered-network '"<SSID>"' true`, where
  `<SSID>` is the network's name (the double quotes keep a name with spaces
  in one piece on the phone's shell). Turning Wi-Fi off does not work on a
  phone without a SIM: it is left with no network at all, and then every
  download waits, urgent ones included.

Deploy a new version without `urgent`, run the job again (the job ID changes
after each run): logcat shows `Download of iris 2.0.0 waits for the download
policy`. Deploy it with `--urgent` and run the job: the model is
activated. Restore with `adb shell dumpsys battery reset`, plus
`adb shell svc wifi enable` on the emulator or, on a phone,
`adb shell cmd netpolicy set metered-network '"<SSID>"' undefined`, which
returns the network to the system's own setting (`false` would mark it
unmetered instead).

## Sample app

`android/sample-app` is the reference integration: a config screen (host, port,
enrollment key), the SDK owned by the `Application`, a bundled Iris TFLite
model (`models/iris`, Apache-2.0) used until the server assigns one, and
inference telemetry. Cleartext traffic is allowed only in debug builds and only
to `10.0.2.2`, `localhost` and `127.0.0.1`.
