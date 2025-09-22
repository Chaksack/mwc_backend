package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	// Test the blog categories endpoint
	baseURL := "http://localhost:8080"
	if len(os.Args) > 1 {
		baseURL = os.Args[1]
	}

	fmt.Printf("Testing blog categories endpoint at: %s\n", baseURL)
	
	// Test public blog categories endpoint
	fmt.Println("\n1. Testing GET /api/v1/blog/categories")
	resp, err := http.Get(baseURL + "/api/v1/blog/categories")
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
	} else {
		fmt.Printf("✅ Success response received\n")
	}
}