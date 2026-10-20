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

`sha256sum -c` prints `fleetml-agent-linux-amd64.tar.gz: OK` (or `arm64`), and
`fleetml-agent -version` prints the release without a `v`, for example
`fleetml-agent 0.5.0`. To build the agent from source instead, `make
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

### Connect it to the server

The agent registers into a project with one of the project's enrollment keys.
Create one with the CLI, wherever the CLI talks to the server with the admin
key (as in the [tour](QUICKSTART.md#2-a-fleet-and-a-rollout)); it prints the
key once:

```bash
fleetml enrollment-key create --project <project ID> --name edge
```

Then, on the machine, set the settings and start the service:

```bash
sudoedit /etc/fleetml-agent/config.yaml
sudo systemctl enable --now fleetml-agent
journalctl -u fleetml-agent -f
```

In `config.yaml`, set `server` (the server's host and gRPC port, `19090`),
`enrollment_key`, and `tls`: `false` for the quickstart's compose server,
which speaks plain gRPC on 19090, or `true` when the server is behind a TLS
proxy. Set `labels` too: deployments and experiments reach the machine through
them (a `target_selector` such as `{"site":"factory-1"}` matches every device
whose labels include it; see [ROLLOUTS.md](ROLLOUTS.md#how-a-device-gets-a-model)).

The journal then shows the agent start and register (the time stamps left out
here):

```text
level=INFO msg="fleetml-agent starting" version=0.5.0 server=fleetml.example.com:19090 models_dir=/var/lib/fleetml-agent/models
level=INFO msg=registered device_id=<device ID>
```

and the machine appears in `fleetml device list --project <project ID>` with
OS `LINUX`. Once a deployment targets its labels, the next heartbeat (every
30 seconds) brings the model:

```text
level=INFO msg="model downloaded" model=iris version=1.0.0 bytes=<size>
level=INFO msg="model active" model=iris version=1.0.0 path=/var/lib/fleetml-agent/models/iris/current
```

`/var/lib/fleetml-agent/models/iris/current` then links to the active
version's folder ([Where models go](#where-models-go)). A
`transport: authentication handshake failed` error means `tls: true` against a
server without TLS; `registration failed` with `Unauthenticated` means the
enrollment key is wrong or revoked.

### Server on another machine

The quickstart's compose stack signs model download URLs for
`localhost:19100`, which only the server's own machine can reach: an agent on
another machine registers and heartbeats, and then every download fails. Start
the stack with the server's LAN address as the object store's public endpoint,
and let the agent's machine reach ports 19090 (gRPC) and 19100 (model files):

```bash
S3_PUBLIC_ENDPOINT=<server LAN IP>:19100 docker compose -f deploy/docker-compose.prod.yml up -d
```

and set `server: <server LAN IP>:19090` in the agent's config.
[CONFIG.md](CONFIG.md#docker-compose) has the compose settings; a server in
production sits behind a TLS proxy with a real host name instead.

## Configure

Settings come from the YAML file (`-config`), and flags override them
(`-server`, `-enrollment-key`, `-label key=value`, `-state-dir`,
`-models-dir`, `-validator`, `-post-activate`, `-telemetry-listen`).
`FLEETML_ENROLLMENT_KEY` in the environment works too.

| Setting | Default | Purpose |
|---|---|---|
| `server` | (required) | FleetML gRPC address, `host:port` |
| `tls` | `false` | TLS for gRPC (use it whenever the server is not on a private network) |
| `enrollment_key` | | Project enrollment key for the first registration |
| `labels` | `{}` | Labels deployments and experiments target |
| `state_dir` | `/var/lib/fleetml-agent` | Registration, telemetry queue, partial downloads |
| `models_dir` | `<state_dir>/models` | Where models are placed (below). The unit runs with `ProtectSystem=strict`, so a folder outside `/var/lib/fleetml-agent` must be added to the unit's `ReadWritePaths=` (a commented line in the unit) |
| `validator` | none | Shell command that checks a downloaded model; `{file}` is its path; exit code 0 means valid |
| `post_activate` | none | Shell command run after a version becomes active |
| `telemetry_listen` | off | Local endpoint for inference events, e.g. `127.0.0.1:9601` |
| `metered`, `on_battery` | `false` | Describe the machine's connection and power for the download policy |
| `heartbeat_interval` | `30s` | Until the server sends its own (`DEVICE_HEARTBEAT_INTERVAL`) |

The server's device settings apply as for the SDKs
([CONFIG.md](CONFIG.md#devices)); the agent runs continuously, so it always
uses the foreground heartbeat interval.

## Where models go

```
/var/lib/fleetml-agent/models/
  detector/
    current -> 2.0.0-1a2b3c4d          # the active version, switched atomically
    2.0.0-1a2b3c4d/model.bin
    1.0.0-9f8e7d6c/model.bin           # last-known-good, kept for rollback
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
   downloaded and applied ([delta updates](ROLLOUTS.md#delta-updates));
3. runs the `validator` command, if any, with `FLEETML_MODEL_FILE`,
   `FLEETML_MODEL_NAME` and `FLEETML_MODEL_VERSION` in its environment;
4. if it passes, switches the `current` symlink, keeps the previous version as
   last-known-good, reports `MODEL_LOAD`, and runs `post_activate` (with
   `FLEETML_MODEL_PATH` set to the `current` link);
5. if it fails, leaves `current` alone, reports `MODEL_LOAD_FAILED` with the
   command's output (so a canary's health gate can roll the release back), and
   does not retry that variant for an hour.

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
active models and their paths.

`POST /v1/events` takes one event or an array; set `"error"` to a message for
a failed inference (it counts against the health gate), and `"features"` to
the input values by feature name (`{"petal_length": [5.1]}`) for
[feature-level drift](ROLLOUTS.md#feature-level-drift): the agent bins them
with the edges the server sent and reports only the counts. Events are attributed
to the model's active version and experiment arm, queued on disk (up to 10,000;
oldest dropped first) and sent in batches. Keep the endpoint on `127.0.0.1`.

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
hook, local endpoint). The runtime check ran two `linux/amd64` agents in
Debian containers next to the dev stack: a 50% canary reached exactly one of
them, a failing validator on that one kept the last-known-good version and
reported `MODEL_LOAD_FAILED`, and completing the rollout moved both `current`
links and ran the hook. `./scripts/signing-e2e.sh agent` checks signatures:
a model whose stored signature was overwritten is rejected, and one signed
with the previous key is accepted during the rotation's grace period and
rejected after it.
