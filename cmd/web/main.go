package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type resizeMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	cmdPath := flag.String("cmd", "go", "command to run for the TUI")
	cmdArgs := flag.String("args", "run ./cmd/tui", "command arguments")
	staticDir := flag.String("static", "web", "static files directory")
	dataDir := flag.String("data-dir", ".", "batch JSON directory for API")
	flag.Parse()

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(*staticDir)))
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/batches", handleBatches(*dataDir))
	mux.HandleFunc("/api/batches/", handleBatchByID(*dataDir))
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveTUI(w, r, *cmdPath, *cmdArgs)
	})
	mux.HandleFunc("/ws/api", func(w http.ResponseWriter, r *http.Request) {
		serveTUI(w, r, "go", "run ./cmd/api_tui")
	})

	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Web TUI server listening on %s", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func serveTUI(w http.ResponseWriter, r *http.Request, cmdPath, cmdArgs string) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	args := strings.Fields(cmdArgs)
	ptmx, err := startPTY(r.Context(), cmdPath, args)
	if err != nil {
		log.Printf("pty start error: %v", err)
		return
	}
	defer func() {
		_ = ptmx.Close()
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var closeOnce sync.Once
	closeConn := func() {
		closeOnce.Do(func() {
			_ = conn.Close()
		})
	}

	go func() {
		defer cancel()
		buf := make([]byte, 8192)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					log.Printf("pty read error: %v", readErr)
				}
				closeConn()
				return
			}
		}
	}()

	go func() {
		defer cancel()
		if waitErr := ptmx.Wait(ctx); waitErr != nil {
			log.Printf("tui exit: %v", waitErr)
		}
		closeConn()
	}()

	for {
		select {
		case <-ctx.Done():
			closeConn()
			return
		default:
		}

		mt, data, readErr := conn.ReadMessage()
		if readErr != nil {
			return
		}
		if mt != websocket.TextMessage && mt != websocket.BinaryMessage {
			continue
		}
		if mt == websocket.TextMessage && len(data) > 0 && data[0] == '{' {
			var msg resizeMsg
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "resize" {
				_ = ptmx.Resize(msg.Cols, msg.Rows)
				continue
			}
		}
		if _, err := ptmx.Write(data); err != nil {
			return
		}
	}
}
