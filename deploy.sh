#!/usr/bin/env bash
# deploy.sh - rebuild and redeploy the sandbox MVP (pingpong image + backend).
#
# Steps:
#   1. remove all running sandbox containers (sbx-*)
#   2. rebuild the pingpong image
#   3. recompile the backend
#   4. restart the backend
#   5. health check (create + delete a throwaway sandbox)
#
# Note on the slow first build: the golang/alpine base images are pulled only
# once (through the CN mirrors) and stay cached in the local image store, so
# rebuilds skip that step and finish in seconds. They are only re-pulled if
# you `docker rmi` the base images yourself.
#
# Usage:
#   ./deploy.sh               full redeploy
#   ./deploy.sh --no-build    skip the docker build (backend-only change)
set -euo pipefail
cd "$(dirname "$0")"

BUILD=1
[ "${1:-}" = "--no-build" ] && BUILD=0

echo "==> [1/4] removing all sandbox containers (long-residency sandbox will be killed)"
docker rm -f $(docker ps -aq --filter name=sbx-) 2>/dev/null || true

if [ "$BUILD" -eq 1 ]; then
  echo "==> [2/4] building pingpong image (base images cached -> fast)"
  docker build -t pingpong:latest pingpong/
else
  echo "==> [2/4] skipping docker build (--no-build)"
fi

echo "==> [3/4] recompiling backend"
(cd backend && go build -o backend .)

echo "==> [4/4] restarting backend"
pkill -f './backend' 2>/dev/null || true
sleep 1
(cd backend && nohup ./backend > backend.log 2>&1 &)
sleep 1

echo "==> health check: create + delete a throwaway sandbox"
RESP=$(curl -s -X POST localhost:8080/sandbox)
echo "   create -> $RESP"
SID=$(echo "$RESP" | sed -n 's/.*"sandbox_id":"\([^"]*\)".*/\1/p')
if [ -n "$SID" ]; then
  CODE=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "localhost:8080/sandbox/$SID")
  echo "   delete -> HTTP $CODE"
fi

echo "==> done. manage sandboxes with ./sandboxctl.sh"
