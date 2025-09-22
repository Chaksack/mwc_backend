package services

import (
	"fmt"
	"log"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"time"

	"gorm.io/gorm"
)

// NotificationService handles subscription notifications
type NotificationService struct {
	db           *gorm.DB
	emailService email.EmailService
}

// NewNotificationService creates a new notification service
func NewNotificationService(db *gorm.DB, emailService email.EmailService) *NotificationService {
	return &NotificationService{
		db:           db,
		emailService: emailService,
	}
}

// SendSubscriptionDueNotifications sends notifications for subscriptions due within the specified days
func (s *NotificationService) SendSubscriptionDueNotifications(daysAhead int) error {
	// Calculate the date range for subscriptions due
	now := time.Now()
	dueDate := now.AddDate(0, 0, daysAhead)
	
	// Find active subscriptions that will expire within the specified days
	var subscriptions []models.Subscription
	err := s.db.Preload("User").Where(
		"status = ? AND end_date >= ? AND end_date <= ? AND auto_renew = ?",
		models.SubscriptionActive,
		now.Format("2006-01-02"),
		dueDate.Format("2006-01-02 23:59:59"),
		true, // Only notify for auto-renewing subscriptions that might fail
	).Find(&subscriptions).Error
	
	if err != nil {
		return fmt.Errorf("failed to query due subscriptions: %w", err)
	}

	log.Printf("Found %d subscriptions due within %d days", len(subscriptions), daysAhead)

	for _, subscription := range subscriptions {
		err := s.SendSubscriptionDueEmail(subscription)
		if err != nil {
			log.Printf("Failed to send due notification to user %d: %v", subscription.UserID, err)
			continue
		}
		log.Printf("Sent subscription due notification to %s", subscription.User.Email)
	}

	return nil
}

// SendSubscriptionCompletedNotifications sends notifications for newly completed subscriptions
func (s *NotificationService) SendSubscriptionCompletedNotifications() error {
	// Find subscriptions that were created today (completed payments)
	today := time.Now().Format("2006-01-02")
	
	var subscriptions []models.Subscription
	err := s.db.Preload("User").Where(
		"status = ? AND DATE(created_at) = ?",
		models.SubscriptionActive,
		today,
	).Find(&subscriptions).Error
	
	if err != nil {
		return fmt.Errorf("failed to query completed subscriptions: %w", err)
	}

	log.Printf("Found %d subscriptions completed today", len(subscriptions))

	for _, subscription := range subscriptions {
		err := s.SendSubscriptionCompletedEmail(subscription)
		if err != nil {
			log.Printf("Failed to send completion notification to user %d: %v", subscription.UserID, err)
			continue
		}
		log.Printf("Sent subscription completed notification to %s", subscription.User.Email)
	}

	return nil
}

// SendSubscriptionDueEmail sends an email notification for subscription due
func (s *NotificationService) SendSubscriptionDueEmail(subscription models.Subscription) error {
	subject := "Your Subscription is Due for Renewal Soon"
	
	// Calculate days until expiration
	daysUntil := int(time.Until(subscription.EndDate).Hours() / 24)
	
	body := fmt.Sprintf(`
		<h1>Hello %s,</h1>
		<p>Your <strong>%s</strong> subscription is due for renewal in <strong>%d days</strong>.</p>
		<p><strong>Current Subscription Details:</strong></p>
		<ul>
			<li>Plan: %s</li>
			<li>Expires on: %s</li>
			<li>Auto-renewal: %s</li>
		</ul>
		<p>If you have auto-renewal enabled, your subscription will automatically renew. If you need to update your payment method or make any changes, please log in to your account.</p>
		<p>If you have any questions or need assistance, please don't hesitate to contact our support team.</p>
		<p>Thank you for being a valued member of Montessori World Connect!</p>
	`,
		subscription.User.FirstName,
		subscription.Plan,
		daysUntil,
		subscription.Plan,
		subscription.EndDate.Format("January 2, 2006"),
		func() string {
			if subscription.AutoRenew {
				return "Enabled"
			}
			return "Disabled"
		}(),
	)

	return s.emailService.SendEmail(subscription.User.Email, subject, body)
}

// SendSubscriptionCompletedEmail sends an email notification for completed subscription
func (s *NotificationService) SendSubscriptionCompletedEmail(subscription models.Subscription) error {
	subject := "Welcome! Your Subscription is Now Active"
	
	body := fmt.Sprintf(`
		<h1>Welcome %s!</h1>
		<p>Congratulations! Your <strong>%s</strong> subscription has been successfully activated.</p>
		<p><strong>Subscription Details:</strong></p>
		<ul>
			<li>Plan: %s</li>
			<li>Started on: %s</li>
			<li>Expires on: %s</li>
			<li>Auto-renewal: %s</li>
		</ul>
		<p>You now have access to all premium features of Montessori World Connect, including:</p>
		<ul>
			<li>Advanced school search and filtering</li>
			<li>Direct messaging with institutions</li>
			<li>Priority job listings and applications</li>
			<li>Exclusive educational resources</li>
			<li>Community forums and networking opportunities</li>
		</ul>
		<p>Get started by exploring your enhanced dashboard and discovering new opportunities in the Montessori community!</p>
		<p>If you have any questions or need assistance, our support team is here to help.</p>
		<p>Thank you for choosing Montessori World Connect!</p>
	`,
		subscription.User.FirstName,
		subscription.Plan,
		subscription.Plan,
		subscription.StartDate.Format("January 2, 2006"),
		subscription.EndDate.Format("January 2, 2006"),
		func() string {
			if subscription.AutoRenew {
				return "Enabled"
			}
			return "Disabled"
		}(),
	)

	return s.emailService.SendEmail(subscription.User.Email, subject, body)
}