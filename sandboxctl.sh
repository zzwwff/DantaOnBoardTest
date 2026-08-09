#!/usr/bin/env bash
# sandboxctl.sh - control the sandbox MVP through the backend API.
#
# Maintains a name -> sandbox_id mapping file (sandboxes.map) so you can use
# friendly names instead of raw ids. The backend still generates the real id;
# this script only keeps the local alias table.
#
# Usage:
#   ./sandboxctl.sh create <name>              create a sandbox and register <name>
#   ./sandboxctl.sh send <name> [message]      chat with that sandbox
#   ./sandboxctl.sh delete <name>              delete the sandbox and its mapping
#   ./sandboxctl.sh list                       show the name mapping table
#   ./sandboxctl.sh status                     show the active sandbox
#   ./sandboxctl.sh                            interactive menu
set -euo pipefail
cd "$(dirname "$0")"

BACKEND="${BACKEND_URL:-http://127.0.0.1:8080}"
MAP_FILE="${SANDBOX_MAP:-./build/sandboxes.map}"   # runtime state, gitignored

mkdir -p "$(dirname "$MAP_FILE")"

# name <tab> sandbox_id <tab> addr   (one line per sandbox)
sid_of() { grep -P "^$1\t" "$MAP_FILE" | cut -f2 | head -1; }

create() {
  local name="$1"
  [ -n "$name" ] || { echo "error: name required"; exit 1; }
  [ -z "$(sid_of "$name")" ] || { echo "error: name already in use: $name"; exit 1; }

  local resp sid addr
  resp=$(curl -s -X POST "$BACKEND/sandbox")
  sid=$(echo "$resp" | sed -n 's/.*"sandbox_id":"\([^"]*\)".*/\1/p')
  addr=$(echo "$resp" | sed -n 's/.*"addr":"\([^"]*\)".*/\1/p')
  [ -n "$sid" ] || { echo "create failed: $resp"; exit 1; }

  printf '%s\t%s\t%s\n' "$name" "$sid" "$addr" >> "$MAP_FILE"
  echo "created: $name -> $sid ($addr)"
}

send() {
  local name="$1"
  local sid
  sid=$(sid_of "$name")
  [ -n "$sid" ] || { echo "error: unknown name '$name' (see: ./sandboxctl.sh list)"; exit 1; }

  local msg="${2:-}"
  [ -n "$msg" ] || { printf 'message: '; read -r msg; }
  [ -n "$msg" ] || msg="ping"

  # each name maps to its own sandbox (own container, own conversation);
  # route by the real sandbox id so names never share a session
  curl -s -X POST "$BACKEND/sandbox/$sid/chat" \
    -H 'Content-Type: application/json' \
    -d "{\"message\":\"$msg\"}"
  echo
}

delete() {
  local name="$1"
  local sid
  sid=$(sid_of "$name")
  [ -n "$sid" ] || { echo "error: unknown name '$name' (see: ./sandboxctl.sh list)"; exit 1; }

  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$BACKEND/sandbox/$sid")
  echo "delete -> HTTP $code"
  sed -i "/^$name\t/d" "$MAP_FILE"
  echo "removed mapping: $name"
}

list() {
  if [ ! -f "$MAP_FILE" ]; then
    echo "(no sandboxes yet - create one: ./sandboxctl.sh create <name>)"
    return 0
  fi
  echo "name / id / addr"
  cat "$MAP_FILE"
}

status() {
  curl -s "$BACKEND/api/sandbox"
  echo
}

menu() {
  while true; do
    echo
    echo "=== sandboxctl ==="
    echo "1) create   2) send   3) delete   4) list   q) quit"
    printf '> '
    read -r cmd
    case "$cmd" in
      1) printf 'name: '; read -r n; create "$n" ;;
      2) printf 'name: '; read -r n; send "$n" ;;
      3) printf 'name: '; read -r n; delete "$n" ;;
      4) list ;;
      q|Q) break ;;
      *) echo "unknown command: $cmd" ;;
    esac
  done
}

case "${1:-menu}" in
  create) create "${2:-}" ;;
  send)   send "${2:-}" "${3:-}" ;;
  delete) delete "${2:-}" ;;
  list)   list ;;
  status) status ;;
  menu|"") menu ;;
  *) echo "usage: $0 {create <name> | send <name> [msg] | delete <name> | list | status | menu}"; exit 1 ;;
esac
