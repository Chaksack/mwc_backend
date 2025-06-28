package handlers

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"log"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
)

// @Summary Webhook for Unread Message Notification
// @Description Receives a payload from the message queue system to process and send email notifications for unread messages. This endpoint is intended for internal system use and should be secured.
// @Tags webhooks,notifications
// @Accept json
// @Produce json
// @Param payload body UnreadMessagePayload true "Details of the unread message to be processed for notification"
// @Success 200 {object} map[string]string "Notification processed successfully (email sent or message already read)"
// @Failure 400 {object} map[string]string "Bad request or invalid payload"
// @Failure 401 {object} map[string]string "Unauthorized access (if webhook security is implemented)"
// @Failure 500 {object} map[string]string "Internal server error (e.g., database error, email sending failure)"
// @Router /webhooks/notify-unread-message [post]
func HandleUnreadMessageNotification(db *gorm.DB, emailService email.EmailService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// TODO: Implement robust webhook security. Example: Check a secret header.
		// webhookSecret := os.Getenv("WEBHOOK_SECRET")
		// if c.Get("X-Webhook-Secret") != webhookSecret {
		//    log.Println("[Webhook] Unauthorized attempt to access unread message notifier.")
		//    return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		// }

		var payload UnreadMessagePayload // Defined in parent_handler.go
		if err := c.BodyParser(&payload); err != nil {
			log.Printf("[Webhook] Error parsing unread message payload: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse payload: " + err.Error()})
		}

		log.Printf("[Webhook] Received task to process unread message notification for MessageID %d, RecipientID %d", payload.MessageID, payload.RecipientID)

		var message models.Message
		// Check if the message still exists and is still unread by the recipient.
		err := db.Preload("Sender").Preload("Recipient").Where("id = ? AND recipient_id = ? AND is_read = ?", payload.MessageID, payload.RecipientID, false).First(&message).Error

		if err != nil {
			if err == gorm.ErrRecordNotFound {
				log.Printf("[Webhook] MessageID %d for RecipientID %d not found, already read, or deleted. No notification needed.", payload.MessageID, payload.RecipientID)
				return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Message already read or not found, notification not sent."}) // 200 OK, task handled.
			}
			log.Printf("[Webhook] Database error fetching message %d: %v", payload.MessageID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error fetching message details"}) // 500, might retry if consumer is set up for it.
		}

		// If we found the message and it's still unread, send the email.
		log.Printf("[Webhook] MessageID %d is confirmed unread. Sending email notification to Recipient: %s.", message.ID, message.Recipient.Email)

		senderName := "A user"
		if message.Sender.ID != 0 { // Check if Sender was preloaded
			senderName = fmt.Sprintf("%s %s", message.Sender.FirstName, message.Sender.LastName)
		}

		recipientName := message.Recipient.FirstName
		if recipientName == "" {
			recipientName = message.Recipient.Email // Fallback to email if name is empty
		}

		emailSubject := fmt.Sprintf("You have an unread message from %s", senderName)
		emailBody := fmt.Sprintf(
			"<h1>Hi %s,</h1><p>You have an unread message on our platform from %s.</p><p><b>Message snippet:</b> \"%s\"</p><p>Please log in to view the full message and reply.</p><p>Thank you,<br/>The Platform Team</p>",
			recipientName,
			senderName,
			truncateMessage(message.Content, 100), // Use the helper
		)

		if err := emailService.SendEmail(message.Recipient.Email, emailSubject, emailBody); err != nil {
			log.Printf("[Webhook] Failed to send unread message email to %s for MessageID %d: %v", message.Recipient.Email, message.ID, err)
			// This is an error in processing, might warrant a 5xx for retry by consumer.
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send notification email"})
		}

		log.Printf("[Webhook] Unread message email notification sent successfully to %s for MessageID %d.", message.Recipient.Email, message.ID)
		LogUserAction(db, 0, "SYSTEM_UNREAD_MSG_EMAIL_SENT", message.ID, "Message", fmt.Sprintf("Email sent to %s", message.Recipient.Email), c)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Unread message notification email sent successfully."})
	}
}
