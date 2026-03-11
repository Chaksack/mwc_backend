package handlers

import (
	"mwc_backend/internal/models"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type NotificationHandler struct {
	db *gorm.DB
}

func NewNotificationHandler(db *gorm.DB) *NotificationHandler {
	return &NotificationHandler{db: db}
}

// GetNotifications returns notifications for the logged-in user
// @Summary Get user notifications
// @Tags notification
// @Produce json
// @Success 200 {array} models.Notification
// @Failure 401 {object} map[string]string
// @Router /notifications [get]
func (h *NotificationHandler) GetNotifications(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	var notifications []models.Notification
	h.db.Where("user_id = ?", userID).Order("created_at desc").Find(&notifications)
	return c.JSON(notifications)
}

// MarkNotificationRead marks a notification as read
// @Summary Mark notification as read
// @Tags notification
// @Param id path int true "Notification ID"
// @Produce json
// @Success 200 {object} models.Notification
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /notifications/{id}/read [put]
func (h *NotificationHandler) MarkNotificationRead(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	id := c.Params("id")
	var notification models.Notification
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&notification).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Notification not found"})
	}
	notification.Read = true
	h.db.Save(&notification)
	return c.JSON(notification)
}
