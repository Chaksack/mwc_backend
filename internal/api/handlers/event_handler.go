package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// EventHandler handles event-related requests
type EventHandler struct {
	db        *gorm.DB
	cfg       *config.Config
	mqService queue.MessageQueueService
}

// NewEventHandler creates a new EventHandler
func NewEventHandler(db *gorm.DB, cfg *config.Config, mqService queue.MessageQueueService) *EventHandler {
	return &EventHandler{db: db, cfg: cfg, mqService: mqService}
}

// CreateEventRequest is the request body for creating an event
type CreateEventRequest struct {
	Title           string            `json:"title" validate:"required"`
	Description     string            `json:"description" validate:"required"`
	StartDate       time.Time         `json:"start_date" validate:"required"`
	EndDate         time.Time         `json:"end_date" validate:"required"`
	Location        string            `json:"location"`
	VirtualEvent    bool              `json:"virtual_event"`
	VirtualEventURL string            `json:"virtual_event_url"`
	EventType       string            `json:"event_type" validate:"required"`
	Audience        string            `json:"audience" validate:"required"`
	MaxAttendees    int               `json:"max_attendees"`
	IsPublished     bool              `json:"is_published"`
	Localizations   map[string]string `json:"localizations"` // Map of language code to localized title/description
}

// CreateEvent creates a new event
func (h *EventHandler) CreateEvent(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only institutions and training centers can create events
	if user.Role != models.InstitutionRole && user.Role != models.TrainingCenterRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only institutions and training centers can create events"})
	}

	// Get institution profile
	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", userID).First(&institutionProfile).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve institution profile"})
	}

	// Parse request
	var req CreateEventRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Title is required"})
	}

	if req.Description == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Description is required"})
	}

	if req.StartDate.IsZero() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Start date is required"})
	}

	if req.EndDate.IsZero() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "End date is required"})
	}

	if req.StartDate.After(req.EndDate) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Start date must be before end date"})
	}

	if req.VirtualEvent && req.VirtualEventURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Virtual event URL is required for virtual events"})
	}

	// Process localizations
	localizedTitles := make(map[string]string)
	localizedDescriptions := make(map[string]string)

	for lang, content := range req.Localizations {
		var localization struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(content), &localization); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("Invalid localization format for language %s", lang)})
		}
		localizedTitles[lang] = localization.Title
		localizedDescriptions[lang] = localization.Description
	}

	// Create event
	event := models.Event{
		CreatorID:             userID,
		InstitutionID:         institutionProfile.ID,
		Title:                 req.Title,
		Description:           req.Description,
		StartDate:             req.StartDate,
		EndDate:               req.EndDate,
		Location:              req.Location,
		VirtualEvent:          req.VirtualEvent,
		VirtualEventURL:       req.VirtualEventURL,
		EventType:             req.EventType,
		Audience:              req.Audience,
		MaxAttendees:          req.MaxAttendees,
		IsPublished:           req.IsPublished,
		LocalizedTitles:       localizedTitles,
		LocalizedDescriptions: localizedDescriptions,
	}

	if req.IsPublished {
		event.PublishedAt = time.Now()
	}

	if err := h.db.Create(&event).Error; err != nil {
		log.Printf("Error creating event: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create event"})
	}

	LogUserAction(h.db, userID, "EVENT_CREATED", event.ID, "Event", fmt.Sprintf("Event created: %s", req.Title), c)

	// If WebSocket is enabled, broadcast event creation to all connected clients
	if h.cfg.WebSocketEnabled {
		// This would be implemented if we had a reference to the WebSocketHandler
		// For now, we'll just log it
		log.Printf("WebSocket: Event created: %s", req.Title)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Event created successfully",
		"event": fiber.Map{
			"id":             event.ID,
			"title":          event.Title,
			"description":    event.Description,
			"start_date":     event.StartDate,
			"end_date":       event.EndDate,
			"location":       event.Location,
			"virtual_event":  event.VirtualEvent,
			"event_type":     event.EventType,
			"audience":       event.Audience,
			"is_published":   event.IsPublished,
			"published_at":   event.PublishedAt,
			"localizations":  event.LocalizedTitles,
		},
	})
}

// GetEvents gets all published events
func (h *EventHandler) GetEvents(c *fiber.Ctx) error {
	// Parse query parameters
	eventType := c.Query("event_type")
	audience := c.Query("audience")
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	language := c.Query("language", h.cfg.DefaultLanguage)

	// Build query
	query := h.db.Where("is_published = ?", true)

	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}

	if audience != "" {
		query = query.Where("audience = ?", audience)
	}

	if startDateStr != "" {
		startDate, err := time.Parse(time.RFC3339, startDateStr)
		if err == nil {
			query = query.Where("start_date >= ?", startDate)
		}
	}

	if endDateStr != "" {
		endDate, err := time.Parse(time.RFC3339, endDateStr)
		if err == nil {
			query = query.Where("end_date <= ?", endDate)
		}
	}

	// Get events
	var events []models.Event
	if err := query.
		Preload("Institution").
		Order("start_date ASC").
		Find(&events).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve events"})
	}

	// Format response
	var formattedEvents []fiber.Map
	for _, event := range events {
		// Get localized title and description if available
		title := event.Title
		description := event.Description

		if localizedTitle, ok := event.LocalizedTitles[language]; ok && localizedTitle != "" {
			title = localizedTitle
		}

		if localizedDescription, ok := event.LocalizedDescriptions[language]; ok && localizedDescription != "" {
			description = localizedDescription
		}

		formattedEvents = append(formattedEvents, fiber.Map{
			"id":             event.ID,
			"title":          title,
			"description":    description,
			"start_date":     event.StartDate,
			"end_date":       event.EndDate,
			"location":       event.Location,
			"virtual_event":  event.VirtualEvent,
			"virtual_event_url": event.VirtualEventURL,
			"event_type":     event.EventType,
			"audience":       event.Audience,
			"published_at":   event.PublishedAt,
			"institution": fiber.Map{
				"id":   event.Institution.ID,
				"name": event.Institution.InstitutionName,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"events": formattedEvents,
	})
}

// GetEvent gets a specific event by ID
func (h *EventHandler) GetEvent(c *fiber.Ctx) error {
	eventID, err := c.ParamsInt("event_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid event ID"})
	}

	language := c.Query("language", h.cfg.DefaultLanguage)

	// Get event
	var event models.Event
	if err := h.db.Preload("Institution").First(&event, eventID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
	}

	// Check if event is published
	if !event.IsPublished {
		// If user is authenticated, check if they are the creator or an admin
		userID, ok := c.Locals("user_id").(uint)
		if !ok || (userID != event.CreatorID && userID != event.Institution.UserID) {
			var user models.User
			if !ok || h.db.First(&user, userID).Error != nil || user.Role != models.AdminRole {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
			}
		}
	}

	// Get localized title and description if available
	title := event.Title
	description := event.Description

	if localizedTitle, ok := event.LocalizedTitles[language]; ok && localizedTitle != "" {
		title = localizedTitle
	}

	if localizedDescription, ok := event.LocalizedDescriptions[language]; ok && localizedDescription != "" {
		description = localizedDescription
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"event": fiber.Map{
			"id":             event.ID,
			"title":          title,
			"description":    description,
			"start_date":     event.StartDate,
			"end_date":       event.EndDate,
			"location":       event.Location,
			"virtual_event":  event.VirtualEvent,
			"virtual_event_url": event.VirtualEventURL,
			"event_type":     event.EventType,
			"audience":       event.Audience,
			"published_at":   event.PublishedAt,
			"institution": fiber.Map{
				"id":   event.Institution.ID,
				"name": event.Institution.InstitutionName,
			},
		},
	})
}

// GetInstitutionEvents gets all events for the current institution
func (h *EventHandler) GetInstitutionEvents(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get institution profile
	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", userID).First(&institutionProfile).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve institution profile"})
	}

	// Get events
	var events []models.Event
	if err := h.db.Where("institution_id = ?", institutionProfile.ID).
		Order("created_at DESC").
		Find(&events).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve events"})
	}

	// Format response
	var formattedEvents []fiber.Map
	for _, event := range events {
		formattedEvents = append(formattedEvents, fiber.Map{
			"id":             event.ID,
			"title":          event.Title,
			"description":    event.Description,
			"start_date":     event.StartDate,
			"end_date":       event.EndDate,
			"location":       event.Location,
			"virtual_event":  event.VirtualEvent,
			"virtual_event_url": event.VirtualEventURL,
			"event_type":     event.EventType,
			"audience":       event.Audience,
			"is_published":   event.IsPublished,
			"published_at":   event.PublishedAt,
			"localizations":  event.LocalizedTitles,
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"events": formattedEvents,
	})
}

// UpdateEvent updates an event
func (h *EventHandler) UpdateEvent(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	eventID, err := c.ParamsInt("event_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid event ID"})
	}

	// Get event
	var event models.Event
	if err := h.db.First(&event, eventID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
	}

	// Check if user is the creator or an admin
	if event.CreatorID != userID {
		var user models.User
		if h.db.First(&user, userID).Error != nil || user.Role != models.AdminRole {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You can only update your own events"})
		}
	}

	// Parse request
	var req CreateEventRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Title is required"})
	}

	if req.Description == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Description is required"})
	}

	if req.StartDate.IsZero() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Start date is required"})
	}

	if req.EndDate.IsZero() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "End date is required"})
	}

	if req.StartDate.After(req.EndDate) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Start date must be before end date"})
	}

	if req.VirtualEvent && req.VirtualEventURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Virtual event URL is required for virtual events"})
	}

	// Process localizations
	localizedTitles := make(map[string]string)
	localizedDescriptions := make(map[string]string)

	for lang, content := range req.Localizations {
		var localization struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(content), &localization); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("Invalid localization format for language %s", lang)})
		}
		localizedTitles[lang] = localization.Title
		localizedDescriptions[lang] = localization.Description
	}

	// Update event
	event.Title = req.Title
	event.Description = req.Description
	event.StartDate = req.StartDate
	event.EndDate = req.EndDate
	event.Location = req.Location
	event.VirtualEvent = req.VirtualEvent
	event.VirtualEventURL = req.VirtualEventURL
	event.EventType = req.EventType
	event.Audience = req.Audience
	event.MaxAttendees = req.MaxAttendees
	event.LocalizedTitles = localizedTitles
	event.LocalizedDescriptions = localizedDescriptions

	// Update published status if changed
	if req.IsPublished != event.IsPublished {
		event.IsPublished = req.IsPublished
		if req.IsPublished {
			event.PublishedAt = time.Now()
		}
	}

	if err := h.db.Save(&event).Error; err != nil {
		log.Printf("Error updating event: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update event"})
	}

	LogUserAction(h.db, userID, "EVENT_UPDATED", event.ID, "Event", fmt.Sprintf("Event updated: %s", req.Title), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Event updated successfully",
		"event": fiber.Map{
			"id":             event.ID,
			"title":          event.Title,
			"description":    event.Description,
			"start_date":     event.StartDate,
			"end_date":       event.EndDate,
			"location":       event.Location,
			"virtual_event":  event.VirtualEvent,
			"event_type":     event.EventType,
			"audience":       event.Audience,
			"is_published":   event.IsPublished,
			"published_at":   event.PublishedAt,
			"localizations":  event.LocalizedTitles,
		},
	})
}

// DeleteEvent deletes an event
func (h *EventHandler) DeleteEvent(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	eventID, err := c.ParamsInt("event_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid event ID"})
	}

	// Get event
	var event models.Event
	if err := h.db.First(&event, eventID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
	}

	// Check if user is the creator or an admin
	if event.CreatorID != userID {
		var user models.User
		if h.db.First(&user, userID).Error != nil || user.Role != models.AdminRole {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You can only delete your own events"})
		}
	}

	// Delete event
	if err := h.db.Delete(&event).Error; err != nil {
		log.Printf("Error deleting event: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete event"})
	}

	LogUserAction(h.db, userID, "EVENT_DELETED", event.ID, "Event", fmt.Sprintf("Event deleted: %s", event.Title), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Event deleted successfully",
	})
}

// FeatureEvent marks an event as featured (admin only)
func (h *EventHandler) FeatureEvent(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Check if user is an admin
	var user models.User
	if h.db.First(&user, userID).Error != nil || user.Role != models.AdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins can feature events"})
	}

	eventID, err := c.ParamsInt("event_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid event ID"})
	}

	// Parse request
	type FeatureRequest struct {
		Featured bool `json:"featured"`
	}
	var req FeatureRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Get event
	var event models.Event
	if err := h.db.First(&event, eventID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
	}

	// Update event
	event.IsFeatured = req.Featured
	if err := h.db.Save(&event).Error; err != nil {
		log.Printf("Error featuring event: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update event"})
	}

	LogUserAction(h.db, userID, "EVENT_FEATURED", event.ID, "Event", fmt.Sprintf("Event featured status changed to %t: %s", req.Featured, event.Title), c)

	var message string
	if req.Featured {
		message = "Event featured successfully"
	} else {
		message = "Event unfeatured successfully"
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": message,
		"event": fiber.Map{
			"id":         event.ID,
			"title":      event.Title,
			"is_featured": event.IsFeatured,
		},
	})
}

// GetFeaturedEvents gets all featured events
func (h *EventHandler) GetFeaturedEvents(c *fiber.Ctx) error {
	language := c.Query("language", h.cfg.DefaultLanguage)

	// Get featured events
	var events []models.Event
	if err := h.db.Where("is_featured = ? AND is_published = ?", true, true).
		Preload("Institution").
		Order("start_date ASC").
		Find(&events).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve featured events"})
	}

	// Format response
	var formattedEvents []fiber.Map
	for _, event := range events {
		// Get localized title and description if available
		title := event.Title
		description := event.Description

		if localizedTitle, ok := event.LocalizedTitles[language]; ok && localizedTitle != "" {
			title = localizedTitle
		}

		if localizedDescription, ok := event.LocalizedDescriptions[language]; ok && localizedDescription != "" {
			description = localizedDescription
		}

		formattedEvents = append(formattedEvents, fiber.Map{
			"id":             event.ID,
			"title":          title,
			"description":    description,
			"start_date":     event.StartDate,
			"end_date":       event.EndDate,
			"location":       event.Location,
			"virtual_event":  event.VirtualEvent,
			"virtual_event_url": event.VirtualEventURL,
			"event_type":     event.EventType,
			"audience":       event.Audience,
			"published_at":   event.PublishedAt,
			"institution": fiber.Map{
				"id":   event.Institution.ID,
				"name": event.Institution.InstitutionName,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"featured_events": formattedEvents,
	})
}
