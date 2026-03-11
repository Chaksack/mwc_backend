package handlers

import (
    "bytes"
    "encoding/csv"
    "fmt"
    "log"
    "regexp"
    "strconv"
    "strings"
    "time"

    "mwc_backend/config"
    "mwc_backend/internal/email"
    "mwc_backend/internal/models"

    "github.com/gofiber/fiber/v2"
    "gorm.io/gorm"
)

// RecruitingAdminHandler handles admin/HR recruiting endpoints
type RecruitingAdminHandler struct {
	db       *gorm.DB
	cfg      *config.Config
	emailSvc email.EmailService
}

func NewRecruitingAdminHandler(db *gorm.DB, cfg *config.Config, emailSvc email.EmailService) *RecruitingAdminHandler {
	return &RecruitingAdminHandler{db: db, cfg: cfg, emailSvc: emailSvc}
}

// --- Jobs management (stubs) ---

// CreateJob creates a new job (admin can create on behalf of an institution or training center)
// @Summary Create a recruiting job (admin)
// @Tags authenticated, recruiting
// @Accept json
// @Produce json
// @Success 201 {object} models.Job "Job created"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 404 {object} map[string]string "Institution/training center profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/recruiting/jobs [post]
func (h *RecruitingAdminHandler) CreateJob(c *fiber.Ctx) error {
	// Separate admin request allows specifying InstitutionProfileID
	type AdminJobRequest struct {
		InstitutionProfileID uint   `json:"institution_profile_id"`
		Title                string `json:"title"`
		Description          string `json:"description"`
		Location             string `json:"location"`
		EmploymentType       string `json:"employment_type"`
		SalaryRange          string `json:"salary_range"`
		ExpiresAt            string `json:"expires_at"` // RFC3339 optional
		Department           string `json:"department"`
		Slug                 string `json:"slug"`
		Publish              *bool  `json:"publish"` // if true, set IsPublished and PublishedAt
	}
	var req AdminJobRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	if req.InstitutionProfileID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "institution_profile_id is required"})
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Description) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "title and description are required"})
	}

	// Verify institution/training center profile exists
	var inst models.InstitutionProfile
	if err := h.db.First(&inst, req.InstitutionProfileID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution/training center profile not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}

	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		if t, err := time.Parse(time.RFC3339, req.ExpiresAt); err == nil {
			expiresAt = &t
		} else {
			log.Printf("admin CreateJob: invalid expires_at '%s': %v", req.ExpiresAt, err)
		}
	}

 now := time.Now()
 job := models.Job{
     InstitutionProfileID: inst.ID,
     Title:                req.Title,
     Description:          req.Description,
     Location:             req.Location,
     EmploymentType:       req.EmploymentType,
     SalaryRange:          req.SalaryRange,
     Department:           req.Department,
     Slug:                 strings.TrimSpace(req.Slug),
     IsActive:             true,
     ExpiresAt:            expiresAt,
 }
 // Ensure slug exists and is unique
 if strings.TrimSpace(job.Slug) == "" {
     job.Slug = generateSlug(job.Title)
 }
 uSlug, err := ensureUniqueJobSlug(h.db, job.Slug, 0)
 if err != nil { return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()}) }
 job.Slug = uSlug
	if req.Publish != nil && *req.Publish {
		job.IsPublished = true
		job.PublishedAt = &now
	}

	if err := h.db.Create(&job).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create job: " + err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(job)
}

// UpdateJob updates a job
// @Summary Update a recruiting job (admin)
// @Tags authenticated, recruiting
// @Accept json
// @Produce json
// @Param id path int true "Job ID"
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/jobs/{id} [put]
func (h *RecruitingAdminHandler) UpdateJob(c *fiber.Ctx) error {
    idStr := c.Params("id")
    if strings.TrimSpace(idStr) == "" { return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id is required"}) }
    var job models.Job
    if err := h.db.First(&job, idStr).Error; err != nil {
        if err == gorm.ErrRecordNotFound { return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found"}) }
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: "+err.Error()})
    }
    var req struct {
        Title          *string `json:"title"`
        Description    *string `json:"description"`
        Location       *string `json:"location"`
        EmploymentType *string `json:"employment_type"`
        SalaryRange    *string `json:"salary_range"`
        Department     *string `json:"department"`
        ExpiresAt      *string `json:"expires_at"` // RFC3339
        Slug           *string `json:"slug"`
        IsActive       *bool   `json:"is_active"`
    }
    if err := c.BodyParser(&req); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: "+err.Error()})
    }
    if req.Title != nil { job.Title = strings.TrimSpace(*req.Title) }
    if req.Description != nil { job.Description = *req.Description }
    if req.Location != nil { job.Location = *req.Location }
    if req.EmploymentType != nil { job.EmploymentType = *req.EmploymentType }
    if req.SalaryRange != nil { job.SalaryRange = *req.SalaryRange }
    if req.Department != nil { job.Department = *req.Department }
    if req.IsActive != nil { job.IsActive = *req.IsActive }
    if req.ExpiresAt != nil {
        if strings.TrimSpace(*req.ExpiresAt) == "" {
            job.ExpiresAt = nil
        } else if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
            job.ExpiresAt = &t
        } else {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid expires_at; must be RFC3339"})
        }
    }
    if req.Slug != nil {
        desired := strings.TrimSpace(*req.Slug)
        if desired == "" { desired = generateSlug(job.Title) }
        if uSlug, err := ensureUniqueJobSlug(h.db, desired, job.ID); err == nil {
            job.Slug = uSlug
        } else {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
        }
    }
    if err := h.db.Save(&job).Error; err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update job: "+err.Error()})
    }
    return c.JSON(job)
}

// PublishJob publishes/unpublishes a job
// @Summary Publish/unpublish a recruiting job (admin)
// @Tags authenticated, recruiting
// @Produce json
// @Param id path int true "Job ID"
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/jobs/{id}/publish [patch]
func (h *RecruitingAdminHandler) PublishJob(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "job id is required"})
	}
	var req struct {
		IsPublished *bool `json:"is_published"`
	}
	if err := c.BodyParser(&req); err != nil {
		// Allow empty body to toggle publish on
	}
	var job models.Job
	if err := h.db.First(&job, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}
	now := time.Now()
	// Decide new publish state
	newState := true
	if req.IsPublished != nil {
		newState = *req.IsPublished
	}
	job.IsPublished = newState
	if newState {
		job.PublishedAt = &now
	} else {
		job.PublishedAt = nil
	}
	if err := h.db.Save(&job).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update job: " + err.Error()})
	}
	return c.JSON(fiber.Map{"id": job.ID, "is_published": job.IsPublished, "published_at": job.PublishedAt})
}

// ListJobs lists recruiting jobs
// @Summary List recruiting jobs (admin)
// @Tags authenticated, recruiting
// @Produce json
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/jobs [get]
func (h *RecruitingAdminHandler) ListJobs(c *fiber.Ctx) error {
    // Filters
    page := c.QueryInt("page", 1); if page < 1 { page = 1 }
    pageSize := c.QueryInt("page_size", 20); if pageSize < 1 { pageSize = 20 }; if pageSize > 100 { pageSize = 100 }
    q := strings.TrimSpace(c.Query("q"))
    location := strings.TrimSpace(c.Query("location"))
    empType := strings.TrimSpace(c.Query("employment_type"))
    isPublished := c.Query("is_published")
    isActive := c.Query("is_active")
    instID := c.Query("institution_profile_id")

    dbq := h.db.Model(&models.Job{})
    if q != "" { like := "%"+strings.ToLower(q)+"%"; dbq = dbq.Where("LOWER(title) LIKE ? OR LOWER(description) LIKE ?", like, like) }
    if location != "" { like := "%"+strings.ToLower(location)+"%"; dbq = dbq.Where("LOWER(location) LIKE ?", like) }
    if empType != "" { dbq = dbq.Where("LOWER(employment_type) = ?", strings.ToLower(empType)) }
    if isPublished != "" { if isPublished == "true" || isPublished == "1" { dbq = dbq.Where("is_published = ?", true) } else { dbq = dbq.Where("is_published = ?", false) } }
    if isActive != "" { if isActive == "true" || isActive == "1" { dbq = dbq.Where("is_active = ?", true) } else { dbq = dbq.Where("is_active = ?", false) } }
    if instID != "" { dbq = dbq.Where("institution_profile_id = ?", instID) }

    var total int64
    if err := dbq.Count(&total).Error; err != nil { return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "count error: "+err.Error()}) }
    var jobs []models.Job
    if err := dbq.Order("published_at DESC NULLS LAST, posted_at DESC").Offset((page-1)*pageSize).Limit(pageSize).Find(&jobs).Error; err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "query error: "+err.Error()})
    }
    return c.JSON(fiber.Map{"data": jobs, "pagination": fiber.Map{"page": page, "page_size": pageSize, "total": total, "total_pages": (total+int64(pageSize)-1)/int64(pageSize)}})
}

// --- Applications management (stubs) ---

// ListApplications lists applications
// @Summary List applications (admin)
// @Tags authenticated, recruiting
// @Produce json
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/applications [get]
func (h *RecruitingAdminHandler) ListApplications(c *fiber.Ctx) error {
    page := c.QueryInt("page", 1); if page < 1 { page = 1 }
    pageSize := c.QueryInt("page_size", 20); if pageSize < 1 { pageSize = 20 }; if pageSize > 100 { pageSize = 100 }
    jobID := strings.TrimSpace(c.Query("job_id"))
    status := strings.TrimSpace(c.Query("status"))
    q := strings.TrimSpace(c.Query("q")) // name or email
    from := strings.TrimSpace(c.Query("from"))
    to := strings.TrimSpace(c.Query("to"))

    dbq := h.db.Model(&models.JobApplication{}).Preload("Job").Preload("Job.InstitutionProfile")
    if jobID != "" { dbq = dbq.Where("job_id = ?", jobID) }
    if status != "" { dbq = dbq.Where("LOWER(status) = ?", strings.ToLower(status)) }
    if q != "" { like := "%"+strings.ToLower(q)+"%"; dbq = dbq.Where("LOWER(applicant_name) LIKE ? OR LOWER(applicant_email) LIKE ?", like, like) }
    if from != "" { if t, err := time.Parse(time.RFC3339, from); err == nil { dbq = dbq.Where("applied_at >= ?", t) } }
    if to != "" { if t, err := time.Parse(time.RFC3339, to); err == nil { dbq = dbq.Where("applied_at <= ?", t) } }

    var total int64
    if err := dbq.Count(&total).Error; err != nil { return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "count error: "+err.Error()}) }
    var apps []models.JobApplication
    if err := dbq.Order("applied_at DESC").Offset((page-1)*pageSize).Limit(pageSize).Find(&apps).Error; err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "query error: "+err.Error()})
    }
    return c.JSON(fiber.Map{"data": apps, "pagination": fiber.Map{"page": page, "page_size": pageSize, "total": total, "total_pages": (total+int64(pageSize)-1)/int64(pageSize)}})
}

// GetApplication gets an application by ID
// @Summary Get application details (admin)
// @Tags authenticated, recruiting
// @Produce json
// @Param id path int true "Application ID"
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/applications/{id} [get]
func (h *RecruitingAdminHandler) GetApplication(c *fiber.Ctx) error {
    id := c.Params("id")
    var app models.JobApplication
    if err := h.db.Preload("Job").Preload("Job.InstitutionProfile").First(&app, id).Error; err != nil {
        if err == gorm.ErrRecordNotFound { return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Application not found"}) }
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: "+err.Error()})
    }
    var events []models.ApplicationEvent
    if err := h.db.Where("application_id = ?", app.ID).Order("created_at DESC").Find(&events).Error; err != nil {
        // non-fatal
        events = []models.ApplicationEvent{}
    }
    return c.JSON(fiber.Map{"application": app, "events": events})
}

// UpdateApplicationStatus changes application status
// @Summary Update application status (admin)
// @Tags authenticated, recruiting
// @Produce json
// @Param id path int true "Application ID"
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/applications/{id}/status [patch]
func (h *RecruitingAdminHandler) UpdateApplicationStatus(c *fiber.Ctx) error {
    id := c.Params("id")
    var app models.JobApplication
    if err := h.db.Preload("Job").First(&app, id).Error; err != nil {
        if err == gorm.ErrRecordNotFound { return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Application not found"}) }
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: "+err.Error()})
    }
    var req struct{
        Status string `json:"status"`
        Notes  string `json:"notes"`
        Notify bool   `json:"notify"`
        Subject string `json:"subject"`
        Body    string `json:"body"`
    }
    if err := c.BodyParser(&req); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: "+err.Error()})
    }
    if strings.TrimSpace(req.Status) == "" { return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "status is required"}) }

    app.Status = strings.ToLower(strings.TrimSpace(req.Status))
    if err := h.db.Save(&app).Error; err != nil { return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update status: "+err.Error()}) }

    _ = h.db.Create(&models.ApplicationEvent{ApplicationID: app.ID, Action: "status_changed", Notes: req.Notes}).Error

    // Optional notify applicant
    if req.Notify && app.ApplicantEmail != "" && h.emailSvc != nil {
        subj := req.Subject
        if strings.TrimSpace(subj) == "" { subj = fmt.Sprintf("Update on your application for %s", app.Job.Title) }
        body := req.Body
        if strings.TrimSpace(body) == "" { body = fmt.Sprintf("<p>Your application status is now: <strong>%s</strong>.</p>", app.Status) }
        _ = h.emailSvc.SendEmail(app.ApplicantEmail, subj, body)
        _ = h.db.Create(&models.ApplicationEvent{ApplicationID: app.ID, Action: "emailed", Notes: "status update notification sent"}).Error
    }
    return c.JSON(fiber.Map{"id": app.ID, "status": app.Status})
}

// SendApplicationEmail sends an email to the applicant
// @Summary Send email to applicant (admin)
// @Tags authenticated, recruiting
// @Accept json
// @Produce json
// @Param id path int true "Application ID"
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/applications/{id}/email [post]
func (h *RecruitingAdminHandler) SendApplicationEmail(c *fiber.Ctx) error {
    id := c.Params("id")
    var app models.JobApplication
    if err := h.db.Preload("Job").First(&app, id).Error; err != nil {
        if err == gorm.ErrRecordNotFound { return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Application not found"}) }
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: "+err.Error()})
    }
    var req struct{ Subject string `json:"subject"`; Body string `json:"body"` }
    if err := c.BodyParser(&req); err != nil { return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: "+err.Error()}) }
    if strings.TrimSpace(req.Subject) == "" || strings.TrimSpace(req.Body) == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "subject and body are required"})
    }
    if app.ApplicantEmail == "" || h.emailSvc == nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "applicant has no email or email service disabled"})
    }
    if err := h.emailSvc.SendEmail(app.ApplicantEmail, req.Subject, req.Body); err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send email: "+err.Error()})
    }
    _ = h.db.Create(&models.ApplicationEvent{ApplicationID: app.ID, Action: "emailed", Notes: req.Subject}).Error
    return c.JSON(fiber.Map{"sent": true})
}

// --- Reports & Export (stubs) ---

// GetOverviewReport returns overview metrics
// @Summary Recruiting overview report (admin)
// @Tags authenticated, recruiting
// @Produce json
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/reports/overview [get]
func (h *RecruitingAdminHandler) GetOverviewReport(c *fiber.Ctx) error {
    var totalJobs, publishedJobs, totalApps int64
    if err := h.db.Model(&models.Job{}).Count(&totalJobs).Error; err != nil { return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()}) }
    if err := h.db.Model(&models.Job{}).Where("is_published = ?", true).Count(&publishedJobs).Error; err != nil { return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()}) }
    if err := h.db.Model(&models.JobApplication{}).Count(&totalApps).Error; err != nil { return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()}) }

    // Applications by status
    var byStatus []struct{ Status string; Cnt int64 }
    _ = h.db.Model(&models.JobApplication{}).Select("status, COUNT(*) as cnt").Group("status").Scan(&byStatus).Error
    m := map[string]int64{}
    for _, r := range byStatus { m[r.Status] = r.Cnt }

    // Last 7 and 30 days
    since7 := time.Now().Add(-7*24*time.Hour)
    since30 := time.Now().Add(-30*24*time.Hour)
    var apps7, apps30 int64
    _ = h.db.Model(&models.JobApplication{}).Where("applied_at >= ?", since7).Count(&apps7).Error
    _ = h.db.Model(&models.JobApplication{}).Where("applied_at >= ?", since30).Count(&apps30).Error

    return c.JSON(fiber.Map{
        "total_jobs": totalJobs,
        "published_jobs": publishedJobs,
        "total_applications": totalApps,
        "applications_by_status": m,
        "applications_last_7d": apps7,
        "applications_last_30d": apps30,
    })
}

// ExportCSV exports applications as CSV
// @Summary Export applications CSV (admin)
// @Tags authenticated, recruiting
// @Produce text/csv
// @Success 501 {object} map[string]string "Not implemented"
// @Router /admin/recruiting/export.csv [get]
func (h *RecruitingAdminHandler) ExportCSV(c *fiber.Ctx) error {
    // Reuse filters from ListApplications
    jobID := strings.TrimSpace(c.Query("job_id"))
    status := strings.TrimSpace(c.Query("status"))
    q := strings.TrimSpace(c.Query("q"))
    from := strings.TrimSpace(c.Query("from"))
    to := strings.TrimSpace(c.Query("to"))

    dbq := h.db.Model(&models.JobApplication{}).Preload("Job")
    if jobID != "" { dbq = dbq.Where("job_id = ?", jobID) }
    if status != "" { dbq = dbq.Where("LOWER(status) = ?", strings.ToLower(status)) }
    if q != "" { like := "%"+strings.ToLower(q)+"%"; dbq = dbq.Where("LOWER(applicant_name) LIKE ? OR LOWER(applicant_email) LIKE ?", like, like) }
    if from != "" { if t, err := time.Parse(time.RFC3339, from); err == nil { dbq = dbq.Where("applied_at >= ?", t) } }
    if to != "" { if t, err := time.Parse(time.RFC3339, to); err == nil { dbq = dbq.Where("applied_at <= ?", t) } }

    var apps []models.JobApplication
    if err := dbq.Order("applied_at DESC").Find(&apps).Error; err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
    }

    var buf bytes.Buffer
    w := csv.NewWriter(&buf)
    _ = w.Write([]string{"id","job_id","job_title","applicant_name","applicant_email","applicant_phone","source","status","applied_at","resume_url"})
    for _, a := range apps {
        _ = w.Write([]string{
            strconv.FormatUint(uint64(a.ID),10),
            strconv.FormatUint(uint64(a.JobID),10),
            a.Job.Title,
            a.ApplicantName,
            a.ApplicantEmail,
            a.ApplicantPhone,
            a.Source,
            a.Status,
            a.AppliedAt.Format(time.RFC3339),
            a.ResumeURL,
        })
    }
    w.Flush()

    c.Set("Content-Type", "text/csv")
    c.Attachment("applications.csv")
    return c.Send(buf.Bytes())
}

// --- helpers ---
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func generateSlug(title string) string {
    s := strings.ToLower(strings.TrimSpace(title))
    s = nonAlnum.ReplaceAllString(s, "-")
    s = strings.Trim(s, "-")
    if s == "" { s = fmt.Sprintf("job-%d", time.Now().Unix()) }
    return s
}

func ensureUniqueJobSlug(db *gorm.DB, base string, excludeID uint) (string, error) {
    slug := base
    for i := 0; i < 50; i++ {
        var count int64
        q := db.Model(&models.Job{}).Where("slug = ?", slug)
        if excludeID != 0 { q = q.Where("id <> ?", excludeID) }
        if err := q.Count(&count).Error; err != nil { return "", err }
        if count == 0 { return slug, nil }
        slug = fmt.Sprintf("%s-%d", base, i+2)
    }
    return "", fmt.Errorf("could not generate unique slug")
}
