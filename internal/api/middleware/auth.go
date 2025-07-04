package middleware

import (
	"log"
	"mwc_backend/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims.
type Claims struct {
	UserID uint            `json:"user_id"` // Changed to uint to match gorm.Model.ID
	Email  string          `json:"email"`
	Role   models.UserRole `json:"role"`
	jwt.RegisteredClaims
}

// Protected returns a middleware that protects routes requiring authentication.
func Protected(jwtSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			log.Printf("Missing Authorization header in request to %s", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing Authorization header. Please include your token in the Authorization header."})
		}

		// Only accept tokens without the "Bearer" prefix
		tokenStr := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format. Please provide the token directly in the Authorization header without the 'Bearer' prefix."})
		}

		if tokenStr == "" {
			log.Printf("Empty token in request to %s", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Empty JWT token. Please include a valid token in the Authorization header."})
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.NewError(fiber.StatusUnauthorized, "unexpected signing method")
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			// Log the error for debugging, e.g., token expired, signature invalid
			log.Printf("JWT validation error for request to %s: %v", c.Path(), err)

			var errorMessage string
			if err != nil {
				switch {
				case strings.Contains(err.Error(), "token is expired"):
					errorMessage = "JWT token has expired. Please login again to get a new token."
				case strings.Contains(err.Error(), "signature is invalid"):
					errorMessage = "JWT token signature is invalid. Please ensure you're using the correct token."
				default:
					errorMessage = "Invalid JWT token: " + err.Error()
				}
			} else {
				errorMessage = "Invalid JWT token."
			}

			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": errorMessage})
		}

		// Store user information in context for handlers
		c.Locals("user_id", claims.UserID) // Storing as uint
		c.Locals("user_email", claims.Email)
		c.Locals("user_role", claims.Role)
		c.Locals("user_claims", claims)

		return c.Next()
	}
}

// RoleAuth returns a middleware that checks if the authenticated user has one of the required roles.
func RoleAuth(allowedRoles ...models.UserRole) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole, ok := c.Locals("user_role").(models.UserRole)
		if !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "User role not found in context"})
		}

		for _, role := range allowedRoles {
			if userRole == role {
				return c.Next()
			}
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Insufficient permissions for this resource"})
	}
}

// GenerateJWT generates a new JWT token.
// UserID is now uint to match gorm.Model.ID
func GenerateJWT(userID uint, email string, role models.UserRole, jwtSecret string, expiresIn time.Duration) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "go_fiber_app", // App name
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", err
	}
	return signedToken, nil
}
