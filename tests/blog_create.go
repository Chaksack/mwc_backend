//go:build blogcreate
// +build blogcreate

package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "time"

    "mwc_backend/internal/api/middleware"
    "mwc_backend/internal/models"
)

// This test utility exercises the Blog creation API to verify that:
// 1) Slug is auto-generated from the title when not provided.
// 2) Media embedded in the content field (e.g., data-URI images) is accepted and preserved.
//
// How to run (ensure the server is running locally on :8080):
//   go run tests/blog_create.go

func main() {
    baseURL := getenv("BASE_URL", "http://localhost:8080/api/v1")

    // Generate an admin JWT compatible with our middleware (token only, no Bearer prefix)
    jwtSecret := getenv("JWT_SECRET", "test_secret_key")
    token, err := middleware.GenerateJWT(1, "admin@test.com", models.AdminRole, jwtSecret, 24*time.Hour)
    if err != nil {
        log.Fatalf("Failed to generate JWT token: %v", err)
    }

    // Prepare request payload WITHOUT slug and with embedded media in content
    contentWithImage := `<p>Hello world</p><img alt="red" src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHyAKm1GQ+QgAAAABJRU5ErkJggg==" />`
    payload := map[string]interface{}{
        "title":          fmt.Sprintf("Test Blog %d", time.Now().UnixNano()),
        "content":        contentWithImage,
        "summary":        "A short summary",
        "featured_image": "",
        "tags":           []string{"test", "blog"},
        "is_published":   true,
        "is_featured":    false,
    }

    body, _ := json.Marshal(payload)
    req, err := http.NewRequest("POST", baseURL+"/admin/blogs", bytes.NewReader(body))
    if err != nil {
        log.Fatalf("Failed to create request: %v", err)
    }
    req.Header.Set("Content-Type", "application/json")
    // IMPORTANT: Our auth middleware expects the token directly (no 'Bearer ' prefix)
    req.Header.Set("Authorization", token)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        log.Fatalf("Request failed: %v", err)
    }
    defer resp.Body.Close()

    respBody, _ := io.ReadAll(resp.Body)
    fmt.Println("Status:", resp.Status)
    fmt.Println("Response:", string(respBody))

    if resp.StatusCode != http.StatusCreated {
        log.Fatalf("Expected status 201 Created, got %d", resp.StatusCode)
    }

    // Basic assertions on the response
    var out struct {
        ID            uint      `json:"id"`
        Title         string    `json:"title"`
        Slug          string    `json:"slug"`
        Content       string    `json:"content"`
        Summary       string    `json:"summary"`
        FeaturedImage string    `json:"featured_image"`
        Tags          []string  `json:"tags"`
        AuthorID      uint      `json:"author_id"`
        AuthorName    string    `json:"author_name"`
        IsPublished   bool      `json:"is_published"`
        PublishedAt   *time.Time `json:"published_at"`
        ViewCount     int       `json:"view_count"`
        IsFeatured    bool      `json:"is_featured"`
        CreatedAt     time.Time `json:"created_at"`
        UpdatedAt     time.Time `json:"updated_at"`
    }
    if err := json.Unmarshal(respBody, &out); err != nil {
        log.Fatalf("Failed to decode response JSON: %v", err)
    }

    if out.Slug == "" {
        log.Fatalf("Expected auto-generated slug, got empty")
    }
    if out.Content != contentWithImage {
        log.Fatalf("Expected content with embedded media to be preserved")
    }

    fmt.Println("✓ Blog created successfully with auto-generated slug and embedded media preserved")
}

func getenv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}
