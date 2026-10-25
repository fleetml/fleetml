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
version as `dev`). The other platforms and the PowerShell block are in
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
