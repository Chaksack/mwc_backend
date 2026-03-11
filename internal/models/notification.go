package models

import (
	"time"
)

// Notification represents a user notification
// @Description Notification information
// @Schema models.Notification
type Notification struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	UserID    uint      `json:"user_id" gorm:"index"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Read      bool      `json:"read" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at"`
}
