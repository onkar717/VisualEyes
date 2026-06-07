// Package ws provides a WebSocket broadcaster for real-time metric streaming.
// A single Broadcaster fans out byte payloads to all connected clients using
// per-client buffered channels, so one slow client can never block others.
package ws

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 64 * 1024
	chanBufSize    = 16
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 64 * 1024,
	// Origin validation is handled by the CORS middleware, so we allow all here.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Broadcaster fans out payloads to all registered WebSocket clients.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

// NewBroadcaster creates a ready-to-use Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{clients: make(map[chan []byte]struct{})}
}

// register adds a client channel under the write lock.
func (b *Broadcaster) register(ch chan []byte) {
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
}

// unregister removes and closes a client channel under the write lock.
func (b *Broadcaster) unregister(ch chan []byte) {
	b.mu.Lock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Send fans out payload to every registered client. Clients whose buffers are
// full are skipped   they will receive the next broadcast instead.
func (b *Broadcaster) Send(payload []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- payload:
		default:
			slog.Debug("ws broadcaster: client channel full, skipping message")
		}
	}
}

// Len returns the number of currently connected clients.
func (b *Broadcaster) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// ServeClient upgrades the HTTP connection to WebSocket and pumps messages from
// the Broadcaster until the client disconnects. It blocks until the connection
// is closed, so call it in its own goroutine when you need non-blocking behaviour.
func (b *Broadcaster) ServeClient(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "remote", r.RemoteAddr, "error", err)
		return
	}
	defer conn.Close()

	ch := make(chan []byte, chanBufSize)
	b.register(ch)
	defer b.unregister(ch)

	// Reader goroutine: handle pong frames and detect disconnection.
	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()

	slog.Debug("ws client connected", "remote", r.RemoteAddr)
	defer slog.Debug("ws client disconnected", "remote", r.RemoteAddr)

	for {
		select {
		case <-done:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-pingTicker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
