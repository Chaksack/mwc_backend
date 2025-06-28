package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/api"
	"mwc_backend/internal/email"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"mwc_backend/internal/store"
	"os"
)

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

	// Initialize RabbitMQ
	rabbitMQService, err := queue.NewRabbitMQService(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer rabbitMQService.Close() // Ensure RabbitMQ connection is closed on exit
	log.Println("RabbitMQ connected successfully.")

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
	})

	// Middleware
	app.Use(recover.New())            // Recovers from panics anywhere in the stack
	app.Use(logger.New(logger.Config{ // Logs HTTP requests
		Format: "[${time}] ${status} - ${latency} ${method} ${path} ${ip}\n",
	}))
	// General CORS for most API endpoints
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // TODO: Restrict this in production to your frontend's domain(s)
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// Setup API routes
	api.SetupRoutes(app, db, rabbitMQService, emailService, cfg)

	// Setup Swagger documentation
	api.SetupSwagger(app)
	log.Println("Swagger documentation available at /swagger/index.html")

	// Start server
	port := os.Getenv("PORT")
	log.Printf("Server starting on port %s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
