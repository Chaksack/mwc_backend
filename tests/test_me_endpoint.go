package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	// Test the /me endpoint with a sample JWT token
	// Note: This requires a valid JWT token and running server
	
	baseURL := "http://localhost:8080/api/v1"
	
	// Check if JWT token is provided as environment variable
	token := os.Getenv("TEST_JWT_TOKEN")
	if token == "" {
		fmt.Println("Please set TEST_JWT_TOKEN environment variable with a valid JWT token")
		fmt.Println("Usage: TEST_JWT_TOKEN=your_token_here go run test_me_endpoint.go")
		return
	}
	
	// Test /me endpoint
	fmt.Println("Testing /me endpoint...")
	
	client := &http.Client{}
	
	req, err := http.NewRequest("GET", baseURL+"/me", nil)
	if err != nil {
		log.Fatal("Error creating request:", err)
	}
	
	// Add JWT token to Authorization header
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")
	
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Error making request:", err)
	}
	defer resp.Body.Close()
	
	fmt.Printf("Status Code: %d\n", resp.StatusCode)
	
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatal("Error decoding response:", err)
	}
	
	// Pretty print the response
	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal("Error formatting JSON:", err)
	}
	
	fmt.Println("Response:")
	fmt.Println(string(prettyJSON))
	
	// Verify that expected fields are present
	if user, ok := result["user"].(map[string]interface{}); ok {
		expectedFields := []string{"id", "email", "firstName", "lastName", "role", "isActive", "emailVerified", "createdAt", "updatedAt"}
		
		fmt.Println("\nVerifying required fields:")
		for _, field := range expectedFields {
			if _, exists := user[field]; exists {
				fmt.Printf("✓ %s: present\n", field)
			} else {
				fmt.Printf("✗ %s: missing\n", field)
			}
		}
		
		// Check if profile is present and has appropriate fields based on role
		if profile, hasProfile := user["profile"].(map[string]interface{}); hasProfile {
			fmt.Println("\nProfile fields:")
			for key, value := range profile {
				fmt.Printf("  %s: %v\n", key, value)
			}
		} else {
			fmt.Println("\nNo profile data found")
		}
	} else {
		fmt.Println("User data not found in response")
	}
}