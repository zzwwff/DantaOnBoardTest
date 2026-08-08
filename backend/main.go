// backend — sandbox management backend (Docker Provider MVP)
//
// Three endpoints:
//   POST   /sandbox            create sandbox -> {"sandbox_id": "...", "addr": "..."}
//   DELETE /sandbox/{id}       delete sandbox
//   POST   /sandbox/{id}/ping  body {"message": "..."} -> {"reply": "<msg> -sandbox- <id>"}
//
// Note: "sandbox" is currently a Docker container managed via the docker CLI.
// The API contract stays unchanged if the sandbox backend is later swapped to
// CubeSandbox — only createSandbox/deleteSandbox internals change.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const image = "pingpong:latest"

var (
	mu    sync.RWMutex
	ports = map[string]string{} // sandbox_id -> host port
)

type createResp struct {
	SandboxID string `json:"sandbox_id"`
	Addr      string `json:"addr"`
}

type pingResp struct {
	Reply string `json:"reply"`
}

// sh runs a docker command and returns trimmed output.
func sh(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// POST /sandbox
func createSandbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := fmt.Sprintf("s%d", time.Now().UnixNano()%100000000)
	name := "sbx-" + id

	// Create the sandbox; "-p 49999" without a host port binds a random host
	// port, so concurrent creates never conflict.
	if out, err := sh("run", "-d", "--name", name,
		"-e", "SANDBOX_ID="+id, "-p", "49999", image); err != nil {
		http.Error(w, "create failed: "+out, http.StatusInternalServerError)
		return
	}

	// Resolve the mapped port: docker port <name> 49999 -> "0.0.0.0:32768"
	out, err := sh("port", name, "49999")
	if err != nil {
		http.Error(w, "port lookup failed: "+out, http.StatusInternalServerError)
		return
	}
	hostPort := strings.TrimPrefix(out, "0.0.0.0:")

	mu.Lock()
	ports[id] = hostPort
	mu.Unlock()

	writeJSON(w, createResp{SandboxID: id, Addr: "http://127.0.0.1:" + hostPort})
}

// DELETE /sandbox/{id}
func deleteSandbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/sandbox/")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	mu.Lock()
	delete(ports, id)
	mu.Unlock()

	if out, err := sh("rm", "-f", "sbx-"+id); err != nil {
		http.Error(w, "delete failed: "+out, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /sandbox/{id}/ping
func pingSandbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/sandbox/")
	id = strings.TrimSuffix(id, "/ping")

	mu.RLock()
	hostPort, ok := ports[id]
	mu.RUnlock()
	if !ok {
		http.Error(w, "sandbox not found: "+id, http.StatusNotFound)
		return
	}

	body, _ := io.ReadAll(r.Body)
	msg := string(body)
	if msg == "" {
		msg = "ping"
	}

	// Forward to the sandbox service and return the signed message as-is.
	resp, err := http.Post("http://127.0.0.1:"+hostPort+"/", "text/plain",
		strings.NewReader(msg))
	if err != nil {
		http.Error(w, "sandbox unreachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	reply, _ := io.ReadAll(resp.Body)

	writeJSON(w, pingResp{Reply: string(reply)})
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

	mux := http.NewServeMux()
	mux.HandleFunc("/sandbox", createSandbox)
	mux.HandleFunc("/sandbox/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/sandbox/")
		if strings.HasSuffix(p, "/ping") {
			pingSandbox(w, r)
			return
		}
		deleteSandbox(w, r)
	})

	fmt.Println("backend listening on", listenAddr)
	http.ListenAndServe(listenAddr, mux)
}
