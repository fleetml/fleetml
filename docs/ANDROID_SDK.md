# Android SDK

The FleetML Android SDK (`android/fleetml-sdk`, Kotlin, minSdk 26) puts a
device under FleetML's control: it registers with an enrollment key, receives
model assignments over gRPC, downloads and verifies them, validates each model
before using it, keeps the last-known-good version, works offline, and reports
telemetry. It does not run your models: your app does, with TFLite, ONNX
Runtime or anything else.

![Sample app](screenshots/android-sample.png)

## Install

The SDK is not published to a Maven repository yet. Include the module from
this repository in your Gradle build:

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

Building needs JDK 17 (`JAVA_HOME=/opt/homebrew/opt/openjdk@17` on macOS with
Homebrew).

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
                enrollmentKey = BuildConfig.FLEETML_ENROLLMENT_KEY,
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

| `FleetMLConfig` field | Default | Purpose |
|---|---|---|
| `serverHost`, `serverPort` | –, `19090` | The FleetML gRPC endpoint |
| `enrollmentKey` | `""` | Per-project key for the first registration (`POST /api/v1/projects/{id}/enrollment-keys`) |
| `useTls` | `false` | TLS for gRPC (put the server behind a TLS proxy in production) |
| `labels` | `{}` | Labels deployments and experiments target (`target_selector`) |
| `heartbeatIntervalSeconds` | `30` | Foreground heartbeat interval, until the server sends its own |
| `backgroundHeartbeatIntervalSeconds` | `3600` | Background check interval, until the server sends its own (at least 15 minutes) |
| `telemetryBatchSize`, `telemetryFlushIntervalSeconds` | `50`, `30` | Until the server's `DeviceConfig` overrides them |
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
| Telemetry | batches of `telemetry_batch_size` (100), every `telemetry_flush_interval_seconds` (30 s) and when a batch fills | one flush when the app goes to the background, one per background check |
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

This is how the SDK and sample app were verified, against the dev stack on the
host machine (the emulator reaches the host's gRPC port at `10.0.2.2`).

1. Start the server: `make dev`.

2. Start an emulator (API 26 or later), e.g. from Android Studio or
   `emulator -avd <name>`, and forward the object store port so the
   presigned download URLs (`localhost:19000`) work inside it:

   ```bash
   adb reverse tcp:19000 tcp:19000
   ```

3. Upload the sample models, create an enrollment key and deploy v1 to the
   sample app's label (`app=sample`):

   ```bash
   export FLEETML_API_KEY=$(grep '^FLEETML_ADMIN_KEY=' .env | cut -d= -f2)
   api() { curl -s -X "$1" -H "Authorization: Bearer $FLEETML_API_KEY" -H 'Content-Type: application/json' -d "$3" "localhost:18080$2"; }
   upload() { local out=$(api POST /api/v1/versions/$1/variants "{\"quantization\":\"FP32\",\"target_runtime\":\"TFLITE\",\"blob_size_bytes\":$(wc -c < $2 | tr -d ' '),\"blob_sha256\":\"$(shasum -a 256 $2 | cut -d' ' -f1)\"}")
     curl -s -o /dev/null -X PUT --data-binary @$2 "$(echo "$out" | jq -r .data.upload_url)"
     api POST /api/v1/variants/$(echo "$out" | jq -r .data.variant.id)/finalize "" | jq -r .data.status; }
   P=$(api POST /api/v1/projects '{"name":"android-demo"}' | jq -r .data.id)
   M=$(api POST /api/v1/projects/$P/models '{"name":"iris","framework":"TFLITE","task_type":"CLASSIFICATION"}' | jq -r .data.id)
   REQ='{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}'
   V1=$(api POST /api/v1/models/$M/versions "{\"version\":\"1.0.0\",\"metadata\":$REQ}" | jq -r .data.id); upload $V1 models/iris/iris_v1.tflite
   V2=$(api POST /api/v1/models/$M/versions "{\"version\":\"2.0.0\",\"metadata\":$REQ}" | jq -r .data.id); upload $V2 models/iris/iris_v2.tflite
   EK=$(api POST /api/v1/projects/$P/enrollment-keys '{"name":"emulator"}' | jq -r .data.key); echo "enrollment key: $EK"
   D=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V1\",\"target_selector\":{\"app\":\"sample\"}}" | jq -r .data.id)
   api POST /api/v1/deployments/$D/start "" | jq -r .data.status
   ```

4. Build and install the sample app with the key prefilled (or type it on the
   config screen), open it and tap **Save and connect**. A fresh clone has no
   `android/local.properties`, so Gradle on the command line finds the Android
   SDK through `ANDROID_HOME`, and Gradle 8.7 needs JDK 17 (it fails on a newer
   default JDK). Android Studio writes `local.properties` when it opens the
   project, which makes the first export unnecessary there:

   ```bash
   export ANDROID_HOME=$HOME/Library/Android/sdk
   export JAVA_HOME=/opt/homebrew/opt/openjdk@17
   cd android
   ./gradlew :sample-app:installDebug -PfleetmlEnrollmentKey=$EK
   ```

   The app registers, downloads iris 1.0.0, validates it and shows
   **Active model: iris 1.0.0**. **Classify sample flower** runs it and reports
   inference telemetry. Deploying `$V2` updates the app to 2.0.0 (1.0.0 is kept
   as last-known-good).

5. Offline start: enable airplane mode
   (`adb shell cmd connectivity airplane-mode enable`), force-stop and reopen
   the app. The cached model is active immediately and inference works; events
   are queued and sent after `adb shell cmd connectivity airplane-mode disable`.
   On a phone connected over USB, airplane mode does not cut the ports
   forwarded with `adb reverse`: remove them with `adb reverse --remove-all`
   instead, force-stop and reopen the app, and set the reverses again
   afterwards ([Physical device](#physical-device)).

### Physical device

The runbook works the same on a phone connected over USB:

1. Turn on USB debugging in the phone's developer options, connect it and
   accept the authorisation prompt; `adb devices` then lists it as `device`.
2. Forward the REST, gRPC and object store ports, so the phone reaches the
   host machine as `localhost`:

   ```bash
   adb reverse tcp:18080 tcp:18080
   adb reverse tcp:19090 tcp:19090
   adb reverse tcp:19000 tcp:19000
   ```

3. On the sample app's config screen, replace `10.0.2.2` with `localhost`.

Everything else is the same as on the emulator, including the debug network
security config, which already allows cleartext to `localhost`. The
instrumented tests below run on a phone the same way once the reverses are
set. They connect to `10.0.2.2` unless told otherwise, so pass the host
through Gradle's environment:

```bash
env 'ORG_GRADLE_PROJECT_android.testInstrumentationRunnerArguments.host=localhost' FLEETML_API_KEY=$FLEETML_API_KEY ./scripts/android-e2e.sh
```

### Instrumented test

`LiveServerTest` (in `fleetml-sdk/src/androidTest`) runs register → download →
validate → activate → inference telemetry against the running stack and checks
through the REST API that the server stored the event. With the server from
step 1 and an emulator running:

```bash
FLEETML_API_KEY=$FLEETML_API_KEY ./scripts/android-e2e.sh
```

The scripts need `ANDROID_HOME` (or `local.properties`) as in step 4. They
already run Gradle with JDK 17 whatever `JAVA_HOME` says: from `JAVA_HOME_17`
if it is set, otherwise from `/opt/homebrew/opt/openjdk@17`.

`./scripts/signing-e2e.sh android` runs `SignatureLiveTest` twice against
the same stack: a model whose stored signature was overwritten is rejected,
and one signed with the previous key is accepted during the rotation's grace
period and rejected after it.

They need a device or emulator and a running stack, so they are not part of
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
policy`. Deploy it with `"urgent": true` and run the job: the model is
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
