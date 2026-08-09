# backend — sandbox management backend (OpenClaw provider)

Go stdlib-only backend: exposes the chat API and manages "sandboxes" (currently hardened OpenClaw gateway containers with a DeepSeek model).

## Endpoints

| Method | Path | Behavior |
|---|---|---|
| GET | `/` | Chat UI (served from `../web`) |
| GET | `/api/sandbox` | Current sandbox → `{"sandbox_id", "addr"}` |
| POST | `/api/chat` | `{"message": "..."}` → `{"reply": "..."}` — creates the sandbox on the first message; routes to the single active sandbox (web UI) |
| POST | `/sandbox` | Create sandbox → `{"sandbox_id", "addr"}` (becomes active) |
| POST | `/sandbox/{id}/chat` | `{"message": "..."}` → `{"reply": "..."}` — chat with the named sandbox (own container, own session); 404 if unknown |
| DELETE | `/sandbox` or `/sandbox/{id}` | Delete sandbox (container + data dir) |

## Architecture

```
Caller ──HTTP──► backend :8080 ──docker CLI──► sandbox container (OpenClaw gateway :18789)
                     │                              │
                     ├─ build/data-<id>/            └─ DeepSeek model, data dir at /home/node/.openclaw
                     │    openclaw.json (generated)
                     └─ port map (in-memory, rebuilt on start)
```

- Create: `docker run` with hardening flags (`--memory 512m --cpus 1 --pids-limit 256 --cap-drop ALL --security-opt no-new-privileges`), the generated config dir mounted at `/home/node/.openclaw`, and a random host port for 18789.
- The generated `openclaw.json` enables the `/v1/chat/completions` endpoint, sets token auth (`gateway.auth.token`, random per sandbox), registers the DeepSeek provider (`models.providers.deepseek`, `api: "openai-completions"`), and makes `deepseek/deepseek-chat` the default agent model.
- Chat: forward to `POST 127.0.0.1:<port>/v1/chat/completions` with `model: "openclaw"`, `user: <sandbox_id>` (stable session key → conversation state lives in the sandbox), and the gateway bearer token. Each sandbox is addressed by id (`/sandbox/{id}/chat`) so conversations never mix.
- Recovery: on start the backend scans `sbx-*` containers, reads each token from its `openclaw.json`, and rebuilds the port map — sandboxes survive backend restarts (long residency).
- Long residency: containers have no idle reaper; they live until deleted.

## Dependencies

- docker CLI on the host
- Image `ghcr.io/openclaw/openclaw:latest` (pulled by `deploy.sh`, CN mirrors as fallback)
- Go standard library only, no third-party deps

## Env vars

| Var | Default | Purpose |
|---|---|---|
| `LISTEN_ADDR` | `127.0.0.1:8080` | bind address |
| `DEEPSEEK_API_KEY` | — | required to create a sandbox |
| `WEB_DIR` | `../web` | directory with the chat UI |

## Build & run

```bash
cd backend
go build -o ../build/backend .
DEEPSEEK_API_KEY=sk-xxx ../build/backend
```

(Prefer `./deploy.sh` at the repo root — it does image pull, key handling, build, start and health check.)

## Try it

```bash
# chat (boots the sandbox on the first call; first reply may take 1-2 min)
curl -X POST localhost:8080/api/chat -H 'Content-Type: application/json' -d '{"message":"hi"}'
# {"reply":"..."}

# current sandbox
curl -s localhost:8080/api/sandbox
# {"sandbox_id":"s12345678","addr":"http://127.0.0.1:32768"}

# chat with a specific sandbox (own container, own session)
curl -X POST localhost:8080/sandbox/s12345678/chat -H 'Content-Type: application/json' -d '{"message":"hi"}'

# delete
curl -X DELETE localhost:8080/sandbox/s12345678
```

## Future: swap to CubeSandbox

Contract unchanged; replace internals only:
- `newSandbox` → E2B-compatible API (`E2B_API_URL` / `CUBE_TEMPLATE_ID` / `NEVER_TIMEOUT`)
- `deleteSandbox` → `sandbox.kill()`
- chat → CubeProxy `<port>-<id>.<domain>`

## Status

✅ Code ready (OpenClaw provider), pending on-server deployment & verification
