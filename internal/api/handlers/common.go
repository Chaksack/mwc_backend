package handlers

import (
	"log"
	"mwc_backend/internal/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// LogUserAction creates an entry in the ActionLog table.
// actorUserID is the ID of the user performing the action. Can be 0 for system actions.
// actionType is a string describing the action, e.g., "USER_LOGIN", "SCHOOL_CREATE".
// targetID is the ID of the entity being affected, if any.
// targetType is the type of the entity being affected, e.g., "User", "School".
// details can be a string or JSON string with more info.
func LogUserAction(db *gorm.DB, actorUserID uint, actionType string, targetID uint, targetType string, details string, c *fiber.Ctx) {
	var userIDForLog *uint
	if actorUserID != 0 {
		userIDForLog = &actorUserID
	}

	logEntry := models.ActionLog{
		UserID:     userIDForLog,
		ActionType: actionType,
		TargetID:   targetID,
		TargetType: targetType,
		Details:    details,
		IPAddress:  c.IP(),
		UserAgent:  string(c.Request().Header.UserAgent()),
	}

	if err := db.Create(&logEntry).Error; err != nil {
		log.Printf("Error logging action '%s' for user %v: %v", actionType, actorUserID, err)
	}
}

// GetPublicSchools allows anyone to search for schools.
func GetPublicSchools(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		query := db.Model(&models.School{})

		// Example filters (can be expanded)
		if name := c.Query("name"); name != "" {
			query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+name+"%")
		}
		if city := c.Query("city"); city != "" {
			query = query.Where("LOWER(city) LIKE LOWER(?)", "%"+city+"%")
		}
		if countryCode := c.Query("country_code"); countryCode != "" {
			query = query.Where("country_code = ?", countryCode)
		}
		// Add pagination
		page, _ := strconv.Atoi(c.Query("page", "1"))
		limit, _ := strconv.Atoi(c.Query("limit", "10"))
		offset := (page - 1) * limit
		query = query.Offset(offset).Limit(limit)

		var schools []models.School
		if err := query.Find(&schools).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error while fetching schools: " + err.Error()})
		}

		var total int64
		db.Model(&models.School{}).Count(&total) // Get total count for pagination metadata

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"data": schools,
			"meta": fiber.Map{
				"total":     total,
				"page":      page,
				"limit":     limit,
				"last_page": (total + int64(limit) - 1) / int64(limit),
			},
		})
	}
}

// truncateMessage shortens a message string to a max length, adding ellipsis.
func truncateMessage(msg string, maxLength int) string {
	if len(msg) <= maxLength {
		return msg
	}
	if maxLength <= 3 {
		return msg[:maxLength] // Not enough space for ellipsis
	}
	return msg[:maxLength-3] + "..."
}
