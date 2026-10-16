#!/usr/bin/env bash
# The Windows run workflow's quickstart in WSL 2
# (.github/workflows/windows-run.yml): runs the "Run FleetML" block of
# docs/QUICKSTART.md and the tour's first block and sections 1 to 3 in one bash
# session, as a reader pastes them, each block read from the docs at run time;
# then checks what they printed against the docs' text, as scripts/e2e.sh and
# the Python live test do (COMPLETE, ROLLING_OUT, ROLLED_BACK). Run it from the
# repository root (the checkout, under /mnt); the clone and the stack live in
# the Linux file system (~/fleetml), as "On Windows" asks.
#
# While the repository is private (REPO_PRIVATE=true) the block's `git clone`
# from GitHub is replaced by a clone of this checkout. The stack is left
# running for the workflow's later steps, with its secrets in
# ~/.fleetml-run.env; the workflow ends with `docker compose ... down -v`.
set -e
CHECKOUT=$PWD
DOCS=$CHECKOUT/docs/QUICKSTART.md
OUT=$(mktemp -d)
NOTES=()

# note <text>: a line for the step's row in the job summary.
note() {
  NOTES+=("$1")
  local joined
  printf -v joined '%s; ' "${NOTES[@]}"
  [ -z "${GITHUB_OUTPUT:-}" ] || printf 'note=%s\n' "${joined%; }" >>"$GITHUB_OUTPUT"
}

# check <what> <got> <command...>: prints the check and fails unless the command succeeds.
check() {
  local what=$1 got=$2
  shift 2
  echo "Expect: $what"
  echo "Got:    $got"
  if "$@"; then echo PASS; else note "FAILED: $what"; echo "FAILED: $what"; exit 1; fi
}

# block <heading> [n]: the n-th bash block after the heading in the docs.
block() {
  tr -d '\r' <"$DOCS" | awk -v h="$1" -v n="${2:-1}" '
    $0 == h { f = 1; next }
    f && !p && /^#/ { exit }
    f && /^```bash$/ { if (++c == n) { p = 1; next } }
    p && /^```$/ { exit }
    p'
}

# run <label> <code>: prints the block, runs it in this shell and prints what it printed.
CURRENT=""
run() {
  echo "::group::$1: the block as run"
  printf '%s\n' "$2"
  echo "::endgroup::"
  CURRENT=$OUT/$1
  echo "Running $1 ..."
  eval "$2" >"$CURRENT" 2>&1
  [ -z "${FLEETML_ADMIN_KEY:-}" ] || echo "::add-mask::$FLEETML_ADMIN_KEY"
  cat "$CURRENT"
  CURRENT=""
}

on_exit() {
  local rc=$?
  [ "$rc" -eq 0 ] && return
  [ -z "${FLEETML_ADMIN_KEY:-}" ] || echo "::add-mask::$FLEETML_ADMIN_KEY"
  if [ -n "$CURRENT" ]; then echo "--- the block failed; it printed:"; cat "$CURRENT"; fi
  if [ -d ~/fleetml ]; then
    echo "--- docker compose ps and the server's last log lines"
    (cd ~/fleetml && docker compose -f deploy/docker-compose.prod.yml ps && docker compose -f deploy/docker-compose.prod.yml logs --tail 50 server) || true
  fi
  [ ! -f /tmp/fleetml-sim.log ] || { echo "--- simulator log, last lines"; tail -20 /tmp/fleetml-sim.log; }
  [ "${#NOTES[@]}" -gt 0 ] || note "FAILED (exit $rc), see the log"
}
trap on_exit EXIT

sim_fleet() { grep '^simulator: .* on ' /tmp/fleetml-sim.log 2>/dev/null | tail -1 | sed 's/^simulator: //'; }
wait_fleet() {
  for _ in $(seq 90); do [ "$(sim_fleet)" = "$1" ] && return 0; sleep 1; done
  return 1
}

# Run FleetML, from ~ as "On Windows" says.
code=$(block '## Run FleetML')
if [ "${REPO_PRIVATE:-}" = true ]; then
  code=$(printf '%s\n' "$code" | sed "s|^git clone https://github.com/fleetml/fleetml.git |git clone '$CHECKOUT' fleetml |")
  check "the block's first line clones from GitHub (replaced while private)" "$(printf '%s\n' "$code" | head -1)" \
    grep -q "^git clone '$CHECKOUT' fleetml " <<<"$code"
  # The checkout belongs to Windows (another owner as git sees it).
  git config --global --add safe.directory "$CHECKOUT"
  note "clone: from the checkout via /mnt (private repository)"
else
  note "clone: from GitHub as written"
fi
cd ~
rm -rf fleetml
run run-fleetml "$code"
(umask 077 && printf 'export JWT_SECRET=%q SIGNING_MASTER_KEY=%q FLEETML_ADMIN_KEY=%q\n' \
  "$JWT_SECRET" "$SIGNING_MASTER_KEY" "$FLEETML_ADMIN_KEY" >~/.fleetml-run.env)
ready='{"db":"connected","nats":"connected","s3":"connected","status":"ok"}'
check "/readyz answers $ready" "$(grep -o '{"db".*}' "$OUT/run-fleetml" | head -1)" grep -qF "$ready" "$OUT/run-fleetml"
check 'the project list answers {"data":[],"pagination":{...}}' "$(grep -o '{"data".*' "$OUT/run-fleetml" | head -1)" grep -q '^{"data":\[\],"pagination":' "$OUT/run-fleetml"
check "the Dashboard line" "$(grep -c '^Dashboard: http://localhost' "$OUT/run-fleetml") line(s)" grep -q '^Dashboard: http://localhost' "$OUT/run-fleetml"
note "Run FleetML: readyz all connected"

# The tour, from the repository root in the same shell.
run tour-setup "$(block '## Tour')"
check "bin/fleetml built" "$(./bin/fleetml version 2>&1)" test -x bin/fleetml

run tour-1 "$(block '### 1. Publish a model' 1)"
check "three pushes print Pushed" "$(grep -c Pushed "$OUT/tour-1") of 3" test "$(grep -c Pushed "$OUT/tour-1")" -eq 3
check "P, V1, V2 and V21 are IDs" "P=$P V1=$V1 V2=$V2 V21=$V21" test -n "$P" -a -n "$V1" -a -n "$V2" -a -n "$V21"
run tour-1-integrity "$(block '### 1. Publish a model' 2)"
check 'the finalize call answers {"code":"INTEGRITY_ERROR",...}' "$(cat "$OUT/tour-1-integrity")" grep -q '"code":"INTEGRITY_ERROR"' "$OUT/tour-1-integrity"
note "1: three pushes, INTEGRITY_ERROR on a bad sha256"

run tour-2 "$(block '### 2. A fleet and a rollout' 1)"
check "the deployment of 1.0.0 starts as COMPLETE" "$(grep -x 'COMPLETE\|PENDING\|ROLLING_OUT\|FAILED' "$OUT/tour-2" | head -1)" grep -qx COMPLETE "$OUT/tour-2"
wait_fleet "iris 1.0.0 on 20" || true
check "the simulator reads iris 1.0.0 on 20 (within 90 s)" "$(sim_fleet)" test "$(sim_fleet)" = "iris 1.0.0 on 20"
check "fleetml device list prints its table" "$(grep -m1 '^ID' "$OUT/tour-2")" grep -Eq '^ID +OS +RAM\(MB\) +Chipset' "$OUT/tour-2"
note "2: COMPLETE, iris 1.0.0 on 20"

run tour-3 "$(block '### 3. A canary that rolls itself back' 1)"
check "the canary of 2.0.0 starts as ROLLING_OUT" "$(head -1 "$OUT/tour-3")" grep -qx ROLLING_OUT "$OUT/tour-3"
status=$(api GET "/api/v1/deployments/$D2" | jq -r .data.status)
reason=$(api GET "/api/v1/deployments/$D2" | jq -r .data.rollback_reason)
check "the canary ends ROLLED_BACK" "$status" test "$status" = ROLLED_BACK
check "the rollback reason reads error rate ... exceeds threshold 5.00% ..." "$reason" grep -q 'error rate .* exceeds threshold 5.00%' <<<"$reason"
wait_fleet "iris 1.0.0 on 20" || true
check "the simulator is back to iris 1.0.0 on 20 (within 90 s)" "$(sim_fleet)" test "$(sim_fleet)" = "iris 1.0.0 on 20"
note "3: ROLLING_OUT then ROLLED_BACK on the error rate, back on 1.0.0"

# The tour's "Clean up" block also runs `down -v`; the workflow does that at its end.
kill "$SIM"
echo "Simulator stopped; the stack keeps running for the Windows-side step."
