package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type RegisterRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
}

func main() {
	// Test parent registration
	testParentRegistration()
}

func testParentRegistration() {
	// Create a test parent registration request
	parentReq := RegisterRequest{
		Email:     fmt.Sprintf("testparent_%d@example.com", time.Now().Unix()),
		Password:  "testpassword123",
		FirstName: "John",
		LastName:  "Doe",
		Role:      "parent",
	}

	// Convert to JSON
	jsonData, err := json.Marshal(parentReq)
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}

	// Make HTTP request to registration endpoint
	// Assuming the server runs on localhost:3000, adjust if different
	resp, err := http.Post("http://localhost:3000/api/v1/register", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error making HTTP request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return
	}

	fmt.Printf("Status Code: %d\n", resp.StatusCode)
	fmt.Printf("Response Body: %s\n", string(body))

	if resp.StatusCode == 201 {
		fmt.Println("✅ Parent registration successful!")
		
		// Parse response to verify user details
		var response map[string]interface{}
		if err := json.Unmarshal(body, &response); err == nil {
			if user, ok := response["user"].(map[string]interface{}); ok {
				fmt.Printf("User ID: %.0f\n", user["id"])
				fmt.Printf("Email: %s\n", user["email"])
				fmt.Printf("First Name: %s\n", user["firstName"])
				fmt.Printf("Last Name: %s\n", user["lastName"])
				fmt.Printf("Role: %s\n", user["role"])
				fmt.Printf("Email Verified: %v\n", user["email_verified"])
			}
		}
	} else {
		fmt.Printf("❌ Parent registration failed with status: %d\n", resp.StatusCode)
	}
}