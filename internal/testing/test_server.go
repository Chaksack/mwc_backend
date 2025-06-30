package testing

import (
	"fmt"
	"log"
	"mwc_backend/internal/api/middleware"
	"mwc_backend/internal/models"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// RunTestServer sets up environment variables for testing and runs the main server
// in a separate process, then tests API endpoints.
func RunTestServer() {
	// Set required environment variables for testing
	os.Setenv("PORT", "8080")
	os.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/mwc_test?sslmode=disable")
	os.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	os.Setenv("JWT_SECRET", "test_secret_key")
	os.Setenv("STRIPE_SECRET_KEY", "sk_test_example")
	os.Setenv("STRIPE_PUBLISHABLE_KEY", "pk_test_example")
	os.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_example")
	os.Setenv("WEBSOCKET_ENABLED", "true")

	// Start the server in a goroutine using exec.Command
	cmd := exec.Command("go", "run", "main.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Ensure we clean up the process when this function returns
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			log.Printf("Failed to kill server process: %v", err)
		}
	}()

	// Wait for the server to start
	time.Sleep(2 * time.Second)

	// Generate a valid JWT token for testing
	jwtSecret := os.Getenv("JWT_SECRET")
	token, err := middleware.GenerateJWT(1, "test@example.com", models.AdminRole, jwtSecret, 24*time.Hour)
	if err != nil {
		log.Fatalf("Failed to generate JWT token: %v", err)
	}

	// Test API endpoints
	endpoints := []string{
		"http://localhost:8080/api/v1/schools/public",
		"http://localhost:8080/swagger/index.html",
		"http://localhost:8080/api/v1/admin/users", // Protected route that requires admin role
	}

	fmt.Println("Testing API endpoints:")
	for _, endpoint := range endpoints {
		// Create a new request
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			log.Printf("Error creating request for %s: %v", endpoint, err)
			continue
		}

		// Add Authorization header with valid JWT token
		req.Header.Add("Authorization", "Bearer "+token)

		// Send the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error accessing %s: %v", endpoint, err)
			continue
		}
		defer resp.Body.Close()

		fmt.Printf("- %s: %s\n", endpoint, resp.Status)
	}

	fmt.Println("\nServer is running. Press Ctrl+C to stop.")

	// Wait for the server process to finish
	if err := cmd.Wait(); err != nil {
		log.Printf("Server process ended with error: %v", err)
	}
}
