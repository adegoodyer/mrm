package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// hub fans server-sent events out to every open page.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newHub() *hub { return &hub{clients: map[chan []byte]struct{}{}} }

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// publish sends an event to every page. A page too slow to keep up misses
// it; it catches up from the next "panels" event or a reload.
func (h *hub) publish(event string, v any) {
	msg := sseMessage(event, v)
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func sseMessage(event string, v any) []byte {
	data, _ := json.Marshal(v)
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

// events is the SSE stream. A new page gets the current busy state and
// panels at once, so a reconnect never shows stale counts.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	s.mu.Lock()
	busy := s.busyLocked(false)
	panels := s.panelsLocked()
	s.mu.Unlock()
	_, _ = w.Write(sseMessage("busy", busy))
	_, _ = w.Write(sseMessage("panels", panels))
	flusher.Flush()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case msg := <-ch:
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
