package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// @title Montessori World Connect API
// @version 1.0
// @description API for the Montessori World Connect platform. Introduction: Welcome to the Montessori World Connect API documentation. This API provides access to various resources and functionalities of the Montessori World Connect platform, including schools, educators, institutions, events, blogs, and more. The API is designed to be RESTful and uses standard HTTP methods (GET, POST, PUT, DELETE) for operations. Responses are returned in JSON format. Getting Started: Authentication - Most endpoints require authentication using JWT (JSON Web Token). To authenticate, you need to: 1) Register a new account or login with existing credentials, 2) Include the received token in the Authorization header of your requests, 3) Format: 'Authorization: Bearer your_token_here'. Public Endpoints - Some endpoints are publicly accessible without authentication: /api/v1/register (Register a new user), /api/v1/login (Login and get authentication token), /api/v1/schools/public (Get list of public schools), /api/v1/jobs (Get list of available jobs), /api/v1/events (Get list of events), /api/v1/blog (Get list of blog posts). Rate Limiting - API requests are subject to rate limiting to ensure fair usage. Please design your applications to handle rate limit responses (HTTP 429) gracefully. Pagination - List endpoints support pagination using 'page' and 'limit' query parameters.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email support@montessoriworldconnect.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:8080
// @BasePath /api/v1
// @schemes http
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and the JWT token.

// SetupSwagger sets up the Swagger documentation
func SetupSwagger(app *fiber.App) {
	// Serve Swagger UI with custom configuration
	app.Get("/swagger/*", swagger.New(swagger.Config{
		URL:         "/docs/swagger.json", // The URL pointing to API definition
		DeepLinking: true,                // Enable deep linking for tags and operations
	}))
}

// Note: To generate Swagger documentation, you need to run the following command:
// swag init -g internal/api/swagger.go -o ./docs
// This will generate the necessary files in the docs directory.
// You'll need to install swaggo/swag first: go get -u github.com/swaggo/swag/cmd/swag
