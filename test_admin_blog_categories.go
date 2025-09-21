package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	// Test the admin blog categories endpoint
	baseURL := "http://localhost:8080"
	if len(os.Args) > 1 {
		baseURL = os.Args[1]
	}

	fmt.Printf("Testing admin blog categories endpoint at: %s\n", baseURL)

	// First, we need to login to get an admin token
	// For testing purposes, we'll assume there's an admin user
	fmt.Println("\n1. Testing admin login (you'll need admin credentials)")
	
	loginData := map[string]string{
		"email":    "admin@example.com", // Replace with actual admin email
		"password": "password123",       // Replace with actual admin password
	}
	
	loginJSON, _ := json.Marshal(loginData)
	loginResp, err := http.Post(baseURL+"/api/v1/login", "application/json", bytes.NewBuffer(loginJSON))
	if err != nil {
		fmt.Printf("Error logging in: %v\n", err)
		fmt.Println("Note: Make sure server is running and admin credentials are correct")
		return
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != 200 {
		fmt.Printf("Login failed with status: %d\n", loginResp.StatusCode)
		body, _ := io.ReadAll(loginResp.Body)
		fmt.Printf("Response: %s\n", string(body))
		fmt.Println("Note: You need valid admin credentials to test this endpoint")
		return
	}

	// Extract token from login response
	var loginResult map[string]interface{}
	body, _ := io.ReadAll(loginResp.Body)
	json.Unmarshal(body, &loginResult)
	
	token, ok := loginResult["token"].(string)
	if !ok {
		fmt.Println("Failed to extract token from login response")
		return
	}

	fmt.Printf("✅ Successfully logged in as admin\n")

	// Test admin blog categories endpoint
	fmt.Println("\n2. Testing GET /api/v1/admin/blog/categories")
	
	req, err := http.NewRequest("GET", baseURL+"/api/v1/admin/blog/categories", nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	
	req.Header.Set("Authorization", "Bearer "+token)
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(body))

	// Parse JSON to see structure
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err == nil {
		fmt.Printf("Parsed JSON: %+v\n", result)
	}

	// Test if we can identify the specific error mentioned
	if resp.StatusCode != 200 {
		fmt.Printf("❌ Error response received\n")
		if responseStr := string(body); responseStr == `{"error":"Slug is required"}` {
			fmt.Printf("🔍 Found the exact 'Slug is required' error - routing issue confirmed\n")
		}
	} else {
		fmt.Printf("✅ Success response received - routing issue should be fixed\n")
	}
}