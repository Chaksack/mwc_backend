package api

import (
	"log"

	"mwc_backend/config"
	"mwc_backend/internal/api/handlers"
	"mwc_backend/internal/api/middleware"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"mwc_backend/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"gorm.io/gorm"
)

// SetupRoutes initializes all the API routes.
func SetupRoutes(
	app *fiber.App,
	db *gorm.DB,
	mqService queue.MessageQueueService,
	emailService email.EmailService,
	cfg *config.Config,
) {
	// Create instances of handlers, passing dependencies
	authHandler := handlers.NewAuthHandler(db, cfg, emailService, mqService) // Pass full cfg
	adminHandler := handlers.NewAdminHandler(db, mqService, cfg)
	institutionHandler := handlers.NewInstitutionHandler(db, mqService, emailService)
	montessoriProfessionalHandler := handlers.NewMontessoriProfessionalHandler(db, mqService, emailService)
	parentHandler := handlers.NewParentHandler(db, mqService, emailService)
	subscriptionHandler := handlers.NewSubscriptionHandler(db, cfg, mqService, emailService)
	websocketHandler := handlers.NewWebSocketHandler(db, cfg)
	reviewHandler := handlers.NewReviewHandler(db, mqService)
	eventHandler := handlers.NewEventHandler(db, cfg, mqService)
	blogHandler := handlers.NewBlogHandler(db)
	savedSchoolHandler := handlers.NewSavedSchoolHandler(db)
	healthHandler := handlers.NewHealthHandler(db, cfg, mqService)

	// Initialize and start notification scheduler
	notificationService := services.NewNotificationService(db, emailService)
	schedulerService := services.NewSchedulerService(notificationService)
	go schedulerService.Start()
	log.Println("Notification scheduler service started")

	// Root route handler
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Status(200).JSON(fiber.Map{
			"message":       "Welcome to Montessori World Connect API",
			"version":       "2.2.0",
			"documentation": "/swagger/index.html",
		})
	})

	// Public routes
	// Base URL is configured in config.Config.BaseURL
	// For development: http://localhost:8080/api/v1
	// For production: https://api.montessoriworldconnect.com/api/v1
	// Health endpoints
	app.Get("/health", healthHandler.GetHealth)
	apiV1 := app.Group("/api/v1")
	apiV1.Get("/health", healthHandler.GetHealth)
	continentHandler := handlers.NewContinentHandler(db)
	apiV1.Get("/schools/continent-counts", continentHandler.GetSchoolCountsByContinent)
	apiV1.Get("/schools/continents", continentHandler.ListContinents)

	// API v1 root handler
	apiV1.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Montessori World Connect API v1",
			"endpoints": []string{
				"/register", "/login", "/schools/public",
				"/events", "/schools/:school_id/reviews",
			},
			"documentation": "/swagger/index.html",
		})
	})

	apiV1.Post("/register", authHandler.Register)
	apiV1.Post("/login", authHandler.Login)
	apiV1.Get("/verify-email", authHandler.VerifyEmail)                            // Email verification endpoint
	apiV1.Post("/forgot-password", authHandler.ForgotPassword)                     // Forgot password endpoint
	apiV1.Post("/reset-password", authHandler.ResetPassword)                       // Reset password endpoint
	apiV1.Get("/schools/public", handlers.GetPublicSchools(db))                    // Publicly searchable schools
	apiV1.Get("/institutions/:id", institutionHandler.GetInstitutionPublicDetails) // Public institution details
	// Public subscription plans
	apiV1.Get("/subscription/plans", subscriptionHandler.ListPublicPlans)

	// Public Blog Routes (no auth required)
	apiV1.Get("/blogs", blogHandler.GetBlogs)            // Get all published blogs
	apiV1.Get("/blogs/:slug", blogHandler.GetBlogBySlug) // Get blog by slug

	// Auth Middleware
	authMw := middleware.Protected(cfg.JWTSecret)
	subscriptionMw := middleware.SubscriptionAuth(db)

	// User Routes
	apiV1.Get("/me", authMw, authHandler.GetCurrentUser) // New endpoint to retrieve logged-in user
	// Unified Saved Schools Routes for all user roles (Parent, Montessori Professional)
	apiV1.Post("/me/schools/saved/:school_id", authMw, savedSchoolHandler.SaveSchool)
	apiV1.Delete("/me/schools/saved/:school_id", authMw, savedSchoolHandler.DeleteSavedSchool)
	apiV1.Get("/me/schools/saved", authMw, savedSchoolHandler.GetSavedSchools)
	// Profile picture management for authenticated users
	apiV1.Post("/me/profile/pictures", authMw, authHandler.UploadProfilePicture)
	apiV1.Get("/me/profile/pictures", authMw, authHandler.ListProfilePictures)
	apiV1.Delete("/me/profile/pictures/:picture_id", authMw, authHandler.DeleteProfilePicture)
	apiV1.Put("/me/profile/pictures/:picture_id/primary", authMw, authHandler.SetPrimaryProfilePicture)

	// Jobs endpoint - requires paid subscription
	apiV1.Get("/jobs", authMw, subscriptionMw, institutionHandler.GetAllJobs)

	// Admin Routes (accessible by both Admin and SuperAdmin)
	adminRoutes := apiV1.Group("/admin", authMw, middleware.RoleAuth(models.AdminRole, models.SuperAdminRole))
	adminRoutes.Post("/schools/batch-upload", adminHandler.BatchUploadSchools)
	adminRoutes.Post("/schools/create", adminHandler.CreateSchool)                  // New: Manual school creation
	adminRoutes.Post("/training-centers/create", adminHandler.CreateTrainingCenter) // New: Manual training center creation
	adminRoutes.Put("/schools/:id", adminHandler.UpdateSchool)
	adminRoutes.Get("/schools", adminHandler.GetSchoolsByCountry) // ?country_code=US
	adminRoutes.Delete("/schools/:id", adminHandler.DeleteSchool)
	adminRoutes.Get("/users", adminHandler.GetAllUsers)
	adminRoutes.Put("/users/:id/status", adminHandler.UpdateUserStatus) // New: Update user active status
	adminRoutes.Put("/users/:id/role", adminHandler.UpdateUserRole)     // New: Update user role
	adminRoutes.Delete("/users/:id", adminHandler.DeleteUser)           // New: Delete a user
	adminRoutes.Get("/action-logs", adminHandler.GetActionLogs)

	// Blog Management Routes (Admin/SuperAdmin)
	adminRoutes.Post("/blogs", blogHandler.CreateBlog)
	adminRoutes.Put("/blogs/:id", blogHandler.UpdateBlog)
	adminRoutes.Delete("/blogs/:id", blogHandler.DeleteBlog)

	// Dynamic Subscription Management Routes
	adminRoutes.Post("/subscription-plans", adminHandler.CreateSubscriptionPlan)
	adminRoutes.Get("/subscription-plans", adminHandler.GetSubscriptionPlans)
	adminRoutes.Put("/subscription-plans/:id", adminHandler.UpdateSubscriptionPlan)
	adminRoutes.Delete("/subscription-plans/:id", adminHandler.DeleteSubscriptionPlan)
	adminRoutes.Get("/role-subscriptions", adminHandler.GetRoleSubscriptionMappings) // ?role=parent
	adminRoutes.Post("/assign-subscription", adminHandler.AssignUserSubscription)

	// Super Admin only Routes (for admin user management)
	superAdminRoutes := apiV1.Group("/admin", authMw, middleware.RoleAuth(models.SuperAdminRole))
	superAdminRoutes.Post("/admins", adminHandler.CreateAdmin) // Create new admin users

	// Institution and Training Center Routes (shared logic)
	instTcRoutes := apiV1.Group("/institution", authMw, middleware.RoleAuth(models.InstitutionRole, models.TrainingCenterRole))
	instTcRoutes.Post("/profile", institutionHandler.CreateOrUpdateInstitutionProfile)
	instTcRoutes.Get("/schools/available", institutionHandler.GetAvailableSchools) // Get schools available for selection
	instTcRoutes.Post("/schools", institutionHandler.CreateSchool)                 // If school not in admin list
	instTcRoutes.Put("/schools/select/:school_id", institutionHandler.SelectSchool)
	instTcRoutes.Post("/jobs", institutionHandler.PostJob)
	instTcRoutes.Put("/jobs/:job_id", institutionHandler.UpdateJob)
	instTcRoutes.Delete("/jobs/:job_id", institutionHandler.DeleteJob)
	instTcRoutes.Get("/jobs/:job_id/applicants", institutionHandler.GetJobApplicants)
	instTcRoutes.Get("/jobs", institutionHandler.GetMyJobs)

	// Allow anyone to discover montessori professionals actively looking for jobs (public)
	apiV1.Get("/institution/montessori-professionals/looking-for-jobs", montessoriProfessionalHandler.ListLookingForJobs)
	// Allow institutions/training centers to view a professional's public profile and contact them
	instTcRoutes.Get("/montessori-professionals/:id", montessoriProfessionalHandler.GetPublicProfessional)
	instTcRoutes.Post("/montessori-professionals/:id/contact", montessoriProfessionalHandler.ContactProfessional)

	// Montessori Professional Routes
	montessoriProfessionalRoutes := apiV1.Group("/montessori-professional", authMw, middleware.RoleAuth(models.MontessoriProfessionalRole))
	montessoriProfessionalRoutes.Post("/profile", montessoriProfessionalHandler.CreateOrUpdateMontessoriProfessionalProfile)
	montessoriProfessionalRoutes.Get("/schools/search", montessoriProfessionalHandler.SearchSchools)
	montessoriProfessionalRoutes.Post("/schools/save/:school_id", montessoriProfessionalHandler.SaveSchool)
	montessoriProfessionalRoutes.Delete("/schools/save/:school_id", montessoriProfessionalHandler.DeleteSavedSchool)
	montessoriProfessionalRoutes.Get("/schools/saved", montessoriProfessionalHandler.GetSavedSchools)
	montessoriProfessionalRoutes.Post("/jobs/:job_id/apply", montessoriProfessionalHandler.ApplyForJob)
	montessoriProfessionalRoutes.Get("/jobs/applied", montessoriProfessionalHandler.GetAppliedJobs)
	// Montessori Professional Job Preferences
	montessoriProfessionalRoutes.Get("/job-preferences", montessoriProfessionalHandler.GetJobPreference)
	montessoriProfessionalRoutes.Put("/job-preferences", montessoriProfessionalHandler.UpsertJobPreference)
	montessoriProfessionalRoutes.Delete("/job-preferences", montessoriProfessionalHandler.DeleteJobPreference)

	// Parent Routes
	parentRoutes := apiV1.Group("/parent", authMw, middleware.RoleAuth(models.ParentRole))
	parentRoutes.Post("/profile", parentHandler.CreateOrUpdateParentProfile)
	parentRoutes.Get("/schools/search", parentHandler.SearchSchools)                                // Can reuse educator's or have its own
	parentRoutes.Get("/schools/:school_id/details", subscriptionMw, parentHandler.GetSchoolDetails) // Get school details with public parent profiles (requires active subscription)
	parentRoutes.Post("/schools/save/:school_id", parentHandler.SaveSchool)
	parentRoutes.Delete("/schools/save/:school_id", parentHandler.DeleteSavedSchool)
	parentRoutes.Get("/schools/saved", parentHandler.GetSavedSchools)
	parentRoutes.Post("/messages/send/:recipient_id", parentHandler.SendMessage)
	parentRoutes.Get("/messages", parentHandler.GetMessages)
	parentRoutes.Post("/messages/:message_id/read", parentHandler.MarkMessageAsRead)

	// Subscription Routes
	subscriptionRoutes := apiV1.Group("/subscription", authMw)
	subscriptionRoutes.Post("/checkout", subscriptionHandler.CreateCheckoutSession)
	subscriptionRoutes.Get("/status", subscriptionHandler.GetUserSubscription)
	subscriptionRoutes.Post("/cancel", subscriptionHandler.CancelSubscription)
	subscriptionRoutes.Post("/portal", subscriptionHandler.CreateBillingPortalSession)

	// Protected routes that require authentication
	apiV1.Get("/institutions/:id/details", authMw, subscriptionMw, institutionHandler.GetInstitutionDetails) // Detailed institution info (requires active subscription)

	// Review Routes
	reviewRoutes := apiV1.Group("/reviews", authMw)
	reviewRoutes.Post("/", reviewHandler.CreateReview)
	reviewRoutes.Get("/user", reviewHandler.GetUserReviews)
	reviewRoutes.Put("/:review_id", reviewHandler.UpdateReview)
	reviewRoutes.Delete("/:review_id", reviewHandler.DeleteReview)

	// Public Review Routes (no auth required)
	apiV1.Get("/schools/:school_id/reviews", reviewHandler.GetSchoolReviews)

	// Admin Review Routes
	adminReviewRoutes := apiV1.Group("/admin/reviews", authMw, middleware.RoleAuth(models.AdminRole))
	adminReviewRoutes.Get("/pending", reviewHandler.GetPendingReviews)
	adminReviewRoutes.Put("/:review_id/moderate", reviewHandler.ModerateReview)

	// Event Routes
	// Public event routes
	apiV1.Get("/events", eventHandler.GetEvents)
	apiV1.Get("/events/:event_id", eventHandler.GetEvent)
	apiV1.Get("/events/featured", eventHandler.GetFeaturedEvents)

	// Institution event routes
	institutionEventRoutes := apiV1.Group("/institution/events", authMw, middleware.RoleAuth(models.InstitutionRole, models.TrainingCenterRole))
	institutionEventRoutes.Post("/", eventHandler.CreateEvent)
	institutionEventRoutes.Get("/", eventHandler.GetInstitutionEvents)
	institutionEventRoutes.Put("/:event_id", eventHandler.UpdateEvent)
	institutionEventRoutes.Delete("/:event_id", eventHandler.DeleteEvent)

	// Admin event routes
	adminEventRoutes := apiV1.Group("/admin/events", authMw, middleware.RoleAuth(models.AdminRole))
	adminEventRoutes.Put("/:event_id/feature", eventHandler.FeatureEvent)

	// WebSocket Routes
	if cfg.WebSocketEnabled {
		// Use the WebSocket middleware to upgrade HTTP connections to WebSocket
		wsGroup := app.Group("/wss", authMw, handlers.WebSocketUpgradeMiddleware())
		// Use the * pattern to handle all WebSocket connections
		wsGroup.Get("/*", websocket.New(websocketHandler.HandleWebSocket))
		log.Println("WebSocket server enabled at", cfg.WebSocketPath)
	}

	// Webhook Route for RabbitMQ consumer (e.g., to trigger email for unread messages)
	// This endpoint should be secured differently than general API routes.
	// It's intended for server-to-server communication.
	// Consider IP whitelisting, a dedicated secret token in headers, or mTLS.
	// The default CORS policy might be too open for this.
	webhookGroup := app.Group("/webhooks") // No broad CORS middleware here by default
	// Add specific security middleware for webhooks if needed, e.g., middleware.WebhookAuth(cfg.WebhookSecret)
	webhookGroup.Post("/notify-unread-message", handlers.HandleUnreadMessageNotification(db, emailService))
	webhookGroup.Post("/stripe", subscriptionHandler.HandleStripeWebhook)

	// Additional Stripe webhook endpoints (outside /api/v1 namespace)
	app.Post("/stripe/event", subscriptionHandler.HandleStripeSnapshotWebhook)
	app.Post("/stripe/payload", subscriptionHandler.HandleStripeThinPayloadWebhook)
}
