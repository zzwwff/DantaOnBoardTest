// backend — sandbox management backend (OpenClaw provider)
//
// Each sandbox is one hardened OpenClaw gateway container backed by DeepSeek.
// Endpoints:
//   GET   /                    chat UI (web/index.html)
//   GET   /api/sandbox         current sandbox info -> {"sandbox_id","addr"}
//   POST  /api/chat            {"message": "..."} -> {"reply": "..."}
//   POST  /sandbox             create sandbox -> {"sandbox_id","addr"}
//   DELETE /sandbox            delete the current sandbox
//   DELETE /sandbox/{id}       delete the named sandbox
//
// Env:
//   LISTEN_ADDR       bind address (default 127.0.0.1:8080)
//   DEEPSEEK_API_KEY  API key for DeepSeek (required to create a sandbox)
//   WEB_DIR           directory with the chat UI (default ../web)
//
// Sandbox lifecycle: on start the backend scans existing sbx-* containers and
// recovers the port map, so sandboxes survive backend restarts (long residency).
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	openclawImage = "ghcr.io/openclaw/openclaw:latest"
	sandboxPort   = "18789" // gateway HTTP port inside the container
	// the OpenClaw image runs as user "node" (uid 1000); data dirs must be
	// owned by it so the gateway can write its state
	containerUID = 1000
)

// sandbox tracks one OpenClaw container.
type sandbox struct {
	id      string
	port    string // host port of the gateway
	token   string // gateway auth token (also inside openclaw.json)
	dataDir string // host dir mounted at /home/node/.openclaw
}

var (
	mu      sync.Mutex
	current *sandbox                // active sandbox for the web-chat flow
	byID    = map[string]*sandbox{} // all live sandboxes, keyed by id
)

type createResp struct {
	SandboxID string `json:"sandbox_id"`
	Addr      string `json:"addr"`
}

type chatResp struct {
	Reply string `json:"reply"`
}

// sh runs a docker command and returns trimmed output.
func sh(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// writeConfig writes openclaw.json for a sandbox: chat-completions endpoint on,
// token auth, DeepSeek provider, DeepSeek as the default agent model.
func writeConfig(cfgPath, token, apiKey string) error {
	cfg := map[string]any{
		"gateway": map[string]any{
			// required by the gateway on start; otherwise it refuses to boot
			// ("existing config is missing gateway.mode")
			"mode": "local",
			"auth": map[string]any{
				"mode":  "token",
				"token": token,
			},
			"http": map[string]any{
				"endpoints": map[string]any{
					"chatCompletions": map[string]any{"enabled": true},
				},
			},
		},
		"models": map[string]any{
			"mode": "merge",
			"providers": map[string]any{
				"deepseek": map[string]any{
					"baseUrl": "https://api.deepseek.com",
					"apiKey":  apiKey,
					"api":     "openai-completions",
					"models": []any{
						// fields per the official ModelDefinitionSchema:
						// id/name/reasoning/input/cost/contextWindow/maxTokens
						map[string]any{"id": "deepseek-chat", "name": "DeepSeek Chat",
							"contextWindow": 128000, "maxTokens": 8192},
					},
				},
			},
		},
		"agents": map[string]any{
			"defaults": map[string]any{
				"model": map[string]any{"primary": "deepseek/deepseek-chat"},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0o600)
}

// hostPortOf extracts the mapped host port from "docker port" output
// (one line per bound address; IPv4 first).
func hostPortOf(name string) (string, error) {
	out, err := sh("port", name, sandboxPort)
	if err != nil {
		return "", fmt.Errorf("%s (%w)", out, err)
	}
	first := strings.SplitN(out, "\n", 2)[0]
	port := strings.TrimPrefix(first, "0.0.0.0:")
	port = strings.TrimPrefix(port, "[::]:")
	return port, nil
}

func newSandbox() (*sandbox, error) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY not set")
	}

	id := "s" + fmt.Sprintf("%d", time.Now().UnixNano()%100000000)
	name := "sbx-" + id
	base := filepath.Join("build", "data-"+id)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chown(base, containerUID, containerUID); err != nil {
		return nil, err
	}

	token := randomHex(32) // 64 hex chars, same length as "openssl rand -hex 32"
	cfgPath := filepath.Join(base, "openclaw.json")
	if err := writeConfig(cfgPath, token, apiKey); err != nil {
		return nil, err
	}
	if err := os.Chown(cfgPath, containerUID, containerUID); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}

	// Hardened run: resource caps, no capabilities, no privilege escalation.
	// The data dir is mounted at /home/node/.openclaw (config + agent state).
	// The sandbox keeps running until removed (long residency, no reaper).
	if out, err := sh("run", "-d", "--name", name,
		"-p", sandboxPort,
		"--memory", "512m", "--cpus", "1", "--pids-limit", "256",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"-v", abs+":/home/node/.openclaw",
		openclawImage); err != nil {
		return nil, fmt.Errorf("docker run: %s (%w)", out, err)
	}

	port, err := hostPortOf(name)
	if err != nil {
		sh("rm", "-f", name)
		return nil, fmt.Errorf("port lookup: %w", err)
	}

	sb := &sandbox{id: id, port: port, token: token, dataDir: base}

	// Wait for the gateway to serve /v1/models before returning, so the first
	// chat request never races the container boot. On failure the container is
	// kept (not removed) and its log tail is embedded in the error, so the
	// cause is visible without a manual "docker logs" round trip.
	if err := sb.waitReady(60 * time.Second); err != nil {
		logs, _ := sh("logs", "--tail", "20", name)
		fmt.Printf("sandbox %s failed to become ready, kept for inspection: docker logs %s\n", id, name)
		return nil, fmt.Errorf("%v\n--- container log tail ---\n%s", err, logs)
	}
	mu.Lock()
	byID[id] = sb
	mu.Unlock()
	return sb, nil
}

func (s *sandbox) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := "http://127.0.0.1:" + s.port + "/v1/models"
	client := &http.Client{Timeout: 5 * time.Second}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+s.token)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("gateway not ready within %v", timeout)
}

// chat sends one user message to the gateway and returns the reply text.
// The OpenAI "user" field is the sandbox id -> stable session key, so the
// conversation state lives inside the sandbox (across requests, no sessions
// needed on the caller side).
func (s *sandbox) chat(msg string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model": "openclaw",
		"user":  s.id,
		"messages": []any{
			map[string]string{"role": "user", "content": msg},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST",
		"http://127.0.0.1:"+s.port+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("x-openclaw-model", "deepseek/deepseek-chat")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gateway unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gateway HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("bad gateway response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty gateway response")
	}
	reply := strings.TrimSpace(out.Choices[0].Message.Content)
	if reply == "" {
		reply = "(no text reply)"
	}
	return reply, nil
}

func ensureSandbox() *sandbox {
	mu.Lock()
	defer mu.Unlock()
	return current
}

// recoverExisting rebuilds the in-memory state from containers that are still
// running (long residency across backend restarts).
func recoverExisting() {
	out, err := sh("ps", "-a", "--filter", "name=sbx-", "--format", "{{.Names}}")
	if err != nil {
		return
	}
	for _, name := range strings.Fields(out) {
		if !strings.HasPrefix(name, "sbx-") {
			continue
		}
		id := strings.TrimPrefix(name, "sbx-")
		cfgPath := filepath.Join("build", "data-"+id, "openclaw.json")
		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			continue
		}
		var cfg struct {
			Gateway struct {
				Auth struct {
					Token string `json:"token"`
				} `json:"auth"`
			} `json:"gateway"`
		}
		if json.Unmarshal(raw, &cfg) != nil || cfg.Gateway.Auth.Token == "" {
			continue
		}
		port, err := hostPortOf(name)
		if err != nil {
			continue
		}
		sb := &sandbox{id: id, port: port, token: cfg.Gateway.Auth.Token,
			dataDir: filepath.Join("build", "data-"+id)}
		mu.Lock()
		byID[id] = sb
		if current == nil {
			current = sb
		}
		mu.Unlock()
		fmt.Printf("recovered sandbox %s on 127.0.0.1:%s\n", id, port)
	}
}

// GET /api/sandbox
func sandboxInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sb := ensureSandbox()
	if sb == nil {
		http.Error(w, "no sandbox yet", http.StatusNotFound)
		return
	}
	writeJSON(w, createResp{SandboxID: sb.id, Addr: "http://127.0.0.1:" + sb.port})
}

// POST /api/chat — create the sandbox lazily on the first message.
func chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json body", http.StatusBadRequest)
		return
	}
	if in.Message == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}

	sb := ensureSandbox()
	if sb == nil {
		var err error
		if sb, err = newSandbox(); err != nil {
			http.Error(w, "create sandbox failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		current = sb
		mu.Unlock()
	}
	chatWith(w, sb, in.Message)
}

// POST /sandbox/{id}/chat — chat with the named sandbox (own container, own
// session). This is the per-sandbox contract; /api/chat above is the
// single-active-sandbox shortcut used by the web UI.
func sandboxSubHandler(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/sandbox/")
	if r.Method == http.MethodPost && strings.HasSuffix(p, "/chat") {
		id := strings.TrimSuffix(p, "/chat")
		mu.Lock()
		sb := byID[id]
		mu.Unlock()
		if sb == nil {
			http.Error(w, "sandbox not found: "+id, http.StatusNotFound)
			return
		}
		var in struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json body", http.StatusBadRequest)
			return
		}
		if in.Message == "" {
			http.Error(w, "message required", http.StatusBadRequest)
			return
		}
		chatWith(w, sb, in.Message)
		return
	}
	deleteSandboxHandler(w, r)
}

func chatWith(w http.ResponseWriter, sb *sandbox, msg string) {
	reply, err := sb.chat(msg)
	if err != nil {
		http.Error(w, "chat failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, chatResp{Reply: reply})
}

// POST /sandbox — explicit create; DELETE /sandbox — delete the current sandbox.
func createSandboxHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		sb, err := newSandbox()
		if err != nil {
			http.Error(w, "create failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		current = sb
		mu.Unlock()
		writeJSON(w, createResp{SandboxID: sb.id, Addr: "http://127.0.0.1:" + sb.port})
	case http.MethodDelete:
		// DELETE /sandbox (no id) — handled here; DELETE /sandbox/{id} hits the
		// "/sandbox/" pattern below.
		deleteSandboxHandler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func deleteSandboxHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/sandbox")
	id = strings.Trim(id, "/")
	if id == "" {
		mu.Lock()
		sb := current
		current = nil
		if sb != nil {
			delete(byID, sb.id)
		}
		mu.Unlock()
		if sb == nil {
			http.Error(w, "no sandbox", http.StatusNotFound)
			return
		}
		id = sb.id
	} else {
		mu.Lock()
		delete(byID, id)
		if current != nil && current.id == id {
			current = nil
		}
		mu.Unlock()
	}

	if out, err := sh("rm", "-f", "sbx-"+id); err != nil {
		http.Error(w, "delete failed: "+out, http.StatusInternalServerError)
		return
	}
	os.RemoveAll(filepath.Join("build", "data-"+id))
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func main() {
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = "127.0.0.1:8080"
	}
	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "../web"
	}

	recoverExisting()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", chatHandler)
	mux.HandleFunc("/api/sandbox", sandboxInfoHandler)
	mux.HandleFunc("/sandbox", createSandboxHandler)
	mux.HandleFunc("/sandbox/", sandboxSubHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})

	fmt.Println("backend listening on", listenAddr)
	http.ListenAndServe(listenAddr, mux)
}
