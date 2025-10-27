package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"mwc_backend/internal/services"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stripe/stripe-go/v72"
	bpsession "github.com/stripe/stripe-go/v72/billingportal/session"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/sub"
	"github.com/stripe/stripe-go/v72/webhook"
	"gorm.io/gorm"
)

// SubscriptionHandler handles subscription-related requests
type SubscriptionHandler struct {
	db                  *gorm.DB
	cfg                 *config.Config
	mqService           queue.MessageQueueService
	notificationService *services.NotificationService
}

// verifyStripeAndParseEvent verifies the Stripe signature with the provided secret and returns the Event.
func (h *SubscriptionHandler) verifyStripeAndParseEvent(c *fiber.Ctx, signingSecret string) (stripe.Event, error) {
	var empty stripe.Event
	if signingSecret == "" {
		return empty, fmt.Errorf("missing Stripe webhook signing secret")
	}
	signature := c.Get("Stripe-Signature")
	if signature == "" {
		return empty, fmt.Errorf("missing Stripe-Signature header")
	}
	body := c.Body()
	event, err := webhook.ConstructEvent(body, signature, signingSecret)
	if err != nil {
		return empty, fmt.Errorf("invalid Stripe signature: %w", err)
	}
	return event, nil
}

// HandleStripeSnapshotWebhook receives Stripe snapshot events (separate endpoint with its own signing secret)
func (h *SubscriptionHandler) HandleStripeSnapshotWebhook(c *fiber.Ctx) error {
	event, err := h.verifyStripeAndParseEvent(c, h.cfg.StripeSnapshotWebhookSecret)
	if err != nil {
		log.Printf("Stripe snapshot webhook verification failed: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// For now, acknowledge receipt. Optionally log minimal info.
	log.Printf("[Stripe Snapshot] received: id=%s type=%s", event.ID, event.Type)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"received": true})
}

// HandleStripeThinPayloadWebhook receives Stripe thin payload events (separate endpoint with its own signing secret)
func (h *SubscriptionHandler) HandleStripeThinPayloadWebhook(c *fiber.Ctx) error {
	event, err := h.verifyStripeAndParseEvent(c, h.cfg.StripeThinPayloadWebhookSecret)
	if err != nil {
		log.Printf("Stripe thin payload webhook verification failed: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[Stripe ThinPayload] received: id=%s type=%s", event.ID, event.Type)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"received": true})
}

// NewSubscriptionHandler creates a new SubscriptionHandler
func NewSubscriptionHandler(db *gorm.DB, cfg *config.Config, mqService queue.MessageQueueService, emailService email.EmailService) *SubscriptionHandler {
	// Initialize Stripe with the API key
	stripe.Key = cfg.StripeSecretKey
	notificationService := services.NewNotificationService(db, emailService)
	return &SubscriptionHandler{
		db:                  db,
		cfg:                 cfg,
		mqService:           mqService,
		notificationService: notificationService,
	}
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
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &cs); err != nil {
			log.Printf("Error parsing checkout session: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid checkout session"})
		}

		// Fetch full Checkout Session from Stripe with expanded subscription and customer to reliably access metadata and IDs
		getParams := &stripe.CheckoutSessionParams{}
		getParams.AddExpand("subscription")
		getParams.AddExpand("customer")
		fullSession, err := session.Get(cs.ID, getParams)
		if err != nil {
			log.Printf("Error retrieving full checkout session %s: %v", cs.ID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve checkout session"})
		}

		// Safety: ensure subscription object exists
		if fullSession.Subscription == nil {
			log.Printf("No subscription found on checkout session %s", cs.ID)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Subscription not found on checkout session"})
		}

		// Read metadata set during Checkout creation
		var userID uint64
		var plan string
		if fullSession.Subscription.Metadata != nil {
			if v, ok := fullSession.Subscription.Metadata["user_id"]; ok {
				if parsed, perr := strconv.ParseUint(v, 10, 64); perr == nil {
					userID = parsed
				}
			}
			if v, ok := fullSession.Subscription.Metadata["plan"]; ok {
				plan = v
			}
		}
		if userID == 0 || plan == "" {
			// As a fallback, try to retrieve subscription directly and read metadata again
			if fullSession.Subscription.ID != "" {
				if s, gerr := sub.Get(fullSession.Subscription.ID, nil); gerr == nil {
					if v, ok := s.Metadata["user_id"]; ok {
						if parsed, perr := strconv.ParseUint(v, 10, 64); perr == nil {
							userID = parsed
						}
					}
					if v, ok := s.Metadata["plan"]; ok {
						plan = v
					}
				}
			}
		}
		if userID == 0 {
			log.Printf("User ID not found in subscription metadata for checkout session %s", cs.ID)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "User ID not found in subscription metadata"})
		}
		if plan != string(models.MonthlyPlan) && plan != string(models.AnnualPlan) {
			log.Printf("Plan not found or invalid in subscription metadata for checkout session %s", cs.ID)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Plan not found in subscription metadata"})
		}

		// If we've already created a local record for this Stripe subscription, skip creating a duplicate (idempotent)
		var existingByStripe models.Subscription
		if err := h.db.Where("stripe_subscription_id = ?", fullSession.Subscription.ID).First(&existingByStripe).Error; err == nil {
			log.Printf("Subscription record already exists for Stripe subscription %s, skipping create", fullSession.Subscription.ID)
			break
		}

		// Create a new subscription record
		var endDate time.Time
		if plan == string(models.MonthlyPlan) {
			endDate = time.Now().AddDate(0, 1, 0) // Add 1 month
		} else {
			endDate = time.Now().AddDate(1, 0, 0) // Add 1 year
		}

		newSub := models.Subscription{
			UserID:               uint(userID),
			Plan:                 models.SubscriptionPlan(plan),
			Status:               models.SubscriptionActive,
			StartDate:            time.Now(),
			EndDate:              endDate,
			AutoRenew:            true,
			StripeCustomerID:     fullSession.Customer.ID,
			StripeSubscriptionID: fullSession.Subscription.ID,
		}

		if err := h.db.Create(&newSub).Error; err != nil {
			log.Printf("Error creating subscription record: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create subscription record"})
		}

		// Send subscription completion notification
		if err := h.db.Preload("User").First(&newSub, newSub.ID).Error; err == nil {
			if err := h.notificationService.SendSubscriptionCompletedEmail(newSub); err != nil {
				log.Printf("Failed to send subscription completion notification to user %d: %v", newSub.UserID, err)
			} else {
				log.Printf("Sent subscription completion notification to %s", newSub.User.Email)
			}
		}

		LogUserAction(h.db, uint(userID), "SUBSCRIPTION_CREATED", uint(userID), "User", fmt.Sprintf("Subscription created for %s plan", plan), c)

	case "customer.subscription.updated":
		var s stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
			log.Printf("Error parsing subscription: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid subscription"})
		}

		// Identify user from metadata if available (older records rely on local DB relation)
		var userID uint64
		if v, ok := s.Metadata["user_id"]; ok {
			if parsed, perr := strconv.ParseUint(v, 10, 64); perr == nil {
				userID = parsed
			}
		}

		// Load local subscription by Stripe subscription ID
		var local models.Subscription
		if err := h.db.Where("stripe_subscription_id = ?", s.ID).First(&local).Error; err != nil {
			log.Printf("Error finding subscription record for %s: %v", s.ID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find subscription record"})
		}

		// Map Stripe status to local status
		var status models.SubscriptionStatus
		switch s.Status {
		case "active":
			status = models.SubscriptionActive
		case "trialing":
			status = models.SubscriptionActive
		case "canceled", "unpaid", "past_due", "incomplete", "incomplete_expired":
			status = models.SubscriptionInactive
		default:
			status = models.SubscriptionInactive
		}

		changed := false
		if local.Status != status {
			local.Status = status
			changed = true
		}

		// Auto-renew and cancellation timing
		local.AutoRenew = !s.CancelAtPeriodEnd
		if s.CancelAt > 0 {
			cancelAt := time.Unix(s.CancelAt, 0)
			local.CancelledAt = &cancelAt
			changed = true
		} else if s.CanceledAt > 0 {
			canceledAt := time.Unix(s.CanceledAt, 0)
			local.CancelledAt = &canceledAt
			changed = true
		} else if local.CancelledAt != nil && status == models.SubscriptionActive {
			// Clear cancellation when reactivated
			local.CancelledAt = nil
			changed = true
		}

		// Sync EndDate to Stripe current_period_end if provided
		if s.CurrentPeriodEnd > 0 {
			periodEnd := time.Unix(s.CurrentPeriodEnd, 0)
			if !local.EndDate.Equal(periodEnd) {
				local.EndDate = periodEnd
				changed = true
			}
		}

		// Derive plan from Stripe price ID (first item)
		if len(s.Items.Data) > 0 && s.Items.Data[0].Price != nil {
			priceID := s.Items.Data[0].Price.ID
			if priceID == h.cfg.StripeMonthlyPriceID && local.Plan != models.MonthlyPlan {
				local.Plan = models.MonthlyPlan
				changed = true
			}
			if priceID == h.cfg.StripeAnnualPriceID && local.Plan != models.AnnualPlan {
				local.Plan = models.AnnualPlan
				changed = true
			}
		}

		if changed {
			if err := h.db.Save(&local).Error; err != nil {
				log.Printf("Error updating subscription record: %v", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update subscription record"})
			}
		}

		if userID != 0 {
			LogUserAction(h.db, uint(userID), "SUBSCRIPTION_UPDATED", uint(userID), "User", fmt.Sprintf("Subscription updated to status: %s", status), c)
		}

	case "invoice.payment_succeeded":
		// On successful invoice payment (including renewals), sync local subscription
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			log.Printf("Error parsing invoice: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid invoice"})
		}
		if inv.Subscription == nil || inv.Subscription.ID == "" {
			break
		}
		// Fetch latest subscription from Stripe for accurate period end and status
		stripeSub, getErr := sub.Get(inv.Subscription.ID, nil)
		if getErr != nil {
			log.Printf("Failed to fetch Stripe subscription %s: %v", inv.Subscription.ID, getErr)
			break
		}
		var local models.Subscription
		if err := h.db.Where("stripe_subscription_id = ?", stripeSub.ID).First(&local).Error; err != nil {
			log.Printf("Local subscription not found for Stripe %s: %v", stripeSub.ID, err)
			break
		}
		// Update fields
		periodEnd := local.EndDate
		if stripeSub.CurrentPeriodEnd > 0 {
			periodEnd = time.Unix(stripeSub.CurrentPeriodEnd, 0)
		}
		status := models.SubscriptionActive
		autoRenew := !stripeSub.CancelAtPeriodEnd
		// Plan mapping from price
		if len(stripeSub.Items.Data) > 0 && stripeSub.Items.Data[0].Price != nil {
			priceID := stripeSub.Items.Data[0].Price.ID
			if priceID == h.cfg.StripeMonthlyPriceID {
				local.Plan = models.MonthlyPlan
			} else if priceID == h.cfg.StripeAnnualPriceID {
				local.Plan = models.AnnualPlan
			}
		}
		changed := false
		if local.Status != status {
			local.Status = status
			changed = true
		}
		if !local.EndDate.Equal(periodEnd) {
			local.EndDate = periodEnd
			changed = true
		}
		if local.AutoRenew != autoRenew {
			local.AutoRenew = autoRenew
			changed = true
		}
		if changed {
			if err := h.db.Save(&local).Error; err != nil {
				log.Printf("Error updating subscription after invoice payment: %v", err)
			}
		}

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

// CreateBillingPortalSession creates a Stripe Billing Portal session for the current user
// @Summary Create Stripe Billing Portal session
// @Description Creates a Stripe Billing Portal session so the user can manage subscription, payment method, invoices
// @Tags subscription
// @Produce json
// @Success 200 {object} map[string]string "Billing portal session created successfully"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 404 {object} map[string]string "No Stripe customer found and cannot create"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /subscription/portal [post]
func (h *SubscriptionHandler) CreateBillingPortalSession(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Try to find existing Stripe customer from any subscription record
	var subRec models.Subscription
	err := h.db.Where("user_id = ? AND stripe_customer_id <> ''", userID).Order("created_at DESC").First(&subRec).Error
	var stripeCustomerID string
	if err == nil && subRec.StripeCustomerID != "" {
		stripeCustomerID = subRec.StripeCustomerID
	} else {
		// Fallback: ensure user exists to (re)create a customer if needed
		var user models.User
		if uerr := h.db.First(&user, userID).Error; uerr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
		}
		custParams := &stripe.CustomerParams{
			Email: stripe.String(user.Email),
			Name:  stripe.String(fmt.Sprintf("%s %s", user.FirstName, user.LastName)),
		}
		custParams.AddMetadata("user_id", strconv.FormatUint(uint64(userID), 10))
		cust, cerr := customer.New(custParams)
		if cerr != nil {
			log.Printf("Error creating Stripe customer for billing portal: %v", cerr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create Stripe customer"})
		}
		stripeCustomerID = cust.ID
	}

	returnURL := h.cfg.BaseURL
	if returnURL == "" {
		returnURL = c.BaseURL()
	}
	// Append a sensible return path
	returnURL = fmt.Sprintf("%s/account/billing", strings.TrimRight(returnURL, "/"))

	bpParams := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(stripeCustomerID),
		ReturnURL: stripe.String(returnURL),
	}
	sess, perr := bpsession.New(bpParams)
	if perr != nil {
		log.Printf("Error creating Billing Portal session: %v", perr)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create billing portal session"})
	}

	LogUserAction(h.db, userID, "SUBSCRIPTION_PORTAL_CREATED", userID, "User", "Billing Portal session created", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"url": sess.URL})
}
