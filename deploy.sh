#!/usr/bin/env bash
# deploy.sh - deploy the OpenClaw chat-sandbox MVP.
#
# Steps:
#   1. remove all existing sandbox containers + their data dirs
#   2. pull the OpenClaw image (ghcr.io; CN mirrors as fallback)
#   3. obtain the DeepSeek API key (env / build/.env / interactive prompt)
#   4. compile the backend
#   5. restart the backend
#   6. health check: send one chat message
#
# Usage:
#   ./deploy.sh                        full deploy (asks for the API key if needed)
#   DEEPSEEK_API_KEY=sk-xxx ./deploy.sh
#   ./deploy.sh --no-pull              skip the image pull (already pulled)
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p build

PULL=1
if [ "${1:-}" = "--no-pull" ]; then PULL=0; fi

echo "==> [1/6] removing old sandbox containers + data"
docker rm -f $(docker ps -aq --filter name=sbx-) 2>/dev/null || true
rm -rf build/data-*

if [ "$PULL" -eq 1 ]; then
  echo "==> [2/6] pulling OpenClaw image (ghcr.io, CN mirrors as fallback)"
  if ! docker pull ghcr.io/openclaw/openclaw:latest; then
    MIRROR=""
    for m in ghcr.nju.edu.cn ghcr.m.daocloud.io; do
      if docker pull "$m/openclaw/openclaw:latest"; then MIRROR="$m"; break; fi
    done
    [ -n "$MIRROR" ] || { echo "error: could not pull the OpenClaw image from any registry"; exit 1; }
    docker tag "$MIRROR/openclaw/openclaw:latest" ghcr.io/openclaw/openclaw:latest
    echo "   pulled via mirror: $MIRROR"
  fi
else
  echo "==> [2/6] skipping image pull (--no-pull)"
fi

echo "==> [3/6] DeepSeek API key"
ENV_FILE=build/.env
if [ -n "${DEEPSEEK_API_KEY:-}" ]; then
  :
elif [ -f "$ENV_FILE" ] && grep -q '^DEEPSEEK_API_KEY=sk-' "$ENV_FILE"; then
  DEEPSEEK_API_KEY=$(grep '^DEEPSEEK_API_KEY=' "$ENV_FILE" | cut -d= -f2-)
  echo "   using key already saved in $ENV_FILE"
else
  printf '   paste your DeepSeek API key (sk-...): '
  read -r DEEPSEEK_API_KEY
fi
[ -n "${DEEPSEEK_API_KEY:-}" ] || { echo "error: DEEPSEEK_API_KEY required"; exit 1; }
umask 077
printf 'DEEPSEEK_API_KEY=%s\n' "$DEEPSEEK_API_KEY" > "$ENV_FILE"
echo "   saved to $ENV_FILE (mode 600, gitignored)"

echo "==> [4/6] compiling backend"
(cd backend && go build -o ../build/backend .)

echo "==> [5/6] restarting backend"
# stop any previous backend on :8080 — '/backend' matches every binary layout
# (./backend, ./build/backend), unlike path-specific patterns that silently
# miss one layout and leave the old process holding the port
pkill -f '/backend' 2>/dev/null || true
sleep 1
DEEPSEEK_API_KEY="$DEEPSEEK_API_KEY" nohup ./build/backend > build/backend.log 2>&1 &
sleep 1
ss -tlnp | grep -q 8080 || { echo "error: backend not listening"; tail -20 build/backend.log; exit 1; }

echo "==> [6/6] health check: send one chat message"
echo "    (first reply includes the sandbox boot — can take 1-2 min)"
curl -s --max-time 180 -X POST localhost:8080/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"hi, reply with: pong"}'
echo

echo "==> done."
echo "    chat UI:  http://<claw-host>:8080/   (or a tunnel/port-forward to 8080)"
echo "    sandboxes: ./sandboxctl.sh"
