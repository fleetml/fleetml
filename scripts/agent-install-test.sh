#!/usr/bin/env bash
# Runs deploy/agent/install.sh in throwaway Ubuntu containers, without
# systemd and with a stand-in systemctl, and checks what it leaves behind.
#   ./scripts/agent-install-test.sh [path to a linux fleetml-agent binary]
# Without an argument it uses bin/fleetml-agent-linux-<Docker's arch>
# (make build-agent).
set -euo pipefail
cd "$(dirname "$0")/.."
ARCH=$(docker version -f '{{.Server.Arch}}')
BIN=${1:-bin/fleetml-agent-linux-$ARCH}
[ -x "$BIN" ] || { echo "no agent binary at $BIN (run make build-agent)" >&2; exit 1; }
IMAGE=${IMAGE:-ubuntu:24.04}

# run <name> <script>: the script runs as root in a fresh container.
run() {
  if docker run --rm -v "$PWD/deploy/agent:/src/agent:ro" -v "$PWD/$BIN:/src/fleetml-agent:ro" "$IMAGE" bash -euo pipefail -c "$2"; then
    echo "PASS $1"
  else
    echo "FAIL $1" >&2
    exit 1
  fi
}

# shellcheck disable=SC2016 # the scripts expand inside the container

run "without systemd" '
cp -r /src/agent /tmp/agent
out=$(/tmp/agent/install.sh /src/fleetml-agent)
echo "$out"
grep -q "No systemd on this machine" <<<"$out"
grep -q "su -s /bin/sh fleetml-agent -c" <<<"$out"
! grep -q "systemctl" <<<"$out"
[ ! -e /etc/systemd/system/fleetml-agent.service ]
[ "$(stat -c "%U %a" /var/lib/fleetml-agent)" = "fleetml-agent 755" ]
sed -i -e "s/^server: .*/server: 127.0.0.1:1/" -e "s/<project enrollment key>/test-key/" /etc/fleetml-agent/config.yaml
log=$(su -s /bin/sh fleetml-agent -c "exec timeout 3 /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml" 2>&1 || true)
echo "$log"
grep -q "fleetml-agent starting" <<<"$log"
! grep -q "permission denied" <<<"$log"
[ -f /var/lib/fleetml-agent/state.json ]
'

# shellcheck disable=SC2016
run "with systemd" '
cp -r /src/agent /tmp/agent
printf "#!/bin/sh\necho \"\$*\" >> /tmp/systemctl.log\n" > /usr/local/bin/systemctl && chmod +x /usr/local/bin/systemctl
out=$(/tmp/agent/install.sh /src/fleetml-agent)
echo "$out"
grep -q "Service installed but not started" <<<"$out"
[ -f /etc/systemd/system/fleetml-agent.service ]
[ "$(cat /tmp/systemctl.log)" = "daemon-reload" ]
sed -i "s/<project enrollment key>/test-key/" /etc/fleetml-agent/config.yaml
/tmp/agent/install.sh /src/fleetml-agent
grep -qx "enable --now fleetml-agent" /tmp/systemctl.log
'
