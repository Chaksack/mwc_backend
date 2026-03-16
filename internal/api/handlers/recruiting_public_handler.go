package handlers

import (
    "net/http"
    "net/url"
    "regexp"
    "strings"
    "time"

    "mwc_backend/config"
    "mwc_backend/internal/email"
    "mwc_backend/internal/metrics"
    "mwc_backend/internal/models"
    "mwc_backend/internal/services"

    "github.com/gofiber/fiber/v2"
    "gorm.io/gorm"
)

// RecruitingPublicHandler handles public careers endpoints (anonymous)
type RecruitingPublicHandler struct {
	db       *gorm.DB
	cfg      *config.Config
	emailSvc email.EmailService
}

func NewRecruitingPublicHandler(db *gorm.DB, cfg *config.Config, emailSvc email.EmailService) *RecruitingPublicHandler {
	return &RecruitingPublicHandler{db: db, cfg: cfg, emailSvc: emailSvc}
}

// ListPublishedJobs lists published jobs with optional filters
// @Summary List published jobs (public)
// @Tags public, careers
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page (max 100)" default(20)
// @Param q query string false "Free-text query across title/description"
// @Param location query string false "Filter by location (contains)"
// @Param type query string false "Employment type (e.g., full-time, part-time)"
// @Param department query string false "Department filter"
// @Param sort query string false "Sort: recent (default) or expiring_soon" default(recent)
// @Param include_institution query bool false "Include institution profile in response"
// @Success 200 {object} map[string]interface{} "Jobs list with pagination"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /careers/jobs [get]
func (h *RecruitingPublicHandler) ListPublishedJobs(c *fiber.Ctx) error {
	// Minimal MVP: return active, non-expired jobs with basic filters and pagination
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("page_size", 20)
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

 q := strings.TrimSpace(c.Query("q"))
 location := strings.TrimSpace(c.Query("location"))
 empType := strings.TrimSpace(c.Query("type"))
 dept := strings.TrimSpace(c.Query("department"))
 sort := strings.TrimSpace(strings.ToLower(c.Query("sort"))) // recent (default), expiring_soon
 includeInst := c.Query("include_institution") == "true"

 now := time.Now()
 dbq := h.db.Model(&models.Job{}).
     Where("is_active = ?", true).
     Where("is_published = ?", true).
     Where("expires_at IS NULL OR expires_at > ?", now)
 if includeInst {
     dbq = dbq.Preload("InstitutionProfile")
 }

	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		dbq = dbq.Where("LOWER(title) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}
	if location != "" {
		like := "%" + strings.ToLower(location) + "%"
		dbq = dbq.Where("LOWER(location) LIKE ?", like)
	}
 if empType != "" {
        dbq = dbq.Where("LOWER(employment_type) = ?", strings.ToLower(empType))
    }
    if dept != "" {
        dbq = dbq.Where("LOWER(department) = ?", strings.ToLower(dept))
    }

	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to count jobs: " + err.Error()})
	}

 var jobs []models.Job
 order := "published_at DESC NULLS LAST, posted_at DESC"
 if sort == "expiring_soon" {
     order = "expires_at ASC NULLS LAST, posted_at DESC"
 }
 if err := dbq.Order(order).Offset((page - 1) * pageSize).Limit(pageSize).Find(&jobs).Error; err != nil {
     return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load jobs: " + err.Error()})
 }

	return c.JSON(fiber.Map{
		"data": jobs,
		"pagination": fiber.Map{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
		},
	})
}

// GetJobBySlug gets a published job by slug
// @Summary Get a job by slug (public)
// @Tags public, careers
// @Produce json
// @Param slug path string true "Job slug"
// @Param include_institution query bool false "Include institution profile in response"
// @Success 200 {object} models.Job
// @Failure 404 {object} map[string]string "Job not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /careers/jobs/{slug} [get]
func (h *RecruitingPublicHandler) GetJobBySlug(c *fiber.Ctx) error {
    slug := strings.TrimSpace(c.Params("slug"))
    if slug == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "slug is required"})
    }
    var job models.Job
    now := time.Now()
    q := h.db.Where("slug = ? AND is_active = ? AND is_published = ? AND (expires_at IS NULL OR expires_at > ?)", slug, true, true, now)
    if c.Query("include_institution") == "true" {
        q = q.Preload("InstitutionProfile")
    }
    if err := q.First(&job).Error; err != nil {
        if err == gorm.ErrRecordNotFound {
            return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found"})
        }
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
    }
    return c.JSON(job)
}

// PresignResumeUpload returns a presigned URL/fields for resume upload
// @Summary Get presigned upload for resume (public)
// @Tags public, careers
// @Accept json
// @Produce json
// @Param data body struct{FileName string `json:"file_name"`; ContentType string `json:"content_type"`; SizeBytes int64 `json:"size_bytes"`; Captcha string `json:"captcha_token"`} true "Presign request"
// @Success 200 {object} services.PresignedPost
// @Failure 400 {object} map[string]string "Validation error"
// @Failure 403 {object} map[string]string "Captcha failed or presign disabled"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /careers/resume/presign [post]
func (h *RecruitingPublicHandler) PresignResumeUpload(c *fiber.Ctx) error {
	type PresignReq struct {
		FileName    string `json:"file_name"`
		ContentType string `json:"content_type"`
		SizeBytes   int64  `json:"size_bytes"`
		Captcha     string `json:"captcha_token"`
	}
	var req PresignReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON: " + err.Error()})
	}
	if req.FileName == "" || req.ContentType == "" || req.SizeBytes <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file_name, content_type, size_bytes are required"})
	}
	// Captcha verification if enabled
	if strings.ToLower(h.cfg.CaptchaProvider) != "" && strings.ToLower(h.cfg.CaptchaProvider) != "none" {
		capSvc := services.NewCaptchaService(h.cfg)
		ok, err := capSvc.Verify(c.Context(), req.Captcha)
		if err != nil || !ok {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "captcha verification failed"})
		}
	}
	// Validate content type and size
	if h.cfg.ResumeMaxSizeMB > 0 && req.SizeBytes > int64(h.cfg.ResumeMaxSizeMB)*1024*1024 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file too large"})
	}
	if ats := strings.TrimSpace(h.cfg.ResumeAllowedTypes); ats != "" {
		allowed := false
		for _, t := range strings.Split(ats, ",") {
			if strings.EqualFold(strings.TrimSpace(t), req.ContentType) {
				allowed = true
				break
			}
		}
		if !allowed {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "content type not allowed"})
		}
	}
	// Create a key (not secure but fine for MVP): resumes/yyyy-mm/<timestamp>-<filename>
	key := services.GenerateResumeObjectKey(req.FileName)
	store := services.NewStorageService(h.cfg)
	presigned, err := store.CreatePresignedUpload(key, req.ContentType, req.SizeBytes)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create presign: " + err.Error()})
	}
	return c.JSON(presigned)
}

// ApplyToJob accepts an application for a job (restricted to Montessori Professionals)
// @Summary Apply to a job (Montessori Professional only)
// @Tags careers, authenticated
// @Accept json
// @Produce json
// @Param id path int true "Job ID"
// @Param data body struct{ApplicantName string `json:"applicant_name"`; ApplicantEmail string `json:"applicant_email"`; ApplicantPhone string `json:"applicant_phone"`; ResumeURL string `json:"resume_url"`; CoverLetter string `json:"cover_letter"`; Consent bool `json:"consent"`; Captcha string `json:"captcha_token"`; Source string `json:"source"`} true "Application payload"
// @Success 201 {object} map[string]interface{} "Application created"
// @Failure 400 {object} map[string]string "Validation error"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden (role or captcha failed)"
// @Failure 404 {object} map[string]string "Job not available"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /careers/jobs/{id}/apply [post]
func (h *RecruitingPublicHandler) ApplyToJob(c *fiber.Ctx) error {
    // Enforce role at handler level as defense-in-depth (routes also restrict)
    if role, ok := c.Locals("user_role").(models.UserRole); !ok || role != models.MontessoriProfessionalRole {
        return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only Montessori Professionals can apply to jobs."})
    }
    userID, _ := c.Locals("user_id").(uint)
    jobID := c.Params("id")
    if strings.TrimSpace(jobID) == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "job id is required"})
    }
	type ApplyReq struct {
		ApplicantName  string `json:"applicant_name"`
		ApplicantEmail string `json:"applicant_email"`
		ApplicantPhone string `json:"applicant_phone"`
		ResumeURL      string `json:"resume_url"`
		CoverLetter    string `json:"cover_letter"`
		Consent        bool   `json:"consent"`
		Captcha        string `json:"captcha_token"`
		Source         string `json:"source"`
	}
	var req ApplyReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON: " + err.Error()})
	}
 if req.ApplicantName == "" || req.ApplicantEmail == "" || req.ResumeURL == "" || !req.Consent {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name, email, resume_url and consent are required"})
    }
    // Tighten email format validation (simple RFC5322-like regex)
    emailRe := regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)
    if !emailRe.MatchString(strings.TrimSpace(req.ApplicantEmail)) {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid email format"})
    }
    // Resume URL allowlist: if S3 bucket configured, require resume_url to point to that bucket on amazonaws.com
    if u, err := url.Parse(req.ResumeURL); err == nil {
        if u.Scheme != "https" {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "resume_url must use https"})
        }
        bucket := strings.TrimSpace(h.cfg.S3Bucket)
        region := strings.TrimSpace(h.cfg.S3Region)
        if bucket != "" {
            host := strings.ToLower(u.Host)
            path := u.Path
            // Accept virtual-hosted-style: <bucket>.s3.<region>.amazonaws.com or <bucket>.s3.amazonaws.com
            ok := strings.HasPrefix(host, strings.ToLower(bucket)+".s3.") && strings.HasSuffix(host, ".amazonaws.com")
            if !ok && region != "" {
                ok = host == "s3."+strings.ToLower(region)+".amazonaws.com" && strings.HasPrefix(path, "/"+bucket+"/")
            }
            if !ok {
                return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "resume_url must point to configured S3 bucket"})
            }
        }
    } else {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid resume_url"})
    }
 // Captcha if enabled
 if strings.ToLower(h.cfg.CaptchaProvider) != "" && strings.ToLower(h.cfg.CaptchaProvider) != "none" {
     capSvc := services.NewCaptchaService(h.cfg)
     ok, err := capSvc.Verify(c.Context(), req.Captcha)
     if err != nil || !ok {
         return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "captcha verification failed"})
     }
 }
 // Validate job is published and active; preload institution user for notifications
 var job models.Job
 now := time.Now()
 if err := h.db.Preload("InstitutionProfile").Preload("InstitutionProfile.User").
     Where("id = ? AND is_active = ? AND is_published = ? AND (expires_at IS NULL OR expires_at > ?)", jobID, true, true, now).
     First(&job).Error; err != nil {
     if err == gorm.ErrRecordNotFound {
         return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not available"})
     }
     return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
 }
 // Resolve applicant Montessori Professional profile (required)
    var prof models.MontessoriProfessionalProfile
    if err := h.db.Where("user_id = ?", userID).First(&prof).Error; err != nil {
        if err == gorm.ErrRecordNotFound {
            return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Please complete a Montessori Professional profile before applying."})
        }
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load professional profile: " + err.Error()})
    }

    app := models.JobApplication{
        JobID:          job.ID,
        MontessoriProfessionalProfileID: &prof.ID,
        ApplicantName:  req.ApplicantName,
        ApplicantEmail: req.ApplicantEmail,
        ApplicantPhone: req.ApplicantPhone,
        ResumeURL:      req.ResumeURL,
        CoverLetter:    req.CoverLetter,
		Consent:        req.Consent,
		Source:         defaultSource(req.Source),
		AppliedAt:      time.Now(),
		Status:         "pending",
	}
 if err := h.db.Create(&app).Error; err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save application: " + err.Error()})
    }
    // Metrics: increment applications counter
    metrics.IncrementApplications()
	// Create application event (system)
	_ = h.db.Create(&models.ApplicationEvent{
		ApplicationID: app.ID,
		Action:        "created",
		Notes:         "Public application submitted",
	}).Error
    // Send receipt to applicant
    if h.emailSvc != nil && req.ApplicantEmail != "" {
        subject := "Application received: " + job.Title
        body := "<p>Thank you for applying to '" + job.Title + "'. We have received your application.</p>"
        _ = h.emailSvc.SendEmail(req.ApplicantEmail, subject, body)
    }
    // Notify institution/training center by email if possible
    if h.emailSvc != nil && job.InstitutionProfile.User.Email != "" {
        subj := "New application for: " + job.Title
        body := "<p>A new application has been received for <strong>" + job.Title + "</strong>.</p>" +
            "<ul>" +
            "<li>Name: " + req.ApplicantName + "</li>" +
            "<li>Email: " + req.ApplicantEmail + "</li>" +
            "<li>Phone: " + req.ApplicantPhone + "</li>" +
            "</ul>" +
            "<p>Resume: <a href='" + req.ResumeURL + "'>View Resume</a></p>"
        _ = h.emailSvc.SendEmail(job.InstitutionProfile.User.Email, subj, body)
    }
    return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": app.ID, "status": app.Status})
}

func defaultSource(src string) string {
	s := strings.TrimSpace(strings.ToLower(src))
	if s == "" {
		return "website"
	}
	return s
}
