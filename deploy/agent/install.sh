#!/usr/bin/env bash
# Installs the FleetML agent as a systemd service:
#   sudo ./deploy/agent/install.sh ./bin/fleetml-agent-linux-amd64
# Creates the fleetml-agent user, /etc/fleetml-agent/config.yaml (from the
# example, if missing) and the service, and starts it once the config has a
# server and an enrollment key. On a machine without systemd it installs the
# same files without the unit, creates the state folder the unit would have
# created, and prints how to run the agent under the machine's own supervisor.
set -euo pipefail
BIN=${1:?usage: install.sh <path to the fleetml-agent binary for this machine>}
HERE=$(cd "$(dirname "$0")" && pwd)
[ "$(id -u)" = 0 ] || { echo "run as root (sudo)" >&2; exit 1; }
CONFIG=/etc/fleetml-agent/config.yaml
RUN="/usr/local/bin/fleetml-agent -config $CONFIG"

install -m 0755 "$BIN" /usr/local/bin/fleetml-agent
id fleetml-agent >/dev/null 2>&1 || useradd --system --home-dir /var/lib/fleetml-agent --shell /usr/sbin/nologin fleetml-agent
install -d -m 0755 /etc/fleetml-agent
if [ ! -f "$CONFIG" ]; then
  install -m 0640 -g fleetml-agent "$HERE/config.example.yaml" "$CONFIG"
  echo "Wrote $CONFIG: set server, enrollment_key and tls (false for a server without TLS)."
fi

if ! command -v systemctl >/dev/null 2>&1; then
  # The unit's StateDirectory= would create this; without systemd we do.
  install -d -m 0755 -o fleetml-agent -g fleetml-agent /var/lib/fleetml-agent
  echo "No systemd on this machine: the service unit was not installed, and /var/lib/fleetml-agent was created for the fleetml-agent user."
  echo "Once $CONFIG is set, run the agent as that user under this machine's supervisor (runit, s6, OpenRC, supervisord, a container's entrypoint), for example:"
  echo "  su -s /bin/sh fleetml-agent -c '$RUN'"
  exit 0
fi

install -m 0644 "$HERE/fleetml-agent.service" /etc/systemd/system/fleetml-agent.service
systemctl daemon-reload
if grep -q '<project enrollment key>' "$CONFIG"; then
  echo "Service installed but not started: edit $CONFIG first, then run: systemctl enable --now fleetml-agent"
else
  systemctl enable --now fleetml-agent
  systemctl --no-pager status fleetml-agent | head -5
fi
