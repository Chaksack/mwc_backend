package handlers

import (
	"net/http"
	"time"

	"mwc_backend/config"
	"mwc_backend/internal/queue"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// HealthHandler serves health and readiness information.
type HealthHandler struct {
	db        *gorm.DB
	cfg       *config.Config
	mqService queue.MessageQueueService
}

// NewHealthHandler constructs a HealthHandler with dependencies.
func NewHealthHandler(db *gorm.DB, cfg *config.Config, mqService queue.MessageQueueService) *HealthHandler {
	return &HealthHandler{db: db, cfg: cfg, mqService: mqService}
}

// healthStatus represents the health response payload.
type healthStatus struct {
	Status    string                 `json:"status"`
	Version   string                 `json:"version"`
	Timestamp time.Time              `json:"timestamp"`
	Checks    map[string]interface{} `json:"checks"`
}

// GetHealth returns overall service health including basic dependency checks.
// @Summary Health status
// @Description Returns service health information and basic dependency checks
// @Tags public
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
func (h *HealthHandler) GetHealth(c *fiber.Ctx) error {
	checks := make(map[string]interface{})

	// App version: keep in sync with internal/api/routes.go welcome endpoint
 version := "2.1.7"

	// Database check
	dbOK := true
	if h.db != nil {
		if err := h.db.Exec("SELECT 1").Error; err != nil {
			dbOK = false
			checks["database_error"] = err.Error()
		}
	} else {
		dbOK = false
		checks["database_error"] = "db not initialized"
	}
	checks["database"] = map[string]interface{}{"ok": dbOK}

	// RabbitMQ check (initialized state)
	mqOK := true
	if h.mqService == nil || !h.mqService.IsInitialized() {
		mqOK = false
	}
	checks["rabbitmq"] = map[string]interface{}{"ok": mqOK}

	// Stripe configuration check (key presence only; no external call)
	stripeOK := h.cfg != nil && h.cfg.StripeSecretKey != ""
	checks["stripe_config"] = map[string]interface{}{"ok": stripeOK}

	// Webhook secrets presence (informational)
	checks["stripe_webhooks"] = map[string]interface{}{
		"snapshot_secret_configured":    h.cfg != nil && h.cfg.StripeSnapshotWebhookSecret != "",
		"thinpayload_secret_configured": h.cfg != nil && h.cfg.StripeThinPayloadWebhookSecret != "",
	}

	// Email configuration (only if enabled)
	emailOK := true
	if h.cfg != nil && h.cfg.EmailEnabled {
		if h.cfg.SMTPHost == "" || h.cfg.SMTPPort == 0 || h.cfg.SMTPUser == "" || h.cfg.SMTPPassword == "" || h.cfg.EmailFrom == "" {
			emailOK = false
		}
		checks["email"] = map[string]interface{}{"enabled": true, "ok": emailOK}
	} else {
		checks["email"] = map[string]interface{}{"enabled": false}
	}

	// Billing portal fallback URL presence (informational)
	checks["billing_portal_fallback"] = map[string]interface{}{
		"configured": h.cfg != nil && h.cfg.StripeBillingPortalLoginURL != "",
	}

	// Determine overall status
	overall := "ok"
	if !dbOK || !mqOK {
		overall = "degraded"
	}

	payload := healthStatus{
		Status:    overall,
		Version:   version,
		Timestamp: time.Now(),
		Checks:    checks,
	}

	code := http.StatusOK
	if overall != "ok" {
		code = http.StatusServiceUnavailable
	}
	return c.Status(code).JSON(payload)
}
