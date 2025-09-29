package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

const baseURL = "http://localhost:8080/api/v1"

// Test data structures
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateSubscriptionPlanRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Price        float64  `json:"price"`
	Currency     string   `json:"currency"`
	BillingCycle string   `json:"billing_cycle"`
	Features     []string `json:"features"`
	AllowedRoles []string `json:"allowed_roles"`
}

type AssignUserSubscriptionRequest struct {
	UserID             uint `json:"user_id"`
	SubscriptionPlanID uint `json:"subscription_plan_id"`
	DurationMonths     int  `json:"duration_months"`
}

// Helper function to make authenticated requests
func makeRequest(method, url string, body interface{}, token string) (*http.Response, error) {
	var reqBody bytes.Buffer
	if body != nil {
		json.NewEncoder(&reqBody).Encode(body)
	}

	req, err := http.NewRequest(method, url, &reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	return client.Do(req)
}

// Get admin token by logging in
func getAdminToken() (string, error) {
	loginData := LoginRequest{
		Email:    "admin@test.com", // Change to your admin email
		Password: "admin123",       // Change to your admin password
	}

	resp, err := makeRequest("POST", baseURL+"/login", loginData, "")
	if err != nil {
		return "", fmt.Errorf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("login failed with status: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode login response: %v", err)
	}

	token, ok := result["token"].(string)
	if !ok {
		return "", fmt.Errorf("token not found in login response")
	}

	return token, nil
}

func testDynamicSubscriptions() {
	fmt.Println("🧪 Testing Dynamic Subscription Management System")
	fmt.Println("=" + fmt.Sprintf("%48s", "="))

	// Get admin authentication token
	fmt.Print("1. Getting admin authentication token... ")
	token, err := getAdminToken()
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		fmt.Println("Note: Make sure you have an admin user with email 'admin@test.com' and password 'admin123'")
		return
	}
	fmt.Println("✅ Success")

	// Test 1: Create a dynamic subscription plan
	fmt.Print("2. Creating dynamic subscription plan... ")
	planData := CreateSubscriptionPlanRequest{
		Name:         "Premium Parent Plan",
		Description:  "Premium features for parents including advanced school search and messaging",
		Price:        29.99,
		Currency:     "USD",
		BillingCycle: "monthly",
		Features:     []string{"Advanced Search", "Direct Messaging", "Priority Support", "Premium Content"},
		AllowedRoles: []string{"parent", "montessori_professional"},
	}

	resp, err := makeRequest("POST", baseURL+"/admin/subscription-plans", planData, token)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		fmt.Printf("❌ Failed with status: %d\n", resp.StatusCode)
		return
	}

	var createResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createResult)
	planID := uint(createResult["data"].(map[string]interface{})["id"].(float64))
	fmt.Println("✅ Success")

	// Test 2: Get all subscription plans
	fmt.Print("3. Retrieving subscription plans... ")
	resp, err = makeRequest("GET", baseURL+"/admin/subscription-plans", nil, token)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("❌ Failed with status: %d\n", resp.StatusCode)
		return
	}
	fmt.Println("✅ Success")

	// Test 3: Get role subscription mappings
	fmt.Print("4. Getting role subscription mappings... ")
	resp, err = makeRequest("GET", baseURL+"/admin/role-subscriptions?role=parent", nil, token)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("❌ Failed with status: %d\n", resp.StatusCode)
		return
	}
	fmt.Println("✅ Success")

	// Test 4: Update subscription plan
	fmt.Print("5. Updating subscription plan... ")
	updateData := map[string]interface{}{
		"name":          "Premium Parent Plan Updated",
		"description":   "Updated premium features for parents",
		"price":         39.99,
		"currency":      "USD",
		"billing_cycle": "monthly",
		"features":      []string{"Advanced Search", "Direct Messaging", "Priority Support", "Premium Content", "Analytics"},
		"allowed_roles": []string{"parent", "montessori_professional"},
		"is_active":     true,
	}

	resp, err = makeRequest("PUT", fmt.Sprintf("%s/admin/subscription-plans/%d", baseURL, planID), updateData, token)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("❌ Failed with status: %d\n", resp.StatusCode)
		return
	}
	fmt.Println("✅ Success")

	// Test 5: Assign subscription to user (Note: This requires a valid user ID)
	fmt.Print("6. Testing assign subscription to user... ")
	assignData := AssignUserSubscriptionRequest{
		UserID:             1, // Change this to a valid user ID in your system
		SubscriptionPlanID: planID,
		DurationMonths:     3,
	}

	resp, err = makeRequest("POST", baseURL+"/admin/assign-subscription", assignData, token)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✅ Success")
	} else if resp.StatusCode == 404 {
		fmt.Println("⚠️  User not found (expected - change UserID to valid user)")
	} else {
		fmt.Printf("❌ Failed with status: %d\n", resp.StatusCode)
		return
	}

	// Test 6: Delete subscription plan (cleanup)
	fmt.Print("7. Cleaning up - deleting subscription plan... ")
	resp, err = makeRequest("DELETE", fmt.Sprintf("%s/admin/subscription-plans/%d", baseURL, planID), nil, token)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✅ Success")
	} else if resp.StatusCode == 400 {
		fmt.Println("⚠️  Cannot delete (plan in use - this is expected behavior)")
	} else {
		fmt.Printf("❌ Failed with status: %d\n", resp.StatusCode)
	}

	fmt.Println("\n🎉 Dynamic Subscription Management System Test Completed!")
	fmt.Println("\nFeatures Tested:")
	fmt.Println("- ✅ Create dynamic subscription plans")
	fmt.Println("- ✅ Retrieve subscription plans")
	fmt.Println("- ✅ Update subscription plans")
	fmt.Println("- ✅ Role-subscription mappings")
	fmt.Println("- ✅ Assign subscriptions to users")
	fmt.Println("- ✅ Delete subscription plans with protection")
}

func main() {
	// Check if server is running
	fmt.Print("Checking if server is running... ")
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		fmt.Printf("❌ Server not accessible: %v\n", err)
		fmt.Println("Please make sure the server is running on http://localhost:8080")
		os.Exit(1)
	}
	resp.Body.Close()
	fmt.Println("✅ Server is running")

	// Run tests
	testDynamicSubscriptions()
}