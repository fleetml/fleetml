# Linux device agent

`fleetml-agent` puts a Linux machine (a Jetson, a Raspberry Pi, an x86 edge
box, a kiosk) under FleetML's control. It is a single static binary that
speaks the same device protocol as the [Android](ANDROID_SDK.md) and
[iOS](IOS_SDK.md) SDKs and follows the same rules: it registers with an
enrollment key, downloads the models the server assigns (resuming interrupted
downloads and checking the sha256), validates each one before switching to it,
keeps the last-known-good version, and reports telemetry. It does not run your
models: your inference process loads them from disk.

## Try it locally

```bash
./scripts/demo-agent.sh
```

starts a server on the dev stack and four agents on this machine (the agent
also builds for macOS, for local testing), each with a stand-in inference
process that posts to its local endpoint. A canary of a model that fails at
inference trips the health gate and rolls back, and the canary boxes switch
their `current` link back to the last-known-good version. It is the recording
at the top of the [README](../README.md).

## Install

Every [GitHub release](https://github.com/fleetml/fleetml/releases) has the
agent for Linux as `fleetml-agent-linux-amd64.tar.gz` and
`fleetml-agent-linux-arm64.tar.gz`, listed in `SHA256SUMS`. Each archive holds
the static binary `fleetml-agent`, the installer `install.sh`,
`config.example.yaml` and `fleetml-agent.service`. On the machine, the latest
release, checked, then installed as root:

```bash
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
curl -fsSLO "https://github.com/fleetml/fleetml/releases/latest/download/fleetml-agent-linux-$ARCH.tar.gz"
curl -fsSLO https://github.com/fleetml/fleetml/releases/latest/download/SHA256SUMS
grep " fleetml-agent-linux-$ARCH.tar.gz\$" SHA256SUMS | sha256sum -c
mkdir fleetml-agent && tar -xzf "fleetml-agent-linux-$ARCH.tar.gz" -C fleetml-agent
sudo ./fleetml-agent/install.sh ./fleetml-agent/fleetml-agent
fleetml-agent -version
```

On a minimal image without `curl`, `wget -q` with the same URLs downloads
the two files, or copy them over from another machine; in a container's root
shell without `sudo`, leave `sudo` out. `sha256sum -c` prints
`fleetml-agent-linux-amd64.tar.gz: OK` (or `arm64`), and
`fleetml-agent -version` prints the release without a `v`, for example
`fleetml-agent 0.6.0`. To build the agent from source instead, `make
build-agent` in a clone writes `bin/fleetml-agent-linux-amd64`, `-linux-arm64`
and `-darwin-arm64` (version `dev`), and
`sudo ./deploy/agent/install.sh ./bin/fleetml-agent-linux-arm64` installs one
from the clone.

The script installs the binary to `/usr/local/bin`, creates a
`fleetml-agent` system user, the config file (from
[`config.example.yaml`](../deploy/agent/config.example.yaml)) and the
[systemd unit](../deploy/agent/fleetml-agent.service), which keeps its state in
`/var/lib/fleetml-agent`. It starts nothing while the config still holds the
placeholder key.

### Without systemd

On a machine where systemd is not running (a container, also one whose image
carries the systemd package, a chroot, a BusyBox or Yocto image, a kiosk with
its own init; the script checks for `/run/systemd/system`, which only a
running systemd creates), `install.sh` installs the binary, the user and the config
the same way, skips the unit, creates `/var/lib/fleetml-agent` owned by
`fleetml-agent` (mode `0755`; the unit's `StateDirectory=` does this under
systemd) and says so. Once the config is set, run the agent as that user under
the machine's own supervisor (runit, s6, OpenRC, supervisord, a container's
entrypoint), with the same command the unit runs:

```bash
su -s /bin/sh fleetml-agent -c '/usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml'
```

It logs to standard error, which is where the supervisor's log picks it up;
the journal lines below are the same. The supervisor restarts it if it exits,
as `Restart=always` does in the unit.

In a container, make the agent the main process (`exec` it from the
entrypoint script) or run the container with an init that reaps processes,
such as `docker run --init` or `tini`/`dumb-init` as the entrypoint. Started
in the background under a placeholder such as `sleep infinity`, an agent
that exits leaves its `su` behind as a `<defunct>` process, because the
placeholder does not collect it.

### Connect it to the server

> **First, on the server: the address models are downloaded from.** The
> server hands out each model as a download URL for its object store's public
> address, `S3_PUBLIC_ENDPOINT`, which is `localhost:19100` in the
> quickstart's compose stack: only the server's own machine reaches it. For an
> agent on another machine, whoever runs the server starts the stack with the
> server's LAN address there, and the agent's machine must reach ports 19090
> (gRPC) and 19100 (model files):
>
> ```bash
> S3_PUBLIC_ENDPOINT=<server LAN IP>:19100 docker compose -f deploy/docker-compose.prod.yml up -d
> ```
>
> On the server's machine,
> `docker inspect fleetml-prod-server-1 --format '{{range .Config.Env}}{{println .}}{{end}}' | grep S3_PUBLIC_ENDPOINT`
> shows the value in use. Without a reachable address the agent registers and
> heartbeats normally, and the server reports the deployment `COMPLETE`, which
> means assigned, not delivered; only the agent's log shows the failure, a
> `download failed` line on every heartbeat. From the CLI,
> `fleetml deploy get <deployment ID> | jq -c .devices` counts the devices
> that report the model loaded (`{"loaded":0,"targeted":1}` while downloads
> fail), and `fleetml device get <device ID> | jq .last_model_event` shows the
> agent's `MODEL_LOAD_FAILED` with the error once the agent's next telemetry
> flush has sent it (30 seconds by default). `loaded` counts the devices whose
> last heartbeat reported the version, so it turns to 1 one heartbeat (60
> seconds by default) after the agent logs `model active`; it is counted each
> time you ask, so a deployment that a newer one replaced counts down to 0 as
> its devices move on. A server in production sits behind a TLS proxy with a
> real host name instead ([CONFIG.md](CONFIG.md#docker-compose) has the compose
> settings).
>
> **If you cannot restart the server**, fix it on the agent's side: set
> `download_host_override` in the agent's config to an address of the
> server's object store that this machine reaches, for example
> `download_host_override: <server LAN IP>:19100`. The agent then sends every
> download, full model or delta, to that address with the URL's path and
> signature unchanged (the request keeps the original host in its `Host`
> header, which the signature covers), logs
> `level=INFO msg="model downloads go to download_host_override" host=<server LAN IP>:19100`
> at start, and the next heartbeat's download succeeds.

The agent registers into a project with one of the project's enrollment keys,
and a deployment reaches it through its labels. From the root of a clone (for
the sample model), wherever the CLI talks to the server with the admin key (as
in the [tour](QUICKSTART.md#tour)), this creates a project, publishes a model,
creates an enrollment key, which it prints once, and deploys the model to the
labels the example config sets (`site: factory-1`):

```bash
P=$(fleetml project create --name edge -o json | jq -r .id)
fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
M=$(fleetml model list --project $P -o json | jq -r '.[] | select(.name == "iris") | .id')
vid() { fleetml ver list --model $M -o json | jq -r --arg v "$1" '.[] | select(.version == $v) | .id'; }
fleetml enrollment-key create --project $P --name edge
fleetml deploy start $(fleetml deploy create --project $P --version $(vid 1.0.0) --target '{"site":"factory-1"}' -o json | jq -r .id)
```

`vid` looks a version up by its number, so the block picks 1.0.0 even after
more versions are pushed (`fleetml ver list` lists the newest first). The
deployment starts as `COMPLETE` (`✓ Deployment start → COMPLETE`): it waits
for the machine. Then, on the machine, set `server` (the server's host and
gRPC port, `19090`; `<server LAN IP>:19090` for a server on another machine)
and `enrollment_key` in the config, and start the service:

```bash
sudo sed -i -e 's|^server: .*|server: <server LAN IP>:19090|' -e 's|^enrollment_key: .*|enrollment_key: "<enrollment key>"|' /etc/fleetml-agent/config.yaml
sudo systemctl enable --now fleetml-agent
journalctl -u fleetml-agent -f
```

`sed` is on every image, BusyBox included; with an editor, `sudo vi
/etc/fleetml-agent/config.yaml` does the same. Leave `tls: false` for the
quickstart's compose server, which speaks plain gRPC on 19090, and set it to
`true` when the server is behind a TLS proxy. Set `labels` to what your
deployments target (the example's `site: factory-1` matches the block above):
a `target_selector` such as `{"site":"factory-1"}` matches every device whose
labels include it ([ROLLOUTS.md](ROLLOUTS.md#how-a-device-gets-a-model)).

The journal then shows the agent start and register (the time stamps left out
here):

```text
level=INFO msg="fleetml-agent starting" version=0.6.0 server=fleetml.example.com:19090 models_dir=/var/lib/fleetml-agent/models
level=INFO msg=registered device_id=<device ID>
```

and the machine appears in `fleetml device list --project <project ID>` with
OS `LINUX`. A deployment that already targets its labels arrives right after
registration; one created later arrives with the next heartbeat (60 seconds
with the server's default `DEVICE_HEARTBEAT_INTERVAL`; the agent's own `30s`
applies only until the server answers):

```text
level=INFO msg="model downloaded" model=iris version=1.0.0 bytes=<size>
level=INFO msg="model active" model=iris version=1.0.0 path=/var/lib/fleetml-agent/models/iris/current
```

`/var/lib/fleetml-agent/models/iris/current` then links to the active
version's folder ([Where models go](#where-models-go)). A second version,
pushed and deployed to the same labels in the same shell as the first block:

```bash
fleetml push models/iris/iris_v2.tflite --project $P --model iris --version 2.0.0
fleetml deploy start $(fleetml deploy create --project $P --version $(vid 2.0.0) --target '{"site":"factory-1"}' -o json | jq -r .id)
```

arrives with the next heartbeat, usually as a delta from the first:

```text
level=INFO msg="model built from a delta" model=iris version=2.0.0 delta_bytes=<size> model_bytes=<size>
level=INFO msg="model active" model=iris version=2.0.0 path=/var/lib/fleetml-agent/models/iris/current
```

After a restart, the agent keeps its registration and logs the model it has
instead of registering again:

```text
level=INFO msg="active model (from the last run)" model=iris version=2.0.0 path=/var/lib/fleetml-agent/models/iris/current
```

With `telemetry_listen` set, the start also logs
`level=INFO msg="local telemetry endpoint listening" addr=127.0.0.1:9601`. A
version that the `validator` rejects is logged as

```text
level=WARN msg="model failed validation; keeping the current version" model=iris version=3.0.0 error="exit status 1: not a tflite file" rejected_file=/var/lib/fleetml-agent/rejected/iris/3.0.0-<variant>/model.bin
```

with the end of the command's output in `error` ([Where models go](#where-models-go), step 5).

A `transport: authentication handshake failed` error means `tls: true` against
a server without TLS; `registration failed` with `Unauthenticated` means the
enrollment key is wrong or revoked; `download failed` with
`connection refused` and `localhost:19100` in the URL means the server's
`S3_PUBLIC_ENDPOINT` (above; `download_host_override` is the agent-side fix).

On Windows, the agent can run in the same WSL 2 Ubuntu as the quickstart's
stack: Docker Desktop publishes 19090 and 19100 there too, so
`server: localhost:19090` and the stack's default `S3_PUBLIC_ENDPOINT` work
as they are. Current Ubuntu images for WSL run systemd; without it,
`install.sh` takes the [Without systemd](#without-systemd) path (not verified
on Windows by CI).

## Configure

Settings come from the YAML file (`-config`), and flags override them
(`-server`, `-enrollment-key`, `-label key=value`, `-state-dir`,
`-models-dir`, `-validator`, `-post-activate`, `-telemetry-listen`).
`FLEETML_ENROLLMENT_KEY` in the environment works too. `-retry-failed`
retries the versions that failed validation right away instead of after the
hour's wait ([Where models go](#where-models-go), step 5).

| Setting | Default | Purpose |
|---|---|---|
| `server` | (required) | FleetML gRPC address, `host:port` |
| `tls` | `false` | TLS for gRPC (use it whenever the server is not on a private network) |
| `enrollment_key` | | Project enrollment key for the first registration |
| `labels` | `{}` | Labels deployments and experiments target |
| `state_dir` | `/var/lib/fleetml-agent` | Registration, telemetry queue, partial downloads |
| `models_dir` | `<state_dir>/models` | Where models are placed (below). The unit runs with `ProtectSystem=strict`, so a folder outside `/var/lib/fleetml-agent` must be added to the unit's `ReadWritePaths=` (a commented line in the unit) |
| `validator` | none | Shell command that checks a downloaded model; exit code 0 means valid. `{file}` in the command is replaced by the model's path, and the `FLEETML_MODEL_FILE`, `FLEETML_MODEL_NAME` and `FLEETML_MODEL_VERSION` environment variables are set too; use either |
| `post_activate` | none | Shell command run after a version becomes active |
| `telemetry_listen` | off | Local endpoint for inference events, e.g. `127.0.0.1:9601` |
| `metered`, `on_battery` | `false` | Describe the machine's connection and power for the download policy |
| `heartbeat_interval` | `30s` | Until the server sends its own (`DEVICE_HEARTBEAT_INTERVAL`, default 60 s) |
| `download_host_override` | none | `host:port` that every model download goes to instead of the URL's host, for a server whose `S3_PUBLIC_ENDPOINT` this machine cannot reach ([Connect it to the server](#connect-it-to-the-server)) |

The server's device settings apply as for the SDKs
([CONFIG.md](CONFIG.md#devices)); the agent runs continuously, so it always
uses the foreground heartbeat interval.

## Where models go

```
/var/lib/fleetml-agent/
  state.json, telemetry.queue          # 0600: registration, failures, queued events
  downloads/                           # 0700: partial downloads
  rejected/detector/3.0.0-5e6f7a8b/model.bin   # 0700 folders, 0600 file: the last version that failed validation
  models/
    detector/
      current -> 2.0.0-1a2b3c4d        # the active version, switched atomically
      2.0.0-1a2b3c4d/model.bin
      1.0.0-9f8e7d6c/model.bin         # last-known-good, kept for rollback
```

Point your inference process at `models/<model>/current/model.bin`. Only the
active and last-known-good versions are kept. Each `model.bin` is mode `0644`
and the folders above it `0755`, owned by `fleetml-agent`, the same for a full
download and a version built from a delta, so an inference process running as
any user reads the model without joining the `fleetml-agent` group. Partial
downloads in `state_dir/downloads` stay private (`0700`) until they are
complete and verified.

For each assignment the agent:

1. checks the assignment's signature with the project's signing keys and, if
   it does not verify, reports `SIGNATURE_INVALID` and downloads nothing
   ([artifact signing](ROLLOUTS.md#artifact-signing));
2. downloads to `state_dir/downloads/<variant>.part`, resuming with an HTTP
   `Range` request after an interruption, and checks the sha256; when the
   server offers a delta from the version the agent has, only the delta is
   downloaded and applied ([delta updates](ROLLOUTS.md#delta-updates)); a
   download that fails is retried on every heartbeat and reported once as
   `MODEL_LOAD_FAILED` with the error, which `fleetml device get` shows as the
   device's `last_model_event`;
3. runs the `validator` command, if any, with `FLEETML_MODEL_FILE`,
   `FLEETML_MODEL_NAME` and `FLEETML_MODEL_VERSION` in its environment;
4. if it passes, switches the `current` symlink, keeps the previous version as
   last-known-good, reports `MODEL_LOAD`, and runs `post_activate` (with
   `FLEETML_MODEL_PATH` set to the `current` link);
5. if it fails, leaves `current` alone, reports `MODEL_LOAD_FAILED` with the
   command's output (so a canary's health gate can roll the release back),
   moves the file to `state_dir/rejected/<model>/<version>-<variant>/model.bin`
   (the last rejected version of each model is kept there, readable only by
   `fleetml-agent`), and does not retry that variant for an hour, a wait it
   keeps in `state.json` across restarts.

While you work on a validator, run it against the rejected file by hand, as
the agent's user, and see its exit code:

```bash
sudo -u fleetml-agent sh -c 'python3 /opt/inference/check_model.py /var/lib/fleetml-agent/rejected/iris/3.0.0-<variant>/model.bin; echo "exit $?"'
```

Once it passes, start the agent once with `-retry-failed`, which clears the
saved failures and retries them on its first heartbeat, then go back to the
service:

```bash
sudo systemctl stop fleetml-agent
sudo -u fleetml-agent /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml -retry-failed
sudo systemctl start fleetml-agent
```

The foreground run logs
`level=INFO msg="cleared the failed versions; they are retried on the next heartbeat" count=1`,
and its first heartbeat, right away, retries the version
(`level=INFO msg="model active" model=iris version=3.0.0 ...` once the
validator passes); stop it with Ctrl-C then. Without systemd, stop the agent under its supervisor and run the
`su` command from [Without systemd](#without-systemd) with `-retry-failed`
added in the same way. A restart without the flag keeps waiting out the
hour.

A validator can be anything that loads the model the way production does, for
example a Python script that runs one inference with ONNX Runtime on a known
input. There is no built-in ONNX or TFLite validator: the agent is a static Go
binary without those runtimes, and the command validator lets you use exactly
the runtime and version your inference process uses.

To reload the inference process after an update, either use `post_activate`
(e.g. `touch /run/inference/reload`) or a systemd path unit that watches the
`current` link. The agent runs unprivileged, so restarting another service
from `post_activate` needs a sudo or polkit rule.

## Telemetry from the inference process

With `telemetry_listen` set, the inference process (Python, C++, anything)
reports inferences over HTTP on the machine, without a client library:

```bash
curl -s -X POST localhost:9601/v1/events -H 'Content-Type: application/json' \
  -d '{"model":"detector","latency_ms":12,"confidence":0.93,"label":"person"}'
curl -s localhost:9601/v1/models
```

The first call answers `{"accepted":1,"rejected":0}`; the second lists the
active models with the same `model` key and their `current` links:

```json
[{"model":"detector","version":"2.0.0","version_id":"<version ID>","path":"/var/lib/fleetml-agent/models/detector/current"}]
```

`POST /v1/events` takes one event or an array; set `"error"` to a message for
a failed inference (it counts against the health gate), and `"features"` to
the input values by feature name (`{"petal_length": [5.1]}`) for
[feature-level drift](ROLLOUTS.md#feature-level-drift): the agent bins them
with the edges the server sent and reports only the counts. Events are attributed
to the model's active version and experiment arm, queued on disk (up to 10,000;
oldest dropped first) and sent in batches. Keep the endpoint on `127.0.0.1`.

An event the agent cannot attribute is rejected, and the answer says why, one
reason per rejected event (the first one is also logged at INFO):

```json
{"accepted":0,"rejected":1,"rejected_reasons":["event 0: unknown model \"foo\" (active models: detector)"]}
```

On the server, wherever the CLI reaches it, the events arrive with the next
telemetry flush (30 seconds by default):
`fleetml telemetry summary <version ID>` counts a version's events (the
`version_id` from `/v1/models`), and `fleetml telemetry events --device <device ID>`
lists one machine's events (type, latency, confidence, error). To narrow
`events` to a version, the flag is `--model-version <version ID>`.

## Identity and offline behaviour

The fingerprint is the sha256 of the machine ID (`/etc/machine-id`) and a
random salt stored in the state file, so it is stable across restarts, two
installs on one machine are separate devices, and the machine ID itself is
not sent. The device token is refreshed before it expires, and the agent
registers again if the server rejects it.

When the server is unreachable the agent keeps the models it has: `current`
links stay in place, telemetry stays queued, and heartbeats resume when the
server is back.

## Verified

`go test ./internal/agent` covers the queue, state, fingerprint, download
resume, the validator command and the install path (symlink, last-known-good,
hook, local endpoint and its rejection reasons, file modes, the rejected file
and `-retry-failed`, `download_host_override`), and the e2e test
`TestE2E_AgentDownloadHostOverride` downloads through the override from a
real SeaweedFS that refuses a plain host rewrite. The agent ran in an Ubuntu
24.04 container with systemd against the quickstart's stack started without
`S3_PUBLIC_ENDPOINT`: the download failed as described, then
`download_host_override` with the Mac's LAN address brought 1.0.0 and the
2.0.0 delta; a failing validator kept 2.0.0 and the rejected file, a restart
did not retry it, and `-retry-failed` as above activated it. `./scripts/agent-install-test.sh` runs
`install.sh` in Ubuntu containers with and without systemd. The runtime check ran two `linux/amd64` agents in
Debian containers next to the dev stack: a 50% canary reached exactly one of
them, a failing validator on that one kept the last-known-good version and
reported `MODEL_LOAD_FAILED`, and completing the rollout moved both `current`
links and ran the hook. `./scripts/signing-e2e.sh agent` checks signatures:
a model whose stored signature was overwritten is rejected, and one signed
with the previous key is accepted during the rotation's grace period and
rejected after it.
