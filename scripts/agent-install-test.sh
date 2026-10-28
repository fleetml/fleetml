#!/usr/bin/env bash
# Runs deploy/agent/install.sh in throwaway Ubuntu containers, without
# systemd, with systemd installed but not running, and with a stand-in for a
# running systemd, and in an Alpine (BusyBox) container without bash or
# useradd, and checks what it leaves behind.
#   ./scripts/agent-install-test.sh [path to a linux fleetml-agent binary]
# Without an argument it uses bin/fleetml-agent-linux-<Docker's arch>
# (make build-agent).
set -euo pipefail
cd "$(dirname "$0")/.."
ARCH=$(docker version -f '{{.Server.Arch}}')
BIN=${1:-bin/fleetml-agent-linux-$ARCH}
[ -x "$BIN" ] || { echo "no agent binary at $BIN (run make build-agent)" >&2; exit 1; }
IMAGE=${IMAGE:-ubuntu:24.04}
BUSYBOX_IMAGE=${BUSYBOX_IMAGE:-alpine:3.20}

# run <name> <script> [image shell...]: the script runs as root in a fresh
# container, by default in $IMAGE with bash.
run() {
  name=$1 script=$2
  shift 2
  [ $# -gt 0 ] || set -- "$IMAGE" bash -euo pipefail
  if docker run --rm -v "$PWD/deploy/agent:/src/agent:ro" -v "$PWD/$BIN:/src/fleetml-agent:ro" "$@" -c "$script"; then
    echo "PASS $name"
  else
    echo "FAIL $name" >&2
    exit 1
  fi
}

# shellcheck disable=SC2016 # the scripts expand inside the container

run "without systemd" '
cp -r /src/agent /tmp/agent
out=$(/tmp/agent/install.sh /src/fleetml-agent)
echo "$out"
grep -q "No running systemd on this machine" <<<"$out"
grep -q "exec setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml" <<<"$out"
! grep -q "systemctl" <<<"$out"
[ ! -e /etc/systemd/system/fleetml-agent.service ]
[ "$(stat -c "%U %a" /var/lib/fleetml-agent)" = "fleetml-agent 755" ]
sed -i -e "s/^server: .*/server: 127.0.0.1:1/" -e "s/<project enrollment key>/test-key/" /etc/fleetml-agent/config.yaml
log=$(su -s /bin/sh fleetml-agent -c "exec timeout 3 /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml" 2>&1 || true)
echo "$log"
grep -q "fleetml-agent starting" <<<"$log"
! grep -q "permission denied" <<<"$log"
[ -f /var/lib/fleetml-agent/state.json ]
# The printed command, as a supervisor runs it: its SIGTERM reaches the agent,
# which stops by itself (su forked and SIGKILLed it after 2 s; without an
# inner exec it killed only its shell and the agent ran on).
agents() { for p in /proc/[0-9]*; do tr "\0" " " <"$p/cmdline" 2>/dev/null | grep -q "^/usr/local/bin/fleetml-agent " && echo "$p"; done; true; }
cmd=$(grep "^  exec setpriv " <<<"$out" | sed "s/^  //")
sh -c "$cmd" >/tmp/agent.log 2>&1 &
pid=$!
for _ in $(seq 20); do [ -n "$(agents)" ] && break; sleep 0.25; done
[ -n "$(agents)" ]
kill -TERM "$pid"
for _ in $(seq 40); do [ -z "$(agents)" ] && break; sleep 0.25; done
[ -z "$(agents)" ]
'

# The systemd package without systemd as PID 1 (a Docker image derived from
# one with systemd, a chroot): systemctl is there but cannot reach systemd.
# shellcheck disable=SC2016
run "systemd installed but not running" '
cp -r /src/agent /tmp/agent
printf "#!/bin/sh\necho \"\$*\" >> /tmp/systemctl.log\necho \"System has not been booted with systemd as init system (PID 1). Can'"'"'t operate.\" >&2\nexit 1\n" > /usr/local/bin/systemctl && chmod +x /usr/local/bin/systemctl
[ ! -d /run/systemd/system ]
out=$(/tmp/agent/install.sh /src/fleetml-agent)
echo "$out"
grep -q "No running systemd on this machine" <<<"$out"
grep -q "exec setpriv --reuid=fleetml-agent" <<<"$out"
[ ! -e /etc/systemd/system/fleetml-agent.service ]
[ ! -e /tmp/systemctl.log ]
[ "$(stat -c "%U %a" /var/lib/fleetml-agent)" = "fleetml-agent 755" ]
'

# shellcheck disable=SC2016
run "with systemd" '
cp -r /src/agent /tmp/agent
mkdir -p /run/systemd/system
printf "#!/bin/sh\necho \"\$*\" >> /tmp/systemctl.log\n" > /usr/local/bin/systemctl && chmod +x /usr/local/bin/systemctl
out=$(/tmp/agent/install.sh /src/fleetml-agent)
echo "$out"
grep -q "Service installed but not started" <<<"$out"
[ -f /etc/systemd/system/fleetml-agent.service ]
[ "$(cat /tmp/systemctl.log)" = "daemon-reload" ]
sed -i "s/<project enrollment key>/test-key/" /etc/fleetml-agent/config.yaml
/tmp/agent/install.sh /src/fleetml-agent
grep -qx "enable fleetml-agent" /tmp/systemctl.log
grep -qx "restart fleetml-agent" /tmp/systemctl.log
# A re-run (an upgrade) restarts the running service with the new binary.
: > /tmp/systemctl.log
/tmp/agent/install.sh /src/fleetml-agent
[ "$(grep -c "restart fleetml-agent" /tmp/systemctl.log)" = 1 ]
# A unit edited by hand survives an upgrade; the shipped one goes next to it.
sed -i "s|^ProtectHome=true|ReadWritePaths=/srv/models\nProtectHome=true|" /etc/systemd/system/fleetml-agent.service
out=$(/tmp/agent/install.sh /src/fleetml-agent)
echo "$out"
grep -q "Kept /etc/systemd/system/fleetml-agent.service: it was edited" <<<"$out"
grep -qx "ReadWritePaths=/srv/models" /etc/systemd/system/fleetml-agent.service
cmp -s /src/agent/fleetml-agent.service /etc/systemd/system/fleetml-agent.service.new
'

# BusyBox: install.sh runs under ash and creates the user with addgroup and
# adduser. The script below is POSIX sh too.
# shellcheck disable=SC2016
run "busybox (alpine, no bash, no useradd)" '
! command -v bash
! command -v useradd
cp -r /src/agent /tmp/agent
out=$(/tmp/agent/install.sh /src/fleetml-agent)
echo "$out"
echo "$out" | grep -q "No running systemd on this machine"
# BusyBox setpriv has no --reuid: the su form, whose su execs the shell.
echo "$out" | grep -q "exec su -s /bin/sh fleetml-agent -c '"'"'exec /usr/local/bin/fleetml-agent"
[ "$(id -gn fleetml-agent)" = fleetml-agent ]
[ "$(stat -c "%U %G %a" /var/lib/fleetml-agent)" = "fleetml-agent fleetml-agent 755" ]
[ "$(stat -c "%G %a" /etc/fleetml-agent/config.yaml)" = "fleetml-agent 640" ]
sed -i -e "s/^server: .*/server: 127.0.0.1:1/" -e "s/<project enrollment key>/test-key/" /etc/fleetml-agent/config.yaml
log=$(su -s /bin/sh fleetml-agent -c "exec timeout 3 /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml" 2>&1 || true)
echo "$log"
echo "$log" | grep -q "fleetml-agent starting"
! echo "$log" | grep -q "permission denied"
[ -f /var/lib/fleetml-agent/state.json ]
' "$BUSYBOX_IMAGE" sh -eu
