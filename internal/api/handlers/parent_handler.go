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
	PhoneNumber       string `json:"phone_number,omitempty"`
	ProfileVisibility string `json:"profile_visibility,omitempty"` // "public" or "private"
	ParentAge         int    `json:"parent_age,omitempty"`
	SchoolIDs         []uint `json:"school_ids,omitempty"` // Schools the parent's children attend
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

// CreateOrUpdateParentProfile creates or updates a parent's profile
// @Summary Create or update parent profile
// @Description Creates a new parent profile or updates an existing one
// @Tags parent,profile
// @Accept multipart/form-data
// @Produce json
// @Param profile_visibility formData string false "Profile visibility (public or private)"
// @Param parent_age formData integer false "Parent age"
// @Param school_ids formData string false "Comma-separated school IDs"
// @Param profile_picture formData file false "Profile picture file"
// @Success 200 {object} models.ParentProfile "Profile created or updated successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/profile [post]
func (h *ParentHandler) CreateOrUpdateParentProfile(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	// Parse form data instead of JSON
	profileVisibility := c.FormValue("profile_visibility")
	parentAgeStr := c.FormValue("parent_age")
	schoolIDsStr := c.FormValue("school_ids")

	// Parse parent age
	var parentAge int
	if parentAgeStr != "" {
		if val, err := strconv.Atoi(parentAgeStr); err == nil {
			parentAge = val
		}
	}

	// Parse school IDs
	var schoolIDs []uint
	if schoolIDsStr != "" {
		for _, idStr := range strings.Split(schoolIDsStr, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := strconv.ParseUint(idStr, 10, 32); err == nil {
				schoolIDs = append(schoolIDs, uint(id))
			}
		}
	}
	// Validate profile visibility
	if profileVisibility != "" && profileVisibility != "public" && profileVisibility != "private" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Profile visibility must be 'public' or 'private'"})
	}

	var profile models.ParentProfile
	err := h.db.Preload("Schools").Where("user_id = ?", actorUserID).First(&profile).Error
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

	// Update fields from request
	if profileVisibility != "" {
		profile.ProfileVisibility = profileVisibility
	}
	if parentAge > 0 {
		profile.ParentAge = parentAge
	}

	// Handle schools relationship
	if len(schoolIDs) > 0 {
		// Clear existing schools and add new ones
		if err := h.db.Model(&profile).Association("Schools").Clear(); err != nil {
			LogUserAction(h.db, actorUserID, "PARENT_PROFILE_SCHOOLS_CLEAR_FAIL", actorUserID, "ParentProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to clear schools: " + err.Error()})
		}

		// Fetch and validate schools
		var schools []models.School
		if err := h.db.Where("id IN ?", schoolIDs).Find(&schools).Error; err != nil {
			LogUserAction(h.db, actorUserID, "PARENT_PROFILE_SCHOOLS_FETCH_FAIL", actorUserID, "School", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schools: " + err.Error()})
		}

		if len(schools) != len(schoolIDs) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Some school IDs are invalid"})
		}

		// Associate schools with profile
		if err := h.db.Model(&profile).Association("Schools").Append(schools); err != nil {
			LogUserAction(h.db, actorUserID, "PARENT_PROFILE_SCHOOLS_APPEND_FAIL", actorUserID, "ParentProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to associate schools: " + err.Error()})
		}
	}

	// Handle profile picture upload if provided
	fileHeader, err := c.FormFile("profile_picture")
	if err == nil && fileHeader != nil {
		// Get user to ensure it exists
		var user models.User
		if err := h.db.First(&user, actorUserID).Error; err == nil {
			// Ensure uploads directory exists
			uploadDir := "./uploads/parent_profiles"
			if err := ensureDir(uploadDir); err == nil {
				// Save file
				dst := fmt.Sprintf("%s/%d_%s", uploadDir, actorUserID, fileHeader.Filename)
				if err := c.SaveFile(fileHeader, dst); err == nil {
					urlPath := "/uploads/parent_profiles/" + fmt.Sprintf("%d_%s", actorUserID, fileHeader.Filename)

					// Set any existing pictures as non-primary
					h.db.Model(&models.UserProfilePicture{}).Where("user_id = ?", actorUserID).Update("is_primary", false)

					// Create new profile picture record
					picture := models.UserProfilePicture{
						UserID:    actorUserID,
						URL:       urlPath,
						FileName:  fileHeader.Filename,
						IsPrimary: true,
					}

					if err := h.db.Create(&picture).Error; err != nil {
						LogUserAction(h.db, actorUserID, "PARENT_PROFILE_PICTURE_FAIL", actorUserID, "UserProfilePicture", "Failed to save picture: "+err.Error(), c)
					} else {
						LogUserAction(h.db, actorUserID, "PARENT_PROFILE_PICTURE_SUCCESS", actorUserID, "UserProfilePicture", "Profile picture uploaded", c)
					}
				} else {
					LogUserAction(h.db, actorUserID, "PARENT_PROFILE_PICTURE_SAVE_FAIL", actorUserID, "System", "Failed to save file: "+err.Error(), c)
				}
			}
		}
	}

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

// GetSchoolDetails returns school details with public parent profiles
// @Summary Get school details with parent profiles
// @Description Get detailed information about a school including public parent profiles whose children attend this school
// @Tags parent,schools
// @Produce json
// @Param school_id path int true "School ID"
// @Success 200 {object} map[string]interface{} "School details with parent profiles"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 404 {object} map[string]string "School not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/schools/{school_id}/details [get]
func (h *ParentHandler) GetSchoolDetails(c *fiber.Ctx) error {
	schoolIDStr := c.Params("school_id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	// Get school details
	var school models.School
	if err := h.db.First(&school, uint(schoolID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch school: " + err.Error()})
	}

	// Get public parent profiles whose children attend this school
	var parentProfiles []models.ParentProfile
	if err := h.db.Joins("JOIN parent_children_schools ON parent_children_schools.parent_profile_id = parent_profiles.id").
		Where("parent_children_schools.school_id = ? AND parent_profiles.profile_visibility = ?", uint(schoolID), "public").
		Preload("User").
		Find(&parentProfiles).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch parent profiles: " + err.Error()})
	}

	// Format parent profiles for response
	var publicParents []map[string]interface{}
	for _, profile := range parentProfiles {
		parentInfo := map[string]interface{}{
			"id":         profile.ID,
			"user_id":    profile.UserID,
			"first_name": profile.User.FirstName,
			"last_name":  profile.User.LastName,
			"parent_age": profile.ParentAge,
		}
		publicParents = append(publicParents, parentInfo)
	}

	response := map[string]interface{}{
		"school": map[string]interface{}{
			"id":            school.ID,
			"name":          school.Name,
			"category":      school.Category,
			"address":       school.Address,
			"city":          school.City,
			"state":         school.State,
			"country":       school.Country,
			"country_code":  school.CountryCode,
			"zip_code":      school.ZipCode,
			"contact_email": school.ContactEmail,
			"contact_phone": school.ContactPhone,
			"website":       school.Website,
		},
		"public_parents": publicParents,
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// SearchSchools for parents (can reuse EducatorHandler.SearchSchools or GetPublicSchools)
// @Summary Search for schools
// @Description Search for schools with various filters and pagination
// @Tags parent,schools
// @Produce json
// @Param name query string false "Filter by school name"
// @Param city query string false "Filter by city"
// @Param country_code query string false "Filter by country code"
// @Param category query string false "Filter by category (school or training_center)"
// @Param page query int false "Page number for pagination" default(1)
// @Param limit query int false "Number of items per page" default(10)
// @Success 200 {object} map[string]interface{} "List of schools with pagination metadata"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/schools/search [get]
func (h *ParentHandler) SearchSchools(c *fiber.Ctx) error {
	return GetPublicSchools(h.db)(c)
}

// SaveSchool for parents
// @Summary Save a school
// @Description Adds a school to the parent's saved schools list
// @Tags parent,schools
// @Produce json
// @Param school_id path int true "School ID"
// @Success 200 {object} map[string]string "School saved successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Parent profile or school not found"
// @Failure 409 {object} map[string]string "School already saved"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/schools/save/{school_id} [post]
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
// @Summary Delete a saved school
// @Description Removes a school from the parent's saved schools list
// @Tags parent,schools
// @Produce json
// @Param school_id path int true "School ID"
// @Success 200 {object} map[string]string "School removed successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Parent profile or school not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/schools/save/{school_id} [delete]
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
// @Summary Get saved schools
// @Description Retrieves the list of schools saved by the parent
// @Tags parent,schools
// @Produce json
// @Success 200 {array} models.School "List of saved schools"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Parent profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/schools/saved [get]
func (h *ParentHandler) GetSavedSchools(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	var parentProfile models.ParentProfile
	if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&parentProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Parent profile not found."})
	}
	return c.Status(fiber.StatusOK).JSON(parentProfile.SavedSchools)
}

// SendMessage handles sending a message from one parent to another.
// @Summary Send a message
// @Description Sends a message to an institution or educator
// @Tags parent,messages
// @Accept json
// @Produce json
// @Param recipient_id path int true "Recipient User ID"
// @Param message body MessageRequest true "Message content"
// @Success 201 {object} models.Message "Message sent successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid recipient ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Recipient not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/messages/send/{recipient_id} [post]
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
// @Summary Get user messages
// @Description Retrieves all messages sent to or by the current user
// @Tags parent,messages
// @Produce json
// @Param unread_only query boolean false "Filter to show only unread messages" default(false)
// @Success 200 {array} models.Message "List of messages"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/messages [get]
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
// @Summary Mark message as read
// @Description Marks a specific message as read by the recipient
// @Tags parent,messages
// @Produce json
// @Param message_id path int true "Message ID"
// @Success 200 {object} map[string]string "Message marked as read successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid message ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden - can only mark messages addressed to you"
// @Failure 404 {object} map[string]string "Message not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /parent/messages/{message_id}/read [post]
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

// ListPublicParents returns a list of parents with public profiles (visible only to other parents)
// @Summary List public parent profiles
// @Description Retrieves a list of parents whose profiles are set to public. Only accessible by parent role users.
// @Tags parent,profiles
// @Produce json
// @Param page query int false "Page number for pagination" default(1)
// @Param limit query int false "Number of items per page" default(10)
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "List of public parent profiles with pagination"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /parent/public-parents [get]
func (h *ParentHandler) ListPublicParents(c *fiber.Ctx) error {
	actorUserID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User ID not found in token"})
	}

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var profiles []models.ParentProfile
	// Query for public profiles, exclude the current user, preload User info
	query := h.db.Where("profile_visibility = ? AND user_id != ?", "public", actorUserID).
		Preload("User").
		Preload("Schools").
		Offset(offset).
		Limit(limit)

	if err := query.Find(&profiles).Error; err != nil {
		LogUserAction(h.db, actorUserID, "PARENT_LIST_PUBLIC_FAIL", 0, "ParentProfile", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve public parent profiles: " + err.Error()})
	}

	// Get total count for pagination
	var total int64
	h.db.Model(&models.ParentProfile{}).Where("profile_visibility = ? AND user_id != ?", "public", actorUserID).Count(&total)

	LogUserAction(h.db, actorUserID, "PARENT_LIST_PUBLIC_SUCCESS", 0, "ParentProfile", fmt.Sprintf("Listed %d public parents", len(profiles)), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": profiles,
		"meta": fiber.Map{
			"total":     total,
			"page":      page,
			"limit":     limit,
			"last_page": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetPublicParentDetails returns detailed information about a specific public parent profile
// @Summary Get public parent profile details
// @Description Retrieves detailed information about a parent with a public profile. Only accessible by parent role users.
// @Tags parent,profiles
// @Produce json
// @Param id path int true "Parent Profile ID"
// @Security BearerAuth
// @Success 200 {object} models.ParentProfile "Parent profile details"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Profile is private"
// @Failure 404 {object} map[string]string "Parent profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /parent/public-parents/{id} [get]
func (h *ParentHandler) GetPublicParentDetails(c *fiber.Ctx) error {
	actorUserID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User ID not found in token"})
	}

	parentIDStr := c.Params("id")
	parentID, err := strconv.ParseUint(parentIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid parent profile ID format"})
	}

	var profile models.ParentProfile
	if err := h.db.Preload("User").Preload("Schools").First(&profile, uint(parentID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			LogUserAction(h.db, actorUserID, "PARENT_VIEW_PUBLIC_FAIL_NOTFOUND", uint(parentID), "ParentProfile", "Profile not found", c)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Parent profile not found"})
		}
		LogUserAction(h.db, actorUserID, "PARENT_VIEW_PUBLIC_FAIL_DB", uint(parentID), "ParentProfile", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}

	// Check if profile is public
	if profile.ProfileVisibility != "public" {
		LogUserAction(h.db, actorUserID, "PARENT_VIEW_PUBLIC_FAIL_PRIVATE", uint(parentID), "ParentProfile", "Profile is private", c)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "This profile is private and cannot be viewed"})
	}

	LogUserAction(h.db, actorUserID, "PARENT_VIEW_PUBLIC_SUCCESS", uint(parentID), "ParentProfile", "Viewed public parent profile", c)

	return c.Status(fiber.StatusOK).JSON(profile)
}
