#!/bin/sh
# Installs the FleetML agent as a systemd service:
#   sudo ./deploy/agent/install.sh ./bin/fleetml-agent-linux-amd64
# Creates the fleetml-agent user, /etc/fleetml-agent/config.yaml (from the
# example, if missing) and the service, and starts it once the config has a
# server and an enrollment key; run again (to upgrade the binary), it keeps
# the config and restarts a running service. On a machine where systemd is not running
# (none installed, or the package in a container or chroot where it is not
# PID 1) it installs the same files without the unit, creates the state
# folder the unit would have created, and prints how to run the agent under
# the machine's own supervisor. POSIX sh, so that it also runs on BusyBox
# images (ash, adduser, install) that have no bash or useradd.
set -eu
BIN=${1:?usage: install.sh <path to the fleetml-agent binary for this machine>}
HERE=$(cd "$(dirname "$0")" && pwd)
[ "$(id -u)" = 0 ] || { echo "run as root (sudo)" >&2; exit 1; }
CONFIG=/etc/fleetml-agent/config.yaml
RUN="/usr/local/bin/fleetml-agent -config $CONFIG"

install -m 0755 "$BIN" /usr/local/bin/fleetml-agent
if ! id fleetml-agent >/dev/null 2>&1; then
  if command -v useradd >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/fleetml-agent --shell /usr/sbin/nologin fleetml-agent
  else
    # BusyBox: adduser -S without -G would put the user in nogroup.
    addgroup -S fleetml-agent
    adduser -S -D -H -h /var/lib/fleetml-agent -s /sbin/nologin -G fleetml-agent fleetml-agent
  fi
fi
install -d -m 0755 /etc/fleetml-agent
if [ ! -f "$CONFIG" ]; then
  install -m 0640 -g fleetml-agent "$HERE/config.example.yaml" "$CONFIG"
  echo "Wrote $CONFIG: set server, enrollment_key and tls (false for a server without TLS)."
fi

# systemd creates /run/systemd/system when it boots as PID 1; systemctl on
# the PATH only means the package is installed.
if [ ! -d /run/systemd/system ]; then
  # The unit's StateDirectory= would create this; without systemd we do.
  install -d -m 0755 -o fleetml-agent -g fleetml-agent /var/lib/fleetml-agent
  echo "No running systemd on this machine: the service unit was not installed, and /var/lib/fleetml-agent was created for the fleetml-agent user."
  echo "Once $CONFIG is set, run the agent as that user under this machine's supervisor (runit, s6, OpenRC, supervisord, a container's entrypoint), for example:"
  if setpriv --help 2>&1 | grep -q -- --reuid; then
    # util-linux setpriv execs the agent as the user, so the supervisor's
    # SIGTERM reaches it (su forks, and SIGKILLs its child 2 s after a SIGTERM).
    echo "  exec setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups $RUN"
  else
    # BusyBox has no setpriv --reuid; its su execs the shell, which execs the agent.
    echo "  exec su -s /bin/sh fleetml-agent -c 'exec $RUN'"
  fi
  echo "If an agent is running, restart it under the supervisor to use the new binary."
  exit 0
fi

install -m 0644 "$HERE/fleetml-agent.service" /etc/systemd/system/fleetml-agent.service
systemctl daemon-reload
if grep -q '<project enrollment key>' "$CONFIG"; then
  echo "Service installed but not started: edit $CONFIG first, then run: systemctl enable --now fleetml-agent"
else
  # restart, not enable --now: on a re-run the old binary may still be running.
  systemctl enable fleetml-agent
  systemctl restart fleetml-agent
  systemctl --no-pager status fleetml-agent | head -5
fi
