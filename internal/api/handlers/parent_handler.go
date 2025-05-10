package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const (
	UnreadMessageNotificationExchange = "notifications.unread_messages.delay.exchange"  // Exchange to publish to with TTL
	UnreadMessageNotificationQueue    = "q.notifications.unread_messages.delay"         // Queue that holds messages for TTL duration
	ActualNotificationExchange        = "notifications.unread_messages.actual.exchange" // DLX where messages go after TTL
	ActualNotificationRoutingKey      = "process.unread.email"                          // Routing key for DLX to route to email processing queue/consumer
	UnreadMessageCheckDelayMs         = 5 * 60 * 1000                                   // 5 minutes in milliseconds
	// Actual processing queue that a worker would listen to:
	ActualUnreadEmailQueue = "q.notifications.unread_messages.email.processing"
)

type ParentHandler struct {
	db           *gorm.DB
	mqService    queue.MessageQueueService
	emailService email.EmailService
}

func NewParentHandler(db *gorm.DB, mq queue.MessageQueueService, emailSvc email.EmailService) *ParentHandler {
	handler := &ParentHandler{db: db, mqService: mq, emailService: emailSvc}
	if mq != nil && mq.(*queue.RabbitMQService).IsInitialized() { // Check if mqService is the actual RabbitMQService and initialized
		// Declare RabbitMQ topology for delayed unread message notifications
		err := mq.DeclareDelayedMessageExchangeAndQueue(
			UnreadMessageNotificationExchange, // This is the exchange messages with TTL are published TO
			UnreadMessageNotificationQueue,    // This is the queue that HOLDS the messages for 5 mins (bound to UnreadMessageNotificationExchange)
			ActualNotificationExchange,        // This is the DLX messages go TO from UnreadMessageNotificationQueue
			ActualNotificationRoutingKey,      // This is the routing key used when messages arrive at ActualNotificationExchange
		)
		if err != nil {
			log.Printf("Error declaring RabbitMQ topology for delayed unread messages: %v", err)
		} else {
			log.Println("RabbitMQ topology for delayed unread message notifications declared.")
			// Also declare the final processing queue and bind it to the ActualNotificationExchange
			_, qErr := mq.DeclareQueue(ActualUnreadEmailQueue, true, false, false, false, nil)
			if qErr != nil {
				log.Printf("Error declaring actual email processing queue '%s': %v", ActualUnreadEmailQueue, qErr)
			} else {
				bErr := mq.BindQueue(ActualUnreadEmailQueue, ActualNotificationRoutingKey, ActualNotificationExchange, false, nil)
				if bErr != nil {
					log.Printf("Error binding queue '%s' to exchange '%s' with key '%s': %v", ActualUnreadEmailQueue, ActualNotificationExchange, ActualNotificationRoutingKey, bErr)
				} else {
					log.Printf("Queue '%s' bound to exchange '%s' for processing unread message emails.", ActualUnreadEmailQueue, ActualNotificationExchange)
					// A separate worker process/goroutine should consume from ActualUnreadEmailQueue
					// For this example, the webhook /webhooks/notify-unread-message simulates that consumer's action.
				}
			}
		}
	} else {
		log.Println("RabbitMQ service not fully initialized, skipping DLX setup for parent handler.")
	}
	return handler
}

type ParentProfileRequest struct {
	PhoneNumber string `json:"phone_number,omitempty"`
	// Add other parent-specific profile fields here
}

type MessageRequest struct {
	Content string `json:"content" validate:"required"`
}

// UnreadMessagePayload is the data sent to RabbitMQ
type UnreadMessagePayload struct {
	MessageID   uint `json:"message_id"`
	RecipientID uint `json:"recipient_id"`
	SenderID    uint `json:"sender_id"`
}

func (h *ParentHandler) CreateOrUpdateParentProfile(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	req := new(ParentProfileRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate req

	var profile models.ParentProfile
	err := h.db.Where("user_id = ?", actorUserID).First(&profile).Error
	isNewProfile := false
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			isNewProfile = true
			profile.UserID = actorUserID
		} else {
			LogUserAction(h.db, actorUserID, "PARENT_PROFILE_FETCH_FAIL", actorUserID, "ParentProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
		}
	}
	// Update fields from req, e.g., profile.PhoneNumber = req.PhoneNumber

	if err := h.db.Save(&profile).Error; err != nil {
		actionType := "PARENT_PROFILE_UPDATE_FAIL"
		if isNewProfile {
			actionType = "PARENT_PROFILE_CREATE_FAIL"
		}
		LogUserAction(h.db, actorUserID, actionType, profile.ID, "ParentProfile", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save parent profile: " + err.Error()})
	}
	actionType := "PARENT_PROFILE_UPDATE_SUCCESS"
	if isNewProfile {
		actionType = "PARENT_PROFILE_CREATE_SUCCESS"
	}
	LogUserAction(h.db, actorUserID, actionType, profile.ID, "ParentProfile", "Profile saved", c)
	return c.Status(fiber.StatusOK).JSON(profile)
}

// SearchSchools for parents (can reuse EducatorHandler.SearchSchools or GetPublicSchools)
func (h *ParentHandler) SearchSchools(c *fiber.Ctx) error {
	return GetPublicSchools(h.db)(c)
}

// SaveSchool for parents
func (h *ParentHandler) SaveSchool(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("school_id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	var parentProfile models.ParentProfile
	if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&parentProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Parent profile not found."})
	}
	var school models.School
	if err := h.db.First(&school, uint(schoolID)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found."})
	}

	for _, savedSchool := range parentProfile.SavedSchools {
		if savedSchool.ID == uint(schoolID) {
			LogUserAction(h.db, actorUserID, "PARENT_SCHOOL_SAVE_FAIL_ALREADY_SAVED", uint(schoolID), "School", "Already saved", c)
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "School already saved."})
		}
	}

	if err := h.db.Model(&parentProfile).Association("SavedSchools").Append(&school); err != nil {
		LogUserAction(h.db, actorUserID, "PARENT_SCHOOL_SAVE_FAIL_DB", uint(schoolID), "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save school: " + err.Error()})
	}
	LogUserAction(h.db, actorUserID, "PARENT_SCHOOL_SAVE_SUCCESS", uint(schoolID), "School", "School saved", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "School saved successfully."})
}

// DeleteSavedSchool for parents
func (h *ParentHandler) DeleteSavedSchool(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("school_id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	var parentProfile models.ParentProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&parentProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Parent profile not found."})
	}
	var school models.School
	if err := h.db.First(&school, uint(schoolID)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found."})
	}
	if err := h.db.Model(&parentProfile).Association("SavedSchools").Delete(&school); err != nil {
		LogUserAction(h.db, actorUserID, "PARENT_SCHOOL_UNSAVE_FAIL_DB", uint(schoolID), "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete saved school: " + err.Error()})
	}
	LogUserAction(h.db, actorUserID, "PARENT_SCHOOL_UNSAVE_SUCCESS", uint(schoolID), "School", "School unsaved", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Saved school deleted successfully."})
}

// GetSavedSchools for parents
func (h *ParentHandler) GetSavedSchools(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	var parentProfile models.ParentProfile
	if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&parentProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Parent profile not found."})
	}
	return c.Status(fiber.StatusOK).JSON(parentProfile.SavedSchools)
}

// SendMessage handles sending a message from one parent to another.
func (h *ParentHandler) SendMessage(c *fiber.Ctx) error {
	senderID, _ := c.Locals("user_id").(uint)
	recipientIDStr := c.Params("recipient_id")
	recipientID, err := strconv.ParseUint(recipientIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid recipient ID format"})
	}

	if senderID == uint(recipientID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot send message to yourself."})
	}

	var recipientUser models.User
	if err := h.db.Where("id = ? AND role = ?", uint(recipientID), models.ParentRole).First(&recipientUser).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Recipient parent not found."})
	}

	req := new(MessageRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	if strings.TrimSpace(req.Content) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Message content cannot be empty."})
	}

	message := models.Message{
		SenderID:    senderID,
		RecipientID: uint(recipientID),
		Content:     req.Content,
		IsRead:      false, // Default to unread
	}

	if err := h.db.Create(&message).Error; err != nil {
		LogUserAction(h.db, senderID, "PARENT_MSG_SEND_FAIL_DB", message.ID, "Message", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send message: " + err.Error()})
	}

	// Publish to RabbitMQ for delayed notification check if mqService is available
	if h.mqService != nil && h.mqService.(*queue.RabbitMQService).IsInitialized() {
		payload := UnreadMessagePayload{
			MessageID:   message.ID,
			RecipientID: message.RecipientID,
			SenderID:    message.SenderID,
		}
		payloadBytes, MqErr := json.Marshal(payload)
		if MqErr != nil {
			log.Printf("Error marshalling unread message payload for MQ: %v", MqErr)
			LogUserAction(h.db, senderID, "PARENT_MSG_SEND_WARN_MQ_MARSHAL", message.ID, "Message", MqErr.Error(), c)
		} else {
			// Publish to the delay exchange, using the delay queue name as routing key for direct-to-queue via exchange
			MqErr = h.mqService.Publish(
				c.Context(),
				UnreadMessageNotificationExchange, // Exchange that routes to the delay queue
				UnreadMessageNotificationQueue,    // Routing key (often same as queue name for direct binding)
				payloadBytes,
				UnreadMessageCheckDelayMs,
			)
			if MqErr != nil {
				log.Printf("Error publishing unread message check to RabbitMQ for MessageID %d: %v", message.ID, MqErr)
				LogUserAction(h.db, senderID, "PARENT_MSG_SEND_WARN_MQ_PUBLISH", message.ID, "Message", MqErr.Error(), c)
			} else {
				log.Printf("Published unread message check for MessageID %d to RabbitMQ.", message.ID)
				LogUserAction(h.db, senderID, "PARENT_MSG_SEND_MQ_PUBLISHED", message.ID, "Message", "MQ task for unread check published", c)
			}
		}
	} else {
		log.Println("RabbitMQ service not available or not initialized, skipping delayed notification task for message.")
		LogUserAction(h.db, senderID, "PARENT_MSG_SEND_WARN_MQ_UNAVAILABLE", message.ID, "Message", "MQ unavailable for unread check", c)
	}

	LogUserAction(h.db, senderID, "PARENT_MSG_SEND_SUCCESS", message.ID, "Message", fmt.Sprintf("Message sent to user %d", recipientID), c)
	return c.Status(fiber.StatusCreated).JSON(message)
}

// GetMessages retrieves messages for the logged-in parent.
func (h *ParentHandler) GetMessages(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	// Pagination
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var messages []models.Message
	query := h.db.Preload("Sender").Preload("Recipient").
		Where("sender_id = ? OR recipient_id = ?", actorUserID, actorUserID).
		Order("sent_at desc").
		Offset(offset).Limit(limit)

	if err := query.Find(&messages).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve messages: " + err.Error()})
	}

	var total int64
	h.db.Model(&models.Message{}).Where("sender_id = ? OR recipient_id = ?", actorUserID, actorUserID).Count(&total)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": messages,
		"meta": fiber.Map{
			"total":     total,
			"page":      page,
			"limit":     limit,
			"last_page": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// MarkMessageAsRead marks a specific message as read.
func (h *ParentHandler) MarkMessageAsRead(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	messageIDStr := c.Params("message_id")
	messageID, err := strconv.ParseUint(messageIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid message ID format"})
	}

	var message models.Message
	// Ensure the message is for the current user and they are the recipient
	if err := h.db.Where("id = ? AND recipient_id = ?", uint(messageID), actorUserID).First(&message).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Message not found or you are not the recipient."})
	}

	if message.IsRead {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Message already marked as read.", "message_data": message})
	}

	now := time.Now()
	message.IsRead = true
	message.ReadAt = &now

	if err := h.db.Save(&message).Error; err != nil {
		LogUserAction(h.db, actorUserID, "PARENT_MSG_READ_FAIL_DB", uint(messageID), "Message", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to mark message as read: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "PARENT_MSG_READ_SUCCESS", uint(messageID), "Message", "Message marked as read", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Message marked as read successfully.", "message_data": message})
}
