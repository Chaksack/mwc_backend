package handlers

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "strings"
    "time"

    "mwc_backend/internal/email"
    "mwc_backend/internal/models"

    "github.com/gofiber/fiber/v2"
    "gorm.io/gorm"
)

// DiscountHandler manages discount codes (admin)
type DiscountHandler struct {
    db       *gorm.DB
    emailSvc email.EmailService
}

func NewDiscountHandler(db *gorm.DB, emailSvc email.EmailService) *DiscountHandler {
    return &DiscountHandler{db: db, emailSvc: emailSvc}
}

// generateCode creates a short uppercase code from random bytes
func generateCode(n int) (string, error) {
    if n <= 0 { n = 8 }
    b := make([]byte, (n+1)/2)
    if _, err := rand.Read(b); err != nil { return "", err }
    code := strings.ToUpper(hex.EncodeToString(b))
    if len(code) > n { code = code[:n] }
    return code, nil
}

// CreateDiscountCode creates a new discount code
// @Summary Create a discount code (admin)
// @Tags authenticated, admin, discount-codes
// @Accept json
// @Produce json
// @Param data body struct{Code string `json:"code"`; Description string `json:"description"`; PercentOff *int `json:"percent_off"`; AmountOff *float64 `json:"amount_off"`; StartsAt *string `json:"starts_at"`; EndsAt *string `json:"ends_at"`; MaxRedemptions *int `json:"max_redemptions"`; IsActive *bool `json:"is_active"`} true "Discount code payload (RFC3339 dates)"
// @Success 201 {object} models.DiscountCode
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /admin/discount-codes [post]
func (h *DiscountHandler) CreateDiscountCode(c *fiber.Ctx) error {
    // Defense in depth: ensure only Admin or SuperAdmin can create codes, even if route middleware is misconfigured
    if roleVal := c.Locals("user_role"); roleVal != nil {
        if roleStr, ok := roleVal.(models.UserRole); ok {
            if roleStr != models.AdminRole && roleStr != models.SuperAdminRole {
                return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only admins can create discount codes"})
            }
        }
    }
    type reqT struct {
        Code           string   `json:"code"`
        Description    string   `json:"description"`
        PercentOff     *int     `json:"percent_off"`
        AmountOff      *float64 `json:"amount_off"`
        StartsAt       *string  `json:"starts_at"`
        EndsAt         *string  `json:"ends_at"`
        MaxRedemptions *int     `json:"max_redemptions"`
        IsActive       *bool    `json:"is_active"`
    }
    var req reqT
    if err := c.BodyParser(&req); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json: "+err.Error()})
    }
    // Validate discounts
    if (req.PercentOff == nil && req.AmountOff == nil) || (req.PercentOff != nil && req.AmountOff != nil) {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provide either percent_off or amount_off"})
    }
    if req.PercentOff != nil {
        if *req.PercentOff < 1 || *req.PercentOff > 100 {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "percent_off must be between 1 and 100"})
        }
    }
    if req.AmountOff != nil {
        if *req.AmountOff < 0 {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "amount_off must be >= 0"})
        }
    }
    var startsAtPtr *time.Time
    var endsAtPtr *time.Time
    if req.StartsAt != nil && strings.TrimSpace(*req.StartsAt) != "" {
        if t, err := time.Parse(time.RFC3339, *req.StartsAt); err == nil { startsAtPtr = &t } else { return c.Status(400).JSON(fiber.Map{"error": "invalid starts_at (RFC3339)"}) }
    }
    if req.EndsAt != nil && strings.TrimSpace(*req.EndsAt) != "" {
        if t, err := time.Parse(time.RFC3339, *req.EndsAt); err == nil { endsAtPtr = &t } else { return c.Status(400).JSON(fiber.Map{"error": "invalid ends_at (RFC3339)"}) }
    }
    code := strings.ToUpper(strings.TrimSpace(req.Code))
    if code == "" {
        var err error
        code, err = generateCode(10)
        if err != nil { return c.Status(500).JSON(fiber.Map{"error": "failed to generate code: "+err.Error()}) }
    }
    dc := models.DiscountCode{
        Code:           code,
        Description:    strings.TrimSpace(req.Description),
        PercentOff:     req.PercentOff,
        AmountOff:      req.AmountOff,
        StartsAt:       startsAtPtr,
        EndsAt:         endsAtPtr,
        MaxRedemptions: req.MaxRedemptions,
        IsActive:       true,
    }
    // Record who created the discount code if available
    if uidVal := c.Locals("user_id"); uidVal != nil {
        if uid, ok := uidVal.(uint); ok {
            dc.CreatedByUserID = &uid
        }
    }
    if req.IsActive != nil { dc.IsActive = *req.IsActive }
    if err := h.db.Create(&dc).Error; err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create discount code: "+err.Error()})
    }
    return c.Status(fiber.StatusCreated).JSON(dc)
}

// ListDiscountCodes returns paginated discount codes with basic filters
// @Summary List discount codes (admin)
// @Tags authenticated, admin, discount-codes
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param active query bool false "Filter by active"
// @Param state query string false "Filter by state: valid|upcoming|expired"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /admin/discount-codes [get]
func (h *DiscountHandler) ListDiscountCodes(c *fiber.Ctx) error {
    page := c.QueryInt("page", 1)
    if page < 1 { page = 1 }
    size := c.QueryInt("page_size", 20)
    if size < 1 { size = 20 }
    if size > 100 { size = 100 }
    q := h.db.Model(&models.DiscountCode{})
    if v := c.Query("active"); v != "" {
        if v == "true" { q = q.Where("is_active = ?", true) }
        if v == "false" { q = q.Where("is_active = ?", false) }
    }
    now := time.Now()
    switch strings.ToLower(strings.TrimSpace(c.Query("state"))) {
    case "valid":
        q = q.Where("(starts_at IS NULL OR starts_at <= ?) AND (ends_at IS NULL OR ends_at >= ?)", now, now)
    case "upcoming":
        q = q.Where("starts_at IS NOT NULL AND starts_at > ?", now)
    case "expired":
        q = q.Where("ends_at IS NOT NULL AND ends_at < ?", now)
    }
    var total int64
    if err := q.Count(&total).Error; err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    var items []models.DiscountCode
    if err := q.Order("created_at DESC").Offset((page-1)*size).Limit(size).Find(&items).Error; err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    return c.JSON(fiber.Map{
        "data": items,
        "pagination": fiber.Map{
            "page": page, "page_size": size, "total": total,
            "total_pages": (total + int64(size) - 1) / int64(size),
        },
    })
}

// SendDiscountCodeEmail broadcasts a discount code to all users via email in batches
// @Summary Send discount code to all users by email (admin)
// @Tags authenticated, admin, discount-codes
// @Produce json
// @Param id path int true "Discount code ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /admin/discount-codes/{id}/send-email [post]
func (h *DiscountHandler) SendDiscountCodeEmail(c *fiber.Ctx) error {
    id := c.Params("id")
    var dc models.DiscountCode
    if err := h.db.First(&dc, id).Error; err != nil {
        if err == gorm.ErrRecordNotFound {
            return c.Status(404).JSON(fiber.Map{"error": "discount code not found"})
        }
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    // Email all users in batches
    const batch = 500
    sent := 0
    offset := 0
    for {
        var users []models.User
        if err := h.db.Model(&models.User{}).Order("id ASC").Limit(batch).Offset(offset).Find(&users).Error; err != nil {
            return c.Status(500).JSON(fiber.Map{"error": "failed to query users: "+err.Error()})
        }
        if len(users) == 0 { break }
        for _, u := range users {
            if strings.TrimSpace(u.Email) == "" { continue }
            subject := fmt.Sprintf("Your %s discount code", dc.Code)
            body := "<p>We are excited to offer you a special discount.</p>" +
                "<p><strong>Code: " + dc.Code + "</strong></p>"
            if dc.Description != "" { body += "<p>" + dc.Description + "</p>" }
            // validity window
            if dc.StartsAt != nil || dc.EndsAt != nil {
                body += "<p>Validity: "
                if dc.StartsAt != nil { body += dc.StartsAt.UTC().Format(time.RFC1123) } else { body += "now" }
                body += " to "
                if dc.EndsAt != nil { body += dc.EndsAt.UTC().Format(time.RFC1123) } else { body += "no end date" }
                body += "</p>"
            }
            _ = h.emailSvc.SendEmail(u.Email, subject, body)
            sent++
        }
        offset += len(users)
        if len(users) < batch { break }
    }
    return c.JSON(fiber.Map{"sent": sent})
}
