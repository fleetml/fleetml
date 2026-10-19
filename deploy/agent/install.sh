#!/usr/bin/env bash
# Installs the FleetML agent as a systemd service:
#   sudo ./deploy/agent/install.sh ./bin/fleetml-agent-linux-amd64
# Creates the fleetml-agent user, /etc/fleetml-agent/config.yaml (from the
# example, if missing) and the service, and starts it once the config has a
# server and an enrollment key.
set -euo pipefail
BIN=${1:?usage: install.sh <path to the fleetml-agent binary for this machine>}
HERE=$(cd "$(dirname "$0")" && pwd)
[ "$(id -u)" = 0 ] || { echo "run as root (sudo)" >&2; exit 1; }

install -m 0755 "$BIN" /usr/local/bin/fleetml-agent
id fleetml-agent >/dev/null 2>&1 || useradd --system --home-dir /var/lib/fleetml-agent --shell /usr/sbin/nologin fleetml-agent
install -d -m 0755 /etc/fleetml-agent
if [ ! -f /etc/fleetml-agent/config.yaml ]; then
  install -m 0640 -g fleetml-agent "$HERE/config.example.yaml" /etc/fleetml-agent/config.yaml
  echo "Wrote /etc/fleetml-agent/config.yaml: set server, enrollment_key and tls (false for a server without TLS), then run: systemctl enable --now fleetml-agent"
fi
install -m 0644 "$HERE/fleetml-agent.service" /etc/systemd/system/fleetml-agent.service
systemctl daemon-reload
if grep -q '<project enrollment key>' /etc/fleetml-agent/config.yaml; then
  echo "Service installed but not started: edit /etc/fleetml-agent/config.yaml first."
else
  systemctl enable --now fleetml-agent
  systemctl --no-pager status fleetml-agent | head -5
fi
