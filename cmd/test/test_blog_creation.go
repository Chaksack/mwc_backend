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
	// Test the blog post creation endpoint
	baseURL := "http://localhost:8080"
	if len(os.Args) > 1 {
		baseURL = os.Args[1]
	}

	fmt.Printf("Testing blog post creation endpoint at: %s\n", baseURL)

	// First, we need to login to get an admin token
	fmt.Println("\n1. Testing admin login")
	
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

	// Test blog post creation - case 1: with tags (should work)
	fmt.Println("\n2. Testing POST /api/v1/admin/blog (with tags)")
	
	blogPostData1 := map[string]interface{}{
		"title":        "Test Blog Post with Tags",
		"content":      "This is a test blog post content with tags.",
		"excerpt":      "Test excerpt",
		"category":     "Technology",
		"tags":         []string{"test", "blog", "technology"},
		"is_published": true,
		"is_featured":  false,
	}
	
	testBlogCreation(baseURL, token, blogPostData1, "with tags")

	// Test blog post creation - case 2: without tags (likely to cause 500 error)
	fmt.Println("\n3. Testing POST /api/v1/admin/blog (without tags)")
	
	blogPostData2 := map[string]interface{}{
		"title":        "Test Blog Post without Tags",
		"content":      "This is a test blog post content without tags.",
		"excerpt":      "Test excerpt without tags",
		"category":     "Education",
		"is_published": true,
		"is_featured":  false,
		// Note: no "tags" field provided
	}
	
	testBlogCreation(baseURL, token, blogPostData2, "without tags")

	// Test blog post creation - case 3: with empty tags array
	fmt.Println("\n4. Testing POST /api/v1/admin/blog (with empty tags array)")
	
	blogPostData3 := map[string]interface{}{
		"title":        "Test Blog Post with Empty Tags",
		"content":      "This is a test blog post content with empty tags array.",
		"excerpt":      "Test excerpt with empty tags",
		"category":     "General",
		"tags":         []string{},
		"is_published": true,
		"is_featured":  false,
	}
	
	testBlogCreation(baseURL, token, blogPostData3, "with empty tags array")

	// Test blog post creation - case 4: with null tags
	fmt.Println("\n5. Testing POST /api/v1/admin/blog (with null tags)")
	
	blogPostData4 := map[string]interface{}{
		"title":        "Test Blog Post with Null Tags",
		"content":      "This is a test blog post content with null tags.",
		"excerpt":      "Test excerpt with null tags",
		"category":     "General",
		"tags":         nil,
		"is_published": true,
		"is_featured":  false,
	}
	
	testBlogCreation(baseURL, token, blogPostData4, "with null tags")
}

func testBlogCreation(baseURL, token string, blogData map[string]interface{}, testCase string) {
	blogJSON, _ := json.Marshal(blogData)
	
	req, err := http.NewRequest("POST", baseURL+"/api/v1/admin/blog", bytes.NewBuffer(blogJSON))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	fmt.Printf("Test case (%s):\n", testCase)
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(body))

	// Check for the 500 error
	if resp.StatusCode == 500 {
		fmt.Printf("❌ 500 Internal Server Error found - this confirms the issue\n")
	} else if resp.StatusCode == 201 {
		fmt.Printf("✅ Success - blog post created successfully\n")
	} else {
		fmt.Printf("⚠️  Unexpected status code\n")
	}
	fmt.Println()
}