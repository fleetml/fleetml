#!/usr/bin/env bash
# Prepares Ubuntu in WSL 2 for the Windows run workflow
# (.github/workflows/windows-run.yml), as root: Docker Engine and the compose
# plugin from Docker's apt repository (Docker Desktop cannot be installed on
# the runner), git and jq as docs/QUICKSTART.md "On Windows" asks, Go 1.24 for
# the tour's first block, and a running dockerd that the user named by the
# first argument can use.
#
#   wsl -d Ubuntu -u root --exec bash .github/windows-run/wsl-setup.sh tester
# shellcheck disable=SC1091 # /etc/os-release is read where it runs
set -euo pipefail
user=${1:?usage: wsl-setup.sh <user>}
export DEBIAN_FRONTEND=noninteractive
note() { [ -z "${GITHUB_OUTPUT:-}" ] || printf 'note=%s\n' "$1" >>"$GITHUB_OUTPUT"; }

echo "Ubuntu: $(. /etc/os-release && echo "$PRETTY_NAME"), $(uname -r), $(dpkg --print-architecture)"
# The docs say curl and openssl are already there; record it if not.
missing=""
for c in curl openssl; do command -v "$c" >/dev/null || missing="$missing $c"; done
echo "Expect: curl and openssl already installed"
echo "Got:    ${missing:+missing:}${missing:-both present}"

apt-get update -q
apt-get install -y -q ca-certificates curl openssl git jq
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" \
  >/etc/apt/sources.list.d/docker.list
apt-get update -q
apt-get install -y -q docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
usermod -aG docker "$user"

# The newest Go 1.24.x for this architecture, on every user's PATH.
gover=$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all' | jq -r '[.[].version | select(startswith("go1.24."))][0]')
curl -fsSL "https://go.dev/dl/$gover.linux-$(dpkg --print-architecture).tar.gz" | tar -C /usr/local -xz
ln -sf /usr/local/go/bin/go /usr/local/bin/go

# dockerd as a systemd service when systemd runs (Ubuntu's WSL default), else
# in the background (the workflow keeps the distribution running).
if [ "$(cat /proc/1/comm)" = systemd ]; then
  systemctl enable --now docker
  how="systemd"
else
  nohup dockerd >/var/log/dockerd.log 2>&1 &
  how="dockerd in the background (no systemd)"
fi
for _ in $(seq 60); do docker info >/dev/null 2>&1 && break; sleep 1; done

docker version
docker compose version
go version
git --version
jq --version
echo "Expect: docker answers as $user (Server: Docker Engine - Community)"
server=$(su "$user" -c "docker version --format '{{.Server.Platform.Name}} {{.Server.Version}}'")
echo "Got:    $server"
case "$server" in *"Docker Engine"*) echo PASS ;; *) note "FAILED: docker as $user"; exit 1 ;; esac
note "$(. /etc/os-release && echo "$PRETTY_NAME"); $server, dockerd via $how; $(go version | cut -d' ' -f3); curl/openssl: ${missing:+missing$missing (installed)}${missing:-already there}"
