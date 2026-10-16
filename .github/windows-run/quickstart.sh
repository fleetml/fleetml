#!/usr/bin/env bash
# The Windows qs_run workflow's quickstart in WSL 2
# (.github/workflows/windows-run.yml): runs the "Run FleetML" qs_block of
# docs/QUICKSTART.md and the tour's first qs_block and sections 1 to 3 in one bash
# session, as a reader pastes them, each qs_block read from the docs at qs_run time;
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
# This script's own names start with QS_ or qs_: the docs' blocks qs_run in the
# same shell and set P, M, OUT, SIM, D1, D2 and others.
QS_CHECKOUT=$PWD
QS_DOCS=$QS_CHECKOUT/docs/QUICKSTART.md
QS_OUT=$(mktemp -d)
QS_NOTES=()

# qs_note <text>: a line for the step's row in the job summary.
qs_note() {
  QS_NOTES+=("$1")
  local joined
  printf -v joined '%s; ' "${QS_NOTES[@]}"
  [ -z "${GITHUB_OUTPUT:-}" ] || printf 'note=%s\n' "${joined%; }" >>"$GITHUB_OUTPUT"
}

# qs_check <what> <got> <command...>: prints the check and fails unless the command succeeds.
qs_check() {
  local what=$1 got=$2
  shift 2
  echo "Expect: $what"
  echo "Got:    $got"
  if "$@"; then echo PASS; else qs_note "FAILED: $what"; echo "FAILED: $what"; exit 1; fi
}

# qs_block <heading> [n]: the n-th bash block after the heading in the docs.
qs_block() {
  tr -d '\r' <"$QS_DOCS" | awk -v h="$1" -v n="${2:-1}" '
    $0 == h { f = 1; next }
    f && !p && /^#/ { exit }
    f && /^```bash$/ { if (++c == n) { p = 1; next } }
    p && /^```$/ { exit }
    p'
}

# qs_run <label> <code>: prints the block, runs it in this shell and prints what it printed.
QS_CURRENT=""
QS_LABEL=""
qs_run() {
  echo "::group::$1: the block as run"
  printf '%s\n' "$2"
  echo "::endgroup::"
  QS_LABEL=$1
  QS_CURRENT=$QS_OUT/$1
  echo "Running $1 ..."
  eval "$2" >"$QS_CURRENT" 2>&1
  [ -z "${FLEETML_ADMIN_KEY:-}" ] || echo "::add-mask::$FLEETML_ADMIN_KEY"
  cat "$QS_CURRENT"
  QS_CURRENT=""
}

qs_on_exit() {
  local rc=$?
  [ "$rc" -eq 0 ] && return
  [ -z "${FLEETML_ADMIN_KEY:-}" ] || echo "::add-mask::$FLEETML_ADMIN_KEY"
  if [ -n "$QS_CURRENT" ]; then echo "--- the block $QS_LABEL failed; it printed:"; cat "$QS_CURRENT"; fi
  if [ -d ~/fleetml ]; then
    echo "--- docker compose ps -a, then the last log lines of migrate, postgres and server"
    (cd ~/fleetml && docker compose -f deploy/docker-compose.prod.yml ps -a && docker compose -f deploy/docker-compose.prod.yml logs --tail 50 migrate postgres server) || true
    echo "--- the migrations the migrate container sees"
    echo "$(find ~/fleetml/internal/db/migrations -name '*.sql' 2>/dev/null | wc -l) .sql files in ~/fleetml/internal/db/migrations" 
  fi
  [ ! -f /tmp/fleetml-sim.log ] || { echo "--- simulator log, last lines"; tail -20 /tmp/fleetml-sim.log; }
  case "${QS_NOTES[*]: -1}" in
    FAILED*) ;;
    *) qs_note "FAILED (exit $rc)${QS_LABEL:+ in $QS_LABEL}, see the log" ;;
  esac
}
trap qs_on_exit EXIT

qs_sim_fleet() { grep '^simulator: .* on ' /tmp/fleetml-sim.log 2>/dev/null | tail -1 | sed 's/^simulator: //'; }
qs_wait_fleet() {
  for _ in $(seq 90); do [ "$(qs_sim_fleet)" = "$1" ] && return 0; sleep 1; done
  return 1
}

# Run FleetML, from ~ as "On Windows" says.
qs_code=$(qs_block '## Run FleetML')
if [ "${REPO_PRIVATE:-}" = true ]; then
  qs_code=$(printf '%s\n' "$qs_code" | sed "s|^git clone https://github.com/fleetml/fleetml.git |git clone '$QS_CHECKOUT' fleetml |")
  qs_check "the block's first line clones from GitHub (replaced while private)" "$(printf '%s\n' "$qs_code" | head -1)" \
    grep -q "^git clone '$QS_CHECKOUT' fleetml " <<<"$qs_code"
  # The checkout belongs to Windows (another owner as git sees it).
  git config --global --add safe.directory "$QS_CHECKOUT"
  qs_note "clone: from the checkout via /mnt (private repository)"
else
  qs_note "clone: from GitHub as written"
fi
cd ~
rm -rf fleetml
qs_run run-fleetml "$qs_code"
(umask 077 && printf 'export JWT_SECRET=%q SIGNING_MASTER_KEY=%q FLEETML_ADMIN_KEY=%q\n' \
  "$JWT_SECRET" "$SIGNING_MASTER_KEY" "$FLEETML_ADMIN_KEY" >~/.fleetml-run.env)
ready='{"db":"connected","nats":"connected","s3":"connected","status":"ok"}'
qs_check "/readyz answers $ready" "$(grep -o '{"db".*}' "$QS_OUT/run-fleetml" | head -1)" grep -qF "$ready" "$QS_OUT/run-fleetml"
qs_check 'the project list answers {"data":[],"pagination":{...}}' "$(grep -o '{"data".*' "$QS_OUT/run-fleetml" | head -1)" grep -q '^{"data":\[\],"pagination":' "$QS_OUT/run-fleetml"
qs_check "the Dashboard line" "$(grep -c '^Dashboard: http://localhost' "$QS_OUT/run-fleetml") line(s)" grep -q '^Dashboard: http://localhost' "$QS_OUT/run-fleetml"
qs_note "Run FleetML: readyz all connected"

# The tour, from the repository root in the same shell.
qs_run tour-setup "$(qs_block '## Tour')"
qs_check "bin/fleetml built" "$(./bin/fleetml version 2>&1)" test -x bin/fleetml

qs_run tour-1 "$(qs_block '### 1. Publish a model' 1)"
qs_check "three pushes print Pushed" "$(grep -c Pushed "$QS_OUT/tour-1") of 3" test "$(grep -c Pushed "$QS_OUT/tour-1")" -eq 3
qs_check "P, V1, V2 and V21 are IDs" "P=$P V1=$V1 V2=$V2 V21=$V21" test -n "$P" -a -n "$V1" -a -n "$V2" -a -n "$V21"
qs_run tour-1-integrity "$(qs_block '### 1. Publish a model' 2)"
qs_check 'the finalize call answers {"code":"INTEGRITY_ERROR",...}' "$(cat "$QS_OUT/tour-1-integrity")" grep -q '"code":"INTEGRITY_ERROR"' "$QS_OUT/tour-1-integrity"
qs_note "1: three pushes, INTEGRITY_ERROR on a bad sha256"

qs_run tour-2 "$(qs_block '### 2. A fleet and a rollout' 1)"
qs_check "the deployment of 1.0.0 starts as COMPLETE" "$(grep -x 'COMPLETE\|PENDING\|ROLLING_OUT\|FAILED' "$QS_OUT/tour-2" | head -1)" grep -qx COMPLETE "$QS_OUT/tour-2"
qs_wait_fleet "iris 1.0.0 on 20" || true
qs_check "the simulator reads iris 1.0.0 on 20 (within 90 s)" "$(qs_sim_fleet)" test "$(qs_sim_fleet)" = "iris 1.0.0 on 20"
qs_check "fleetml device list prints its table" "$(grep -m1 '^ID' "$QS_OUT/tour-2")" grep -Eq '^ID +OS +RAM\(MB\) +Chipset' "$QS_OUT/tour-2"
qs_note "2: COMPLETE, iris 1.0.0 on 20"

qs_run tour-3 "$(qs_block '### 3. A canary that rolls itself back' 1)"
qs_check "the canary of 2.0.0 starts as ROLLING_OUT" "$(head -1 "$QS_OUT/tour-3")" grep -qx ROLLING_OUT "$QS_OUT/tour-3"
qs_status=$(api GET "/api/v1/deployments/$D2" | jq -r .data.status)
qs_reason=$(api GET "/api/v1/deployments/$D2" | jq -r .data.rollback_reason)
qs_check "the canary ends ROLLED_BACK" "$qs_status" test "$qs_status" = ROLLED_BACK
qs_check "the rollback reason reads error rate ... exceeds threshold 5.00% ..." "$qs_reason" grep -q 'error rate .* exceeds threshold 5.00%' <<<"$qs_reason"
qs_wait_fleet "iris 1.0.0 on 20" || true
qs_check "the simulator is back to iris 1.0.0 on 20 (within 90 s)" "$(qs_sim_fleet)" test "$(qs_sim_fleet)" = "iris 1.0.0 on 20"
qs_note "3: ROLLING_OUT then ROLLED_BACK on the error rate, back on 1.0.0"

# The tour's "Clean up" qs_block also runs `down -v`; the workflow does that at its end.
kill "$SIM"
echo "Simulator stopped; the stack keeps running for the Windows-side step."
