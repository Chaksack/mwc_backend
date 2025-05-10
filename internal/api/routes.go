package api

import (
	"github.com/gofiber/fiber/v2"
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

	// Public routes
	apiV1 := app.Group("/api/v1")
	apiV1.Post("/register", authHandler.Register)
	apiV1.Post("/login", authHandler.Login)
	apiV1.Get("/schools/public", handlers.GetPublicSchools(db)) // Publicly searchable schools

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

	// Webhook Route for RabbitMQ consumer (e.g., to trigger email for unread messages)
	// This endpoint should be secured differently than general API routes.
	// It's intended for server-to-server communication.
	// Consider IP whitelisting, a dedicated secret token in headers, or mTLS.
	// The default CORS policy might be too open for this.
	webhookGroup := app.Group("/webhooks") // No broad CORS middleware here by default
	// Add specific security middleware for webhooks if needed, e.g., middleware.WebhookAuth(cfg.WebhookSecret)
	webhookGroup.Post("/notify-unread-message", handlers.HandleUnreadMessageNotification(db, emailService))
}
