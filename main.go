package main

import (
	"github.com/MarceloPetrucio/go-scalar-api-reference"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/api"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"mwc_backend/internal/store"
	"os"
)

// createDefaultAdminIfNeeded checks if an admin user exists and creates one if not
func createDefaultAdminIfNeeded(db *gorm.DB, cfg *config.Config) error {
	// Check if any admin user exists
	var adminCount int64
	if err := db.Model(&models.User{}).Where("role = ?", models.AdminRole).Count(&adminCount).Error; err != nil {
		return err
	}

	// If admin user already exists, return
	if adminCount > 0 {
		log.Println("Admin user already exists, skipping default admin creation")
		return nil
	}

	// Hash the default admin password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.DefaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Create the default admin user
	adminUser := models.User{
		Email:        cfg.DefaultAdminEmail,
		PasswordHash: string(hashedPassword),
		FirstName:    cfg.DefaultAdminFirstName,
		LastName:     cfg.DefaultAdminLastName,
		Role:         models.AdminRole,
		IsActive:     true,
	}

	// Start a transaction
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// Create the admin user
	if err := tx.Create(&adminUser).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return err
	}

	log.Printf("Default admin user created with email: %s", cfg.DefaultAdminEmail)
	return nil
}

func main() {
	// Load .env file (optional, for local development)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize Database connection
	db, err := store.NewConnection(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("Database connected successfully.")

	// Auto-migrate database schema
	log.Println("Running database migrations...")
	err = models.AutoMigrate(db)
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Database migration completed.")

	// Create default admin user if no admin exists
	err = createDefaultAdminIfNeeded(db, cfg)
	if err != nil {
		log.Fatalf("Failed to create default admin user: %v", err)
	}

	// Initialize RabbitMQ (Amazon MQ)
	rabbitMQService, err := queue.NewRabbitMQService(cfg.RabbitMQURL, cfg.RabbitMQUseTLS, cfg.RabbitMQCertPath)
	if err != nil {
		// Log the error but continue execution
		log.Printf("Warning: Failed to connect to RabbitMQ/Amazon MQ: %v", err)
		// Create a no-op RabbitMQ service
		rabbitMQService = &queue.RabbitMQService{}
		// Log a message indicating we're using a no-op service but the application can continue
		log.Println("Using no-op RabbitMQ service. Message queue functionality will be disabled.")
	} else {
		defer rabbitMQService.Close() // Ensure RabbitMQ connection is closed on exit
		// Only log success when we actually connected successfully
		log.Println("RabbitMQ/Amazon MQ connected successfully.")
	}

	// Initialize Email Service
	emailService := email.NewGoMailerService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.EmailFrom)
	log.Println("Email service initialized.")

	// Create Fiber app
	app := fiber.New(fiber.Config{
		// Global error handler
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
		// Increase body size limit to 50MB (default is 4MB)
		BodyLimit: 50 * 1024 * 1024,
	})

	// Middleware
	app.Use(recover.New())            // Recovers from panics anywhere in the stack
	app.Use(logger.New(logger.Config{ // Logs HTTP requests
		Format: "[${time}] ${status} - ${latency} ${method} ${path} ${ip}\n",
	}))
	// General CORS for most API endpoints
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	app.Get("/documentation", func(c *fiber.Ctx) error {
		htmlContent, err := scalar.ApiReferenceHTML(&scalar.Options{
			SpecURL: "./docs/swagger.json",
			CustomOptions: scalar.CustomOptions{
				PageTitle: "Montessori World Connect API",
			},
			DarkMode: true,
		})

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		c.Set("Content-Type", "text/html")
		return c.SendString(htmlContent)
	})

	// Setup API routes
	api.SetupRoutes(app, db, rabbitMQService, emailService, cfg)

	// Setup static route for Swagger JSON files
	app.Static("/docs", "./docs")

	// Setup Swagger documentation
	api.SetupSwagger(app)
	log.Println("Swagger documentation available at /swagger/index.html")

	// Setup metrics dashboard
	app.Get("/metrics", func(c *fiber.Ctx) error {
		return c.SendFile("./views/metrics.html")
	})

	// Setup metrics API endpoint with dummy data
	app.Get("/metrics/api", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"uptime": fiber.Map{
				"seconds": 0,
				"human":   "Unknown",
			},
			"memory": fiber.Map{
				"alloc":       0,
				"total_alloc": 0,
				"sys":         0,
				"num_gc":      0,
			},
			"goroutines": 0,
			"http": fiber.Map{
				"request_count":     fiber.Map{},
				"avg_response_time": fiber.Map{},
			},
			"database": fiber.Map{
				"query_count":    0,
				"avg_query_time": 0,
				"connection_stats": fiber.Map{
					"open_connections": 0,
					"in_use":           0,
					"idle":             0,
					"max_open_conns":   0,
				},
			},
			"logs": []fiber.Map{},
		})
	})

	log.Println("Metrics dashboard available at /metrics")

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default to port 8080 if PORT environment variable is not set
		log.Printf("PORT environment variable not set. Defaulting to %s", port)
	}

	// Always start server with HTTP
	log.Printf("Server starting with HTTP on port %s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
