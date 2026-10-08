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
| `heartbeatIntervalSeconds` | `30` | Until the server sends its own interval |
| `telemetryBatchSize`, `telemetryFlushIntervalSeconds` | `50`, `30` | Until the server's `DeviceConfig` overrides them |
| `maxCacheSizeMb` | `500` | Disk budget for models; the active and last-known-good models are never evicted |
| `downloadOnWifiOnly` | `true` | New models wait for an unmetered network |
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

1. **Downloads** the model from the presigned URL, streaming to a `.part` file.
   An interrupted download resumes with an HTTP Range request.
2. **Verifies** the sha256 before moving the file into place.
3. **Validates** it with a `ModelValidator`.
4. **Activates** it only if validation passed: the model becomes active, the
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

## Offline behaviour

Models activated in an earlier run are delivered through `onModelReady` (and
`FleetML.activeModels()`) as soon as `start()` runs, before the server is
reached. A registered device starts heartbeating straight away; until the
server answers it keeps using its cached models, and `onConnectionStatusChanged`
reports reachability.

![Offline start in airplane mode](screenshots/android-offline-start.png)

## Telemetry

```kotlin
val model = FleetML.activeModel("iris")!!
FleetML.getTelemetryReporter().reportInference(model, latencyMs = 3, confidence = 0.97f, predictionLabel = "virginica")
```

Events go into a persistent queue (bounded to 10,000; oldest dropped first):
they survive process death and are kept while offline or before registration.
They are sent in batches every flush interval, with exponential backoff (5 s up
to 5 min) after a failure and an immediate flush when the connection returns.
For a model in an A/B experiment the event carries the device's arm ID
automatically, so results are attributed per arm.

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
   config screen), open it and tap **Save and connect**:

   ```bash
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

### Instrumented test

`LiveServerTest` (in `fleetml-sdk/src/androidTest`) runs register → download →
validate → activate → inference telemetry against the running stack and checks
through the REST API that the server stored the event. With the server from
step 1 and an emulator running:

```bash
FLEETML_API_KEY=$FLEETML_API_KEY ./scripts/android-e2e.sh
```

It needs a device or emulator and a running stack, so it is not part of the
hosted CI; the unit tests (`./gradlew :fleetml-sdk:testDebugUnitTest`) are.

## Sample app

`android/sample-app` is the reference integration: a config screen (host, port,
enrollment key), the SDK owned by the `Application`, a bundled Iris TFLite
model (`models/iris`, Apache-2.0) used until the server assigns one, and
inference telemetry. Cleartext traffic is allowed only in debug builds and only
to `10.0.2.2`, `localhost` and `127.0.0.1`.
