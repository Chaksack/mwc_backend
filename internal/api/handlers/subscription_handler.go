package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/sub"
	"github.com/stripe/stripe-go/v72/webhook"
	"gorm.io/gorm"
)

// SubscriptionHandler handles subscription-related requests
type SubscriptionHandler struct {
	db        *gorm.DB
	cfg       *config.Config
	mqService queue.MessageQueueService
}

// NewSubscriptionHandler creates a new SubscriptionHandler
func NewSubscriptionHandler(db *gorm.DB, cfg *config.Config, mqService queue.MessageQueueService) *SubscriptionHandler {
	// Initialize Stripe with the API key
	stripe.Key = cfg.StripeSecretKey
	return &SubscriptionHandler{db: db, cfg: cfg, mqService: mqService}
}

// CreateCheckoutSession creates a Stripe checkout session for subscription
// @Summary Create a checkout session for subscription
// @Description Creates a Stripe checkout session for a user to subscribe to a plan
// @Tags subscription
// @Accept json
// @Produce json
// @Param plan query string false "Subscription plan (monthly or annual)" Enums(monthly, annual)
// @Success 200 {object} map[string]interface{} "Checkout session created successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 409 {object} map[string]string "User already has an active subscription"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /subscription/checkout [post]
func (h *SubscriptionHandler) CreateCheckoutSession(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get the plan from the request
	plan := c.Query("plan", string(models.MonthlyPlan))
	if plan != string(models.MonthlyPlan) && plan != string(models.AnnualPlan) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid plan. Must be 'monthly' or 'annual'"})
	}

	// Get the user
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Check if user already has an active subscription
	var existingSubscription models.Subscription
	err := h.db.Where("user_id = ? AND status = ?", userID, models.SubscriptionActive).First(&existingSubscription).Error
	if err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "User already has an active subscription"})
	} else if err != gorm.ErrRecordNotFound {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing subscription"})
	}

	// Determine the price ID based on the plan
	var priceID string
	if plan == string(models.MonthlyPlan) {
		priceID = h.cfg.StripeMonthlyPriceID
	} else {
		priceID = h.cfg.StripeAnnualPriceID
	}

	// Create or retrieve Stripe customer
	var stripeCustomerID string
	var existingCustomer models.Subscription
	err = h.db.Where("user_id = ? AND stripe_customer_id != ?", userID, "").First(&existingCustomer).Error
	if err == nil {
		stripeCustomerID = existingCustomer.StripeCustomerID
	} else {
		// Create a new customer in Stripe
		customerParams := &stripe.CustomerParams{
			Email: stripe.String(user.Email),
			Name:  stripe.String(fmt.Sprintf("%s %s", user.FirstName, user.LastName)),
		}
		customerParams.AddMetadata("user_id", strconv.FormatUint(uint64(userID), 10))
		newCustomer, err := customer.New(customerParams)
		if err != nil {
			log.Printf("Error creating Stripe customer: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create Stripe customer"})
		}
		stripeCustomerID = newCustomer.ID
	}

	// Create checkout session
	successURL := fmt.Sprintf("%s/subscription/success?session_id={CHECKOUT_SESSION_ID}", c.BaseURL())
	cancelURL := fmt.Sprintf("%s/subscription/cancel", c.BaseURL())

	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(stripeCustomerID),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{
				"user_id": strconv.FormatUint(uint64(userID), 10),
				"plan":    plan,
			},
		},
	}

	s, err := session.New(params)
	if err != nil {
		log.Printf("Error creating checkout session: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create checkout session"})
	}

	LogUserAction(h.db, userID, "SUBSCRIPTION_CHECKOUT_CREATED", userID, "User", fmt.Sprintf("Checkout session created for %s plan", plan), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"session_id": s.ID,
		"url":        s.URL,
	})
}

// HandleStripeWebhook handles Stripe webhook events
// @Summary Handle Stripe webhook events
// @Description Processes webhook events from Stripe for subscription management
// @Tags webhooks
// @Accept json
// @Produce json
// @Param Stripe-Signature header string true "Stripe signature for webhook verification"
// @Success 200 {object} map[string]bool "Webhook event processed successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /webhooks/stripe [post]
func (h *SubscriptionHandler) HandleStripeWebhook(c *fiber.Ctx) error {
	// Get the webhook secret
	webhookSecret := h.cfg.StripeWebhookSecret

	// Get the signature from the header
	signature := c.Get("Stripe-Signature")
	if signature == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing Stripe signature"})
	}

	// Get the request body
	body := c.Body()

	// Verify the webhook signature
	var event stripe.Event
	if webhookSecret != "" {
		var err error
		event, err = webhook.ConstructEvent(body, signature, webhookSecret)
		if err != nil {
			log.Printf("Error verifying webhook signature: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Stripe signature"})
		}
	} else {
		// If webhook secret is not set, parse the event without verification
		if err := c.BodyParser(&event); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Stripe event"})
		}
	}

	// Handle the event
	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			log.Printf("Error parsing checkout session: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid checkout session"})
		}

		// Get the user ID and plan from the subscription metadata
		userIDStr, ok := session.Subscription.Metadata["user_id"]
		if !ok {
			log.Printf("User ID not found in subscription metadata")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "User ID not found in subscription metadata"})
		}

		userID, err := strconv.ParseUint(userIDStr, 10, 64)
		if err != nil {
			log.Printf("Error parsing user ID: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
		}

		plan, ok := session.Subscription.Metadata["plan"]
		if !ok {
			log.Printf("Plan not found in subscription metadata")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Plan not found in subscription metadata"})
		}

		// Create a new subscription record
		var endDate time.Time
		if plan == string(models.MonthlyPlan) {
			endDate = time.Now().AddDate(0, 1, 0) // Add 1 month
		} else {
			endDate = time.Now().AddDate(1, 0, 0) // Add 1 year
		}

		subscription := models.Subscription{
			UserID:               uint(userID),
			Plan:                 models.SubscriptionPlan(plan),
			Status:               models.SubscriptionActive,
			StartDate:            time.Now(),
			EndDate:              endDate,
			AutoRenew:            true,
			StripeCustomerID:     session.Customer.ID,
			StripeSubscriptionID: session.Subscription.ID,
		}

		if err := h.db.Create(&subscription).Error; err != nil {
			log.Printf("Error creating subscription record: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create subscription record"})
		}

		LogUserAction(h.db, uint(userID), "SUBSCRIPTION_CREATED", uint(userID), "User", fmt.Sprintf("Subscription created for %s plan", plan), c)

	case "customer.subscription.updated":
		var subscription stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &subscription)
		if err != nil {
			log.Printf("Error parsing subscription: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid subscription"})
		}

		// Get the user ID from the subscription metadata
		userIDStr, ok := subscription.Metadata["user_id"]
		if !ok {
			log.Printf("User ID not found in subscription metadata")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "User ID not found in subscription metadata"})
		}

		userID, err := strconv.ParseUint(userIDStr, 10, 64)
		if err != nil {
			log.Printf("Error parsing user ID: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
		}

		// Update the subscription record
		var existingSubscription models.Subscription
		err = h.db.Where("stripe_subscription_id = ?", subscription.ID).First(&existingSubscription).Error
		if err != nil {
			log.Printf("Error finding subscription record: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find subscription record"})
		}

		// Update the subscription status based on the Stripe status
		var status models.SubscriptionStatus
		switch subscription.Status {
		case "active":
			status = models.SubscriptionActive
		case "canceled", "unpaid", "past_due":
			status = models.SubscriptionInactive
		default:
			status = models.SubscriptionInactive
		}

		existingSubscription.Status = status
		if subscription.CancelAt > 0 {
			cancelAt := time.Unix(subscription.CancelAt, 0)
			existingSubscription.CancelledAt = &cancelAt
			existingSubscription.AutoRenew = false
		}

		if err := h.db.Save(&existingSubscription).Error; err != nil {
			log.Printf("Error updating subscription record: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update subscription record"})
		}

		LogUserAction(h.db, uint(userID), "SUBSCRIPTION_UPDATED", uint(userID), "User", fmt.Sprintf("Subscription updated to status: %s", status), c)

	case "customer.subscription.deleted":
		var subscription stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &subscription)
		if err != nil {
			log.Printf("Error parsing subscription: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid subscription"})
		}

		// Get the user ID from the subscription metadata
		userIDStr, ok := subscription.Metadata["user_id"]
		if !ok {
			log.Printf("User ID not found in subscription metadata")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "User ID not found in subscription metadata"})
		}

		userID, err := strconv.ParseUint(userIDStr, 10, 64)
		if err != nil {
			log.Printf("Error parsing user ID: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
		}

		// Update the subscription record
		var existingSubscription models.Subscription
		err = h.db.Where("stripe_subscription_id = ?", subscription.ID).First(&existingSubscription).Error
		if err != nil {
			log.Printf("Error finding subscription record: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find subscription record"})
		}

		existingSubscription.Status = models.SubscriptionCanceled
		now := time.Now()
		existingSubscription.CancelledAt = &now
		existingSubscription.AutoRenew = false

		if err := h.db.Save(&existingSubscription).Error; err != nil {
			log.Printf("Error updating subscription record: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update subscription record"})
		}

		LogUserAction(h.db, uint(userID), "SUBSCRIPTION_CANCELED", uint(userID), "User", "Subscription canceled", c)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"received": true})
}

// GetUserSubscription gets the current user's subscription
// @Summary Get user subscription
// @Description Retrieves the current user's subscription details
// @Tags subscription
// @Produce json
// @Success 200 {object} map[string]interface{} "Subscription details"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 404 {object} map[string]string "No subscription found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /subscription/status [get]
func (h *SubscriptionHandler) GetUserSubscription(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	var subscription models.Subscription
	err := h.db.Where("user_id = ?", userID).Order("created_at DESC").First(&subscription).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No subscription found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve subscription"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"subscription": fiber.Map{
			"id":         subscription.ID,
			"plan":       subscription.Plan,
			"status":     subscription.Status,
			"start_date": subscription.StartDate,
			"end_date":   subscription.EndDate,
			"auto_renew": subscription.AutoRenew,
		},
	})
}

// CancelRequest is the request body for canceling a subscription
type CancelRequest struct {
	Reason string `json:"reason"`
}

// CancelSubscription cancels the current user's subscription
// @Summary Cancel user subscription
// @Description Cancels the current user's active subscription
// @Tags subscription
// @Accept json
// @Produce json
// @Param request body CancelRequest false "Cancellation reason"
// @Success 200 {object} map[string]string "Subscription canceled successfully"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 404 {object} map[string]string "No active subscription found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /subscription/cancel [post]
func (h *SubscriptionHandler) CancelSubscription(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	var subscription models.Subscription
	err := h.db.Where("user_id = ? AND status = ?", userID, models.SubscriptionActive).First(&subscription).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No active subscription found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve subscription"})
	}

	// Cancel the subscription in Stripe
	_, err = sub.Cancel(subscription.StripeSubscriptionID, &stripe.SubscriptionCancelParams{})
	if err != nil {
		log.Printf("Error canceling Stripe subscription: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to cancel subscription in Stripe"})
	}

	// Update the subscription record
	subscription.Status = models.SubscriptionCanceled
	now := time.Now()
	subscription.CancelledAt = &now
	subscription.AutoRenew = false

	// Get cancellation reason from request body
	var req CancelRequest
	if err := c.BodyParser(&req); err == nil && req.Reason != "" {
		subscription.CancellationReason = req.Reason
	}

	if err := h.db.Save(&subscription).Error; err != nil {
		log.Printf("Error updating subscription record: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update subscription record"})
	}

	LogUserAction(h.db, userID, "SUBSCRIPTION_CANCELED_BY_USER", userID, "User", fmt.Sprintf("Subscription canceled by user. Reason: %s", subscription.CancellationReason), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Subscription canceled successfully"})
}
