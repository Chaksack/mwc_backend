package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/api/middleware"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"os"
	"strconv"
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

// generateVerificationToken generates a random verification token
func generateVerificationToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
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
	Role      models.UserRole `json:"role" validate:"required,oneof=institution montessori_professional parent training_center"` // Admin role removed - only superadmin can create admins
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

	// Admin role registration is completely disabled - only superadmin can create admin users

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		LogUserAction(h.db, 0, "REGISTER_FAIL_PW_HASH", 0, "System", "Password hashing failed", c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	// Generate verification token
	verificationToken, err := generateVerificationToken()
	if err != nil {
		LogUserAction(h.db, 0, "REGISTER_FAIL_TOKEN_GEN", 0, "System", "Verification token generation failed", c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate verification token"})
	}

	// Set token expiry to 24 hours from now
	tokenExpiry := time.Now().Add(24 * time.Hour)

	user := models.User{
		Email:                   req.Email,
		PasswordHash:            string(hashedPassword),
		FirstName:               req.FirstName,
		LastName:                req.LastName,
		Role:                    req.Role,
		IsActive:                true,
		EmailVerified:           false, // User must verify email first
		VerificationToken:       &verificationToken,
		VerificationTokenExpiry: &tokenExpiry,
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
	case models.MontessoriProfessionalRole:
		profile := models.MontessoriProfessionalProfile{UserID: user.ID}
		if err := tx.Create(&profile).Error; err != nil {
			tx.Rollback()
			LogUserAction(h.db, user.ID, "REGISTER_FAIL_PROFILE_MONT_PROF_CREATE", user.ID, "MontessoriProfessionalProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create montessori professional profile: " + err.Error()})
		}
		profileDetails = "Montessori Professional Profile created."
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

	// Create free trial subscription for non-admin users
	if user.Role != models.AdminRole {
		// Create 60-day free trial subscription
		startDate := time.Now()
		endDate := startDate.AddDate(0, 0, 60) // 60 days from now

		subscription := models.Subscription{
			UserID:               user.ID,
			Plan:                 models.FreePlan,
			Status:               models.SubscriptionActive,
			StartDate:            startDate,
			EndDate:              endDate,
			AutoRenew:            false, // Free trial doesn't auto-renew
			StripeCustomerID:     "",    // No Stripe customer for free trial
			StripeSubscriptionID: "",    // No Stripe subscription for free trial
		}

		if err := tx.Create(&subscription).Error; err != nil {
			tx.Rollback()
			LogUserAction(h.db, user.ID, "REGISTER_FAIL_FREE_TRIAL", user.ID, "Subscription", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create free trial subscription: " + err.Error()})
		}

		profileDetails += " Free trial subscription (60 days) created."
		LogUserAction(h.db, user.ID, "FREE_TRIAL_CREATED", user.ID, "Subscription", "60-day free trial subscription created", c)
	}

	if err := tx.Commit().Error; err != nil {
		LogUserAction(h.db, user.ID, "REGISTER_FAIL_TX_COMMIT", user.ID, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Transaction failed during registration: " + err.Error()})
	}

	// Send email verification email
	verificationURL := fmt.Sprintf("https://montessoriworldconnect.com/verify-email?token=%s", verificationToken)
	emailSubject := "Please verify your email address"

	// Build free trial information for non-admin users
	freeTrialInfo := ""
	if user.Role != models.AdminRole {
		freeTrialEndDate := time.Now().AddDate(0, 0, 60).Format("January 2, 2006")
		freeTrialInfo = fmt.Sprintf(`
		<h2>🎉 Your Free Trial is Active!</h2>
		<p><strong>Congratulations!</strong> You've been granted a <strong>60-day free trial</strong> with full access to all premium features:</p>
		<ul>
			<li>✓ Advanced school search and filtering</li>
			<li>✓ Direct messaging with institutions</li>
			<li>✓ Priority job listings and applications</li>
			<li>✓ Exclusive educational resources</li>
			<li>✓ Community forums and networking opportunities</li>
		</ul>
		<p><strong>Free Trial Details:</strong></p>
		<ul>
			<li>Plan: Free Trial</li>
			<li>Status: Active</li>
			<li>Expires on: %s</li>
			<li>Auto-renewal: Disabled (you can upgrade anytime)</li>
		</ul>
		<p>Start exploring all the premium features right after verifying your email!</p>
		`, freeTrialEndDate)
	}

	emailBody := fmt.Sprintf(`
		<h1>Hello %s,</h1>
		<p>Thank you for registering on our platform as a %s.</p>%s
		<p>Please click the link below to verify your email address:</p>
		<p><a href="%s">Verify Email Address</a></p>
		<p>If the link doesn't work, you can copy and paste this URL into your browser:</p>
		<p>%s</p>
		<p>This link will expire in 24 hours.</p>
		<p>If you didn't create an account, you can safely ignore this email.</p>
	`, user.FirstName, user.Role, freeTrialInfo, verificationURL, verificationURL)

	if err := h.emailService.SendEmail(user.Email, emailSubject, emailBody); err != nil {
		log.Printf("Failed to send verification email to %s: %v. Registration still successful.", user.Email, err)
		// Log this to action log as well for tracking email failures
		LogUserAction(h.db, user.ID, "REGISTER_VERIFICATION_EMAIL_FAIL", user.ID, "Email", err.Error(), c)
	} else {
		LogUserAction(h.db, user.ID, "REGISTER_VERIFICATION_EMAIL_SENT", user.ID, "Email", "Verification email sent", c)
	}

	logDetails := fmt.Sprintf("User %s registered as %s. %s", user.Email, user.Role, profileDetails)
	LogUserAction(h.db, user.ID, "USER_REGISTER_SUCCESS", user.ID, "User", logDetails, c)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User registered successfully. Please check your email to verify your account.",
		"user": fiber.Map{
			"id":             user.ID,
			"email":          user.Email,
			"firstName":      user.FirstName,
			"lastName":       user.LastName,
			"role":           user.Role,
			"email_verified": user.EmailVerified,
		},
	})
}

// VerifyEmail handles email verification
// @Summary Verify user email address
// @Description Verifies a user's email address using the verification token sent to their email
// @Tags auth,public
// @Accept json
// @Produce json
// @Param token query string true "Verification token"
// @Success 200 {object} map[string]interface{} "Email verified successfully"
// @Failure 400 {object} map[string]string "Bad request - missing or invalid token"
// @Failure 404 {object} map[string]string "Invalid or expired token"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /verify-email [get]
func (h *AuthHandler) VerifyEmail(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Verification token is required"})
	}

	// Find user with this verification token
	var user models.User
	err := h.db.Where("verification_token = ? AND verification_token_expiry > ?", token, time.Now()).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			LogUserAction(h.db, 0, "EMAIL_VERIFICATION_INVALID_TOKEN", 0, "User", fmt.Sprintf("Invalid or expired verification token: %s", token), c)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Invalid or expired verification token"})
		}
		LogUserAction(h.db, 0, "EMAIL_VERIFICATION_DB_ERROR", 0, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to verify email"})
	}

	// Check if email is already verified
	if user.EmailVerified {
		LogUserAction(h.db, user.ID, "EMAIL_VERIFICATION_ALREADY_VERIFIED", user.ID, "User", "Email already verified", c)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Email is already verified",
			"user": fiber.Map{
				"id":             user.ID,
				"email":          user.Email,
				"email_verified": user.EmailVerified,
			},
		})
	}

	// Update user's email verification status
	user.EmailVerified = true
	user.VerificationToken = nil       // Clear the token
	user.VerificationTokenExpiry = nil // Clear the expiry

	if err := h.db.Save(&user).Error; err != nil {
		LogUserAction(h.db, user.ID, "EMAIL_VERIFICATION_UPDATE_FAIL", user.ID, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update email verification status"})
	}

	LogUserAction(h.db, user.ID, "EMAIL_VERIFICATION_SUCCESS", user.ID, "User", fmt.Sprintf("Email verified for user: %s", user.Email), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Email verified successfully! You can now log in to your account.",
		"user": fiber.Map{
			"id":             user.ID,
			"email":          user.Email,
			"firstName":      user.FirstName,
			"lastName":       user.LastName,
			"email_verified": user.EmailVerified,
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

	// Check if email is verified (skip verification for admin and superadmin users)
	if !user.EmailVerified && user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		LogUserAction(h.db, user.ID, "LOGIN_FAIL_EMAIL_NOT_VERIFIED", user.ID, "User", fmt.Sprintf("Login attempt with unverified email: %s", req.Email), c)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":          "Please verify your email address before logging in. Check your inbox for a verification link.",
			"email_verified": false,
		})
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

	// Now preload the appropriate profile based on user role with all related data
	switch user.Role {
	case models.InstitutionRole, models.TrainingCenterRole:
		if err := h.db.Preload("InstitutionProfile").Preload("InstitutionProfile.School").First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load institution profile: " + err.Error()})
		}
	case models.MontessoriProfessionalRole:
		if err := h.db.Preload("MontessoriProfessionalProfile").Preload("MontessoriProfessionalProfile.SavedSchools").First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load montessori professional profile: " + err.Error()})
		}
	case models.ParentRole:
		if err := h.db.Preload("ParentProfile").Preload("ParentProfile.SavedSchools").Preload("ParentProfile.Schools").First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load parent profile: " + err.Error()})
		}
	}

	// Log the action
	LogUserAction(h.db, user.ID, "USER_GET_CURRENT", user.ID, "User", "User retrieved their full profile", c)

	// Prepare comprehensive response with all user details
	userMap := fiber.Map{
		"id":            user.ID,
		"email":         user.Email,
		"firstName":     user.FirstName,
		"lastName":      user.LastName,
		"role":          user.Role,
		"isActive":      user.IsActive,
		"emailVerified": user.EmailVerified,
		"createdAt":     user.CreatedAt,
		"updatedAt":     user.UpdatedAt,
		"lastLogin":     user.LastLogin,
	}

	// Add comprehensive profile information based on role
	switch user.Role {
	case models.InstitutionRole, models.TrainingCenterRole:
		if user.InstitutionProfile != nil {
			userMap["profile"] = fiber.Map{
				"id":               user.InstitutionProfile.ID,
				"institutionName":  user.InstitutionProfile.InstitutionName,
				"isVerified":       user.InstitutionProfile.IsVerified,
				"schoolId":         user.InstitutionProfile.SchoolID,
				"school":           user.InstitutionProfile.School,
				"verificationDocs": user.InstitutionProfile.VerificationDocs,
				"createdAt":        user.InstitutionProfile.CreatedAt,
				"updatedAt":        user.InstitutionProfile.UpdatedAt,
			}
		}
	case models.MontessoriProfessionalRole:
		if user.MontessoriProfessionalProfile != nil {
			userMap["profile"] = fiber.Map{
				"id":             user.MontessoriProfessionalProfile.ID,
				"bio":            user.MontessoriProfessionalProfile.Bio,
				"qualifications": user.MontessoriProfessionalProfile.Qualifications,
				"experience":     user.MontessoriProfessionalProfile.Experience,
				"savedSchools":   user.MontessoriProfessionalProfile.SavedSchools,
				"createdAt":      user.MontessoriProfessionalProfile.CreatedAt,
				"updatedAt":      user.MontessoriProfessionalProfile.UpdatedAt,
			}
		}
	case models.ParentRole:
		if user.ParentProfile != nil {
			userMap["profile"] = fiber.Map{
				"id":                user.ParentProfile.ID,
				"profileVisibility": user.ParentProfile.ProfileVisibility,
				"parentAge":         user.ParentProfile.ParentAge,
				"savedSchools":      user.ParentProfile.SavedSchools,
				"schools":           user.ParentProfile.Schools,
				"createdAt":         user.ParentProfile.CreatedAt,
				"updatedAt":         user.ParentProfile.UpdatedAt,
			}
		}
	}

	// Return user information with profile
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"user": userMap,
	})
}


// UploadProfilePicture handles uploading a new profile picture for the authenticated user
// @Summary Upload profile picture
// @Description Uploads a new profile picture for the current user
// @Tags auth,profile
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Profile picture file"
// @Security BearerAuth
// @Success 201 {object} models.UserProfilePicture
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /me/profile/pictures [post]
func (h *AuthHandler) UploadProfilePicture(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "File is required"})
	}

	// Ensure uploads directory exists
	uploadDir := "./uploads/profile_pictures"
	if err := ensureDir(uploadDir); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create upload directory"})
	}

	// Save file
	dst := fmt.Sprintf("%s/%d_%s", uploadDir, userID, fileHeader.Filename)
	if err := c.SaveFile(fileHeader, dst); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save file: " + err.Error()})
	}

	urlPath := "/uploads/profile_pictures/" + fmt.Sprintf("%d_%s", userID, fileHeader.Filename)

	picture := models.UserProfilePicture{
		UserID:    userID,
		URL:       urlPath,
		FileName:  fileHeader.Filename,
		IsPrimary: false,
	}

	if err := h.db.Create(&picture).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save picture record: " + err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(picture)
}

// ListProfilePictures returns all profile pictures for the authenticated user
// @Summary List profile pictures
// @Description Lists all profile pictures for the current user
// @Tags auth,profile
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.UserProfilePicture
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /me/profile/pictures [get]
func (h *AuthHandler) ListProfilePictures(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	var pics []models.UserProfilePicture
	if err := h.db.Where("user_id = ?", userID).Find(&pics).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load pictures: " + err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(pics)
}

// DeleteProfilePicture deletes a profile picture by ID for the authenticated user
// @Summary Delete profile picture
// @Description Deletes a specific profile picture
// @Tags auth,profile
// @Produce json
// @Param picture_id path int true "Picture ID"
// @Security BearerAuth
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /me/profile/pictures/{picture_id} [delete]
func (h *AuthHandler) DeleteProfilePicture(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	picIDStr := c.Params("picture_id")
	picID, err := strconv.ParseUint(picIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid picture ID"})
	}

	var pic models.UserProfilePicture
	if err := h.db.First(&pic, uint(picID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Picture not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}

	if pic.UserID != userID {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Not authorized to delete this picture"})
	}

	// Delete file from disk (best-effort)
	// Map URL back to path
	filePath := "." + pic.URL
	_ = os.Remove(filePath)

	if err := h.db.Delete(&pic).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete picture record: " + err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Picture deleted"})
}

// SetPrimaryProfilePicture sets a specific picture as primary for the authenticated user
// @Summary Set primary profile picture
// @Description Marks a given profile picture as the primary picture
// @Tags auth,profile
// @Produce json
// @Param picture_id path int true "Picture ID"
// @Security BearerAuth
// @Success 200 {object} models.UserProfilePicture
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /me/profile/pictures/{picture_id}/primary [put]
func (h *AuthHandler) SetPrimaryProfilePicture(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	picIDStr := c.Params("picture_id")
	picID, err := strconv.ParseUint(picIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid picture ID"})
	}

	var pic models.UserProfilePicture
	if err := h.db.First(&pic, uint(picID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Picture not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}
	if pic.UserID != userID {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Not authorized to modify this picture"})
	}

	// Unset other primary flags for this user
	if err := h.db.Model(&models.UserProfilePicture{}).Where("user_id = ?", userID).Update("is_primary", false).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to unset previous primary pictures: " + err.Error()})
	}

	pic.IsPrimary = true
	if err := h.db.Save(&pic).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to set primary picture: " + err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(pic)
}

// ForgotPasswordRequest is the request body for forgot password.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordRequest is the request body for reset password.
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

// generatePasswordResetToken generates a random password reset token
func generatePasswordResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ForgotPassword handles forgot password requests
// @Summary Request password reset
// @Description Sends a password reset link to the user's email address
// @Tags auth,public
// @Accept json
// @Produce json
// @Param request body ForgotPasswordRequest true "Email address"
// @Success 200 {object} map[string]interface{} "Password reset email sent"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 404 {object} map[string]string "Email not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /forgot-password [post]
func (h *AuthHandler) ForgotPassword(c *fiber.Ctx) error {
	req := new(ForgotPasswordRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}

	// Validate request
	if err := h.validateRequest(c, req); err != nil {
		return err
	}

	// Find user by email
	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// For security, don't reveal if email exists or not
			LogUserAction(h.db, 0, "FORGOT_PASSWORD_EMAIL_NOT_FOUND", 0, "User", fmt.Sprintf("Password reset requested for non-existent email: %s", req.Email), c)
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "If your email address is registered, you will receive a password reset link shortly."})
		}
		LogUserAction(h.db, 0, "FORGOT_PASSWORD_DB_ERROR", 0, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error during password reset"})
	}

	// Check if user is active
	if !user.IsActive {
		LogUserAction(h.db, user.ID, "FORGOT_PASSWORD_INACTIVE_USER", user.ID, "User", fmt.Sprintf("Password reset requested for inactive user: %s", req.Email), c)
		// For security, don't reveal account status
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "If your email address is registered, you will receive a password reset link shortly."})
	}

	// Generate password reset token
	resetToken, err := generatePasswordResetToken()
	if err != nil {
		LogUserAction(h.db, user.ID, "FORGOT_PASSWORD_TOKEN_GEN_FAIL", user.ID, "System", "Password reset token generation failed", c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate reset token"})
	}

	// Set token expiry to 1 hour from now
	tokenExpiry := time.Now().Add(1 * time.Hour)

	// Update user with reset token
	user.PasswordResetToken = &resetToken
	user.PasswordResetTokenExpiry = &tokenExpiry

	if err := h.db.Save(&user).Error; err != nil {
		LogUserAction(h.db, user.ID, "FORGOT_PASSWORD_UPDATE_FAIL", user.ID, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save reset token"})
	}

	// Send password reset email
	resetURL := fmt.Sprintf("https://montessoriworldconnect.com/reset-password?token=%s", resetToken)
	emailSubject := "Password Reset Request"
	emailBody := fmt.Sprintf(`
		<h1>Hello %s,</h1>
		<p>You have requested to reset your password for your Montessori World Connect account.</p>
		<p>Please click the link below to reset your password:</p>
		<p><a href="%s">Reset Password</a></p>
		<p>If the link doesn't work, you can copy and paste this URL into your browser:</p>
		<p>%s</p>
		<p>This link will expire in 1 hour for security reasons.</p>
		<p>If you didn't request a password reset, you can safely ignore this email.</p>
		<p>Best regards,<br>The Montessori World Connect Team</p>
	`, user.FirstName, resetURL, resetURL)

	if err := h.emailService.SendEmail(user.Email, emailSubject, emailBody); err != nil {
		log.Printf("Failed to send password reset email to %s: %v", user.Email, err)
		LogUserAction(h.db, user.ID, "FORGOT_PASSWORD_EMAIL_FAIL", user.ID, "Email", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send reset email"})
	}

	LogUserAction(h.db, user.ID, "FORGOT_PASSWORD_SUCCESS", user.ID, "User", fmt.Sprintf("Password reset email sent to: %s", user.Email), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "If your email address is registered, you will receive a password reset link shortly.",
	})
}

// ResetPassword handles password reset requests
// @Summary Reset password using token
// @Description Resets the user's password using a valid reset token
// @Tags auth,public
// @Accept json
// @Produce json
// @Param request body ResetPasswordRequest true "Reset token and new password"
// @Success 200 {object} map[string]interface{} "Password reset successful"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 404 {object} map[string]string "Invalid or expired token"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /reset-password [post]
func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	req := new(ResetPasswordRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}

	// Validate request
	if err := h.validateRequest(c, req); err != nil {
		return err
	}

	// Find user with valid reset token
	var user models.User
	err := h.db.Where("password_reset_token = ? AND password_reset_token_expiry > ?", req.Token, time.Now()).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			LogUserAction(h.db, 0, "RESET_PASSWORD_INVALID_TOKEN", 0, "User", fmt.Sprintf("Invalid or expired reset token: %s", req.Token), c)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Invalid or expired reset token"})
		}
		LogUserAction(h.db, 0, "RESET_PASSWORD_DB_ERROR", 0, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error during password reset"})
	}

	// Check if user is active
	if !user.IsActive {
		LogUserAction(h.db, user.ID, "RESET_PASSWORD_INACTIVE_USER", user.ID, "User", fmt.Sprintf("Password reset attempted for inactive user: %s", user.Email), c)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "User account is inactive. Please contact support."})
	}

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		LogUserAction(h.db, user.ID, "RESET_PASSWORD_HASH_FAIL", user.ID, "System", "Password hashing failed", c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash new password"})
	}

	// Update user password and clear reset token
	user.PasswordHash = string(hashedPassword)
	user.PasswordResetToken = nil
	user.PasswordResetTokenExpiry = nil

	if err := h.db.Save(&user).Error; err != nil {
		LogUserAction(h.db, user.ID, "RESET_PASSWORD_UPDATE_FAIL", user.ID, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update password"})
	}

	LogUserAction(h.db, user.ID, "RESET_PASSWORD_SUCCESS", user.ID, "User", fmt.Sprintf("Password successfully reset for user: %s", user.Email), c)

	// Send confirmation email
	emailSubject := "Password Reset Successful"
	emailBody := fmt.Sprintf(`
		<h1>Hello %s,</h1>
		<p>Your password has been successfully reset for your Montessori World Connect account.</p>
		<p>You can now log in with your new password.</p>
		<p>If you didn't reset your password, please contact our support team immediately.</p>
		<p>Best regards,<br>The Montessori World Connect Team</p>
	`, user.FirstName)

	if err := h.emailService.SendEmail(user.Email, emailSubject, emailBody); err != nil {
		log.Printf("Failed to send password reset confirmation email to %s: %v", user.Email, err)
		LogUserAction(h.db, user.ID, "RESET_PASSWORD_CONFIRMATION_EMAIL_FAIL", user.ID, "Email", err.Error(), c)
		// Don't fail the request if email fails, password was already reset
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Password reset successful. You can now log in with your new password.",
	})
}
