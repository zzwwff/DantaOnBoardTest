# Danta OnBoard Test — Sandbox MVP (Docker)

Proof-of-concept for the Danta "one sandbox per user" AI onboarding experience, running on the test server `claw`.

## Goals (4 capabilities)

1. **Create sandbox** — backend creates a sandbox instance and returns a `sandbox_id`
2. **Delete sandbox** — backend destroys the specified sandbox
3. **Communicate with a sandbox** — pingpong: send a message, the sandbox replies with "message + sandbox-ID signature"
4. **Long-term residency** — the sandbox stays alive across idle periods (no idle reaper)

## Current implementation (Docker MVP)

CubeSandbox (microVM) is **not runnable on the test server `claw`**: it is an LXC container on PVE — no `/dev/kvm`, the disk is ext4 (not XFS), and the host kernel cannot be changed from inside a container. The PVM mode fails the same preconditions.

Approved alternative: **one sandbox per user as a Docker container** — the industry-standard pattern used by OpenClaw's official multi-tenant solution (`openclaw fleet`, per-tenant hardened containers) and community projects (e.g. ClawHuddle). The full decision write-up is in `DockerDecision.md` (Chinese).

```
Caller → HTTP → backend (Go) :8080 ── docker CLI ──► sandbox container (per user)
                                                     └─ random host port, HTTP :49999 inside
```

## Directory structure

```
DantaOnBoardTest/
├── README.md              ← this file
├── TODO.md                ← task checklist (Chinese)
├── DockerDecision.md     ← decision write-up for the project lead (Chinese)
├── backend/               ← Go backend (3 endpoints)
└── pingpong/              ← in-sandbox pingpong service + Dockerfile
```

## API contract (fixed; provider swappable)

| Method | Path | Behavior |
|---|---|---|
| POST | `/sandbox` | Create sandbox → `{"sandbox_id": "...", "addr": "http://127.0.0.1:<port>"}` |
| DELETE | `/sandbox/{id}` | Delete sandbox |
| POST | `/sandbox/{id}/ping` | body `{"message": "..."}` → `{"reply": "<message> -sandbox- <id>"}` |

The contract stays unchanged when the sandbox backend is later swapped to CubeSandbox/microVM — only the internal implementation of `createSandbox`/`deleteSandbox` changes.

## Roadmap

1. ✅ Environment check on `claw` — Docker 28.2.2, Go 1.22.2, ports 8080/49999 free
2. Build the pingpong image → build & run the backend → end-to-end verification
3. Long-residency check (overnight)
4. Later: per-user OpenClaw image (read-only base layer + writable user volume), then upgrade isolation (gVisor → microVM on a dedicated VM with KVM)

## Acceptance criteria

- [ ] Create returns `sandbox_id`; container visible in `docker ps`
- [ ] Ping returns "message + `-sandbox-` + id" (signature round-trip proves routing)
- [ ] Sandbox stays alive overnight and is still pingable
- [ ] Delete removes the container
- [ ] Per-instance memory recorded (`docker stats`)

## Status

- Code: ready (`pingpong/`, `backend/`)
- Deployment & verification: in progress on `claw`
