package api

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"gorm.io/gorm"
	"mwc_backend/config"
	"mwc_backend/internal/api/handlers"
	"mwc_backend/internal/api/middleware"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
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
	adminHandler := handlers.NewAdminHandler(db, mqService)
	institutionHandler := handlers.NewInstitutionHandler(db, mqService)
	educatorHandler := handlers.NewEducatorHandler(db, mqService)
	parentHandler := handlers.NewParentHandler(db, mqService, emailService)
	subscriptionHandler := handlers.NewSubscriptionHandler(db, cfg, mqService)
	websocketHandler := handlers.NewWebSocketHandler(db, cfg)
	reviewHandler := handlers.NewReviewHandler(db, mqService)
	eventHandler := handlers.NewEventHandler(db, cfg, mqService)
	blogHandler := handlers.NewBlogHandler(db, cfg, mqService)

	// Root route handler
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Status(200).JSON(fiber.Map{
			"message": "Welcome to Montessori World Connect API",
			"version": "1.0",
			"documentation": "/swagger/index.html",
		})
	})

	// Public routes
	apiV1 := app.Group("/api/v1")

	// API v1 root handler
	apiV1.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Montessori World Connect API v1",
			"endpoints": []string{
				"/register", "/login", "/schools/public", "/jobs",
				"/events", "/blog", "/schools/:school_id/reviews",
			},
			"documentation": "/swagger/index.html",
		})
	})

	apiV1.Post("/register", authHandler.Register)
	apiV1.Post("/login", authHandler.Login)
	apiV1.Get("/schools/public", handlers.GetPublicSchools(db)) // Publicly searchable schools
	apiV1.Get("/jobs", institutionHandler.GetAllJobs) // Publicly searchable jobs

	// Auth Middleware
	authMw := middleware.Protected(cfg.JWTSecret)

	// Admin Routes
	adminRoutes := apiV1.Group("/admin", authMw, middleware.RoleAuth(models.AdminRole))
	adminRoutes.Post("/schools/batch-upload", adminHandler.BatchUploadSchools)
	adminRoutes.Put("/schools/:id", adminHandler.UpdateSchool)
	adminRoutes.Get("/schools", adminHandler.GetSchoolsByCountry) // ?country_code=US
	adminRoutes.Delete("/schools/:id", adminHandler.DeleteSchool)
	adminRoutes.Get("/users", adminHandler.GetAllUsers)
	adminRoutes.Put("/users/:id/status", adminHandler.UpdateUserStatus) // New: Update user active status
	adminRoutes.Put("/users/:id/role", adminHandler.UpdateUserRole)     // New: Update user role
	adminRoutes.Delete("/users/:id", adminHandler.DeleteUser)           // New: Delete a user
	adminRoutes.Get("/action-logs", adminHandler.GetActionLogs)

	// Institution and Training Center Routes (shared logic)
	instTcRoutes := apiV1.Group("/institution", authMw, middleware.RoleAuth(models.InstitutionRole, models.TrainingCenterRole))
	instTcRoutes.Post("/profile", institutionHandler.CreateOrUpdateInstitutionProfile)
	instTcRoutes.Post("/schools", institutionHandler.CreateSchool) // If school not in admin list
	instTcRoutes.Put("/schools/select/:school_id", institutionHandler.SelectSchool)
	instTcRoutes.Post("/jobs", institutionHandler.PostJob)
	instTcRoutes.Put("/jobs/:job_id", institutionHandler.UpdateJob)
	instTcRoutes.Delete("/jobs/:job_id", institutionHandler.DeleteJob)
	instTcRoutes.Get("/jobs/:job_id/applicants", institutionHandler.GetJobApplicants)
	instTcRoutes.Get("/jobs", institutionHandler.GetMyJobs)

	// Educator Routes
	educatorRoutes := apiV1.Group("/educator", authMw, middleware.RoleAuth(models.EducatorRole))
	educatorRoutes.Post("/profile", educatorHandler.CreateOrUpdateEducatorProfile)
	educatorRoutes.Get("/schools/search", educatorHandler.SearchSchools)
	educatorRoutes.Post("/schools/save/:school_id", educatorHandler.SaveSchool)
	educatorRoutes.Delete("/schools/save/:school_id", educatorHandler.DeleteSavedSchool)
	educatorRoutes.Get("/schools/saved", educatorHandler.GetSavedSchools)
	educatorRoutes.Post("/jobs/:job_id/apply", educatorHandler.ApplyForJob)
	educatorRoutes.Get("/jobs/applied", educatorHandler.GetAppliedJobs)

	// Parent Routes
	parentRoutes := apiV1.Group("/parent", authMw, middleware.RoleAuth(models.ParentRole))
	parentRoutes.Post("/profile", parentHandler.CreateOrUpdateParentProfile)
	parentRoutes.Get("/schools/search", parentHandler.SearchSchools) // Can reuse educator's or have its own
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

	// Blog Routes
	// Public blog routes
	apiV1.Get("/blog", blogHandler.GetBlogPosts)
	apiV1.Get("/blog/:slug", blogHandler.GetBlogPost)
	apiV1.Get("/blog/featured", blogHandler.GetFeaturedBlogPosts)
	apiV1.Get("/blog/categories", blogHandler.GetBlogCategories)
	apiV1.Get("/blog/tags", blogHandler.GetBlogTags)

	// Admin blog routes
	adminBlogRoutes := apiV1.Group("/admin/blog", authMw, middleware.RoleAuth(models.AdminRole))
	adminBlogRoutes.Post("/", blogHandler.CreateBlogPost)
	adminBlogRoutes.Put("/:post_id", blogHandler.UpdateBlogPost)
	adminBlogRoutes.Delete("/:post_id", blogHandler.DeleteBlogPost)

	// WebSocket Routes
	if cfg.WebSocketEnabled {
		// Use the WebSocket middleware to upgrade HTTP connections to WebSocket
		wsGroup := app.Group("/ws", authMw, handlers.WebSocketUpgradeMiddleware())
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
}
