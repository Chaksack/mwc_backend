package testing

import (
	"fmt"
	"log"
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

	// Test API endpoints
	endpoints := []string{
		"http://localhost:8080/api/v1/schools/public",
		"http://localhost:8080/swagger/index.html",
	}

	fmt.Println("Testing API endpoints:")
	for _, endpoint := range endpoints {
		resp, err := http.Get(endpoint)
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