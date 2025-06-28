package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Set required environment variables for testing
	os.Setenv("PORT", "8080")
	os.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/mwc_test?sslmode=disable")
	os.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	os.Setenv("JWT_SECRET", "test_secret_key")
	os.Setenv("STRIPE_SECRET_KEY", "sk_test_example")
	os.Setenv("STRIPE_PUBLISHABLE_KEY", "pk_test_example")
	os.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_example")
	os.Setenv("WEBSOCKET_ENABLED", "true")

	// Start the server in a goroutine
	go func() {
		// This will call the main() function in main.go
		main()
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
	select {} // Block forever
}