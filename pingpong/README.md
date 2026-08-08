# pingpong — long-lived in-sandbox service (Docker MVP)

Implements the pingpong signed echo inside the sandbox.

## Behavior

- Listens on `:49999`
- Replies `"<original message> -sandbox- <sandboxID>"`
- Sandbox ID comes from the `SANDBOX_ID` env var (injected by the backend at creation)

## Build

```bash
cd pingpong
docker build -t pingpong:latest .
```

Multi-stage build (golang:alpine compile → alpine runtime), few MB artifact.

## Run standalone (without backend)

```bash
docker run -d --name test-sbx -e SANDBOX_ID=test123 -p 49999 pingpong:latest
curl 127.0.0.1:49999/ -d 'hello'   # hello -sandbox- test123
docker rm -f test-sbx
```

## Status

✅ MVP code ready, pending on-server build verification
