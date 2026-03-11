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

const careersBase = "http://localhost:8080/api/v1/careers"
const apiBase = "http://localhost:8080/api/v1"

type loginReq struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

func do(method, url string, body any, token string) (*http.Response, error) {
    var buf bytes.Buffer
    if body != nil {
        _ = json.NewEncoder(&buf).Encode(body)
    }
    req, err := http.NewRequest(method, url, &buf)
    if err != nil { return nil, err }
    req.Header.Set("Content-Type", "application/json")
    if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
    return http.DefaultClient.Do(req)
}

func readBody(resp *http.Response) string {
    b, _ := io.ReadAll(resp.Body)
    _ = resp.Body.Close()
    return string(b)
}

func main() {
    // Optional: login as institution/training-center to create + publish a job
    instToken := os.Getenv("INSTITUTION_TOKEN")
    instEmail := os.Getenv("INSTITUTION_EMAIL")
    instPass := os.Getenv("INSTITUTION_PASSWORD")

    if instToken == "" && instEmail != "" && instPass != "" {
        fmt.Println("Logging in as institution to obtain token...")
        resp, err := do("POST", apiBase+"/login", loginReq{Email: instEmail, Password: instPass}, "")
        if err != nil { log.Fatalf("login failed: %v", err) }
        if resp.StatusCode != 200 { log.Fatalf("login status %d: %s", resp.StatusCode, readBody(resp)) }
        var out map[string]any
        _ = json.NewDecoder(resp.Body).Decode(&out)
        _ = resp.Body.Close()
        if t, ok := out["token"].(string); ok { instToken = t }
    }

    var jobID any
    // If we have a token, create and publish a job
    if instToken != "" {
        fmt.Println("Creating a draft job as institution...")
        job := map[string]any{
            "title": "Test Teacher",
            "description": "Automated smoke test job",
            "location": "Remote",
            "employment_type": "Full-time",
        }
        resp, err := do("POST", apiBase+"/institution/jobs", job, instToken)
        if err != nil { log.Fatalf("create job error: %v", err) }
        if resp.StatusCode != 201 { log.Fatalf("create job status %d: %s", resp.StatusCode, readBody(resp)) }
        var created map[string]any
        _ = json.NewDecoder(resp.Body).Decode(&created)
        _ = resp.Body.Close()
        jobID = created["id"]

        fmt.Println("Publishing the job...")
        url := fmt.Sprintf(apiBase+"/institution/jobs/%v/publish", jobID)
        resp2, err := do("PATCH", url, map[string]any{"is_published": true}, instToken)
        if err != nil { log.Fatalf("publish error: %v", err) }
        if resp2.StatusCode != 200 { log.Fatalf("publish status %d: %s", resp2.StatusCode, readBody(resp2)) }
        _ = resp2.Body.Close()
    } else {
        fmt.Println("Skipping job creation/publish (no INSTITUTION_TOKEN/LOGIN). Using existing published jobs if any.")
    }

    // List public careers
    fmt.Println("Fetching public careers list...")
    resp, err := do("GET", careersBase+"/jobs", nil, "")
    if err != nil { log.Fatalf("list careers error: %v", err) }
    body := readBody(resp)
    fmt.Printf("/careers/jobs => %d\n%s\n", resp.StatusCode, body)

    // Determine job id to apply to
    var applyJobID any = jobID
    if applyJobID == nil {
        // Try to parse from list response (very loosely)
        var listOut map[string]any
        _ = json.Unmarshal([]byte(body), &listOut)
        if data, ok := listOut["data"].([]any); ok && len(data) > 0 {
            if first, ok2 := data[0].(map[string]any); ok2 { applyJobID = first["id"] }
        }
    }
    if applyJobID == nil {
        log.Fatalf("No job id found to apply to. Ensure at least one published job exists.")
    }

    // Public apply
    fmt.Println("Submitting a public application...")
    apply := map[string]any{
        "applicant_name":  "Smoke Tester",
        "applicant_email": "smoke@test.local",
        "applicant_phone": "+10000000000",
        "resume_url":      "https://example.com/resume.pdf",
        "cover_letter":    "Hello, this is a test.",
        "consent":         true,
        "source":          "smoke_test",
    }
    applyURL := fmt.Sprintf(careersBase+"/jobs/%v/apply", applyJobID)
    resp2, err := do("POST", applyURL, apply, "")
    if err != nil { log.Fatalf("apply error: %v", err) }
    fmt.Printf("apply => %d\n%s\n", resp2.StatusCode, readBody(resp2))

    fmt.Println("Smoke test complete.")

    // Optional: Admin list check to minimally verify admin endpoints
    adminToken := os.Getenv("ADMIN_TOKEN")
    if adminToken != "" {
        fmt.Println("Verifying admin recruiting jobs list...")
        resp, err := do("GET", apiBase+"/admin/recruiting/jobs?page=1&page_size=5", nil, adminToken)
        if err != nil { log.Fatalf("admin list error: %v", err) }
        fmt.Printf("/admin/recruiting/jobs => %d\n%s\n", resp.StatusCode, readBody(resp))
    }
}
