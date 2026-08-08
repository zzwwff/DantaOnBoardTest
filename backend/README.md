# backend — sandbox management backend (Docker Provider MVP)

Exposes 3 HTTP endpoints and manages "sandboxes" (currently Docker containers).

## Endpoints

| Method | Path | Behavior |
|---|---|---|
| POST | `/sandbox` | Create sandbox → `{"sandbox_id": "...", "addr": "http://127.0.0.1:<port>"}` |
| DELETE | `/sandbox/{id}` | Delete sandbox |
| POST | `/sandbox/{id}/ping` | body `{"message": "..."}` → `{"reply": "<message> -sandbox- <id>"}` |

## Architecture (current)

```
Caller ──HTTP──► backend :8080 ──docker CLI──► sandbox container (pingpong)
                    │                              │
                    └─ port map (in-memory)        └─ random host port ← 127.0.0.1:<port>
```

- Create: `docker run -d -p 49999` (random host port, no conflicts); sandbox ID injected via the `SANDBOX_ID` env var
- Communicate: HTTP forwarded to `127.0.0.1:<host-port>/`
- Long residency: containers live until removed; no idle reaper

## Dependencies

- docker CLI on the host (already present on claw)
- Image `pingpong:latest` (build first, see pingpong/README.md)
- Go standard library only, no third-party deps

## Build & run

```bash
cd backend
go build -o backend .
./backend        # listens on 127.0.0.1:8080 (override with LISTEN_ADDR)
```

## Try it

```bash
# create
curl -X POST localhost:8080/sandbox
# {"sandbox_id":"s12345678","addr":"http://127.0.0.1:32768"}

# ping
curl -X POST localhost:8080/sandbox/s12345678/ping -d 'hello'
# {"reply":"hello -sandbox- s12345678"}

# delete
curl -X DELETE localhost:8080/sandbox/s12345678
```

## Future: swap to CubeSandbox

Contract unchanged; replace internals only:
- `createSandbox` → E2B-compatible API (`E2B_API_URL` / `CUBE_TEMPLATE_ID` / `NEVER_TIMEOUT`)
- `deleteSandbox` → `sandbox.kill()`
- ping → CubeProxy `<port>-<id>.<domain>`

## Status

✅ MVP code ready (Docker Provider), pending on-server deployment & verification
