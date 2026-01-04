package services

import (
	"log"
	"time"
)

// SchedulerService handles periodic tasks
type SchedulerService struct {
	notificationService *NotificationService
	stopChan            chan bool
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(notificationService *NotificationService) *SchedulerService {
	return &SchedulerService{
		notificationService: notificationService,
		stopChan:            make(chan bool),
	}
}

// Start begins the scheduler with periodic tasks
func (s *SchedulerService) Start() {
	log.Println("Starting scheduler service...")
	
	// Run daily at 9 AM to check for subscriptions due in 7 days
	go s.scheduleDueNotifications()
	
	// Run daily at 10 AM to send completion notifications for subscriptions created today
	go s.scheduleCompletionNotifications()
}

// Stop stops the scheduler
func (s *SchedulerService) Stop() {
	log.Println("Stopping scheduler service...")
	close(s.stopChan)
}

// scheduleDueNotifications runs daily to check for subscriptions due in 7 days
func (s *SchedulerService) scheduleDueNotifications() {
	ticker := time.NewTicker(24 * time.Hour) // Run every 24 hours
	defer ticker.Stop()
	
	// Run immediately on startup, then every 24 hours
	s.runDueNotificationTask()
	
	for {
		select {
		case <-ticker.C:
			s.runDueNotificationTask()
		case <-s.stopChan:
			log.Println("Due notifications scheduler stopped")
			return
		}
	}
}

// scheduleCompletionNotifications runs daily to send completion notifications
func (s *SchedulerService) scheduleCompletionNotifications() {
	ticker := time.NewTicker(24 * time.Hour) // Run every 24 hours
	defer ticker.Stop()
	
	// Wait 1 hour before starting to avoid overlap with due notifications
	time.Sleep(1 * time.Hour)
	
	// Run immediately, then every 24 hours
	s.runCompletionNotificationTask()
	
	for {
		select {
		case <-ticker.C:
			s.runCompletionNotificationTask()
		case <-s.stopChan:
			log.Println("Completion notifications scheduler stopped")
			return
		}
	}
}

// runDueNotificationTask executes the due notification check
func (s *SchedulerService) runDueNotificationTask() {
	log.Println("Running due subscription notifications task...")
	
	// Check for subscriptions due in 7 days (first reminder)
	if err := s.notificationService.SendSubscriptionDueNotifications(7); err != nil {
		log.Printf("Error sending 7-day due notifications: %v", err)
	}
	
	// Check for subscriptions due tomorrow (final reminder)
	if err := s.notificationService.SendSubscriptionDueNotifications(1); err != nil {
		log.Printf("Error sending 1-day due notifications: %v", err)
	}
	
	log.Println("Due subscription notifications task completed")
}

// runCompletionNotificationTask executes the completion notification check
func (s *SchedulerService) runCompletionNotificationTask() {
	log.Println("Running subscription completion notifications task...")
	
	if err := s.notificationService.SendSubscriptionCompletedNotifications(); err != nil {
		log.Printf("Error sending completion notifications: %v", err)
		return
	}
	
	log.Println("Subscription completion notifications task completed")
}

// RunDueNotificationsNow allows manual triggering of due notifications (for testing/admin)
func (s *SchedulerService) RunDueNotificationsNow(daysAhead int) error {
	log.Printf("Manually running due notifications for %d days ahead...", daysAhead)
	return s.notificationService.SendSubscriptionDueNotifications(daysAhead)
}

// RunCompletionNotificationsNow allows manual triggering of completion notifications (for testing/admin)
func (s *SchedulerService) RunCompletionNotificationsNow() error {
	log.Println("Manually running completion notifications...")
	return s.notificationService.SendSubscriptionCompletedNotifications()
}