package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Message       string                 `json:"message"`
	Token         string                 `json:"token"`
	User          map[string]interface{} `json:"user"`
	Error         string                 `json:"error"`
	EmailVerified bool                   `json:"email_verified"`
}

func main() {
	// Get server URL from environment or use default
	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	// Test admin login credentials
	loginReq := LoginRequest{
		Email:    "admin@test.com",
		Password: "admin123",
	}

	// Convert to JSON
	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		log.Fatalf("Error marshaling JSON: %v", err)
	}

	// Make POST request to login endpoint
	resp, err := http.Post(serverURL+"/login", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Error making request: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response: %v", err)
	}

	// Parse response
	var loginResp LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		log.Fatalf("Error parsing response: %v", err)
	}

	// Print results
	fmt.Printf("Status Code: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(body))

	if resp.StatusCode != 200 {
		fmt.Printf("❌ ADMIN LOGIN FAILED: %s\n", loginResp.Error)
		if loginResp.EmailVerified == false {
			fmt.Printf("🔍 ISSUE CONFIRMED: Admin login blocked by email verification requirement\n")
		}
	} else {
		fmt.Printf("✅ ADMIN LOGIN SUCCESS: %s\n", loginResp.Message)
		if user, ok := loginResp.User["role"].(string); ok {
			fmt.Printf("🔑 User Role: %s\n", user)
		}
	}
}