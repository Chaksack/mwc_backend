package handlers

import (
	"encoding/json"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/models"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"gorm.io/gorm"
)

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	db  *gorm.DB
	cfg *config.Config
	// Map of client connections by user ID
	clients    map[uint]*websocket.Conn
	clientsMux sync.RWMutex
}

// NewWebSocketHandler creates a new WebSocketHandler
func NewWebSocketHandler(db *gorm.DB, cfg *config.Config) *WebSocketHandler {
	return &WebSocketHandler{
		db:      db,
		cfg:     cfg,
		clients: make(map[uint]*websocket.Conn),
	}
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// @Summary Handle WebSocket connection
// @Description Handles WebSocket connections for real-time communication
// @Tags websocket
// @Accept json
// @Produce json
// @Success 200 {object} WebSocketMessage "WebSocket connection established"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Security BearerAuth
// @Router /ws [get]
// HandleWebSocket handles WebSocket connections
func (h *WebSocketHandler) HandleWebSocket(c *websocket.Conn) {
	// Get user ID from context
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		log.Println("WebSocket: User ID not found in context")
		return
	}

	// Register client
	h.clientsMux.Lock()
	h.clients[userID] = c
	h.clientsMux.Unlock()

	log.Printf("WebSocket: Client connected: %d", userID)

	// Send welcome message
	welcomeMsg := WebSocketMessage{
		Type: "welcome",
		Payload: map[string]interface{}{
			"message": "Welcome to Montessori World Connect WebSocket server",
			"time":    time.Now(),
		},
	}
	if err := c.WriteJSON(welcomeMsg); err != nil {
		log.Printf("WebSocket: Error sending welcome message: %v", err)
	}

	// Handle incoming messages
	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			log.Printf("WebSocket: Error reading message: %v", err)
			break
		}

		// Parse message
		var wsMsg WebSocketMessage
		if err := json.Unmarshal(msg, &wsMsg); err != nil {
			log.Printf("WebSocket: Error parsing message: %v", err)
			continue
		}

		// Handle message based on type
		switch wsMsg.Type {
		case "ping":
			// Respond with pong
			pongMsg := WebSocketMessage{
				Type: "pong",
				Payload: map[string]interface{}{
					"time": time.Now(),
				},
			}
			if err := c.WriteJSON(pongMsg); err != nil {
				log.Printf("WebSocket: Error sending pong message: %v", err)
			}
		case "message":
			// Handle direct message
			h.handleDirectMessage(userID, wsMsg.Payload)
		default:
			log.Printf("WebSocket: Unknown message type: %s", wsMsg.Type)
		}
	}

	// Unregister client
	h.clientsMux.Lock()
	delete(h.clients, userID)
	h.clientsMux.Unlock()

	log.Printf("WebSocket: Client disconnected: %d", userID)
}

// handleDirectMessage handles a direct message from one user to another
func (h *WebSocketHandler) handleDirectMessage(senderID uint, payload interface{}) {
	// Parse payload
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("WebSocket: Invalid message payload format")
		return
	}

	// Get recipient ID
	recipientIDFloat, ok := payloadMap["recipient_id"].(float64)
	if !ok {
		log.Printf("WebSocket: Missing or invalid recipient_id in message payload")
		return
	}
	recipientID := uint(recipientIDFloat)

	// Get message content
	content, ok := payloadMap["content"].(string)
	if !ok || content == "" {
		log.Printf("WebSocket: Missing or invalid content in message payload")
		return
	}

	// Create message in database
	message := models.Message{
		SenderID:    senderID,
		RecipientID: recipientID,
		Content:     content,
		SentAt:      time.Now(),
		IsRead:      false,
	}

	if err := h.db.Create(&message).Error; err != nil {
		log.Printf("WebSocket: Error creating message in database: %v", err)
		return
	}

	// Send message to recipient if online
	h.clientsMux.RLock()
	recipientConn, ok := h.clients[recipientID]
	h.clientsMux.RUnlock()

	if ok {
		// Get sender details
		var sender models.User
		if err := h.db.Select("id, first_name, last_name, email").First(&sender, senderID).Error; err != nil {
			log.Printf("WebSocket: Error getting sender details: %v", err)
			return
		}

		// Send message to recipient
		notificationMsg := WebSocketMessage{
			Type: "new_message",
			Payload: map[string]interface{}{
				"message_id": message.ID,
				"sender": map[string]interface{}{
					"id":         sender.ID,
					"first_name": sender.FirstName,
					"last_name":  sender.LastName,
					"email":      sender.Email,
				},
				"content": content,
				"sent_at": message.SentAt,
			},
		}

		if err := recipientConn.WriteJSON(notificationMsg); err != nil {
			log.Printf("WebSocket: Error sending message to recipient: %v", err)
		} else {
			log.Printf("WebSocket: Message sent to recipient %d", recipientID)
		}
	} else {
		log.Printf("WebSocket: Recipient %d is offline, message stored in database", recipientID)
	}
}

// @Summary Send notification to user
// @Description Sends a real-time notification to a specific user via WebSocket
// @Tags websocket
// @Accept json
// @Produce json
// @Param user_id path integer true "User ID to send notification to"
// @Param notification_type body string true "Type of notification"
// @Param payload body interface{} true "Notification payload"
// @Success 200 {object} WebSocketMessage "Notification sent successfully"
// @Failure 404 {object} map[string]string "User not connected"
// @Security BearerAuth
// @Router /ws/notify/{user_id} [post]
// SendNotification sends a notification to a specific user
func (h *WebSocketHandler) SendNotification(userID uint, notificationType string, payload interface{}) {
	h.clientsMux.RLock()
	conn, ok := h.clients[userID]
	h.clientsMux.RUnlock()

	if !ok {
		log.Printf("WebSocket: User %d is not connected", userID)
		return
	}

	notification := WebSocketMessage{
		Type:    notificationType,
		Payload: payload,
	}

	if err := conn.WriteJSON(notification); err != nil {
		log.Printf("WebSocket: Error sending notification to user %d: %v", userID, err)
	} else {
		log.Printf("WebSocket: Notification sent to user %d", userID)
	}
}

// @Summary Broadcast notification to all users
// @Description Sends a real-time notification to all connected users via WebSocket
// @Tags websocket
// @Accept json
// @Produce json
// @Param notification_type body string true "Type of notification"
// @Param payload body interface{} true "Notification payload"
// @Success 200 {object} WebSocketMessage "Notification broadcasted successfully"
// @Security BearerAuth
// @Router /ws/broadcast [post]
// BroadcastNotification sends a notification to all connected users
func (h *WebSocketHandler) BroadcastNotification(notificationType string, payload interface{}) {
	notification := WebSocketMessage{
		Type:    notificationType,
		Payload: payload,
	}

	h.clientsMux.RLock()
	for userID, conn := range h.clients {
		if err := conn.WriteJSON(notification); err != nil {
			log.Printf("WebSocket: Error broadcasting notification to user %d: %v", userID, err)
		}
	}
	h.clientsMux.RUnlock()

	log.Printf("WebSocket: Notification broadcasted to all users")
}

// WebSocketUpgradeMiddleware is a middleware that upgrades HTTP connections to WebSocket
// @Summary WebSocket connection upgrade
// @Description Upgrades HTTP connection to WebSocket protocol for real-time communication
// @Tags websocket
// @Success 101 {string} string "Switching Protocols"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 426 {object} map[string]string "Upgrade Required - Client must request WebSocket upgrade"
// @Security BearerAuth
// @Router /ws [get]
func WebSocketUpgradeMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client requested upgrade to the WebSocket protocol
		if websocket.IsWebSocketUpgrade(c) {
			// Get user ID from context
			userID, ok := c.Locals("user_id").(uint)
			if !ok {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
			}

			// Store user ID in locals for the WebSocket handler
			c.Locals("user_id", userID)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	}
}
