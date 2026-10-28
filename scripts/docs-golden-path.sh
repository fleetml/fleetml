#!/usr/bin/env bash
# Runs the docs' golden path: every fenced bash block marked
#   <!-- golden: <id> [manual] [device] [phone] [cleanup] -->
# in the given Markdown files, in order, in one shell per file (so `export`,
# `cd` and variables carry from block to block, as for a reader who pastes
# them into one terminal), and checks each block's "You should see" line: the
# first line after the block that starts with "You should see" lists one or
# more `code spans`, and every span must appear in what the block printed,
# with `<placeholder>` matching anything.
#
#   manual   needs a person (a browser, the clipboard): never run here.
#   device   needs a simulator, an emulator or a phone: run only with --devices.
#   phone    needs a physical phone (an iPhone): run only with --phones.
#   cleanup  also run after a failure or a timeout, so nothing is left behind.
#
# Usage: scripts/docs-golden-path.sh [--local-release DIR] [--local-python]
#                                    [--devices] [--phones] [--timeout SECONDS] [--verbose] FILE...
#
# The blocks run as written except for three substitutions, so CI tests the
# tree under test rather than the last release:
#   [ -f deploy/docker-compose.prod.yml ] || { [ -d fleetml ] || git clone https://github.com/fleetml/fleetml.git; cd fleetml; }
#       -> cd <this checkout>
#   https://github.com/fleetml/fleetml/releases/latest/download/
#       -> file://<DIR>/ with --local-release DIR (from OUT=DIR make release-binaries)
#   pip install fleetml   -> pip install <this checkout>/python with --local-python
# HOME is a new empty directory for each file (so ~/.fleetml.env, ~/.local/bin
# and the CLI's config are the run's own); DOCKER_CONFIG keeps the real one,
# and with --devices the run's HOME links the real Android SDK and adb keys
# (Library/Android, .android), the Gradle cache (.gradle) and Xcode's
# simulators (Library/Developer), which the device blocks need.
# --verbose prints what every block printed, not only the failing ones.
# Run it from the repository root. Secrets the blocks create are replaced by
# <redacted> in everything this script prints.
set -u

ROOT=$(pwd)
DIST="" LOCAL_PY="" DEVICES="" PHONES="" VERBOSE="" TIMEOUT=900 FILES=()
while [ $# -gt 0 ]; do
  case "$1" in
    --local-release) DIST=$(cd "$2" && pwd); shift 2 ;;
    --local-python) LOCAL_PY=1; shift ;;
    --devices) DEVICES=1; shift ;;
    --phones) PHONES=1; DEVICES=1; shift ;;
    --timeout) TIMEOUT=$2; shift 2 ;;
    --verbose) VERBOSE=1; shift ;;
    -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
    *) FILES+=("$1"); shift ;;
  esac
done
[ ${#FILES[@]} -gt 0 ] || { echo "usage: $0 [--local-release DIR] [--local-python] [--devices] [--phones] [--timeout S] [--verbose] FILE..." >&2; exit 2; }
[ -f deploy/docker-compose.prod.yml ] || { echo "run it from the repository root" >&2; exit 2; }

WORK=$(mktemp -d "${TMPDIR:-/tmp}/fleetml-golden.XXXXXX")
REAL_HOME=$HOME
export DOCKER_CONFIG=${DOCKER_CONFIG:-$REAL_HOME/.docker}
FAILED=0

# extract FILE DIR: one NN.sh, NN.id, NN.flags and NN.expect per golden block.
extract() {
  awk -v dir="$2" '
    function flush() { close(f ".sh"); close(f ".expect") }
    function strip(l) { return substr(l, 1, length(ind)) == ind ? substr(l, length(ind) + 1) : l }
    /^ *<!-- golden: / {
      line = $0; sub(/^ *<!-- golden: /, "", line); sub(/ *-->.*$/, "", line)
      n++; f = sprintf("%s/%02d", dir, n)
      split(line, w, " "); print w[1] > (f ".id"); close(f ".id")
      flags = ""; for (i = 2; i in w; i++) flags = flags w[i] " "
      print flags > (f ".flags"); close(f ".flags")
      state = "fence"; next
    }
    state == "fence" && /^ *```/ { match($0, /^ */); ind = substr($0, 1, RLENGTH); state = "code"; next }
    state == "code" && /^ *```$/ { state = "after"; next }
    state == "code" { print strip($0) > (f ".sh"); next }
    state == "after" && /^ *You should see/ {
      s = $0
      while (match(s, /`[^`]+`/)) { print substr(s, RSTART + 1, RLENGTH - 2) > (f ".expect"); s = substr(s, RSTART + RLENGTH) }
      flush(); state = ""; next
    }
    state == "after" && NF { flush(); state = "" }
  ' "$1"
}

# substitute FILE: the three substitutions, in place.
substitute() {
  local repl_dist=""
  [ -z "$DIST" ] || repl_dist="file://$DIST/"
  perl -pi -e '
    BEGIN { $root = shift @ARGV; $dist = shift @ARGV; $py = shift @ARGV }
    s{^\[ -f deploy/docker-compose\.prod\.yml \] \|\| \{ \[ -d fleetml \] \|\| git clone https://github\.com/fleetml/fleetml\.git; cd fleetml; \}\s*$}{cd "$root"\n};
    s{https://github\.com/fleetml/fleetml/releases/latest/download/}{$dist}g if $dist ne "";
    s{\bpip install fleetml\b(?![-=.\w])}{pip install "$root/python"}g if $py ne "";
  ' "$ROOT" "$repl_dist" "$LOCAL_PY" "$1"
}

# matches OUT EXPECT: every expectation (a line of EXPECT) appears in OUT; a run
# of spaces or tabs matches any run (Compose prints one or two before `Removed`).
matches() {
  perl -e '
    local $/; open my $o, "<", $ARGV[0] or exit 1; my $out = <$o>;
    open my $e, "<", $ARGV[1] or exit 0;
    for my $want (split /\n/, <$e>) {
      next if $want eq "";
      my $re = join(".+?", map { my $p = quotemeta; $p =~ s/(?:\\[ \t])+/\\s+/g; $p } split /<[^<>]+>/, $want, -1);
      if ($out !~ /$re/s) { print "  missing: $want\n"; $bad = 1 }
    }
    exit($bad ? 1 : 0);
  ' "$1" "$2"
}

# redact FILE...: the run's secrets replaced by <redacted>.
redact() {
  perl -pe '
    BEGIN { open my $s, "<", shift @ARGV; @s = grep { length > 7 } map { chomp; $_ } <$s> }
    for my $v (@s) { s/\Q$v\E/<redacted>/g }
  ' "$SECRETS" "$@"
}

run_file() {
  local doc=$1 name dir home n id flags
  name=$(basename "$doc" .md)
  dir=$WORK/$name; home=$WORK/$name-home
  mkdir -p "$dir" "$home"
  if [ -n "$DEVICES" ]; then
    local link
    for link in Library/Android Library/Developer .android .gradle; do
      [ -e "$REAL_HOME/$link" ] || continue
      mkdir -p "$home/$(dirname "$link")"; ln -s "$REAL_HOME/$link" "$home/$link"
    done
  fi
  SECRETS=$WORK/$name.secrets; : > "$SECRETS"; chmod 600 "$SECRETS"
  extract "$doc" "$dir"
  local blocks=() cleanup=()
  for n in "$dir"/*.id; do
    [ -e "$n" ] || continue
    n=${n%.id}; flags=$(cat "$n.flags")
    [ -f "$n.sh" ] || { echo "FAIL $doc: golden block $(cat "$n.id") has no bash block after its marker"; FAILED=1; continue; }
    substitute "$n.sh"
    case " $flags " in *" manual "*) continue ;; esac
    case " $flags " in *" device "*) [ -n "$DEVICES" ] || continue ;; esac
    case " $flags " in *" phone "*) [ -n "$PHONES" ] || continue ;; esac
    blocks+=("$n")
    case " $flags " in *" cleanup "*) cleanup+=("$n") ;; esac
  done
  echo "== $doc: ${#blocks[@]} golden blocks to run"
  [ ${#blocks[@]} -gt 0 ] || return 0

  # One shell for the file: each block sourced in turn, its output kept.
  {
    echo "cd \"$ROOT\" || exit 1"
    for n in "${blocks[@]}"; do
      echo "{ . \"$n.sh\"; } > \"$n.out\" 2>&1 < /dev/null; echo \$? > \"$n.rc\""
      # shellcheck disable=SC2016 # expanded by the driver, not here
      printf 'printf "%%s\\n" "${JWT_SECRET:-}" "${SIGNING_MASTER_KEY:-}" "${FLEETML_ADMIN_KEY:-}" "${FLEETML_API_KEY:-}" "${FLEETML_ENROLLMENT_KEY:-}" "${EK:-}" >> "%s"\n' "$SECRETS"
      # The shell's exports and directory, for a clean-up block run after a failure.
      # shellcheck disable=SC2016 # expanded by the driver, not here
      printf '{ export -p; printf "cd %%q\\n" "$PWD"; } > "%s"\n' "$dir/state.sh"
    done
  } > "$dir/driver.sh"
  HOME=$home perl -e '
    my $t = shift; my $pid = fork;
    if ($pid == 0) { setpgrp(0, 0); exec @ARGV or exit 127 }
    $SIG{ALRM} = sub { kill "TERM", -$pid; sleep 2; kill "KILL", -$pid; print STDERR "timed out after ${t}s\n"; exit 124 };
    alarm $t; waitpid $pid, 0; exit($? >> 8);
  ' "$TIMEOUT" bash "$dir/driver.sh"
  local driver_rc=$?

  local failed_here=0
  for n in "${blocks[@]}"; do
    id=$(cat "$n.id")
    if [ ! -f "$n.rc" ]; then
      echo "  FAIL $id: not reached (timed out or stopped earlier)"; failed_here=1; continue
    fi
    if matches "$n.out" "$n.expect" > "$n.why"; then
      echo "  ok   $id"
      [ -z "$VERBOSE" ] || redact "$n.out" | sed 's/^/  | /'
    else
      echo "  FAIL $id (exit $(cat "$n.rc"))"; redact "$n.why"
      echo "  --- what it printed:"; redact "$n.out" | sed 's/^/  | /'
      failed_here=1
    fi
  done
  if [ "$driver_rc" -ne 0 ] || [ "$failed_here" -ne 0 ]; then
    FAILED=1
    for n in "${cleanup[@]+"${cleanup[@]}"}"; do
      [ -f "$n.rc" ] && continue
      echo "  running $(cat "$n.id") after the failure"
      (cd "$ROOT" && HOME=$home bash -c '[ ! -f "$1" ] || . "$1"; . "$2"' _ "$dir/state.sh" "$n.sh" > "$n.out" 2>&1 < /dev/null) || true
    done
  fi
  rm -f "$SECRETS"
}

for doc in "${FILES[@]}"; do run_file "$doc"; done
if [ "$FAILED" -ne 0 ]; then echo "golden path: FAILED (each block's output is kept in $WORK)"; exit 1; fi
rm -rf "$WORK"
echo "golden path: every block printed what its doc says"
