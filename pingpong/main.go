// pingpong — long-lived in-sandbox service.
// Listens on :49999; replies with "<original message> -sandbox- <sandboxID>".
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	sandboxID := os.Getenv("SANDBOX_ID")
	if sandboxID == "" {
		sandboxID = "unknown"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		msg := string(body)
		if msg == "" {
			msg = "ping"
		}
		fmt.Fprintf(w, "%s -sandbox- %s", msg, sandboxID)
	})

	// Long residency: the process runs in the foreground; the container stays
	// alive until it is removed by the backend.
	http.ListenAndServe(":49999", nil)
}
