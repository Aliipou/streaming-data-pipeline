package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/processor"
	"github.com/aliipou/streaming-data-pipeline/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

type wsUpdate struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Hub manages WebSocket clients and broadcasts pipeline updates.
type Hub struct {
	mu        sync.RWMutex
	clients   map[*websocket.Conn]struct{}
	store     *store.Store
	processor *processor.Processor
	log       *zap.Logger
}

// NewHub creates a Hub.
func NewHub(s *store.Store, p *processor.Processor, log *zap.Logger) *Hub {
	return &Hub{
		clients:   make(map[*websocket.Conn]struct{}),
		store:     s,
		processor: p,
		log:       log,
	}
}

// Run starts the broadcast loop.
func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.broadcast(ctx)
		}
	}
}

func (h *Hub) broadcast(ctx context.Context) {
	h.mu.RLock()
	if len(h.clients) == 0 {
		h.mu.RUnlock()
		return
	}
	h.mu.RUnlock()

	stats := h.processor.GetStats()
	anomalies, _ := h.store.GetRecentAnomalies(ctx, 5)
	events, _ := h.store.GetRecentEvents(ctx, 5, "", "")
	windows := h.processor.GetWindows()

	update := map[string]interface{}{
		"stats":     stats,
		"anomalies": anomalies,
		"events":    events,
		"windows":   windows,
		"ts":        time.Now(),
	}

	data, err := json.Marshal(update)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

// ServeWS upgrades an HTTP connection to WebSocket.
func (h *Hub) ServeWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Warn("ws upgrade", zap.Error(err))
		return
	}
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()

	h.log.Debug("ws client connected", zap.String("remote", conn.RemoteAddr().String()))

	// Read loop to detect disconnection
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}
