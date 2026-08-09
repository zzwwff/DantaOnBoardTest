# Danta OnBoard Test — Sandbox MVP (Docker + OpenClaw)

Proof-of-concept for the Danta "one sandbox per user" AI onboarding experience, running on the test server `claw`.

## Background

CubeSandbox (microVM) is **not runnable on `claw`**: it is an LXC container on PVE — no `/dev/kvm`, ext4 (not XFS), and the host kernel cannot be changed from inside a container. PVM mode fails the same preconditions.

Approved alternative: **one sandbox per user as a hardened Docker container** — the industry-standard pattern used by OpenClaw's official multi-tenant solution (`openclaw fleet`) and community projects (ClawHuddle). The full decision write-up is in `DockerDecision.md` (Chinese).

Two milestones:
1. **Milestone 1 (done, verified): pingpong** — long-lived container with a signed-echo service, validated the control-plane contract. Code kept in `pingpong/` for reference.
2. **Milestone 2 (this repo's current state): OpenClaw chat sandbox** — each sandbox is an [OpenClaw](https://openclaw.ai) gateway container with a DeepSeek model; a web chat UI talks to it through the backend.

## Architecture

```
Browser ──HTTP──► backend (Go) :8080 ──docker CLI──► sandbox container (OpenClaw gateway)
                     │                                   │
                     ├─ GET   /             chat UI      └─ :18789, hardened,
                     ├─ GET   /api/sandbox  current id      DeepSeek model,
                     ├─ POST  /api/chat     {"message"}→{"reply"}    data dir mounted
                     ├─ POST  /sandbox      create sandbox           at /home/node/.openclaw
                     └─ DELETE /sandbox[/{id}]  delete sandbox
```

- The backend generates `build/data-<id>/openclaw.json` per sandbox (chat-completions endpoint, gateway token, DeepSeek provider, default agent model) and mounts the whole dir at `/home/node/.openclaw` — config **and** agent memory persist there.
- Chat uses the OpenAI `user` field (= sandbox id) as the stable session key, so the conversation lives inside the sandbox; the caller needs no session state.
- **Long residency**: containers have no idle reaper; on restart the backend rescans `sbx-*` containers and recovers the port map, so a sandbox survives backend restarts.
- The sandbox is created lazily on the first chat message (or explicitly via `POST /sandbox`).

## Sandbox hardening (applied at `docker run`)

| Measure | Value |
|---|---|
| Memory cap | `--memory 512m` |
| CPU cap | `--cpus 1` |
| Process cap | `--pids-limit 256` |
| Capabilities | `--cap-drop ALL` |
| Privilege escalation | `--security-opt no-new-privileges` |
| State | host dir bind-mounted at `/home/node/.openclaw` (owned by the container's `node` user) |
| Container user | non-root (`node`) — from the image itself |

Not yet applied (future work): `--read-only` rootfs (needs a writable-path audit for OpenClaw), egress allowlist for the LLM API, gVisor runtime. See `TODO.md`.

## Directory structure

```
DantaOnBoardTest/
├── README.md              ← this file
├── TODO.md                ← task checklist (Chinese)
├── DockerDecision.md      ← decision write-up for the project lead (Chinese)
├── deploy.sh              ← one-shot deploy (image pull → build → run → health check)
├── sandboxctl.sh          ← CLI control (create / send / delete / list / status)
├── backend/               ← Go backend (chat + sandbox management)
├── web/                   ← chat UI (single page, no external deps)
└── pingpong/              ← milestone 1 artifact (kept for reference)
```

## API contract

| Method | Path | Behavior |
|---|---|---|
| GET | `/` | Chat UI |
| GET | `/api/sandbox` | Current sandbox → `{"sandbox_id", "addr"}` |
| POST | `/api/chat` | `{"message": "..."}` → `{"reply": "..."}` (creates the sandbox on first message) |
| POST | `/sandbox` | Create sandbox → `{"sandbox_id", "addr"}` |
| DELETE | `/sandbox` or `/sandbox/{id}` | Delete sandbox (container + data dir) |

## Deploy

```bash
cd /mnt/data/go/DantaOnBoardTest
./deploy.sh                     # pulls the image, asks for the DeepSeek key once, builds, starts, health-checks
# or: DEEPSEEK_API_KEY=sk-xxx ./deploy.sh
```

Then open `http://<claw-host>:8080/` (or a tunnel/port-forward to 8080).

The API key is saved in `build/.env` (mode 600, gitignored) and re-used on later deploys.

## Manual control

```bash
./sandboxctl.sh status          # which sandbox is active
./sandboxctl.sh send my-sbx "hello"   # chat (JSON-escaped message; plain text is fine)
./sandboxctl.sh delete my-sbx   # delete a sandbox
./sandboxctl.sh list
```

## Roadmap

1. ✅ Milestone 1 — pingpong control-plane validation (create / ping / delete / long residency)
2. ✅ Milestone 2 code — OpenClaw chat sandbox + chat UI + hardening + deploy script
3. On-server deployment & verification (image pull, first chat, overnight residency)
4. Later: per-user OpenClaw images (read-only base layer), egress allowlist, gVisor runtime, then microVM/KVM when a nested-virt VM is available

## Acceptance criteria (milestone 2)

- [ ] `./deploy.sh` pulls the image (ghcr.io or CN mirror) and starts the backend
- [ ] First chat message boots a sandbox; reply comes back from DeepSeek through OpenClaw
- [ ] Conversation memory persists across messages (same sandbox, same session)
- [ ] Sandbox survives a backend restart (recovered, still chatable)
- [ ] `docker stats` shows each sandbox capped at 512 MiB
- [ ] Long residency: sandbox still chatable the next morning
- [ ] Delete removes container + data dir
