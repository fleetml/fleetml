# Advanced: everything that is a choice

The [Get started](../README.md#get-started) steps, and the Get started
section at the top of each guide, use the defaults and ask you to choose
nothing. This page has the choices they leave out: other ports, a phone or a
second machine on your network, Android Studio instead of the command line,
the Helm chart's variants, upgrades, and the details of the saved settings.
Each guide links to the part that concerns it.

- [Installing the tools](#installing-the-tools)
- [Troubleshooting](#troubleshooting)
- [The settings file, ~/.fleetml.env](#the-settings-file-fleetmlenv)
- [Other ports](#other-ports)
- [A phone or another machine: the download address](#a-phone-or-another-machine-the-download-address)
- [Android: a phone over Wi-Fi](#android-a-phone-over-wi-fi)
- [Android on Windows: Android Studio](#android-on-windows-android-studio)
- [Linux agent: a server on another machine](#linux-agent-a-server-on-another-machine)
- [Kubernetes instead of compose](#kubernetes-instead-of-compose)
- [Helm chart variants](#helm-chart-variants)
- [Upgrading](#upgrading)

## Installing the tools

Get started needs Docker with Compose v2, git, curl, jq and `openssl`, and
about 2 GB of free memory for the five running containers (plus a one-shot
`migrate` job, which `docker compose ps -a` lists as exited) and the two
image builds:

| | Docker | git, curl, openssl | jq |
|---|---|---|---|
| macOS | [Docker Desktop](https://docs.docker.com/desktop/setup/install/mac-install/) | already there (git asks to install the Command Line Tools on first use) | already there on macOS 15 and later; `brew install jq` before |
| Ubuntu, Debian | [Docker Engine](https://docs.docker.com/engine/install/ubuntu/) with the Compose plugin | `sudo apt install -y git curl openssl` | `sudo apt install -y jq` |
| Windows | Docker Desktop with WSL 2, see [On Windows](QUICKSTART.md#on-windows) | in the Ubuntu shell: `sudo apt install -y git jq` | |

On Windows FleetML runs under WSL 2: the quickstart runs in a WSL 2 shell
with Docker Desktop's WSL 2 backend ([On Windows](QUICKSTART.md#on-windows)),
and GitHub's Windows runner exercises the CLI and the Get started blocks in
WSL 2 (with Docker Engine there, not Docker Desktop).

The CLI comes from the release as Get started's step 3 installs it. On
Linux without Perl, `sha256sum -c` replaces `shasum -a 256 -c`. Keep the
`export PATH` line in `~/.bashrc` or `~/.zshrc` (many Linux distributions
already have `~/.local/bin` on the `PATH`); for every user of the machine,
`sudo mv fleetml /usr/local/bin/` in place of the `mkdir` and `mv`. From
source, with Go 1.26: `go build -o bin/fleetml ./cmd/fleetml` (it reports its
version as `dev`; in WSL, install Go from [go.dev](https://go.dev/doc/install): Ubuntu's
`golang` package is older). The other platforms and the PowerShell block are in
[Get the CLI](QUICKSTART.md#get-the-cli).

## Troubleshooting

**The server never answers** (step 2 keeps waiting). The first start takes a
few minutes on a cold Docker cache, most of it pulling the base images and
building the server and dashboard images; with the images cached, well under
a minute. If nothing appears after about two minutes more than the build
took, run `docker compose -f deploy/docker-compose.prod.yml ps -a` and
`docker compose -f deploy/docker-compose.prod.yml logs server` to see which
container stopped and why (a port already in use is the usual cause: see
[Other ports](#other-ports)). On Windows, a port can also sit in a range
that Hyper-V or WSL reserves: Docker then fails with `bind: An attempt was
made to access a socket in a way forbidden by its access permissions`, and
`netsh int ipv4 show excludedportrange protocol=tcp` in PowerShell lists the
reserved ranges; move the port out of them with its variable before the
block ([Other ports](#other-ports)).

**`/readyz`** answers `{"db":"connected","nats":"connected","s3":"connected","status":"ok","version":"<version>"}`
once Postgres, the object store and NATS are reachable, and the project list
answers `{"data":[],"pagination":{...}}`: no projects yet. `version` is the
server's release, from the clone's tag (on `main` after a release it reads
like `0.10.0-3-g1a2b3c4`: three commits past 0.10.0). To confirm the version
later: `curl -s localhost:${FLEETML_REST_PORT:-18080}/readyz`. More keys and
their routes: [API keys](CONFIG.md#api-keys).

**The device never shows `model active`** (step 5). `docker logs fleetml-agent`
shows why, one line per heartbeat: `msg="download failed"` with the address
it tried means the object store's address or port
([the download address](#a-phone-or-another-machine-the-download-address));
`registration failed` with `invalid or revoked enrollment key` means
`FLEETML_ENROLLMENT_KEY` is not set in this terminal (step 4 sets it: run
step 4 again, then `docker rm -f fleetml-agent` and step 5). The
[Linux agent guide](LINUX_AGENT.md) has every line the agent logs.

**The phone never shows `is active`** (Android step 5 says `Not active after
3 minutes`). `adb logcat -d -s FleetML` shows how far the app got:
- `In the background: ...`: the phone locked or the screen went off. Unlock
  it, keep it on, and run step 5's last two lines again.
- `Heartbeat failed: UNAVAILABLE` or nothing after `Starting FleetML`: the
  phone does not reach the server. `adb reverse --list` must show both
  ports; run step 5's two `adb reverse` lines again (a reconnected cable or a
  restarted `adb` drops them).
- `Registered as device`, then `Download of <variant ID> ... failed`: the
  model files' forward is missing (the second `adb reverse` line), or the
  server hands out an address the phone cannot reach
  ([the download address](#a-phone-or-another-machine-the-download-address)).
- No lines at all: the app runs on another device. Step 5's `Device:` line
  says which; `export ANDROID_SERIAL=<serial>` from `adb devices` and run
  step 5 again.

**The iPhone never shows up** (iOS step 6c says `No new device after 3
minutes`): step 6 lists what each error means. The simulator (iOS step 5
says `Not active after 2 minutes`):
`xcrun simctl spawn $SIM log show --last 5m --style compact --predicate 'subsystem == "com.fleetml.sdk"'`
shows the SDK's lines, as above.

**`fleetml` is not found** in a new terminal: the `export PATH` line of step
3 lasts for one terminal ([Installing the tools](#installing-the-tools)).
**`Error: ... 401`** from the CLI in a new terminal: `source ~/.fleetml.env`
and the `export FLEETML_SERVER` line of step 4 again.

## The settings file, ~/.fleetml.env

Get started's step 2 generates the stack's three secrets and saves them in
`~/.fleetml.env`, a file only you can read, before anything starts, with
the four ports (the defaults unless you set them, [Other ports](#other-ports))
and `S3_PUBLIC_ENDPOINT`, the address devices download models from,
`localhost` on the model files' port (a phone or a second machine needs your
LAN address there instead; the device guides change it in this file, see
[the download address](#a-phone-or-another-machine-the-download-address)).
It leaves an existing file alone, so running the block again reuses the
saved values. **Keep the file.** The admin key in it is how you log in to
the dashboard and how the CLI and the REST API reach the server, and the
server stores it on the first start only: later starts ignore
`FLEETML_ADMIN_KEY`, so a lost key cannot be replaced by exporting a new one
(the way back is `down -v` and a fresh start, which deletes the data).
`SIGNING_MASTER_KEY` encrypts the signing keys, so a new one breaks
publishing. Compose reads the file's values for every command on this
stack, `down` included: in a new shell, `source ~/.fleetml.env` first (the
[tour](QUICKSTART.md#tour) adds its own values to the same file).

The block prints no secret. To log in, paste `FLEETML_ADMIN_KEY` from the
file; `printf %s "$FLEETML_ADMIN_KEY" | pbcopy` puts it on the clipboard
without printing it on macOS (`| clip.exe` in WSL, `| xclip -selection
clipboard` on Linux).

## Other ports

Get started uses port 80 for the dashboard, 18080 for the REST API, 19090
for the device gRPC API and 19100 for model files, and 13000 for Grafana if
you start the optional observability profile. To use other ports, export
`FLEETML_DASHBOARD_PORT`, `FLEETML_REST_PORT`, `FLEETML_GRPC_PORT` or
`FLEETML_S3_PUBLIC_PORT` (and `FLEETML_GRAFANA_PORT` for Grafana) before
Get started's step 2, for example `export FLEETML_REST_PORT=28080`: the
block saves the first four in `~/.fleetml.env` (see
[CONFIG.md](CONFIG.md#docker-compose)), and every later block reads them
from there. `export FLEETML_S3_PUBLIC_PORT=29100` saves the port and
`S3_PUBLIC_ENDPOINT=localhost:29100` together.

If port 80 is taken (by IIS or a VPN agent on Windows, for example), move the
dashboard on a stack that already runs by adding the port to `~/.fleetml.env`
(a later line wins) and starting it again, here on http://localhost:8081;
the same works for `FLEETML_REST_PORT` and `FLEETML_GRPC_PORT`, and
`source ~/.fleetml.env` brings back the secrets and the saved
`S3_PUBLIC_ENDPOINT`, which `up -d` would otherwise reset:

```bash
echo 'export FLEETML_DASHBOARD_PORT=8081' >> ~/.fleetml.env
source ~/.fleetml.env
docker compose -f deploy/docker-compose.prod.yml up -d
```

To move the model files' port on a stack already started, change both
`FLEETML_S3_PUBLIC_PORT` and the port in `S3_PUBLIC_ENDPOINT` in
`~/.fleetml.env`, then `source ~/.fleetml.env` and `up -d` as above.

With a port moved, every block in the docs follows it: the blocks read the
saved ports as `${FLEETML_REST_PORT:-18080}`, `${FLEETML_GRPC_PORT:-19090}`
and `${FLEETML_S3_PUBLIC_PORT:-19100}` after `source ~/.fleetml.env`. In
prose the docs name the default; use yours there. The CLI's
`fleetml simulate` takes the gRPC port as `--grpc localhost:${FLEETML_GRPC_PORT:-19090}`,
which the tour's lines already carry.

Before 0.9.1, the only documented way to move the dashboard was an override
file passed with `-f` (`dashboard-port.yml`). The env file's line replaces
it: add the port to `~/.fleetml.env` as above and leave the override file
out of every later compose command, the [Upgrading](#upgrading) block
included.

## A phone or another machine: the download address

The server hands out every model as a download URL for its object store's
public address, `S3_PUBLIC_ENDPOINT`. Get started saves it as `localhost` on
the model files' port (`${FLEETML_S3_PUBLIC_PORT:-19100}`) in
`~/.fleetml.env`, which works for everything on the server's own machine:
the agent in a container (through its `-download-host-override`), the iOS
simulator, and an Android phone over USB or the emulator (through
`adb reverse`).

A phone over Wi-Fi or a second machine downloads models from `S3_PUBLIC_ENDPOINT`,
`localhost` and the model files' port (19100 unless you set
`FLEETML_S3_PUBLIC_PORT`) in `~/.fleetml.env`: the guides set it to the
machine's LAN address there, on the same port. The line that does it, in the stack's clone,
with the machine's LAN address in place of `<LAN IP>` (`ipconfig getifaddr en0`
on a Mac, `en1` on some; `hostname -I` on Linux):

```bash
. ~/.fleetml.env && sed -i.bak "s/S3_PUBLIC_ENDPOINT=[^ ]*/S3_PUBLIC_ENDPOINT=<LAN IP>:${FLEETML_S3_PUBLIC_PORT:-19100}/" ~/.fleetml.env && rm ~/.fleetml.env.bak
source ~/.fleetml.env
docker compose -f deploy/docker-compose.prod.yml up -d
```

From then on downloads go to the LAN address for every device, the USB,
emulator and simulator paths included, so those need that address allowed
too (the guides say how). This shows the value in use:

```bash
docker inspect $(docker compose -f deploy/docker-compose.prod.yml ps -q server) --format '{{range .Config.Env}}{{println .}}{{end}}' | grep S3_PUBLIC_ENDPOINT
```

A phone that registers but never downloads is almost always this address
or its port: the phone must reach `<LAN IP>` on the model files' port, which
a guest network that isolates devices or the machine's firewall can block.

## Android: a phone over Wi-Fi

The [Android guide](ANDROID_SDK.md)'s Get started puts the phone on the
server over its USB cable. Over Wi-Fi instead, the phone reaches the
machine at its LAN address. Do it in the Get started terminal after step 4
(the stack running, `FLEETML_ENROLLMENT_KEY` set), in the clone, with the
phone and the machine on the same Wi-Fi network (not a guest network that
keeps devices apart) and the phone unlocked. The cable stays for the
install; nothing goes through it afterwards.

1. **Point the server's downloads at the machine's LAN address**, save it in
   `~/.fleetml.env` so every later compose command keeps it, and restart the
   server. On a Mac (`en1` instead of `en0` on some; on Linux,
   `LAN=$(hostname -I | awk '{print $1}')`; on Windows, the IPv4 address
   `ipconfig` prints for the Wi-Fi adapter, see
   [From Windows](ANDROID_SDK.md#from-windows)):

   ```bash
   LAN=$(ipconfig getifaddr en0); echo "LAN address: $LAN"
   . ~/.fleetml.env && sed -i.bak "s/S3_PUBLIC_ENDPOINT=[^ ]*/S3_PUBLIC_ENDPOINT=$LAN:${FLEETML_S3_PUBLIC_PORT:-19100}/" ~/.fleetml.env && rm ~/.fleetml.env.bak
   source ~/.fleetml.env && docker compose -f deploy/docker-compose.prod.yml up -d
   ```

   You should see `LAN address: ` and an address such as `192.168.1.20`,
   then `Container <stack>-server-1  Started`. From then on downloads go to
   that address for every device, the USB and emulator paths included
   ([Over USB](ANDROID_SDK.md#over-usb), step 3).

2. **Let the phone in.** With the macOS firewall on, allow Docker's incoming
   connections when macOS asks. A Linux firewall needs TCP
   `${FLEETML_GRPC_PORT:-19090}` and `${FLEETML_S3_PUBLIC_PORT:-19100}` open;
   Windows needs the `New-NetFirewallRule` line in
   [From Windows](ANDROID_SDK.md#from-windows).

3. **Build the sample app with that address, install it, and start it.**
   This removes the USB forwards first, so the phone really uses Wi-Fi; the
   app takes the new address over the one it saved before:

   ```bash
   adb reverse --remove-all
   (cd android && ORG_GRADLE_PROJECT_fleetmlEnrollmentKey=$FLEETML_ENROLLMENT_KEY ./gradlew -q :sample-app:installDebug -PfleetmlDebugHost=$LAN -PfleetmlDebugPort=${FLEETML_GRPC_PORT:-19090} </dev/null)
   adb logcat -c && adb shell am start -S -n com.fleetml.sample/.MainActivity </dev/null
   for i in $(seq 90); do adb logcat -d -s FleetML:I | grep -m 1 'is active' && break; [ $i = 90 ] && echo "Not active after 3 minutes: see Troubleshooting below"; sleep 2; done
   ```

   You should see `Installed on 1 device.` and `I FleetML : Model iris 1.0.0 is active`,
   and the app's screen **Active model: iris 1.0.0**, its config screen
   showing the LAN address.

4. **Clean up** with Get started's step 7: it removes the app, the server
   and its data, and `~/.fleetml.env` with the LAN address in it.

Your own app over Wi-Fi: the same steps 1 and 2, then build it with
`-PfleetmlDebugHost=$LAN` as in [Over USB](ANDROID_SDK.md#over-usb) step 4,
and allow cleartext to that address: the commented line in your debug
network security config ([Install](ANDROID_SDK.md#install)) with the address
in place of `<LAN IP>`. The sample app needs no edit: its build generates the
config with the address given as `-PfleetmlDebugHost`. Without it the app
registers (gRPC does not check the policy) and then every download fails
with `Download of <variant ID> interrupted at 0 bytes: CLEARTEXT communication to <LAN IP> not permitted by network security policy`.

Troubleshooting:

- No `is active`, and `adb logcat -s FleetML` shows `Heartbeat failed: UNAVAILABLE`:
  the phone does not reach the gRPC port. Check that it is on the same
  Wi-Fi as the machine, and the firewall (step 2).
- `Registered as device`, then no download: the phone does not reach the
  model files' port, or the server still hands out `localhost`. This shows
  the address in use:
  `docker inspect $(docker compose -f deploy/docker-compose.prod.yml ps -q server) --format '{{range .Config.Env}}{{println .}}{{end}}' | grep S3_PUBLIC_ENDPOINT`.
- `In the background: next sync in 3600s`: the phone locked or the app
  left the screen. Unlock it and run step 3's last two lines again.

## Android on Windows: Android Studio

[From Windows](ANDROID_SDK.md#from-windows) builds the sample app with
`gradlew.bat` in PowerShell. To run it from Android Studio instead, open the
`android` folder in Android Studio and run `sample-app` there, with Studio's
Gradle JDK set to the Temurin 17 (Settings, Build, Execution, Deployment,
Build Tools, Gradle, Gradle JDK): its default, the bundled Java 25, stops
with `What went wrong: 25.0.2`. The enrollment key and the debug host then
go in `android/local.properties` (`fleetml.enrollmentKey=...`,
`fleetml.debugHost=localhost`; the file is not committed), which the sample
app's build reads as it reads the `-P` properties.

## Linux agent: a server on another machine

The agent talks to the server in two ways, and both must reach it from this
machine:

| | Port | Set by | Where |
|---|---|---|---|
| Registration and heartbeats (gRPC) | the gRPC port: 19090, or the stack's `FLEETML_GRPC_PORT` | you | `server: <server address>:<gRPC port>` in the agent's config |
| Model downloads (presigned URLs) | the model files' port: 19100, or the stack's `FLEETML_S3_PUBLIC_PORT` | whoever runs the server | the server's `S3_PUBLIC_ENDPOINT`, or on this machine `download_host_override` |

The agent's machine has no `~/.fleetml.env` to read them from: ask whoever
runs the server which ports the stack uses (`grep -o 'FLEETML_[A-Z_]*PORT=[0-9]*' ~/.fleetml.env`
on the server's machine prints them), and put them in place of `<gRPC port>`
and `<model files port>` below.

The server hands out each model as a download URL for its object store's
public address, `S3_PUBLIC_ENDPOINT`. In the quickstart's compose stack that
is `localhost:19100`, which only the server's own machine reaches. There are
two ways out:

- **On the server**, whoever runs it saves an address of the server that
  this machine reaches as the stack's `S3_PUBLIC_ENDPOINT` (in the
  quickstart's `~/.fleetml.env`, so every later compose command keeps it) and
  restarts the server with it, in the stack's clone:

  ```bash
  . ~/.fleetml.env && sed -i.bak "s/S3_PUBLIC_ENDPOINT=[^ ]*/S3_PUBLIC_ENDPOINT=<server address>:${FLEETML_S3_PUBLIC_PORT:-19100}/" ~/.fleetml.env && rm ~/.fleetml.env.bak
  source ~/.fleetml.env
  docker compose -f deploy/docker-compose.prod.yml up -d
  ```

  On the server's machine, this shows the value in use:

  ```bash
  docker inspect fleetml-prod-server-1 --format '{{range .Config.Env}}{{println .}}{{end}}' | grep S3_PUBLIC_ENDPOINT
  ```

- **On this machine**, when the server stays as it is: `download_host_override`
  in the agent's config sends every download, full model or delta, to an
  address of the server's object store that this machine reaches, with the
  URL's path and signature unchanged (the request keeps the original host in
  its `Host` header, which the signature covers). The example config ships it
  commented out; together with the two settings every machine needs:

  ```bash
  sudo sed -i -e 's|^server: .*|server: <server address>:<gRPC port>|' \
    -e 's|^# download_host_override: .*|download_host_override: "<server address>:<model files port>"|' \
    -e '/^enrollment_key:/d' /etc/fleetml-agent/config.yaml
  read -rs EK
  printf 'enrollment_key: "%s"\n' "$EK" | sudo tee -a /etc/fleetml-agent/config.yaml > /dev/null
  ```

  `read -rs` waits for the enrollment key (paste it and press Enter; it is
  not shown), and `printf`, a shell builtin, hands it to `tee` on its input,
  so the key is on no command line and in no shell history. At start the
  agent then logs
  `level=INFO msg="model downloads go to download_host_override" host=<server address>:<model files port>`,
  and the next heartbeat's download succeeds. With the override set, the URL
  in a `download failed` line shows the override's host and port, so a
  `connection refused` there means a wrong override port, not a wrong
  `S3_PUBLIC_ENDPOINT`.

**The symptom** of an unreachable download address: the agent registers and
heartbeats normally, and the server reports the deployment `COMPLETE`, which
means assigned, not delivered. Only the agent's log shows the failure, on
every heartbeat:

```text
level=WARN msg="download failed" model=iris version=1.0.0 error="downloading: Get \"http://localhost:19100/fleetml-models/models/<project>/iris/1.0.0/INT8_TFLITE.bin\": dial tcp [::1]:19100: connect: connection refused"
```

**From the CLI**, `fleetml deploy get <deployment ID> | jq -c .devices`
counts the devices that report the model loaded (`{"loaded":0,"targeted":1}`
while downloads fail), and `fleetml device get <device ID> | jq .last_model_event`
shows the agent's `MODEL_LOAD_FAILED` with the error once the agent's next
telemetry flush has sent it (30 seconds by default). `loaded` counts the
devices whose last heartbeat reported the version, so it turns to 1 one
heartbeat (60 seconds by default) after the agent logs `model active`; it is
counted each time you ask, so a deployment that a newer one replaced counts
down to 0 as its devices move on.

A server in production sits behind a TLS proxy with a real host name: then
`tls: true`, `server: <host name>:443` (or the proxy's gRPC port) and an
`S3_PUBLIC_ENDPOINT` on the proxy too, and no override
([CONFIG.md](CONFIG.md#docker-compose) has the compose settings).

On Windows, the agent can run in the same WSL 2 Ubuntu as the quickstart's
stack, where `server: localhost:19090` and the stack's default
`S3_PUBLIC_ENDPOINT` should work as they are, since Docker Desktop publishes
19090 and 19100 there too; current Ubuntu images for WSL run systemd, and
without it `install.sh` takes the [no-systemd path](LINUX_AGENT.md#without-systemd-and-in-containers).
None of this is verified on Windows by CI, which runs Docker Engine inside
the distribution rather than Docker Desktop.

## Kubernetes instead of compose

On Kubernetes, the [Helm chart](../deploy/helm/fleetml) installs the same
server instead, and the tour runs against it: keep `FLEETML_SERVER` and
`FLEETML_API_KEY` from the chart README's Get started instead of the tour's
first `export` line, and add `--grpc localhost:$GRPC` to the tour's
`fleetml simulate` lines in place of theirs. In a second shell, set the
chart's `REST GRPC S3` line and its `export` line again instead of the
tour's [second-shell block](QUICKSTART.md#tour). The tour's
`>> ~/.fleetml.env` lines then add its own variables (the project and
version IDs, the enrollment key) to that file, next to a compose stack's
secrets if one runs on the same machine: they change nothing on that stack,
and the compose [Clean up](QUICKSTART.md#clean-up) removes them with the
file.

## Helm chart variants

The [chart README](../deploy/helm/fleetml/README.md)'s Get started installs
the chart with a secrets file, its bundled services and local ports 38080,
39090 and 39100 for REST, gRPC and the object store. The variants:

**A Secret of your own instead of the file.** A Secret of your own with the keys `JWT_SECRET`,
`SIGNING_MASTER_KEY`, `FLEETML_ADMIN_KEY`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`
and `POSTGRES_PASSWORD` works the same way: `--set secrets.existingSecret=<name>`,
except that a change to your Secret rolls nothing: after one, run
`kubectl -n fleetml rollout restart statefulset/fleetml-s3 deploy/fleetml-server`.

**Other local ports.** If something on this machine already listens on one
of the three ports (`lsof -nP -iTCP:<port> -sTCP:LISTEN` prints it), change
it in step 4's first line (`REST=... GRPC=... S3=...`) before the install,
and every later line follows. Check with `lsof` rather than waiting for an
error: kubectl reports `address already in use` only against another
kubectl, and on macOS a port-forward on a port that Docker Desktop
publishes starts without a word and takes `localhost` from the container.

**External services and TLS**: the chart README's
[Production](../deploy/helm/fleetml/README.md#production).

**Images that are not released yet**: the end of the chart README's
[Values](../deploy/helm/fleetml/README.md#values).

## Upgrading

Every release is in the [CHANGELOG](../CHANGELOG.md). From any earlier
release the block below goes straight to the newest; read each version's
"Changed" section in between. From 0.6.0 to 0.8.0
there are no breaking API changes; 0.9.0 answers a few refused requests
with other error codes (`409 INVALID_TRANSITION` for experiments and
versions, `400 VALIDATION_ERROR` for a `limit` that is not a number) and
makes the Python client's `project`, `limit` and `offset` keyword-only;
0.9.1 answers a `limit` above the page cap with `400 VALIDATION_ERROR` and
counts `DEPENDENCY_TIMEOUT` for the whole start-up; 0.10.0 makes the
Python client's `TelemetryEvent.confidence_score` `None` where no
confidence was reported, and its `push(promote=True)` raise when a
promotion is refused (each version's
"Changed" section lists what behaves differently and what to change), and
the server applies new database
migrations itself when it starts. Upgrade the server first, then the
devices and the tools; each moves on its own:

- **The compose stack**, in its clone, with the saved values. A port moved
  with an override file (`dashboard-port.yml`, the way 0.9.0's README
  showed) goes into `~/.fleetml.env` first, as in [Other ports](#other-ports),
  and the file stays out of the commands below, or `up` fails with
  `Bind for 0.0.0.0:<port> failed`:

  ```bash
  git fetch --tags && git checkout v0.10.0
  source ~/.fleetml.env
  grep -q S3_PUBLIC_ENDPOINT ~/.fleetml.env || docker inspect $(docker compose -f deploy/docker-compose.prod.yml ps -q server) --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^S3_PUBLIC_ENDPOINT=/export S3_PUBLIC_ENDPOINT=/p' >> ~/.fleetml.env
  source ~/.fleetml.env
  docker compose -f deploy/docker-compose.prod.yml up -d --build
  until curl -sf localhost:${FLEETML_REST_PORT:-18080}/readyz; do sleep 2; done; echo
  ```

  The `grep` line matters for a `~/.fleetml.env` written by 0.7.0 or
  earlier, which holds the three secrets only: the device guides of those
  releases set `S3_PUBLIC_ENDPOINT` on the command line, so without it the
  upgraded server would hand out `localhost:19100` and phones and agents
  elsewhere would stop receiving models. It copies the running server's
  value into the file once and does nothing when the file has one; it asks
  compose for the stack's server container, so it also works with your own
  `COMPOSE_PROJECT_NAME`. The
  last line prints `/readyz`, which ends with the new version
  (`"version":"0.10.0"`). The projects, devices, telemetry and the admin key
  stay, and devices reconnect by themselves. `git checkout main` instead follows the newest
  code, ahead of the last release.
- **The CLI:** the [Get the CLI](QUICKSTART.md#get-the-cli) block again (it fetches the
  latest release); `fleetml version` prints the version.
- **An Android app:** the new version in
  `implementation("dev.fleetml:fleetml-sdk:<version>")` ([Install](ANDROID_SDK.md#install)).
- **An iOS app:** Xcode's File, Packages, Update to Latest Package Versions
  (the rule "Up to Next Major Version" takes every 0.x release), or
  `from: "<version>"` in `Package.swift` ([Install](IOS_SDK.md#install)).
- **The Python client:** `pip install -U fleetml`.
- **A Linux agent:** the new archive and `install.sh` again
  ([Upgrade](LINUX_AGENT.md#upgrade)): it keeps the config and restarts the
  running service.
- **The Helm chart:** [Upgrading](../deploy/helm/fleetml/README.md#upgrading).
