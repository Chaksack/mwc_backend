package handlers

import (
	"fmt"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/api/middleware"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthHandler handles authentication related requests.
type AuthHandler struct {
	db           *gorm.DB
	cfg          *config.Config // Changed to pass full config
	emailService email.EmailService
	mqService    queue.MessageQueueService
	validate     *validator.Validate
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db *gorm.DB, cfg *config.Config, emailService email.EmailService, mqService queue.MessageQueueService) *AuthHandler {
	return &AuthHandler{
		db:           db,
		cfg:          cfg,
		emailService: emailService,
		mqService:    mqService,
		validate:     validator.New(),
	}
}

// validateRequest validates a struct using the go-playground/validator library and returns user-friendly error messages.
// It extracts validation errors from the validator and formats them into a more readable format.
// The function handles common validation tags like 'required', 'email', 'min', and 'oneof'.
// For example, if a field with the 'required' tag is empty, it will return an error message like "Field is required".
// This function is used by both Register and Login handlers to validate request bodies.
func (h *AuthHandler) validateRequest(c *fiber.Ctx, req interface{}) error {
	if err := h.validate.Struct(req); err != nil {
		validationErrors := err.(validator.ValidationErrors)

		// Create a more user-friendly error response
		errorDetails := make(map[string]string)
		for _, e := range validationErrors {
			field := e.Field()
			switch e.Tag() {
			case "required":
				errorDetails[field] = field + " is required"
			case "email":
				errorDetails[field] = field + " must be a valid email address"
			case "min":
				errorDetails[field] = field + " must be at least " + e.Param() + " characters long"
			case "oneof":
				errorDetails[field] = field + " must be one of: " + e.Param()
			default:
				errorDetails[field] = "Invalid value for " + field + ": " + e.Tag()
			}
		}

		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"details": errorDetails,
		})
	}
	return nil
}

// RegisterRequest is the request body for user registration.
type RegisterRequest struct {
	Email     string          `json:"email" validate:"required,email"`
	Password  string          `json:"password" validate:"required,min=8"`
	FirstName string          `json:"first_name" validate:"required"`
	LastName  string          `json:"last_name" validate:"required"`
	Role      models.UserRole `json:"role" validate:"required,oneof=institution educator parent training_center admin"` // Added admin for potential setup
	// Role-specific fields
	InstitutionName string `json:"institution_name,omitempty"` // For institution/training_center
}

// LoginRequest is the request body for user login.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// Register handles new user registration.
// @Summary Register a new user
// @Description Register a new user with the specified role and return a JWT token
// @Tags auth,public
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "User registration information"
// @Success 201 {object} map[string]interface{} "User registered successfully with token"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 409 {object} map[string]string "Email already exists"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /register [post]
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	req := new(RegisterRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}

	// Validate request using the helper function
	if err := h.validateRequest(c, req); err != nil {
		return err
	}

	// Prevent non-admin from registering as admin
	authClaims, _ := c.Locals("user_claims").(*middleware.Claims)
	if req.Role == models.AdminRole && (authClaims == nil || authClaims.Role != models.AdminRole) {
		// Check if any admin user exists. If not, allow first admin registration.
		var adminCount int64
		h.db.Model(&models.User{}).Where("role = ?", models.AdminRole).Count(&adminCount)
		if adminCount > 0 {
			LogUserAction(h.db, 0, "REGISTER_ATTEMPT_AS_ADMIN_DENIED", 0, "User", fmt.Sprintf("Attempt to register as admin by %s", req.Email), c)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only existing admins can register new admins."})
		}
		log.Println("First admin user registration allowed.")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		LogUserAction(h.db, 0, "REGISTER_FAIL_PW_HASH", 0, "System", "Password hashing failed", c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	user := models.User{
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Role:         req.Role,
		IsActive:     true, // Default to true, admin can deactivate. Or implement email verification.
	}

	tx := h.db.Begin()

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") || strings.Contains(err.Error(), "UNIQUE constraint failed") {
			LogUserAction(h.db, 0, "REGISTER_FAIL_EMAIL_EXISTS", 0, "User", fmt.Sprintf("Email %s already exists", req.Email), c)
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Email already exists"})
		}
		LogUserAction(h.db, 0, "REGISTER_FAIL_DB_USER", 0, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create user: " + err.Error()})
	}

	// Create role-specific profile
	var profileDetails string
	switch user.Role {
	case models.InstitutionRole, models.TrainingCenterRole:
		if req.InstitutionName == "" {
			tx.Rollback()
			LogUserAction(h.db, user.ID, "REGISTER_FAIL_PROFILE_INST_NAME", user.ID, "User", "Institution name missing", c)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Institution name is required for this role"})
		}
		profile := models.InstitutionProfile{UserID: user.ID, InstitutionName: req.InstitutionName}
		if err := tx.Create(&profile).Error; err != nil {
			tx.Rollback()
			LogUserAction(h.db, user.ID, "REGISTER_FAIL_PROFILE_INST_CREATE", user.ID, "InstitutionProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create institution profile: " + err.Error()})
		}
		profileDetails = fmt.Sprintf("Institution Profile created for %s", req.InstitutionName)
	case models.EducatorRole:
		profile := models.EducatorProfile{UserID: user.ID}
		if err := tx.Create(&profile).Error; err != nil {
			tx.Rollback()
			LogUserAction(h.db, user.ID, "REGISTER_FAIL_PROFILE_EDU_CREATE", user.ID, "EducatorProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create educator profile: " + err.Error()})
		}
		profileDetails = "Educator Profile created."
	case models.ParentRole:
		profile := models.ParentProfile{UserID: user.ID}
		if err := tx.Create(&profile).Error; err != nil {
			tx.Rollback()
			LogUserAction(h.db, user.ID, "REGISTER_FAIL_PROFILE_PARENT_CREATE", user.ID, "ParentProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create parent profile: " + err.Error()})
		}
		profileDetails = "Parent Profile created."
	case models.AdminRole:
		// No specific profile for admin beyond the User model itself, or could add one if needed.
		profileDetails = "Admin user registered."
	}

	if err := tx.Commit().Error; err != nil {
		LogUserAction(h.db, user.ID, "REGISTER_FAIL_TX_COMMIT", user.ID, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Transaction failed during registration: " + err.Error()})
	}

	// Send registration email
	emailSubject := "Welcome to Our Platform!"
	emailBody := fmt.Sprintf("<h1>Hello %s,</h1><p>Thank you for registering on our platform as a %s.</p><p>We are excited to have you on board!</p>", user.FirstName, user.Role)
	if err := h.emailService.SendEmail(user.Email, emailSubject, emailBody); err != nil {
		log.Printf("Failed to send registration email to %s: %v. Registration still successful.", user.Email, err)
		// Log this to action log as well for tracking email failures
		LogUserAction(h.db, user.ID, "REGISTER_EMAIL_FAIL", user.ID, "Email", err.Error(), c)
	} else {
		LogUserAction(h.db, user.ID, "REGISTER_EMAIL_SENT", user.ID, "Email", "Registration email sent", c)
	}

	logDetails := fmt.Sprintf("User %s registered as %s. %s", user.Email, user.Role, profileDetails)
	LogUserAction(h.db, user.ID, "USER_REGISTER_SUCCESS", user.ID, "User", logDetails, c)

	// Generate JWT token for automatic login
	expiresIn := time.Hour * time.Duration(h.cfg.JwtExpirationHours)
	token, err := middleware.GenerateJWT(user.ID, user.Email, user.Role, h.cfg.JWTSecret, expiresIn)
	if err != nil {
		LogUserAction(h.db, user.ID, "REGISTER_WARN_JWT_GEN", user.ID, "System", err.Error(), c)
		log.Printf("Failed to generate token for newly registered user %d: %v", user.ID, err)
		// Continue without token, registration is still successful
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"message": "User registered successfully, but automatic login failed",
			"user_id": user.ID,
			"email":   user.Email,
			"role":    user.Role,
		})
	}

	// Update LastLogin
	now := time.Now()
	user.LastLogin = &now
	if err := h.db.Save(&user).Error; err != nil {
		// Log this error but don't fail the registration
		log.Printf("Failed to update last login for newly registered user %d: %v", user.ID, err)
		LogUserAction(h.db, user.ID, "REGISTER_WARN_LASTLOGIN_FAIL", user.ID, "System", err.Error(), c)
	}

	LogUserAction(h.db, user.ID, "USER_AUTO_LOGIN_SUCCESS", user.ID, "User", "User automatically logged in after registration", c)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User registered and logged in successfully",
		"token":   token,
		"user": fiber.Map{
			"id":        user.ID,
			"email":     user.Email,
			"firstName": user.FirstName,
			"lastName":  user.LastName,
			"role":      user.Role,
		},
	})
}

// Login handles user login.
// @Summary User login
// @Description Authenticate a user and return a JWT token
// @Tags auth,public
// @Accept json
// @Produce json
// @Param request body LoginRequest true "User login credentials"
// @Success 200 {object} map[string]interface{} "Login successful with token"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Invalid credentials"
// @Failure 403 {object} map[string]string "User account is inactive"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	req := new(LoginRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON"})
	}
	// Validate request using the helper function
	if err := h.validateRequest(c, req); err != nil {
		return err
	}

	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			LogUserAction(h.db, 0, "LOGIN_FAIL_INVALID_CRED", 0, "User", fmt.Sprintf("Attempt for email: %s", req.Email), c)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
		}
		LogUserAction(h.db, 0, "LOGIN_FAIL_DB_ERROR", 0, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error during login"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		LogUserAction(h.db, user.ID, "LOGIN_FAIL_PW_MISMATCH", user.ID, "User", fmt.Sprintf("Attempt for email: %s", req.Email), c)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	if !user.IsActive {
		LogUserAction(h.db, user.ID, "LOGIN_FAIL_INACTIVE", user.ID, "User", fmt.Sprintf("Attempt for email: %s", req.Email), c)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "User account is inactive. Please contact support."})
	}

	// Generate JWT
	expiresIn := time.Hour * time.Duration(h.cfg.JwtExpirationHours)
	token, err := middleware.GenerateJWT(user.ID, user.Email, user.Role, h.cfg.JWTSecret, expiresIn)
	if err != nil {
		LogUserAction(h.db, user.ID, "LOGIN_FAIL_JWT_GEN", user.ID, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// Update LastLogin
	now := time.Now()
	user.LastLogin = &now
	if err := h.db.Save(&user).Error; err != nil {
		// Log this error but don't fail the login
		log.Printf("Failed to update last login for user %d: %v", user.ID, err)
		LogUserAction(h.db, user.ID, "LOGIN_WARN_LASTLOGIN_FAIL", user.ID, "System", err.Error(), c)
	}

	LogUserAction(h.db, user.ID, "USER_LOGIN_SUCCESS", user.ID, "User", "User logged in successfully", c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Login successful",
		"token":   token,
		"user": fiber.Map{
			"id":        user.ID,
			"email":     user.Email,
			"firstName": user.FirstName,
			"lastName":  user.LastName,
			"role":      user.Role,
		},
	})
}

// GetCurrentUser retrieves the currently logged-in user's information.
// @Summary Get current user
// @Description Retrieve the currently logged-in user's information with full profile
// @Tags auth,authenticated
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "User information with full profile"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /me [get]
func (h *AuthHandler) GetCurrentUser(c *fiber.Ctx) error {
	// Get user ID from context (set by auth middleware)
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User ID not found in context"})
	}

	// Retrieve user from database with appropriate profile preloaded based on role
	var user models.User
	query := h.db.Model(&models.User{})

	// First get the user to determine the role
	if err := query.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}

	// Check if user is active
	if !user.IsActive {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "User account is inactive"})
	}

	// Now preload the appropriate profile based on user role
	switch user.Role {
	case models.InstitutionRole, models.TrainingCenterRole:
		if err := h.db.Preload("InstitutionProfile").Preload("InstitutionProfile.School").First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load institution profile: " + err.Error()})
		}
	case models.EducatorRole:
		if err := h.db.Preload("EducatorProfile").First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load educator profile: " + err.Error()})
		}
	case models.ParentRole:
		if err := h.db.Preload("ParentProfile").First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load parent profile: " + err.Error()})
		}
	}

	// Log the action
	LogUserAction(h.db, user.ID, "USER_GET_CURRENT", user.ID, "User", "User retrieved their full profile", c)

	// Prepare response based on user role
	userMap := fiber.Map{
		"id":        user.ID,
		"email":     user.Email,
		"firstName": user.FirstName,
		"lastName":  user.LastName,
		"role":      user.Role,
		"isActive":  user.IsActive,
		"createdAt": user.CreatedAt,
		"lastLogin": user.LastLogin,
	}

	// Add profile information based on role
	switch user.Role {
	case models.InstitutionRole, models.TrainingCenterRole:
		if user.InstitutionProfile != nil {
			userMap["profile"] = fiber.Map{
				"id":              user.InstitutionProfile.ID,
				"institutionName": user.InstitutionProfile.InstitutionName,
				"isVerified":      user.InstitutionProfile.IsVerified,
				"schoolId":        user.InstitutionProfile.SchoolID,
				"school":          user.InstitutionProfile.School,
			}
		}
	case models.EducatorRole:
		if user.EducatorProfile != nil {
			userMap["profile"] = fiber.Map{
				"id":            user.EducatorProfile.ID,
				"bio":           user.EducatorProfile.Bio,
				"qualifications": user.EducatorProfile.Qualifications,
				"experience":    user.EducatorProfile.Experience,
			}
		}
	case models.ParentRole:
		if user.ParentProfile != nil {
			userMap["profile"] = fiber.Map{
				"id": user.ParentProfile.ID,
				// Add any other parent profile fields here
			}
		}
	}

	// Return user information with profile
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"user": userMap,
	})
}
