# Linux device agent

`fleetml-agent` puts a Linux machine (a Jetson, a Raspberry Pi, an x86 edge
box, a kiosk) under FleetML's control. It is a single static binary that
speaks the same device protocol as the [Android](ANDROID_SDK.md) and
[iOS](IOS_SDK.md) SDKs and follows the same rules: it registers with an
enrollment key, downloads the models the server assigns (resuming interrupted
downloads and checking the sha256), validates each one before switching to it,
keeps the last-known-good version, and reports telemetry. It does not run your
models: your inference process loads them from disk.

This guide starts with the agent in a container on the machine that runs
the server ([Get started](#get-started)), then installs it on a real machine
as a service, connects it, deploys a model to it and checks that it arrived.
Running everything on one laptop with a demo fleet is at the end, in
[Try it locally](#try-it-locally).

## Get started

The agent in a container on this machine, against a server on this machine:
ten minutes, copy and paste, nothing to choose. Each step says what you should
see; if you see something else, stop there and look it up in
[Troubleshooting](ADVANCED.md#troubleshooting). Use one terminal (bash or
zsh, on macOS or Linux; on Windows, see [On Windows](QUICKSTART.md#on-windows))
and keep it open until the end.

1. **Check the tools.** You need Docker (Docker Desktop on a Mac), git, curl
   and jq ([installing them](ADVANCED.md#installing-the-tools)):

   <!-- golden: tools -->
   ```bash
   docker compose version && git --version && jq --version
   ```

   You should see `Docker Compose version v2` and `jq-` on the last line.

2. **Start FleetML.** The first start takes a few minutes, to download and
   build the images:

   <!-- golden: start -->
   ```bash
   [ -f deploy/docker-compose.prod.yml ] || { [ -d fleetml ] || git clone https://github.com/fleetml/fleetml.git; cd fleetml; }
   [ -f ~/.fleetml.env ] || (umask 077; printf "export JWT_SECRET='%s' SIGNING_MASTER_KEY='%s' FLEETML_ADMIN_KEY='%s' FLEETML_REST_PORT=%s FLEETML_GRPC_PORT=%s FLEETML_DASHBOARD_PORT=%s FLEETML_S3_PUBLIC_PORT=%s S3_PUBLIC_ENDPOINT=localhost:%s\n" "$(openssl rand -base64 32)" "$(openssl rand -base64 32)" "$(openssl rand -hex 32)" "${FLEETML_REST_PORT:-18080}" "${FLEETML_GRPC_PORT:-19090}" "${FLEETML_DASHBOARD_PORT:-80}" "${FLEETML_S3_PUBLIC_PORT:-19100}" "${FLEETML_S3_PUBLIC_PORT:-19100}" > ~/.fleetml.env)
   source ~/.fleetml.env
   docker compose -f deploy/docker-compose.prod.yml up -d --build && for i in $(seq 90); do curl -sf localhost:${FLEETML_REST_PORT:-18080}/readyz && break; [ $i = 90 ] && echo "Not ready after 3 minutes: see Troubleshooting in docs/ADVANCED.md"; sleep 2; done || echo "Start failed: see Troubleshooting in docs/ADVANCED.md"; echo
   curl -sf -H "Authorization: Bearer $FLEETML_ADMIN_KEY" localhost:${FLEETML_REST_PORT:-18080}/api/v1/projects && echo && echo "Dashboard: http://localhost:${FLEETML_DASHBOARD_PORT:-80}  (log in with FLEETML_ADMIN_KEY from ~/.fleetml.env)"
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
   mkdir -p ~/.local/bin && tar -xzf "fleetml-$OS-$ARCH.tar.gz" -C ~/.local/bin fleetml
   rm "fleetml-$OS-$ARCH.tar.gz" SHA256SUMS
   export PATH="$HOME/.local/bin:$PATH"
   fleetml version
   ```

   You should see `fleetml-<os>-<arch>.tar.gz: OK` and `✓ fleetml v<version>`.

4. **Publish the sample model and deploy it.** The project gets the time in
   its name, so running the block again makes a new one:

   <!-- golden: publish -->
   ```bash
   export FLEETML_SERVER=http://localhost:${FLEETML_REST_PORT:-18080} FLEETML_API_KEY=$FLEETML_ADMIN_KEY
   P=$(fleetml project create --name "first-$(date +%H%M%S)" -o json | jq -r .id)
   fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
   V=$(fleetml ver list --model "$(fleetml model list --project $P -o json | jq -r '.[0].id')" -o json | jq -r '.[0].id')
   fleetml deploy start "$(fleetml deploy create --project $P --version $V -o json | jq -r .id)"
   export FLEETML_ENROLLMENT_KEY=$(fleetml enrollment-key create --project $P --name first-device -o json | jq -r .key)
   ```

   You should see `✓ Pushed models/iris/iris_v1.tflite → iris v1.0.0 (INT8)` and `✓ Deployment start → COMPLETE`.

5. **Put the agent on it**, in a container on this machine. It registers with the enrollment key from step 4, downloads the
   model, checks it and makes it active:

   <!-- golden: device -->
   ```bash
   ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
   mkdir -p fleetml-agent && curl -fsSL "https://github.com/fleetml/fleetml/releases/latest/download/fleetml-agent-linux-$ARCH.tar.gz" | tar -xz -C fleetml-agent
   docker run -d --init --name fleetml-agent --add-host=host.docker.internal:host-gateway -e FLEETML_ENROLLMENT_KEY \
     -v "$PWD/fleetml-agent:/opt/fleetml-agent:ro" ubuntu:24.04 sh -c "/opt/fleetml-agent/install.sh /opt/fleetml-agent/fleetml-agent > /dev/null &&
     exec setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml \
       -server host.docker.internal:${FLEETML_GRPC_PORT:-19090} -download-host-override host.docker.internal:${FLEETML_S3_PUBLIC_PORT:-19100}"
   for i in $(seq 60); do docker logs fleetml-agent 2>&1 | grep 'model active' && break; [ $i = 60 ] && echo "No model active after 2 minutes: see Troubleshooting"; sleep 2; done
   ```

   You should see `msg="model active" model=iris version=1.0.0`.
   The container runs `install.sh` and then the agent as the
   `fleetml-agent` user, as [Without systemd](#without-systemd-and-in-containers)
   describes; `-server` and `-download-host-override` point it at this
   machine's ports, which a container reaches as `host.docker.internal`.

6. **See it in the dashboard.** This puts the admin key on the clipboard (on
   a Mac, in WSL, or on Linux with `xclip`; elsewhere it prints it) and prints
   the dashboard's address: open it and paste the key into the login box:

   <!-- golden: dashboard manual -->
   ```bash
   printf %s "$FLEETML_ADMIN_KEY" | { pbcopy || clip.exe || xclip -selection clipboard || cat; } 2> /dev/null; echo
   echo "Dashboard: http://localhost:${FLEETML_DASHBOARD_PORT:-80}"
   ```

   You should see `Dashboard: http://localhost:<port>`, and in the browser the
   Overview page (**Dashboard** in the menu); **Devices** lists one device, and **Deployments** one
   `COMPLETE` deployment of iris 1.0.0.

7. **Clean up**: the device, the server and all its data, and the saved
   secrets (the CLI stays in `~/.local/bin`; `down` without `-v` would keep
   the data):

   <!-- golden: cleanup cleanup -->
   ```bash
   docker rm -f fleetml-agent
   source ~/.fleetml.env && docker compose -f deploy/docker-compose.prod.yml down -v && rm -rf fleetml-agent ~/.fleetml.env
   ```

   You should see `Network <stack>_default  Removed`.

Then the agent on a real machine: [Install](#install) and
[Connect and verify](#connect-and-verify); a server on another machine:
[ADVANCED.md](ADVANCED.md#linux-agent-a-server-on-another-machine).

## Install

Every [GitHub release](https://github.com/fleetml/fleetml/releases) has the
agent for Linux as `fleetml-agent-linux-amd64.tar.gz` and
`fleetml-agent-linux-arm64.tar.gz`, listed in `SHA256SUMS`. Each archive holds
the static binary `fleetml-agent`, the installer `install.sh`,
`config.example.yaml`, `fleetml-agent.service`, `LICENSE` and `NOTICE`. On
the machine, the latest release, checked, then installed as root:

```bash
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
curl -fsSLO "https://github.com/fleetml/fleetml/releases/latest/download/fleetml-agent-linux-$ARCH.tar.gz"
curl -fsSLO https://github.com/fleetml/fleetml/releases/latest/download/SHA256SUMS
grep " fleetml-agent-linux-$ARCH.tar.gz\$" SHA256SUMS | sha256sum -c
mkdir -p fleetml-agent && tar -xzf "fleetml-agent-linux-$ARCH.tar.gz" -C fleetml-agent
sudo ./fleetml-agent/install.sh ./fleetml-agent/fleetml-agent
fleetml-agent -version
```

Debian and Ubuntu images often have neither `curl` nor `wget`
(`ubuntu:24.04` has neither): `apt-get update && apt-get install -y curl ca-certificates`
first. On another minimal image, `wget -q` with the same URLs downloads the
two files, or copy them over from another machine (to check them on a Mac
first, `shasum -a 256 -c` in place of `sha256sum -c`); in a root shell without
`sudo`, leave `sudo` out. `sha256sum -c` prints
`fleetml-agent-linux-amd64.tar.gz: OK` (or `arm64`), and
`fleetml-agent -version` prints the release without a `v`, for example
`fleetml-agent 0.10.4`. To build the agent from source instead, `make
build-agent` in a clone writes `bin/fleetml-agent-linux-amd64`, `-linux-arm64`
and `-darwin-arm64` (version `dev`), and
`sudo ./deploy/agent/install.sh ./bin/fleetml-agent-linux-arm64` installs one
from the clone.

The script (POSIX `sh`, so a BusyBox image without bash runs it too) installs
the binary to `/usr/local/bin`, creates a `fleetml-agent` system user (with
`useradd`, or BusyBox's `addgroup` and `adduser`), the config file (from
[`config.example.yaml`](../deploy/agent/config.example.yaml)) and the
[systemd unit](../deploy/agent/fleetml-agent.service), and starts nothing
while the config still holds the placeholder key. The unit runs the agent as
`fleetml-agent` with its state in `/var/lib/fleetml-agent` (created by
`StateDirectory=`), restarts it when it exits (`Restart=always`), and
hardens it: `NoNewPrivileges=true` (no `sudo` or setuid from the agent or its
hooks), `ProtectSystem=strict` (the system read-only, only the state folder
writable), `ProtectHome=true` and `PrivateTmp=true`.

### Without systemd, and in containers

On a machine where systemd is not running (a container, also one whose image
carries the systemd package, a chroot, a BusyBox or Yocto image, a kiosk with
its own init; the script checks for `/run/systemd/system`, which only a
running systemd creates), `install.sh` installs the binary, the user and the
config the same way, skips the unit, creates `/var/lib/fleetml-agent` owned
by `fleetml-agent` (mode `0755`; the unit's `StateDirectory=` does this under
systemd) and says so:

```text
No running systemd on this machine: the service unit was not installed, and /var/lib/fleetml-agent is ready for the fleetml-agent user.
```

Once the config is set, run the agent as that user under the machine's own
supervisor (runit, s6, OpenRC, supervisord, a container's entrypoint), with
the same command the unit runs; `install.sh` prints this line:

```bash
exec setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml
```

`setpriv` (util-linux; checked in Debian 12 and Ubuntu 24.04) switches to the
user and replaces itself with the agent, so the SIGTERM the supervisor stops
it with reaches the agent itself, and the `exec` keeps the shell of an
entrypoint script out of the way too. `su` is the wrong tool here: it stays
as the agent's parent and kills it with SIGKILL 2 seconds after passing on a
SIGTERM, and with a `-c` command and no inner `exec` it leaves the agent
running as an orphan. On BusyBox (Alpine), whose `setpriv` cannot switch
users, `install.sh` prints the form that works there, where `su` replaces
itself with the shell and the shell with the agent:
`exec su -s /bin/sh fleetml-agent -c 'exec /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml'`. It logs to standard error, which is
where the supervisor's log picks it up; the journal lines below are the
same. The supervisor restarts it if it exits, as `Restart=always` does in
the unit.

In a container, make the agent the main process and give the container an
init that reaps processes. Started in the background under a placeholder such
as `sleep infinity`, an agent that exits stays behind as a `<defunct>`
process, because the placeholder does not collect it. In an
entrypoint script, the last line is

```bash
exec setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml
```

and `docker run --init` (or `tini`/`dumb-init` as the entrypoint) reaps
what is left. To try the agent in a throwaway Ubuntu container, first run the
CLI block of [Connect and verify](#connect-and-verify) on the machine with
the CLI (it prints the enrollment key and creates the deployment), then,
from the folder that holds the unpacked `fleetml-agent` folder, with the
server's address and ports in place of the placeholders (the server's gRPC
port and its model files' port; [Server on another
machine](#server-on-another-machine) says which, and
`-download-host-override` can go when the server's `S3_PUBLIC_ENDPOINT` is
reachable from the container) and that key in `FLEETML_ENROLLMENT_KEY` (the CLI block
exports it). [Get
started](#get-started) runs the same against a server on this machine:

```bash
docker run -d --init --name fleetml-agent -e FLEETML_ENROLLMENT_KEY -v "$PWD/fleetml-agent:/opt/fleetml-agent:ro" \
  -v fleetml-agent-state:/var/lib/fleetml-agent ubuntu:24.04 sh -c "/opt/fleetml-agent/install.sh /opt/fleetml-agent/fleetml-agent > /dev/null &&
  exec setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml \
    -server <server address>:<gRPC port> -download-host-override <server address>:<model files port>"
docker logs -f fleetml-agent
```

The volume `fleetml-agent-state` keeps the agent's state folder: a
container has no machine ID of its own (`/etc/machine-id` is empty), so the
random salt in `state.json` is the device's identity, and the models are
there too. With the volume, a recreated container is the same device with
its models; without it, every new container registers as a new device,
downloads everything again, and the old one stays in `fleetml device list`.
`docker rm -f fleetml-agent && docker volume rm fleetml-agent-state` removes
both. The enrollment key passed with `-e` stays off every command line, the
container's process list (`ps`, `/proc/1/cmdline`) included, but it is in
the container's configuration, which `docker inspect fleetml-agent` shows to
anyone who can use Docker on the machine, and in the agent's environment:
fine for a throwaway test, but for a device you keep, mount a prepared
`config.yaml` read-only instead
(`-v "$PWD/config.yaml:/etc/fleetml-agent/config.yaml:ro"`) and leave out
the `-e`, `-server` and `-download-host-override`.

## Server on another machine

Moved to [ADVANCED.md](ADVANCED.md#linux-agent-a-server-on-another-machine):
which addresses the agent needs from the server's machine, the server's
`S3_PUBLIC_ENDPOINT` or this machine's `download_host_override`, and what an
unreachable download address looks like.

## Connect and verify

The agent registers into a project with one of the project's enrollment keys,
and a deployment reaches it through its labels. On the machine where the
`fleetml` CLI runs ([Get the CLI](QUICKSTART.md#get-the-cli)), with `jq`,
this creates a project, publishes a model, creates an enrollment key and
deploys the model to the labels the example config sets (`site: factory-1`).
The key is the server's admin key or one with the `admin` role in your
project ([which key does what](ANDROID_SDK.md#publish-and-deploy-with-the-cli));
the project name `edge` and the model file are this guide's, yours will differ
(`models/iris/iris_v1.tflite` is in a clone of this repository):

```bash
export FLEETML_SERVER=<server URL> FLEETML_API_KEY=<the key your team gave you>
P=$(fleetml project list -o json | jq -r '.[] | select(.name == "edge") | .id'); [ -n "$P" ] || P=$(fleetml project create --name edge -o json | jq -r .id)
M=$(fleetml model list --project $P -o json | jq -r '.[] | select(.name == "iris") | .id')
vid() { fleetml ver list --model $M -o json | jq -r --arg v "$1" '.[] | select(.version == $v) | .id'; }
[ -n "$M" ] && [ -n "$(vid 1.0.0)" ] || fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
M=$(fleetml model list --project $P -o json | jq -r '.[] | select(.name == "iris") | .id')
export FLEETML_ENROLLMENT_KEY=$(fleetml enrollment-key create --project $P --name edge -o json | jq -r .key); echo "$FLEETML_ENROLLMENT_KEY"
D=$(fleetml deploy create --project $P --version $(vid 1.0.0) --target '{"site":"factory-1"}' -o json | jq -r .id); fleetml deploy start $D
```

The block can run again: it reuses the project `edge` and its iris 1.0.0
when they are there (or `P=<the project ID your team gave you>` in place of
the first line), and each run adds one more enrollment key and one more
deployment of 1.0.0 (`fleetml enrollment-key list --project $P` and
`fleetml deploy list --project $P` show them; they do no harm). `-o json | jq -r .key` takes the enrollment key itself, to paste or to hand
to a provisioning script; without `-o json`, `enrollment-key create` prints a
table with the key once. One key enrolls any number of machines; `fleetml enrollment-key revoke <key ID>` (the ID from `fleetml enrollment-key list --project $P`) stops new registrations with it, while machines already registered keep working. `vid` looks
a version up by its number, so the block picks 1.0.0 even after more versions
are pushed (`fleetml ver list` lists the newest first). The deployment starts
as `COMPLETE` (`✓ Deployment start → COMPLETE`): it waits for the machine.
With the quickstart on the same machine as the CLI, `FLEETML_SERVER` is
`http://localhost:${FLEETML_REST_PORT:-18080}` and the key `$FLEETML_ADMIN_KEY` after
`source ~/.fleetml.env`.

Then, on the agent's machine, set `server` and `enrollment_key` in its
config, with the server's address and its gRPC port in place of the
placeholders (which address and ports, and when `download_host_override`
is needed too: [ADVANCED.md](ADVANCED.md#linux-agent-a-server-on-another-machine)),
and start the service:

```bash
sudo sed -i -e 's|^server: .*|server: <server address>:<gRPC port>|' \
  -e '/^enrollment_key:/d' /etc/fleetml-agent/config.yaml
read -rs FLEETML_ENROLLMENT_KEY
printf 'enrollment_key: "%s"\n' "$FLEETML_ENROLLMENT_KEY" | sudo tee -a /etc/fleetml-agent/config.yaml > /dev/null
sudo systemctl enable --now fleetml-agent
sudo journalctl -u fleetml-agent -f
```

`read -rs` waits for the enrollment key the first block printed (paste it
and press Enter; it is not shown), and `printf`, a shell builtin, hands it
to `tee`, so the key is on no command line and in no shell history. `sed`
is on every image, BusyBox included; with an editor, `sudo vi
/etc/fleetml-agent/config.yaml` does the same. Without systemd, start the
`setpriv` command (on BusyBox, the `su` form) from
[Without systemd](#without-systemd-and-in-containers) under the supervisor
instead of the two `systemctl` and `journalctl` lines; to try it in a shell
first (a container you are in, for example), run that command with `sudo`
and without `exec`: the agent runs in the foreground and logs there, and
Ctrl-C stops it. Leave `tls: false` for the
quickstart's compose server, which speaks plain gRPC on its gRPC port, and set it to
`true` when the server is behind a TLS proxy. Set `labels` to what your
deployments target (the example's `site: factory-1` matches the block above):
a `target_selector` such as `{"site":"factory-1"}` matches every device whose
labels include it ([ROLLOUTS.md](ROLLOUTS.md#how-a-device-gets-a-model)).

The journal then shows the agent start and register (the time stamps left out
here):

```text
level=INFO msg="fleetml-agent starting" version=0.10.4 server=<server address>:<gRPC port> models_dir=/var/lib/fleetml-agent/models
level=INFO msg=registered device_id=<device ID>
```

and the machine appears in `fleetml device list --project $P` with OS
`LINUX`. A deployment that already targets its labels arrives right after
registration; one created later arrives with the next heartbeat (60 seconds
with the server's default `DEVICE_HEARTBEAT_INTERVAL`; the agent's own `30s`
applies only until the server answers):

```text
level=INFO msg="model downloaded" model=iris version=1.0.0 bytes=<size>
level=INFO msg="model active" model=iris version=1.0.0 path=/var/lib/fleetml-agent/models/iris/current
```

`/var/lib/fleetml-agent/models/iris/current` then links to the active
version's folder ([Where models go](#where-models-go)), and
`fleetml deploy get $D | jq -c .devices` turns to
`{"loaded":1,"targeted":1}` one heartbeat later. A second version, pushed and
deployed to the same labels in the same shell as the first block:

```bash
fleetml push models/iris/iris_v2.tflite --project $P --model iris --version 2.0.0
fleetml deploy start $(fleetml deploy create --project $P --version $(vid 2.0.0) --target '{"site":"factory-1"}' -o json | jq -r .id)
```

arrives with the next heartbeat, usually as a delta from the first:

```text
level=INFO msg="model built from a delta" model=iris version=2.0.0 delta_bytes=<size> model_bytes=<size>
level=INFO msg="model active" model=iris version=2.0.0 path=/var/lib/fleetml-agent/models/iris/current
```

After a restart (`sudo systemctl restart fleetml-agent`), the agent keeps its
registration and logs the model it has instead of registering again (after an
upgrade of the agent or a change of its labels it logs `registered` once
more, with the same device ID: it tells the server its new version or
labels, and the box is still one device):

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
a server without TLS (in a container whose server name resolves first to an
address it has no route to, such as `host.docker.internal`, it can read
`connect: network is unreachable` instead, see
[Identity and offline behaviour](#identity-and-offline-behaviour)); `registration failed` with `Unauthenticated` means the
enrollment key is wrong or revoked; `download failed` with
`connection refused` and `localhost` with the model files' port in the URL means the server's
`S3_PUBLIC_ENDPOINT` ([Server on another machine](#server-on-another-machine)).

## Configure

Settings come from the YAML file (`-config`), and flags override them
(`-server`, `-enrollment-key`, `-label key=value`, `-state-dir`,
`-models-dir`, `-validator`, `-post-activate`, `-telemetry-listen`).
`FLEETML_ENROLLMENT_KEY` in the environment works too. `-label` adds to the
config's labels, and the agent registers again with them: a model whose
deployment no longer targets the machine's labels is unloaded and its files
removed. `-retry-failed`
retries the versions that failed validation right away instead of after the
hour's wait ([Where models go](#where-models-go), step 5).

| Setting | Default | Purpose |
|---|---|---|
| `server` | (required) | FleetML gRPC address, `host:port` |
| `tls` | `false` | TLS for gRPC (use it whenever the server is not on a private network) |
| `enrollment_key` | | Project enrollment key for the first registration |
| `labels` | `{}` | Labels deployments and experiments target |
| `state_dir` | `/var/lib/fleetml-agent` | Registration, telemetry queue, partial downloads |
| `models_dir` | `<state_dir>/models` | Where models are placed (below), on any file system. The unit runs with `ProtectSystem=strict`, so a folder outside `/var/lib/fleetml-agent` must be allowed with `ReadWritePaths=` in a drop-in, which upgrades keep ([below](#where-models-go)), and belong to the agent's user (`sudo install -d -o fleetml-agent -g fleetml-agent <folder>`). After a change the agent downloads its models again into the new folder; the old folder's copies stay until you remove them |
| `validator` | none | Shell command that checks a downloaded model; exit code 0 means valid. `{file}` in the command is replaced by the model's path, and the `FLEETML_MODEL_FILE`, `FLEETML_MODEL_NAME` and `FLEETML_MODEL_VERSION` environment variables are set too; use either |
| `post_activate` | none | Shell command run after a version becomes active, as `fleetml-agent`; under the shipped unit it can write only to `/var/lib/fleetml-agent` ([reloading the inference process](#reloading-the-inference-process-after-an-update)) |
| `telemetry_listen` | off | Local endpoint for inference events, e.g. `127.0.0.1:9601` |
| `metered`, `on_battery` | `false` | Describe the machine's connection and power for the download policy |
| `heartbeat_interval` | `30s` | Until the server sends its own (`DEVICE_HEARTBEAT_INTERVAL`, default 60 s) |
| `download_host_override` | none | `host:port` that every model download goes to instead of the URL's host, for a server whose `S3_PUBLIC_ENDPOINT` this machine cannot reach ([Server on another machine](#server-on-another-machine)) |

The server's device settings apply as for the SDKs
([CONFIG.md](CONFIG.md#devices)); the agent runs continuously, so it always
uses the foreground heartbeat interval.

## Where models go

```
/var/lib/fleetml-agent/
  state.json, telemetry.queue          # 0600: registration, failures, queued events
  downloads/                           # 0700: partial downloads
  rejected/iris/3.0.0-5e6f7a8b/model.bin   # 0700 folders, 0600 file: the last version that failed validation
  models/
    iris/
      current -> 2.0.0-1a2b3c4d        # the active version, switched atomically
      2.0.0-1a2b3c4d/model.bin
      1.0.0-9f8e7d6c/model.bin         # last-known-good, kept for rollback
```

A `models_dir` outside `/var/lib/fleetml-agent` (another disk, `/opt/models`
here): the folder, owned by the agent's user, and a systemd drop-in that
allows the unit to write there (`install.sh` replaces the unit on an
upgrade but never touches its drop-ins), then the setting:

```bash
sudo install -d -o fleetml-agent -g fleetml-agent /opt/models
sudo mkdir -p /etc/systemd/system/fleetml-agent.service.d
printf '[Service]\nReadWritePaths=/opt/models\n' | sudo tee /etc/systemd/system/fleetml-agent.service.d/models.conf > /dev/null
sudo sed -i 's|^#* *models_dir:.*|models_dir: /opt/models|' /etc/fleetml-agent/config.yaml
sudo systemctl daemon-reload && sudo systemctl restart fleetml-agent
```

You should see `model active ... path=/opt/models/<model>/current` in the
journal after the next heartbeat. `sudo systemctl edit fleetml-agent` writes
the same drop-in from an editor. Without the drop-in, the journal says
`level=ERROR msg="creating model directory failed" error="mkdir /opt/models/<model>: read-only file system"`
and the active model stays where it was.

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
   download that fails, or a model that cannot be put into `models_dir`, is
   retried on every heartbeat and reported once as
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
the agent's user, and see its exit code; with `sudo`:

```bash
sudo -u fleetml-agent sh -c 'python3 /opt/inference/check_model.py /var/lib/fleetml-agent/rejected/iris/3.0.0-<variant>/model.bin; echo "exit $?"'
```

or in a root shell without `sudo` (a container, a minimal image):

```bash
su -s /bin/sh fleetml-agent -c 'python3 /opt/inference/check_model.py /var/lib/fleetml-agent/rejected/iris/3.0.0-<variant>/model.bin; echo "exit $?"'
```

Once it passes, start the agent once with `-retry-failed`, which clears the
saved failures and retries them on its first heartbeat, then go back to the
service:

```bash
sudo systemctl stop fleetml-agent
sudo -u fleetml-agent /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml -retry-failed
sudo systemctl start fleetml-agent
```

Without systemd, stop the agent under its supervisor and run, as root,

```bash
setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml -retry-failed
```

(on BusyBox, whose `setpriv` cannot switch users,
`su -s /bin/sh fleetml-agent -c 'exec /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml -retry-failed'`;
with `setpriv`, Ctrl-C reaches the agent itself and the prompt comes back at
once)

then start it under the supervisor again. The foreground run logs
`level=INFO msg="cleared the failed versions; they are retried on the next heartbeat" count=1`,
and its first heartbeat, right away, retries the version
(`level=INFO msg="model active" model=iris version=3.0.0 ...` once the
validator passes); stop it with Ctrl-C then. A restart without the flag keeps
waiting out the hour.

The rejected copy stays where it was after the version becomes active: it is
the file as it failed, kept for comparison, and it is replaced only when
another version of the model fails. Once you no longer need it, remove it
(the active copy is under `models/`):

```bash
sudo rm -r /var/lib/fleetml-agent/rejected/iris/3.0.0-<variant>
```

A validator can be anything that loads the model the way production does, for
example a Python script that runs one inference with ONNX Runtime on a known
input. There is no built-in ONNX or TFLite validator: the agent is a static Go
binary without those runtimes, and the command validator lets you use exactly
the runtime and version your inference process uses.

### Reloading the inference process after an update

`post_activate` runs as the unprivileged `fleetml-agent` user under a unit
with `NoNewPrivileges=true` and `ProtectSystem=strict`, so it can neither use
`sudo` nor write outside `/var/lib/fleetml-agent`. A `post_activate` of
`sudo systemctl restart inference` fails with
`sudo: The "no new privileges" flag is set, which prevents sudo from running as root.`,
sudoers rule or not; keep the hardening and let systemd do the restart. The
hook touches a file in the state folder, and a path unit that watches the file
restarts your service. In `/etc/fleetml-agent/config.yaml`:

```yaml
post_activate: "touch /var/lib/fleetml-agent/model-switched"
```

`/etc/systemd/system/inference-reload.path`:

```ini
[Unit]
Description=Restart the inference service when FleetML switches a model

[Path]
PathChanged=/var/lib/fleetml-agent/model-switched

[Install]
WantedBy=multi-user.target
```

`/etc/systemd/system/inference-reload.service` (`inference.service` is your
inference process's unit):

```ini
[Unit]
Description=Restart the inference service after a model switch

[Service]
Type=oneshot
ExecStart=/usr/bin/systemctl restart inference.service
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now inference-reload.path
sudo systemctl restart fleetml-agent
```

After each `model active` line the journal shows
`Finished inference-reload.service - Restart the inference service after a model switch.`,
and the inference service starts again on the new `current`. Do not point
`PathChanged=` at the `current` link itself: inotify follows the link to the
version folder, so the path unit fires when the link is first created and
misses every later switch. An inference process that reloads itself can watch
the same file instead. A polkit rule that lets `fleetml-agent` restart that
one unit also works under `NoNewPrivileges=true`, since systemd checks it, not
a setuid binary; it is not shown here.

## Telemetry from the inference process

With `telemetry_listen` set, the inference process (Python, C++, anything)
reports inferences over HTTP on the machine, without a client library:

```bash
curl -s -X POST localhost:9601/v1/events -H 'Content-Type: application/json' \
  -d '{"model":"iris","latency_ms":12,"confidence":0.93,"label":"virginica"}'
curl -s localhost:9601/v1/models
```

`model` is the model's name, `iris` in the walkthrough above. The first call
answers `{"accepted":1,"rejected":0}`; the second lists the active models
with the same `model` key and their `current` links:

```json
[{"model":"iris","version":"1.0.0","version_id":"<version ID>","path":"/var/lib/fleetml-agent/models/iris/current"}]
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
{"accepted":0,"rejected":1,"rejected_reasons":["event 0: unknown model \"foo\" (active models: iris)"]}
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
server is back. A connection that dies without a reset (a Wi-Fi roam, a NAT
or router restart, a pulled cable) is found within 40 seconds: the agent
pings an idle connection after 30 seconds and gives up on it after 10
without an answer, and every call to the server has a 30-second deadline.
The warning below comes with the first heartbeat after that, so within
those 40 seconds plus one heartbeat interval (60 seconds by default;
measured: 32 to 67 seconds); with telemetry queued, a `sending telemetry failed; the events
stay queued` warning can come first. With the network removed rather than
silent, or when the server's name resolves first to an address this
machine has no route to (an IPv6 one, as `host.docker.internal` can in a
container), the error can read `connect: network is unreachable` instead;
with the server stopped and started again rather than silent, it reads
`connection refused` while it is down, or
`error reading server preface: ... use of closed network connection` at the
moment it goes. The first failed heartbeat is logged once, and the recovery
once:

```text
level=WARN msg="heartbeat failed; retrying every heartbeat, models stay active and telemetry stays queued" error="rpc error: code = Unavailable desc = keepalive ping failed to receive ACK within timeout"
level=INFO msg="server reachable again" unreachable_for=3m10s
```

Measured with the quickstart's compose stack and the agent in a container
(`docker pause` on the server for 3 minutes, which drops nothing and answers
nothing): the warning came 32 to 67 seconds after the pause (different
runs; on Docker Desktop it can read `error reading server preface: ...
use of closed network connection` instead of the keepalive line), and the first
successful heartbeat 42 seconds after the server was back. Recovery comes
with the next heartbeat, so it takes at most one heartbeat interval (60
seconds by default); before 0.8.0 the same test left the agent silent for
about 16 minutes. Stopping the agent while the server is unreachable takes
at most 2 seconds: the last telemetry flush is bounded at 1.5 seconds, and
what it cannot send stays queued for the next start (`telemetry kept queued
for the next start`). Measured with `time systemctl stop fleetml-agent` and
the server frozen: 1.64 seconds (5.11 before 0.9.0, whose bound was 5
seconds).

## Upgrade

On each machine, the new release's archive (the Install block, which takes
the latest release), then `install.sh` again:

```bash
sudo ./fleetml-agent/install.sh ./fleetml-agent/fleetml-agent
fleetml-agent -version
```

It replaces the binary, keeps `/etc/fleetml-agent/config.yaml`, the unit's
drop-ins and the state folder (the device's identity, its models and queued
telemetry), and restarts a running service, so the new version runs at once
(`fleetml-agent <new version>`). A unit you edited by hand is kept too, with
the release's unit next to it as `fleetml-agent.service.new` and a line
saying so: move your settings into a drop-in and replace the unit with it. Without
systemd, restart the agent under its supervisor. Upgrade the server first,
as with every release: agents of the release before keep working against
the new server until you upgrade them (a 0.8.0 agent registered, took a
model and a delta from a 0.9.0 server, and kept its state when upgraded in
place), and a 0.9.x agent also works with a 0.8.0 server (it registered,
activated a model and heartbeated against one without an error).

## Remove the agent

On the agent's machine, first this machine's device ID, which the agent
logged when it registered (without systemd, in the agent's own log), and a
`models_dir` you moved out of the state folder, then the service with its
drop-ins (`fleetml-agent.service.d`) and the `.new` unit an upgrade may
have left, the binary, its config and models, and its user (without
systemd, stop it under its supervisor instead of the `systemctl` line):

```bash
sudo journalctl -u fleetml-agent -o cat | grep -o 'device_id=[^ ]*' | tail -1
sudo grep '^models_dir:' /etc/fleetml-agent/config.yaml
sudo systemctl disable --now fleetml-agent
sudo rm -rf /etc/systemd/system/fleetml-agent.service /etc/systemd/system/fleetml-agent.service.new /etc/systemd/system/fleetml-agent.service.d /usr/local/bin/fleetml-agent && sudo systemctl daemon-reload
sudo rm -rf /etc/fleetml-agent /var/lib/fleetml-agent
sudo userdel fleetml-agent
```

You should see `device_id=` and the ID. A `models_dir:` line means the
models are in that folder, outside `/var/lib/fleetml-agent`: remove it too,
with `sudo rm -rf <that folder>` (no line: they were in the state folder,
which the block removed). Then on the machine with the CLI,
remove the device from its project, with that ID:

```bash
curl -fsS -X DELETE -H "Authorization: Bearer $FLEETML_API_KEY" "$FLEETML_SERVER/api/v1/devices/<device ID>" && echo removed
```

## Try it locally

In a clone, with Go 1.26 or later, Docker, curl and jq:

```bash
./scripts/demo-agent.sh
```

starts the dev stack (compose project `fleetml`, Postgres, SeaweedFS and
NATS on ports 15432, 19000 and 14222), a server on it and four agents on this
machine (the agent also builds for macOS, for local testing), each with a
stand-in inference process that posts to its local endpoint. A canary of a
model that fails at inference trips the health gate and rolls back, and the
canary boxes switch their `current` link back to the last-known-good
version. It is the recording at the top of the [README](../README.md). It
leaves the dev stack running (`docker compose -f deploy/docker-compose.yml down`
stops it). Where that project or those ports are taken, give it others,
the demo server's own REST and gRPC ports (18380 and 19390) among them:
`COMPOSE_PROJECT_NAME=fleetml-try FLEETML_PG_PORT=25432 FLEETML_S3_PORT=29000 FLEETML_NATS_PORT=24222 FLEETML_NATS_MONITOR_PORT=28222 FLEETML_DEMO_REST_PORT=28380 FLEETML_DEMO_GRPC_PORT=29390 ./scripts/demo-agent.sh`.
The stack it leaves then has that project name, so stop it with the same
one: `COMPOSE_PROJECT_NAME=fleetml-try docker compose -f deploy/docker-compose.yml down -v`.

## Verified

`go test ./internal/agent` covers the queue, state, fingerprint, download
resume, the validator command and the install path (symlink, last-known-good,
hook, local endpoint and its rejection reasons, file modes, the rejected file
and `-retry-failed`, `download_host_override`), and the e2e test
`TestE2E_AgentDownloadHostOverride` downloads through the override from a
real SeaweedFS that refuses a plain host rewrite.
`./scripts/agent-install-test.sh` runs `install.sh` in Ubuntu containers
without systemd, with systemd installed but not running, and with a stand-in
for a running systemd, and in an Alpine (BusyBox) container without bash or
`useradd`.

The agent ran in an Ubuntu 24.04 container against the quickstart's stack
started without `S3_PUBLIC_ENDPOINT`, following [Server on another
machine](#server-on-another-machine) and [Connect and
verify](#connect-and-verify) as written: the download failed as described,
then `download_host_override` with the Mac's LAN address brought 1.0.0 and
the 2.0.0 delta. In an Ubuntu 24.04 container with systemd, a failing
validator kept 2.0.0 and the rejected file, a restart did not retry it,
`-retry-failed` as above activated it, and the path unit in [Reloading the
inference process](#reloading-the-inference-process-after-an-update)
restarted a stand-in inference service after each switch. The runtime check
ran two `linux/amd64` agents in Debian containers next to the dev stack: a
50% canary reached exactly one of them, a failing validator on that one kept
the last-known-good version and reported `MODEL_LOAD_FAILED`, and completing
the rollout moved both `current` links and ran the hook.
`./scripts/signing-e2e.sh agent` checks signatures: a model whose stored
signature was overwritten is rejected, and one signed with the previous key
is accepted during the rotation's grace period and rejected after it.

## Contact

- Anything about FleetML: [hello@fleetml.dev](mailto:hello@fleetml.dev).
- A security problem: [security@fleetml.dev](mailto:security@fleetml.dev), see [SECURITY.md](../SECURITY.md).
- Questions in public: [GitHub Discussions](https://github.com/fleetml/fleetml/discussions).
- Bugs: [GitHub Issues](https://github.com/fleetml/fleetml/issues/new/choose), with the FleetML version and the command or log line that shows the problem.
