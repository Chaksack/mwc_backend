package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// @title Montessori World Connect API
// @version 1.0
// @description API for the Montessori World Connect platform
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email support@montessoriworldconnect.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:8080
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and the JWT token.

// SetupSwagger sets up the Swagger documentation
func SetupSwagger(app *fiber.App) {
	// Serve Swagger UI
	app.Get("/swagger/*", swagger.HandlerDefault)
}

// Note: To generate Swagger documentation, you need to run the following command:
// swag init -g internal/api/swagger.go -o ./docs
// This will generate the necessary files in the docs directory.
// You'll need to install swaggo/swag first: go get -u github.com/swaggo/swag/cmd/swag