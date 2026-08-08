// pingpong — 沙箱内常驻服务
// 监听 :49999，收到请求返回 "原消息 -sandbox- <沙箱ID>"
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

	// 长期驻留：进程前台运行，容器不删就一直活着
	http.ListenAndServe(":49999", nil)
}
