package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  struct {
		ID    uint   `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"user"`
}

type CreateCategoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func testUserLogin(baseURL, email, password string) (*LoginResponse, error) {
	loginReq := LoginRequest{
		Email:    email,
		Password: password,
	}
	
	loginData, _ := json.Marshal(loginReq)
	resp, err := http.Post(baseURL+"/login", "application/json", bytes.NewBuffer(loginData))
	if err != nil {
		return nil, fmt.Errorf("failed to login: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed with status: %d", resp.StatusCode)
	}
	
	var loginResp LoginResponse
	json.NewDecoder(resp.Body).Decode(&loginResp)
	
	return &loginResp, nil
}

func testCreateCategory(baseURL, token, userRole string) error {
	categoryReq := CreateCategoryRequest{
		Name:        fmt.Sprintf("Test Category from %s - %d", userRole, time.Now().Unix()),
		Description: fmt.Sprintf("Testing %s permissions for blog category creation", userRole),
	}
	
	categoryData, _ := json.Marshal(categoryReq)
	req, _ := http.NewRequest("POST", baseURL+"/admin/blog/categories", bytes.NewBuffer(categoryData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create category: %v", err)
	}
	defer resp.Body.Close()
	
	var responseBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&responseBody)
	
	fmt.Printf("Create category response for %s (status: %d): %v\n", userRole, resp.StatusCode, responseBody)
	
	if resp.StatusCode == 403 {
		return fmt.Errorf("got 403 Forbidden for %s - fix not working", userRole)
	} else if resp.StatusCode != 201 {
		return fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, userRole)
	}
	
	return nil
}

func main() {
	baseURL := "http://localhost:8080/api/v1"
	
	fmt.Println("Testing blog category creation fix...")
	fmt.Println("=========================================")
	
	// Test cases: admin and superadmin users
	testCases := []struct {
		email    string
		password string
		role     string
	}{
		{"admin@example.com", "password123", "admin"},
		{"superadmin@test.com", "password123", "superadmin"},
	}
	
	allPassed := true
	
	for _, testCase := range testCases {
		fmt.Printf("\n--- Testing %s user ---\n", testCase.role)
		
		// Login
		loginResp, err := testUserLogin(baseURL, testCase.email, testCase.password)
		if err != nil {
			fmt.Printf("❌ %s login failed: %v\n", testCase.role, err)
			allPassed = false
			continue
		}
		
		fmt.Printf("✅ Successfully logged in as: %s (Role: %s)\n", loginResp.User.Email, loginResp.User.Role)
		
		// Test category creation
		err = testCreateCategory(baseURL, loginResp.Token, testCase.role)
		if err != nil {
			fmt.Printf("❌ Category creation failed for %s: %v\n", testCase.role, err)
			allPassed = false
		} else {
			fmt.Printf("✅ Successfully created category as %s\n", testCase.role)
		}
	}
	
	fmt.Println("\n=========================================")
	if allPassed {
		fmt.Println("🎉 All tests passed! The fix is working correctly.")
	} else {
		fmt.Println("❌ Some tests failed. The issue may not be fully resolved.")
	}
}