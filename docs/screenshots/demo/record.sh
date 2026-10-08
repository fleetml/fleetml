#!/usr/bin/env bash
# Regenerates docs/screenshots/demo.gif: the terminal of scripts/demo-agent.sh
# (recorded with vhs) followed by two dashboard screenshots (Playwright with
# the local Chrome), joined with ffmpeg. Needs everything demo-agent.sh needs,
# plus vhs, ffmpeg, Node and Google Chrome. Run from anywhere.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE/../../.."
TMP=$(mktemp -d -t fleetml-gif.XXXXXX)
PREVIEW_PORT=${PREVIEW_PORT:-4174}
trap 'rc=$?; touch "$TMP/info.done" 2>/dev/null; kill ${VHS_PID:-} ${PREVIEW_PID:-} 2>/dev/null; rm -rf "$TMP"; exit $rc' EXIT

if curl -s -o /dev/null "localhost:$PREVIEW_PORT"; then echo "port $PREVIEW_PORT is in use; set PREVIEW_PORT" >&2; exit 1; fi
(cd dashboard && npm ci --silent && npx vite build --outDir "$TMP/dist" >/dev/null)
(cd "$HERE" && npm ci --silent)

# 1. The terminal; demo-agent.sh writes $TMP/info after the story and waits.
DEMO_INFO="$TMP/info" vhs -q -o "$TMP/terminal.mp4" "$HERE/demo.tape" > "$TMP/vhs.log" 2>&1 &
VHS_PID=$!
until [ -f "$TMP/info" ]; do kill -0 "$VHS_PID" 2>/dev/null || { cat "$TMP/vhs.log" >&2; exit 1; }; sleep 1; done

# 2. The dashboard, served on its own origin with /api proxied to the demo server.
. "$TMP/info"
(cd dashboard && FLEETML_API="$API" exec node_modules/.bin/vite preview --outDir "$TMP/dist" --port "$PREVIEW_PORT" --strictPort >/dev/null 2>&1) &
PREVIEW_PID=$!
until curl -sf "localhost:$PREVIEW_PORT/login" >/dev/null; do sleep 0.5; done
node "$HERE/dashboard.mjs" "http://localhost:$PREVIEW_PORT" "$KEY" "$V2" "$TMP"
touch "$TMP/info.done"
wait "$VHS_PID"

# 3. Terminal, then the screenshots (8 and 6 seconds), as one GIF (10 fps, 960 px).
ffmpeg -loglevel error -y -i "$TMP/terminal.mp4" -loop 1 -t 8 -i "$TMP/1-deployments.png" -loop 1 -t 6 -i "$TMP/2-telemetry.png" \
  -filter_complex "[0:v]fps=10,scale=960:-2:flags=lanczos,setsar=1[t];[1:v]fps=10,scale=960:-2:flags=lanczos,setsar=1[d];[2:v]fps=10,scale=1280:-2:flags=lanczos,pad=1280:760:0:(oh-ih)/2:white,scale=960:-2:flags=lanczos,setsar=1[m];[t][d][m]concat=n=3:v=1:a=0,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
  docs/screenshots/demo.gif
ls -la docs/screenshots/demo.gif
