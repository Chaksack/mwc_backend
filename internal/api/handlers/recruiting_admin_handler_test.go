package handlers

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"

    "mwc_backend/config"
    "mwc_backend/internal/models"
)

// setupTestDB initializes an in-memory sqlite DB and automigrates needed models
func setupTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    if err != nil {
        t.Fatalf("failed to open sqlite: %v", err)
    }
    if err := db.AutoMigrate(&models.InstitutionProfile{}, &models.Job{}, &models.JobApplication{}, &models.ApplicationEvent{}); err != nil {
        t.Fatalf("automigrate failed: %v", err)
    }
    return db
}

func TestGenerateSlugAndEnsureUnique(t *testing.T) {
    db := setupTestDB(t)

    base := generateSlug("Test Title!! ")
    if base != "test-title" {
        t.Fatalf("unexpected slug base: %s", base)
    }

    // No collision
    s, err := ensureUniqueJobSlug(db, base, 0)
    if err != nil {
        t.Fatalf("ensureUniqueJobSlug error: %v", err)
    }
    if s != base {
        t.Fatalf("expected same slug when unique, got %s", s)
    }

    // Create a job with that slug to force collision
    j := models.Job{Title: "X", InstitutionProfileID: 1, Slug: base, IsActive: true}
    if err := db.Create(&j).Error; err != nil {
        t.Fatalf("create job error: %v", err)
    }

    s2, err := ensureUniqueJobSlug(db, base, 0)
    if err != nil {
        t.Fatalf("ensureUniqueJobSlug error: %v", err)
    }
    if s2 == base {
        t.Fatalf("expected a different slug due to collision, got %s", s2)
    }
}

func TestUpdateApplicationStatusCreatesEvent(t *testing.T) {
    db := setupTestDB(t)
    cfg := &config.Config{}
    h := NewRecruitingAdminHandler(db, cfg, nil)

    // Seed job and application
    job := models.Job{Title: "Role", InstitutionProfileID: 1, IsActive: true}
    if err := db.Create(&job).Error; err != nil { t.Fatalf("job create: %v", err) }
    app := models.JobApplication{JobID: job.ID, ApplicantEmail: "test@example.com", Status: "pending"}
    if err := db.Create(&app).Error; err != nil { t.Fatalf("app create: %v", err) }

    // Build fiber app and route
    f := fiber.New()
    f.Patch("/admin/recruiting/applications/:id/status", h.UpdateApplicationStatus)

    payload := map[string]any{"status": "shortlisted", "notes": "moved forward", "notify": false}
    var buf bytes.Buffer
    _ = json.NewEncoder(&buf).Encode(payload)
    req := httptest.NewRequest(http.MethodPatch, "/admin/recruiting/applications/"+uintToStr(app.ID)+"/status", &buf)
    req.Header.Set("Content-Type", "application/json")
    resp, err := f.Test(req)
    if err != nil { t.Fatalf("fiber test error: %v", err) }
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }

    // Verify status updated
    var got models.JobApplication
    if err := db.First(&got, app.ID).Error; err != nil { t.Fatalf("reload app: %v", err) }
    if got.Status != "shortlisted" {
        t.Fatalf("status not updated: %s", got.Status)
    }

    // Verify an event exists
    var cnt int64
    if err := db.Model(&models.ApplicationEvent{}).Where("application_id = ? AND action = ?", app.ID, "status_changed").Count(&cnt).Error; err != nil {
        t.Fatalf("count events: %v", err)
    }
    if cnt == 0 {
        t.Fatalf("expected an ApplicationEvent to be created")
    }
}

func uintToStr(v uint) string {
    // simple uint to string without importing strconv for brevity
    return fmtUint(uint64(v))
}

func fmtUint(u uint64) string {
    if u == 0 { return "0" }
    var b [20]byte
    i := len(b)
    for u > 0 {
        i--
        b[i] = byte('0' + u%10)
        u /= 10
    }
    return string(b[i:])
}
