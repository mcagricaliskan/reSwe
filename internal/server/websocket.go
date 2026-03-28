package server

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/cagri/reswe/internal/models"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
)

// Hub manages WebSocket connections and broadcasts messages
type Hub struct {
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]bool),
	}
}

func (h *Hub) HandleWS(c fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}
	return websocket.New(func(conn *websocket.Conn) {
		h.mu.Lock()
		h.clients[conn] = true
		h.mu.Unlock()
		log.Printf("websocket client connected (%d total)", len(h.clients))

		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
			log.Printf("websocket client disconnected (%d total)", len(h.clients))
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	})(c)
}

func (h *Hub) Broadcast(msg models.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ws marshal error: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("ws write error: %v", err)
			conn.Close()
			delete(h.clients, conn)
		}
	}
}
