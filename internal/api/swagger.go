package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// @title Montessori World Connect API
// @version 2.2.8
// @description API for the Montessori World Connect platform - connecting Montessori educators, institutions, and parents worldwide.
// @description
// @description ## 📚 Documentation
// @description
// @description This API documentation is organized into multiple sections for easy navigation:
// @description
// @description ### Quick Links
// @description - **[Introduction](/docs/markdown/introduction.md)** - Platform overview and key features
// @description - **[Getting Started](/docs/markdown/getting-started.md)** - Authentication, registration, and API basics
// @description - **[User Roles](/docs/markdown/user-roles.md)** - Detailed role descriptions and permissions
// @description - **[Subscriptions](/docs/markdown/subscriptions.md)** - Free trial, plans, and subscription management
// @description - **[API Examples](/docs/markdown/examples.md)** - Code examples in multiple languages
// @description - **[Webhooks & Events](/docs/markdown/webhooks.md)** - Real-time updates and webhook integration
// @description
// @description ## 🚀 Quick Start
// @description
// @description 1. **Register**: Create an account at `/api/v1/register`
// @description 2. **Verify Email**: Check your email and verify your account
// @description 3. **Login**: Get your JWT token at `/api/v1/login`
// @description 4. **Authenticate**: Include token in `Authorization` header
// @description 5. **Start Building**: Make API requests with your token
// @description
// @description ## 🔑 Authentication
// @description
// @description Include your JWT token in the Authorization header:
// @description ```
// @description Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
// @description ```
// @description
// @description ## 👥 User Roles
// @description
// @description - `institution` - Schools posting jobs and managing profiles
// @description - `training_center` - Training centers with job posting capabilities
// @description - `montessori_professional` - Educators searching jobs and applying
// @description - `parent` - Parents searching schools and writing reviews
// @description - `admin` - Content moderators with elevated permissions
// @description - `super_admin` - System administrators with full access
// @description
// @description ## 🎁 Free Trial
// @description
// @description All new users get a **60-day free trial** with full premium access!
// @description
// @description ## 📋 API Conventions
// @description
// @description - **Base URL**: `https://api.montessoriworldconnect.com`
// @description - **Format**: All responses in JSON
// @description - **Pagination**: Use `page` and `limit` query parameters
// @description - **Errors**: Standard HTTP status codes with error messages
// @description
// @description ## 🖼️ Media Storage (Uploads)
// @description
// @description Uploads are stored on the local filesystem by default. If `S3_BUCKET` is configured, uploads are stored in AWS S3.
// @description When S3 is enabled, media `url` fields returned by the API are client-loadable HTTPS URLs (either a stable public URL when `S3_PUBLIC_BASE_URL` is set, or a presigned URL otherwise).
// @description
// @description ## 🌐 Public Endpoints
// @description
// @description No authentication required:
// @description - `POST /api/v1/register` - User registration
// @description - `POST /api/v1/login` - User login
// @description - `GET /api/v1/schools/public` - Search schools
// @description - `GET /api/v1/institutions/search` - Search institutions
// @description - `GET /api/v1/jobs` - Browse jobs
// @description - `GET /api/v1/events` - View events
// @description - `GET /api/v1/blogs` - Read blogs
// @description
// @description ## 💬 Support
// @description
// @description Need help? Contact us at **support@montessoriworldconnect.com**
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email support@montessoriworldconnect.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host api.montessoriworldconnect.com
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT token for authenticated endpoints. Prefer `Bearer <token>` (raw token is also accepted).

// @tag.name public
// @tag.description Public endpoints that don't require authentication
// @tag.name authenticated
// @tag.description Endpoints that require authentication

// SetupSwagger sets up the Swagger documentation
func SetupSwagger(app *fiber.App) {
	// Serve Swagger UI with custom configuration
	app.Get("/swagger/*", swagger.New(swagger.Config{
		URL:         "/docs/swagger.json", // The URL pointing to API definition
		DeepLinking: true,                 // Enable deep linking for tags and operations
	}))
}

// Note: To generate Swagger documentation, you need to run the following command:
// swag init -g internal/api/swagger.go -o ./docs
// This will generate the necessary files in the docs directory.
// You'll need to install swaggo/swag first: go get -u github.com/swaggo/swag/cmd/swag
