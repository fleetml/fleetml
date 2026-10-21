# Android SDK

The FleetML Android SDK, `dev.fleetml:fleetml-sdk` on Maven Central (Kotlin,
minSdk 26), puts a device under FleetML's control: it registers with an
enrollment key, receives model assignments over gRPC, downloads and verifies
them, validates each model before using it, keeps the last-known-good
version, works offline, and reports telemetry. It does not run your models:
your app does, with TFLite, ONNX Runtime or anything else.

This guide follows the path to a phone and then to production: add the SDK,
configure it, run it on your phone, publish and deploy a model to it, test
updates and offline behaviour, and check what production needs. The emulator,
Windows and the sample app follow as variants of the same steps.

## Install

The SDK is on Maven Central; 0.6.0 is the version this guide describes (0.4.0
lacks the `passed validation` logcat line and still logs a failed heartbeat
with a stack trace). Add it to the app with TensorFlow Lite next to it:

```kotlin
// app/build.gradle.kts
dependencies {
    implementation("dev.fleetml:fleetml-sdk:0.6.0")
    implementation("org.tensorflow:tensorflow-lite:2.16.1") // if you run TFLite models
}
```

The app's repositories must include `mavenCentral()`, as a new Android Studio
project's `settings.gradle.kts` does. The SDK needs minSdk 26 and is built
with compileSdk and targetSdk 34 and Java 8 bytecode, so the app needs
compileSdk 34 or later. TensorFlow Lite is not a dependency of the SDK: the app
brings the runtime it uses, and the default validator uses it when it is there
([Validators](#validators)).

The app needs no permission of its own for the SDK: the SDK's manifest
declares `INTERNET` and `ACCESS_NETWORK_STATE`, WorkManager, which it uses for
background checks, adds `WAKE_LOCK`, `RECEIVE_BOOT_COMPLETED` and
`FOREGROUND_SERVICE`, and AndroidX Core adds the app's own
`<application ID>.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION` to the merged
manifest.

The SDK brings these libraries with it: `protobuf-kotlin-lite` 3.25 (as an
`api` dependency: protobuf-lite messages appear in its public API),
`grpc-okhttp` and the gRPC stubs 1.62, `okhttp` 4.12, WorkManager 2.9,
`lifecycle-process`, `security-crypto` and BouncyCastle (`bcprov-jdk18on`).
Where the app asks for a newer version of one of them, Gradle uses the newer
one; the SDK is tested with the versions above, and
`./gradlew :app:dependencies --configuration debugRuntimeClasspath` shows what
the app ends up with. An app that already uses the full `protobuf-java` gets
duplicate-class errors from Gradle, because `protobuf-javalite` and
`protobuf-java` hold the same classes; the usual fix is to exclude the lite
runtime, since the full one also contains the classes lite-generated code uses
(not tested here with every protobuf version):

```kotlin
implementation("dev.fleetml:fleetml-sdk:0.6.0") {
    exclude(group = "com.google.protobuf", module = "protobuf-javalite")
}
```

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

The module's own build uses two more Gradle plugins and reads its version from
a property, which the app's build must then provide too: next to the app's
Android and Kotlin plugins in the root `build.gradle.kts`,

```kotlin
plugins {
    id("com.google.protobuf") version "0.9.4" apply false
    id("com.vanniktech.maven.publish") version "0.34.0" apply false
}
```

and `sdkVersion=0.7.0-dev` (any version string) in the app's
`gradle.properties`. Without them Gradle stops with
`Plugin [id: 'com.google.protobuf'] was not found`.

Building the SDK from source on the command line needs JDK 17: the wrapper's
Gradle 8.7 with AGP 8.5 fails on a newer default JDK. Point `JAVA_HOME` at it,
for example `/opt/homebrew/opt/openjdk@17` on macOS with Homebrew or
`/usr/lib/jvm/java-17-openjdk-amd64` on Debian and Ubuntu
(`sudo apt install openjdk-17-jdk`). Android Studio runs Gradle with its own
JDK, set in its Gradle settings.

## Configure and start

Initialize once per process, in `Application.onCreate()`, so screen rotation
never re-initializes the SDK. The configuration below is the production one:
your server's host name, port 443 and TLS, with the server behind a TLS proxy
([Production checklist](#production-checklist)). The host and the enrollment
key come from Gradle properties rather than the source, and debug builds
switch to a server on your own machine without TLS, in `app/build.gradle.kts`:

```kotlin
android {
    buildFeatures { buildConfig = true } // AGP 8 turns BuildConfig off by default
    defaultConfig {
        buildConfigField("String", "FLEETML_HOST", "\"${project.findProperty("fleetmlHost") ?: "fleet.example.com"}\"")
        buildConfigField("int", "FLEETML_PORT", "443")
        buildConfigField("boolean", "FLEETML_TLS", "true")
        buildConfigField("String", "FLEETML_ENROLLMENT_KEY", "\"${project.findProperty("fleetmlEnrollmentKey") ?: ""}\"")
    }
    buildTypes {
        debug {
            // A server on your machine: localhost for a phone over USB, 10.0.2.2 for the emulator,
            // the machine's LAN address for a phone over Wi-Fi.
            buildConfigField("String", "FLEETML_HOST", "\"${project.findProperty("fleetmlDebugHost") ?: "localhost"}\"")
            buildConfigField("int", "FLEETML_PORT", "19090")
            buildConfigField("boolean", "FLEETML_TLS", "false")
        }
    }
}
```

Without `buildFeatures { buildConfig = true }`, an AGP 8 project stops with
`Unresolved reference: BuildConfig`. The build gets the values on the command
line, `./gradlew :app:installDebug -PfleetmlEnrollmentKey=<enrollment key>`,
or from `~/.gradle/gradle.properties`, outside the project
(`fleetmlEnrollmentKey=...`, `fleetmlHost=...`). The enrollment key comes from
`fleetml enrollment-key create --project <project ID>`, which prints it once
([Publish and deploy](#publish-and-deploy-with-the-cli)). The SDK's classes are
in the package `com.fleetml.sdk`; the Maven group, `dev.fleetml`, is a
different name, so `import dev.fleetml...` finds nothing:

```kotlin
import android.app.Application
import com.fleetml.sdk.*

class MyApp : Application(), FleetMLCallback {
    override fun onCreate() {
        super.onCreate()
        FleetML.init(
            context = this,
            config = FleetMLConfig(
                serverHost = BuildConfig.FLEETML_HOST,
                serverPort = BuildConfig.FLEETML_PORT,
                useTls = BuildConfig.FLEETML_TLS,
                enrollmentKey = BuildConfig.FLEETML_ENROLLMENT_KEY,
                labels = mapOf("app" to "myapp", "tier" to "beta"),
            ),
            callback = this,
        )
    }

    override fun onModelReady(model: ActiveModel) {
        // model.file is verified and validated: load it.
    }
}
```

Register the class in `AndroidManifest.xml` with
`<application android:name=".MyApp" ...>`. For a quick test only,
`enrollmentKey = "<enrollment key>"` works too, but the key then sits in a file
you commit. The next two sections use the debug values: [On your
phone](#on-your-phone) with `localhost` over USB or the LAN address over
Wi-Fi, and the [Emulator](#emulator) with `10.0.2.2`.

`FleetMLCallback` has five methods, each with an empty default, so implement
only what the app needs. They run on a background thread: switch to the main
thread before touching views.

| Callback | When |
|---|---|
| `onModelReady(model: ActiveModel)` | A model was downloaded, verified and validated and is now active; also at start for models cached from an earlier run |
| `onModelUnload(modelId: String, modelName: String, variantId: String)` | The server no longer assigns the model |
| `onModelLoadFailed(modelName: String, version: String, reason: String)` | A new version could not be downloaded or failed validation; the last-known-good one stays |
| `onRegistered(deviceId: String)` | The device registered (or re-registered) |
| `onConnectionStatusChanged(connected: Boolean)` | The server became reachable or unreachable |

`onModelReady` hands over an `ActiveModel`:

| `ActiveModel` field | Type | What it is |
|---|---|---|
| `modelName` | `String` | The model's name, for example `iris` |
| `version` | `String` | The version, for example `1.0.0` |
| `file` | `java.io.File` | The model file on disk, sha256-verified and validated: load this |
| `modelId` | `String` | The model's ID, the same for every version |
| `versionId` | `String` | The version's ID; telemetry is attributed to it |
| `variantId` | `String` | The variant (one file of the version) the server chose for this device |
| `sha256` | `String` | The file's sha256 |

The other calls on `FleetML`:

| Call | What it does |
|---|---|
| `fun activeModel(modelName: String): ActiveModel?` | The active version of one model, or `null` before it has one |
| `fun activeModels(): List<ActiveModel>` | Every active model |
| `suspend fun syncNow(): Boolean` | One heartbeat and one telemetry flush now; `true` if the server was reached |
| `fun getTelemetryReporter(): TelemetryReporter` | Reports inferences ([Telemetry](#telemetry)) |
| `fun deviceId(): String?`, `fun isRegistered(): Boolean` | The device's ID once registered |
| `fun stop()` | Stops background work and clears the singleton, so `init` can be called again (for example with a new server address) |
| `fun reset(context: Context? = null)` | Also deletes cached models and the registration; with a `context`, before `init` |

`syncNow()` is a `suspend` function, so call it from a coroutine, for example
from an activity with `androidx.lifecycle:lifecycle-runtime-ktx` in the app's
dependencies and `import androidx.lifecycle.lifecycleScope` and
`import kotlinx.coroutines.launch` in the file:

```kotlin
lifecycleScope.launch { val reached = FleetML.syncNow() }
```

Called while a heartbeat is in flight, at start-up for example, it joins that
heartbeat rather than sending a second one, so a model is downloaded and
`onModelReady` fires once.

| `FleetMLConfig` field | Default | Purpose |
|---|---|---|
| `serverHost`, `serverPort` | –, `19090` | The FleetML gRPC endpoint |
| `enrollmentKey` | `""` | Per-project key for the first registration (`fleetml enrollment-key create`) |
| `useTls` | `false` | TLS for gRPC (`true` with the server behind a TLS proxy, as in production) |
| `labels` | `{}` | Labels deployments and experiments target (`target_selector`) |
| `heartbeatIntervalSeconds` | `30` | Foreground heartbeat interval, until the server sends its own (server default 60) |
| `backgroundHeartbeatIntervalSeconds` | `3600` | Background check interval, until the server sends its own (at least 15 minutes) |
| `telemetryBatchSize`, `telemetryFlushIntervalSeconds` | `50`, `30` | Until the server's `DeviceConfig` overrides them (server default 100; the SDK uses 50 until the first response) |
| `maxCacheSizeMb` | `500` | Disk budget for models; the active and last-known-good models are never evicted |
| `downloadPolicy` | `UNMETERED_OR_CHARGING` | Which networks non-urgent downloads may use, until the server sends its own ([Battery and data](#battery-and-data)) |
| `autoStart` | `true` | Call `FleetML.start()` yourself if `false` |

## On your phone

A phone reaches a server on your machine over USB, as `localhost` through
`adb` port forwards, or over Wi-Fi at the machine's LAN address. Both talk
plain HTTP to it (gRPC on 19090, model downloads on 19100), which Android
blocks by default, so debug builds get a network security config that allows
it to those addresses only; release builds keep Android's default
([Production checklist](#production-checklist)). The server is the one from the
[quickstart](QUICKSTART.md#run-fleetml) or any FleetML server without TLS that
the phone can reach. None of this section is verified on a phone by CI; the
maintainers run it by hand.

```xml
<!-- app/src/debug/res/xml/network_security_config.xml -->
<network-security-config>
    <domain-config cleartextTrafficPermitted="true">
        <domain includeSubdomains="false">localhost</domain>
        <domain includeSubdomains="false">10.0.2.2</domain>
        <!-- over Wi-Fi: the LAN address of the machine that runs the server -->
        <!-- <domain includeSubdomains="false"><LAN IP></domain> -->
    </domain-config>
</network-security-config>
```

```xml
<!-- app/src/debug/AndroidManifest.xml -->
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application android:networkSecurityConfig="@xml/network_security_config" />
</manifest>
```

### Over USB

1. Turn on USB debugging in the phone's developer options, connect it and
   accept the authorisation prompt; `adb devices` then lists it as `device`.
   On Windows, `adb devices` lists a phone only with its maker's USB driver
   (Google's for Pixel phones, from the SDK Manager), see
   [From Windows](#from-windows).
2. Pin `adb` and Gradle to the phone in this shell, with its serial from
   `adb devices`. With more than one device attached, an emulator or a
   colleague's phone, Gradle otherwise installs on every one of them and `adb`
   asks which one to use:

   ```bash
   export ANDROID_SERIAL=<the phone's serial>
   ```

3. Forward the REST, gRPC and model-file ports, so the phone reaches the
   machine as `localhost` (again after each reconnect of the cable):

   ```bash
   adb reverse tcp:18080 tcp:18080
   adb reverse tcp:19090 tcp:19090
   adb reverse tcp:19100 tcp:19100
   ```

   19100 matters as much as 19090: the server hands out model downloads as
   presigned URLs for its `S3_PUBLIC_ENDPOINT`, `localhost:19100` in the
   quickstart, so without that forward the phone registers and then fails
   every download ([CONFIG.md](CONFIG.md#network-and-storage)). On the dev
   stack (`make dev`), forward 19000 instead of 19100.

4. Build and install the debug build, whose host is `localhost`, with the
   enrollment key from [Publish and deploy](#publish-and-deploy-with-the-cli)
   (`$EK`), and start the app:

   ```bash
   ./gradlew :app:installDebug -PfleetmlEnrollmentKey=$EK
   adb shell monkey -p <application ID> -c android.intent.category.LAUNCHER 1
   ```

The SDK logs every step to logcat with the tag `FleetML`
(`adb logcat -s FleetML`). Within seconds of `FleetML.init`,
`onRegistered(deviceId)` fires and the device appears in
`fleetml device list --project $P` and on the dashboard's Devices page. If a
deployment already targets the app's labels, the model arrives right after
registration and `onModelReady` fires; a deployment created later arrives
with the next heartbeat, 60 seconds with the server's default. A first run
that registers and receives iris 1.0.0 prints (time stamps and process IDs
left out):

```text
D FleetML : Registering device with fingerprint <first 12 hex digits>...
I FleetML : In the background: next sync in 3600s through WorkManager
D FleetML : Registration saved for device <device ID>
I FleetML : Registered as device <device ID>
I FleetML : Downloaded model <variant ID> (2KB)
I FleetML : Model iris 1.0.0 passed validation
I FleetML : Model iris 1.0.0 is active
```

The background check is scheduled while the process starts, before the
activity is in front, and cancelled once it is. Each line comes once, also
when the app calls `FleetML.syncNow()` in `onCreate`.

At warning level or above it logs why registration, a heartbeat or a
download failed, one line each, such as an unreachable host or a refused
enrollment key; the stack trace of each failure follows at debug level, which
`adb logcat FleetML:I *:S` leaves out. A missing 19100 forward shows as
`Download of <variant ID> interrupted at 0 bytes: Failed to connect to localhost/127.0.0.1:19100`
on every heartbeat.

After fixing something (a port forward, a key, a deployment), the app does
not have to wait for its next heartbeat: `FleetML.syncNow()` runs one now, and
force-stopping and reopening the app (`adb shell am force-stop <application ID>`,
then start it again) registers and heartbeats straight away. The SDK's timers
run only while the app is in the foreground: an app behind another one waits
for its hourly background check ([Battery and data](#battery-and-data)), so
bring it to the front after a fix.

### Over Wi-Fi

The phone and the machine that runs the server must be on the same network.

1. Find the machine's LAN address: `ipconfig getifaddr en0` on a Mac (`en1`
   on some), `hostname -I` on Linux, and on Windows the IPv4 address
   `ipconfig` prints for the Wi-Fi or Ethernet adapter, not WSL's `ip addr`
   ([From Windows](#from-windows)).
2. Restart the server with its object store on that address, so download URLs
   point where the phone can reach them. In the quickstart's clone, in a new
   shell (the file brings back the secrets Compose needs, see
   [Save the three values](QUICKSTART.md#run-fleetml)):

   ```bash
   source ~/.fleetml.env
   S3_PUBLIC_ENDPOINT=<LAN IP>:19100 docker compose -f deploy/docker-compose.prod.yml up -d
   ```

3. Let the phone in: with the macOS firewall on, allow Docker's incoming
   connections when macOS asks; a Linux firewall needs TCP 19090 and 19100
   open; Windows needs the `New-NetFirewallRule` line in
   [From Windows](#from-windows).
4. Allow cleartext to that address: in your app, the commented line in the
   debug network security config above, with the address in place of
   `<LAN IP>`; in the [sample app](#sample-app), the line
   `<domain includeSubdomains="false">mac-lan-ip.invalid</domain>` in
   `android/sample-app/src/debug/res/xml/network_security_config.xml`, with
   the address in place of `mac-lan-ip.invalid`. Without it the app registers
   (gRPC does not check the policy) and then every download fails with
   `Download of <variant ID> interrupted at 0 bytes: CLEARTEXT communication to <LAN IP> not permitted by network security policy`.
5. Build with the address as the debug host:

   ```bash
   ./gradlew :app:installDebug -PfleetmlEnrollmentKey=$EK -PfleetmlDebugHost=<LAN IP>
   ```

   The sample app takes the address on its config screen instead.

The logcat lines are the same as over USB. A phone that registers but never
downloads is almost always the object store's address: check it with
`docker inspect fleetml-prod-server-1 --format '{{range .Config.Env}}{{println .}}{{end}}' | grep S3_PUBLIC_ENDPOINT`.

## Publish and deploy with the CLI

The CLI side: a project, two versions, an enrollment key, and a deployment to
the labels the app sets (`app=myapp` above). It needs the `fleetml` CLI
([Get the CLI](QUICKSTART.md#get-the-cli)) and `jq`, and a key for the server:

- **The server's admin key** (the quickstart's `$FLEETML_ADMIN_KEY`) does
  everything below, including creating the project.
- **A key for a user with the `admin` role in an existing project** does
  everything but create the project (`Error: this needs a global admin`);
  with `operator` it publishes and deploys, and `enrollment-key create` stops
  with `Error: this needs the admin role in this project`
  ([roles](ARCHITECTURE.md#users-roles-and-the-audit-log)).
- **Only the enrollment key**, when whoever runs the server creates the
  deployment: the app needs nothing else.

The two models are the sample's, from the root of a clone; push your own
`.tflite` files instead. With the quickstart's server, `FLEETML_SERVER` is
`http://localhost:18080` and the key `$FLEETML_ADMIN_KEY` after
`source ~/.fleetml.env`:

```bash
export FLEETML_SERVER=<server URL> FLEETML_API_KEY=<the admin or project key your team gave you>
P=$(fleetml project create --name myapp -o json | jq -r .id)   # or P=<the project ID your team gave you>
fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
fleetml push models/iris/iris_v2.tflite --project $P --model iris --version 2.0.0
M=$(fleetml model list --project $P -o json | jq -r '.[] | select(.name == "iris") | .id')
V1=$(fleetml ver list --model $M -o json | jq -r '.[] | select(.version == "1.0.0") | .id')
V2=$(fleetml ver list --model $M -o json | jq -r '.[] | select(.version == "2.0.0") | .id')
EK=$(fleetml enrollment-key create --project $P --name myapp -o json | jq -r .key)
D1=$(fleetml deploy create --project $P --version $V1 --target '{"app":"myapp"}' -o json | jq -r .id)
fleetml deploy start $D1
```

Each push prints `✓ Pushed ...`. The enrollment key is in `$EK`
(`-o json | jq -r .key` takes the key; the plain command prints it once for
copying); build the app with it from the same shell, as in [On your
phone](#on-your-phone). The deployment starts as `COMPLETE`
(`✓ Deployment start → COMPLETE`), which means assigned; once the app runs,
logcat shows `Model iris 1.0.0 is active`. `fleetml deploy get $D1 | jq -c .devices`
counts what the devices reported in their last heartbeat, so it prints
`{"loaded":0,"targeted":1}` until the app's next heartbeat (60 seconds by
default) and `{"loaded":1,"targeted":1}` after it; it is counted each time you
ask, so once 2.0.0 replaces it, this deployment counts down to
`{"loaded":0,"targeted":1}`.

## Offline and update tests

**An update.** Roll out 2.0.0:

```bash
D2=$(fleetml deploy create --project $P --version $V2 --target '{"app":"myapp"}' -o json | jq -r .id)
fleetml deploy start $D2
```

The app gets it with its next heartbeat, 60 seconds by default; logcat shows
`Model iris 2.0.0 built from a 603-byte delta instead of 2492 bytes` and
`Model iris 2.0.0 is active`, and 1.0.0 stays on the device as
last-known-good. After that, `fleetml telemetry events --model-version $V2`
prints a table of the version's events (type, latency, confidence, error): a
`MODEL_LOAD` when the version became active, and an `INFERENCE` for each
[inference the app reports](#telemetry). Before the app has the version it
prints `No results.`.

**Offline.** Models activated in an earlier run are delivered through
`onModelReady` (and `FleetML.activeModels()`) as soon as the SDK starts,
before the server is reached; until the server answers, the app keeps using
them and `onConnectionStatusChanged` reports reachability. On a phone over
Wi-Fi, turn on airplane mode, then force-stop and reopen the app. On a phone
over USB, airplane mode does not cut the `adb reverse` forwards, so remove them
instead:

```bash
adb reverse --remove-all
adb shell am force-stop <application ID>
adb shell monkey -p <application ID> -c android.intent.category.LAUNCHER 1
```

`onModelReady` fires for the cached model at once and
`onConnectionStatusChanged(false)` follows. Logcat shows each failed heartbeat
on one line, over USB without the forwards:

```text
W FleetML : Heartbeat failed: UNAVAILABLE: connect failed: ECONNREFUSED (Connection refused)
```

and in airplane mode:

```text
W FleetML : Heartbeat failed: UNAVAILABLE: connect failed: ENETUNREACH (Network is unreachable)
```

With events waiting to be sent (an inference reported while offline, say),
each flush adds
`W FleetML : Telemetry flush failed, 1 events kept: UNAVAILABLE: ...`; with
an empty queue there is no such line. Set the three forwards again (or turn
airplane mode off): a later heartbeat reconnects, `onConnectionStatusChanged(true)`
fires and what is queued is sent (`Telemetry flushed: 1 sent, 1 accepted`).
The SDK retries every 30 seconds until the server answers, and after airplane
mode the network itself takes a moment to come back, so expect the reconnect
within about a minute: it took 28 seconds after the forwards were set again,
and 24 to 50 seconds after airplane mode was turned off, on an API 35
emulator.

**On the emulator** the same test runs with airplane mode, which also cuts
`10.0.2.2`, on API 30 and later (on API 26 to 29, the quick settings tile):

```bash
adb shell cmd connectivity airplane-mode enable
adb shell am force-stop <application ID>
adb shell monkey -p <application ID> -c android.intent.category.LAUNCHER 1
```

and `adb shell cmd connectivity airplane-mode disable` brings the network
back.

![Offline start in airplane mode](screenshots/android-offline-start.png)

## Production checklist

- **TLS.** FleetML does not terminate TLS: put a proxy in front that does,
  for gRPC (19090; gRPC needs HTTP/2 end to end) and for the object store's
  downloads (19100, or your S3 endpoint), see the
  [trust boundaries](ARCHITECTURE.md#trust-boundaries) and, on Kubernetes, the
  [Helm chart](../deploy/helm/fleetml/README.md). The app then uses
  `serverPort = 443` and `useTls = true`, the release values in
  [Configure and start](#configure-and-start).
- **The download address.** The server's `S3_PUBLIC_ENDPOINT` must be an
  address phones reach from the public internet, since every download URL is
  signed for it ([CONFIG.md](CONFIG.md#network-and-storage)); with an object
  store behind the same proxy, its public host name on 443.
- **Release builds** carry no network security config: the cleartext config
  lives in `src/debug` only, so a release build keeps Android's default and
  talks TLS only.
- **The enrollment key** is not a secret in the way a password is (it is in
  every installed APK), but it lets a device join the project: keep it out of
  the source (a Gradle property or the CI's secret store), give each app or
  release channel its own (`fleetml enrollment-key create --name ...`), and
  revoke one that leaks: devices that already registered keep their device
  tokens, and only new registrations need the new key.
- **Foreground only.** The SDK's heartbeat and telemetry timers run only
  while the app is in the foreground. In the background a WorkManager job
  checks in every `background_heartbeat_interval_seconds` (an hour by
  default, at least 15 minutes), which survives reboots, though some makers'
  battery optimisations delay or skip background jobs for apps the user has
  not opened in a while; this is not tested per maker here.

## Emulator

The emulator reaches the machine it runs on as `10.0.2.2`, so the debug build
gets that host: `-PfleetmlDebugHost=10.0.2.2`. That covers gRPC on 19090.
Model files are another matter: the server hands each one out as a URL for
`localhost:19100` (the quickstart's `S3_PUBLIC_ENDPOINT`), and `localhost`
inside the emulator is the emulator itself. Forward that one port once per
emulator start, before the app's first download, with `ANDROID_SERIAL` set to
the emulator (the name `adb devices` lists) when a phone is attached too:

```bash
export ANDROID_SERIAL=emulator-5554
adb reverse tcp:19100 tcp:19100
./gradlew :app:installDebug -PfleetmlEnrollmentKey=$EK -PfleetmlDebugHost=10.0.2.2
```

Without the forward the app registers and then logs
`Download of <variant ID> interrupted at 0 bytes: Failed to connect to localhost/127.0.0.1:19100`
on every heartbeat. With all three forwards from [Over USB](#over-usb), the
emulator runs the phone's USB path as it is, with the default `localhost`.
`adb shell pm clear <application ID>` starts the app from nothing: no
registration, no cached model. Airplane mode is in [Offline and update
tests](#offline-and-update-tests).

### Emulator runbook

This is how the SDK and the [sample app](#sample-app) are verified, against
the server from the [quickstart](QUICKSTART.md#run-fleetml) (the production
compose stack, with the admin key in `$FLEETML_ADMIN_KEY`) and the `fleetml`
CLI. It runs from the root of a clone, since the sample app and models are in
the repository. Contributors on the dev stack (`make dev`) use the key from
`.env` and port 19000 instead of 19100.

1. Start the server with the quickstart's "Run FleetML" block (or
   `source ~/.fleetml.env` in a new shell).

2. Start an emulator (API 26 or later), from Android Studio's Device Manager
   or `emulator -avd <name>`, pin to it and forward the object store's port:

   ```bash
   export ANDROID_SERIAL=emulator-5554
   adb reverse tcp:19100 tcp:19100
   ```

   The sample app connects to `10.0.2.2:19090` by default.

3. Publish the sample models, create an enrollment key and deploy 1.0.0 to the
   sample app's label (`app=sample`), as in [Publish and
   deploy](#publish-and-deploy-with-the-cli) with these names:

   ```bash
   export FLEETML_SERVER=http://localhost:18080 FLEETML_API_KEY=$FLEETML_ADMIN_KEY
   P=$(fleetml project create --name android-demo -o json | jq -r .id)
   fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
   fleetml push models/iris/iris_v2.tflite --project $P --model iris --version 2.0.0
   M=$(fleetml model list --project $P -o json | jq -r '.[] | select(.name == "iris") | .id')
   V1=$(fleetml ver list --model $M -o json | jq -r '.[] | select(.version == "1.0.0") | .id')
   V2=$(fleetml ver list --model $M -o json | jq -r '.[] | select(.version == "2.0.0") | .id')
   EK=$(fleetml enrollment-key create --project $P --name emulator -o json | jq -r .key)
   D=$(fleetml deploy create --project $P --version $V1 --target '{"app":"sample"}' -o json | jq -r .id)
   fleetml deploy start $D
   ```

4. Build and install the sample app with the key prefilled (or type it on the
   config screen), open it and tap **Save and connect**. A fresh clone has no
   `android/local.properties`, so Gradle on the command line finds the Android
   SDK through `ANDROID_HOME` (`$HOME/Library/Android/sdk` on macOS,
   `$HOME/Android/Sdk` on Linux, where Android Studio puts it), and it needs JDK
   17 ([From a clone](#from-a-clone)); Android Studio writes `local.properties`
   when it opens the project.

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
   **Active model: iris 1.0.0**; logcat shows the lines in [Over
   USB](#over-usb). **Classify sample flower** runs it and reports inference
   telemetry. `fleetml deploy create --project $P --version $V2 --target '{"app":"sample"}'`
   and `deploy start` update the app to 2.0.0 on its next heartbeat.

5. Offline start: airplane mode as in [Offline and update
   tests](#offline-and-update-tests), force-stop and reopen the app. The
   cached model is active immediately and inference works; events are queued
   and sent once airplane mode is off.

The sample app runs on a phone the same way: over USB, the three forwards and
`localhost` in place of `10.0.2.2` on its config screen; over Wi-Fi, its
network security config line from [Over Wi-Fi](#over-wi-fi) and the LAN
address on the config screen.

## From Windows

With the server in WSL 2 ([On Windows](QUICKSTART.md#on-windows)), the
Android tools run on the Windows side, where Android Studio installs them:
`adb` and the emulator are in `%LOCALAPPDATA%\Android\Sdk` (`adb` in its
`platform-tools`), and an `adb` installed inside Ubuntu sees neither a phone
nor the Windows emulator. The CLI steps ([Publish and
deploy](#publish-and-deploy-with-the-cli)) run in the Ubuntu shell, and the
enrollment key crosses by hand (`echo $EK` there). Gradle runs in PowerShell,
in a clone on the Windows side (`git clone https://github.com/fleetml/fleetml.git`
in PowerShell, for the sample app; Gradle is slow on a `\\wsl.localhost`
path), with Android Studio's SDK and its bundled JDK 17. None of this is
verified on Windows by CI.

A phone over USB needs its maker's USB driver on Windows before
`adb devices` lists it (Google's for Pixel phones, from the SDK Manager).
Then, with the phone's serial from `adb devices`:

```powershell
$env:ANDROID_HOME = "$env:LOCALAPPDATA\Android\Sdk"
$env:JAVA_HOME = "$env:ProgramFiles\Android\Android Studio\jbr"
$env:Path += ";$env:ANDROID_HOME\platform-tools"
$env:ANDROID_SERIAL = '<the phone''s serial>'
adb reverse tcp:18080 tcp:18080
adb reverse tcp:19090 tcp:19090
adb reverse tcp:19100 tcp:19100
.\gradlew.bat :app:installDebug -PfleetmlEnrollmentKey=<enrollment key>
```

(`:sample-app:installDebug` in the clone's `android` folder for the sample
app.) The reverses reach the ports Docker Desktop publishes on Windows.

A phone over Wi-Fi uses the Windows machine's LAN address, the IPv4 address
`ipconfig` prints for the Wi-Fi or Ethernet adapter, not WSL's `ip addr`,
which is an internal address. In the Ubuntu shell, restart the stack with
`source ~/.fleetml.env` and
`S3_PUBLIC_ENDPOINT=<LAN IP>:19100 docker compose -f deploy/docker-compose.prod.yml up -d`
as in [Over Wi-Fi](#over-wi-fi); let TCP 19090 and 19100 through Windows
Firewall in an administrator PowerShell:

```powershell
New-NetFirewallRule -DisplayName FleetML -Direction Inbound -Protocol TCP -LocalPort 19090,19100 -Action Allow
```

and add the address to the debug network security config (for the sample app,
in place of `mac-lan-ip.invalid` in
`android\sample-app\src\debug\res\xml\network_security_config.xml`), then
build with `-PfleetmlDebugHost=<LAN IP>`.

On the emulator, the same PowerShell lines with
`$env:ANDROID_SERIAL = 'emulator-5554'`, only `adb reverse tcp:19100 tcp:19100`,
and `-PfleetmlDebugHost=10.0.2.2` (the sample app uses `10.0.2.2` by
default). Or open the folder in Android Studio.

## Telemetry

```kotlin
val model = FleetML.activeModel("iris")!!
FleetML.getTelemetryReporter().reportInference(model, latencyMs = 3, confidence = 0.97f, predictionLabel = "virginica")
// with input values, for feature-level drift (only features configured on the server are kept):
FleetML.getTelemetryReporter().reportInference(model, latencyMs = 3, features = mapOf("petal_length" to floatArrayOf(5.1f)))
```

On the server, `fleetml telemetry events --model-version <version ID>` lists
the events of a version (`--device <device ID>` narrows them to one device).

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
`local.properties`) as in the runbook's step 4, and runs Gradle with JDK 17
whatever `JAVA_HOME` says: from `JAVA_HOME_17` if it is set, otherwise from
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
the hosted CI; the unit tests (`./gradlew :fleetml-sdk:testDebugUnitTest
:sample-app:testDebugUnitTest`) are. The test process has no activity, so the
SDK counts as being in the background and the test drives it with
`FleetML.syncNow()`.

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

![Sample app](screenshots/android-sample.png)

`android/sample-app` is the reference integration: a config screen (host, port,
enrollment key), the SDK owned by the `Application`, a bundled Iris TFLite
model (`models/iris`, Apache-2.0) used until the server assigns one, and
inference telemetry. Cleartext traffic is allowed only in debug builds and only
to `10.0.2.2`, `localhost`, `127.0.0.1` and the placeholder line for a phone
over Wi-Fi ([Over Wi-Fi](#over-wi-fi)). The [Emulator
runbook](#emulator-runbook) builds and runs it.
