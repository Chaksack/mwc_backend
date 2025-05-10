package handlers

// Imports are assumed to be similar to other handler files:
import (
	"fmt"
	"log"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type InstitutionHandler struct {
	db        *gorm.DB
	mqService queue.MessageQueueService
}

func NewInstitutionHandler(db *gorm.DB, mq queue.MessageQueueService) *InstitutionHandler {
	return &InstitutionHandler{db: db, mqService: mq}
}

type InstitutionProfileRequest struct {
	InstitutionName  string `json:"institution_name" validate:"required"`
	VerificationDocs string `json:"verification_docs,omitempty"` // URL or path
}

type JobRequest struct {
	Title          string `json:"title" validate:"required"`
	Description    string `json:"description" validate:"required"`
	Location       string `json:"location"`
	EmploymentType string `json:"employment_type"`
	SalaryRange    string `json:"salary_range"`
	ExpiresAt      string `json:"expires_at,omitempty"` // e.g., "2024-12-31T23:59:59Z"
}

// CreateOrUpdateInstitutionProfile for an institution/training center
func (h *InstitutionHandler) CreateOrUpdateInstitutionProfile(c *fiber.Ctx) error {
	actorUserID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User ID not found in token"})
	}

	req := new(InstitutionProfileRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate req (e.g., using go-playground/validator)

	var profile models.InstitutionProfile
	// Use FirstOrInit or FirstOrCreate for cleaner logic if profile might not exist
	err := h.db.Where("user_id = ?", actorUserID).First(&profile).Error
	isNewProfile := false
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			isNewProfile = true
			profile.UserID = actorUserID // Set UserID for new profile
		} else {
			LogUserAction(h.db, actorUserID, "INST_PROFILE_FETCH_FAIL", actorUserID, "InstitutionProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error fetching profile: " + err.Error()})
		}
	}

	profile.InstitutionName = req.InstitutionName
	if req.VerificationDocs != "" { // Allow updating verification docs
		profile.VerificationDocs = req.VerificationDocs
	}
	// IsVerified should be handled by an admin usually, not set here directly unless specific logic allows

	if err := h.db.Save(&profile).Error; err != nil {
		actionType := "INST_PROFILE_UPDATE_FAIL"
		if isNewProfile {
			actionType = "INST_PROFILE_CREATE_FAIL"
		}
		LogUserAction(h.db, actorUserID, actionType, profile.ID, "InstitutionProfile", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save institution profile: " + err.Error()})
	}

	actionType := "INST_PROFILE_UPDATE_SUCCESS"
	if isNewProfile {
		actionType = "INST_PROFILE_CREATE_SUCCESS"
	}
	LogUserAction(h.db, actorUserID, actionType, profile.ID, "InstitutionProfile", "Profile saved", c)
	return c.Status(fiber.StatusOK).JSON(profile)
}

// SelectSchool allows an institution to select an existing school uploaded by admin.
func (h *InstitutionHandler) SelectSchool(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("school_id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&institutionProfile).Error; err != nil {
		// ... (error handling)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution profile not found."})
	}

	if institutionProfile.SchoolID != nil && *institutionProfile.SchoolID != 0 {
		LogUserAction(h.db, actorUserID, "INST_SCHOOL_SELECT_FAIL_ALREADY_MAPPED", uint(schoolID), "School", "Institution already has a school", c)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Institution already has a school selected."})
	}

	var school models.School
	// Ensure school exists and was uploaded by admin (or meets other criteria if logic changes)
	if err := h.db.Where("id = ? AND uploaded_by_admin = ?", uint(schoolID), true).First(&school).Error; err != nil {
		// ... (error handling for school not found)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Admin-uploaded school not found."})
	}

	// Critical: Check if this school is already selected by another institution (unique constraint on SchoolID in InstitutionProfile)
	// GORM will enforce this at DB level if `gorm:"uniqueIndex"` is on SchoolID.
	// We can pre-check to provide a friendlier error.
	var existingSelection models.InstitutionProfile
	errCheck := h.db.Where("school_id = ?", uint(schoolID)).First(&existingSelection).Error
	if errCheck == nil { // A record was found
		LogUserAction(h.db, actorUserID, "INST_SCHOOL_SELECT_FAIL_SCHOOL_TAKEN", uint(schoolID), "School", fmt.Sprintf("School taken by inst %d", existingSelection.ID), c)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "This school is already mapped to another institution."})
	}
	if errCheck != gorm.ErrRecordNotFound { // Some other DB error during check
		LogUserAction(h.db, actorUserID, "INST_SCHOOL_SELECT_FAIL_DB_CHECK", uint(schoolID), "School", errCheck.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error checking school availability: " + errCheck.Error()})
	}

	institutionProfile.SchoolID = &school.ID // Assign school.ID (which is uint)
	if err := h.db.Save(&institutionProfile).Error; err != nil {
		// This might fail due to the unique constraint if another request sneaked in.
		LogUserAction(h.db, actorUserID, "INST_SCHOOL_SELECT_FAIL_SAVE", uint(schoolID), "School", err.Error(), c)
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "This school was just mapped by another institution. Please try another."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to select school: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "INST_SCHOOL_SELECT_SUCCESS", uint(schoolID), "School", "School selected", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "School selected successfully", "school_id": school.ID, "school_name": school.Name})
}

// CreateSchool allows an institution to create a new school if not in the admin list.
func (h *InstitutionHandler) CreateSchool(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&institutionProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution profile not found."})
	}

	if institutionProfile.SchoolID != nil && *institutionProfile.SchoolID != 0 {
		LogUserAction(h.db, actorUserID, "INST_SCHOOL_CREATE_FAIL_ALREADY_MAPPED", 0, "School", "Institution already has a school", c)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Institution already has a school mapped."})
	}

	var req SchoolUploadData // Reuse admin's upload struct or create a specific one
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate req (name, country_code are important)
	if req.Name == "" || req.CountryCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "School name and country code are required."})
	}

	newSchool := models.School{
		Name:            req.Name,
		Address:         req.Address,
		City:            req.City,
		State:           req.State,
		CountryCode:     req.CountryCode,
		ZipCode:         req.ZipCode,
		ContactEmail:    req.ContactEmail,
		ContactPhone:    req.ContactPhone,
		Website:         req.Website,
		UploadedByAdmin: false,
		CreatedByUserID: &actorUserID, // Link to the institution user who created it
	}

	tx := h.db.Begin() // Start transaction

	if err := tx.Create(&newSchool).Error; err != nil {
		tx.Rollback()
		LogUserAction(h.db, actorUserID, "INST_SCHOOL_CREATE_FAIL_DB_SCHOOL", 0, "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create school: " + err.Error()})
	}

	// Link this new school to the institution
	institutionProfile.SchoolID = &newSchool.ID
	if err := tx.Save(&institutionProfile).Error; err != nil {
		tx.Rollback()
		LogUserAction(h.db, actorUserID, "INST_SCHOOL_CREATE_FAIL_DB_LINK", newSchool.ID, "InstitutionProfile", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to link new school to institution: " + err.Error()})
	}

	if err := tx.Commit().Error; err != nil {
		LogUserAction(h.db, actorUserID, "INST_SCHOOL_CREATE_FAIL_TX_COMMIT", newSchool.ID, "System", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Transaction failed while creating school: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "INST_SCHOOL_CREATE_SUCCESS", newSchool.ID, "School", "School created and linked", c)
	return c.Status(fiber.StatusCreated).JSON(newSchool)
}

// PostJob allows an institution to post a new job.
func (h *InstitutionHandler) PostJob(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&institutionProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution profile not found."})
	}
	if institutionProfile.SchoolID == nil || *institutionProfile.SchoolID == 0 {
		LogUserAction(h.db, actorUserID, "INST_JOB_POST_FAIL_NO_SCHOOL", 0, "Job", "Institution has no school", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Institution must have a selected/created school to post jobs."})
	}

	req := new(JobRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate req (Title, Description are important)
	if req.Title == "" || req.Description == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Job title and description are required."})
	}

	var expiresAtTime *time.Time
	if req.ExpiresAt != "" {
		parsedTime, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err == nil {
			expiresAtTime = &parsedTime
		} else {
			// Optionally return an error or log a warning
			log.Printf("Warning: Could not parse ExpiresAt '%s' for job posting: %v", req.ExpiresAt, err)
		}
	}

	job := models.Job{
		InstitutionProfileID: institutionProfile.ID,
		Title:                req.Title,
		Description:          req.Description,
		Location:             req.Location,
		EmploymentType:       req.EmploymentType,
		SalaryRange:          req.SalaryRange,
		IsActive:             true, // Default to active
		ExpiresAt:            expiresAtTime,
	}

	if err := h.db.Create(&job).Error; err != nil {
		LogUserAction(h.db, actorUserID, "INST_JOB_POST_FAIL_DB", 0, "Job", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to post job: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "INST_JOB_POST_SUCCESS", job.ID, "Job", "Job posted", c)
	return c.Status(fiber.StatusCreated).JSON(job)
}

// UpdateJob allows an institution to update an existing job.
func (h *InstitutionHandler) UpdateJob(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	jobIDStr := c.Params("job_id")
	jobID, err := strconv.ParseUint(jobIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID format"})
	}

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&institutionProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution profile not found."})
	}

	var job models.Job
	if err := h.db.Where("id = ? AND institution_profile_id = ?", uint(jobID), institutionProfile.ID).First(&job).Error; err != nil {
		// ... (error handling for job not found or not owned)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found or you do not have permission to update it."})
	}

	req := new(JobRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate req

	job.Title = req.Title
	job.Description = req.Description
	job.Location = req.Location
	job.EmploymentType = req.EmploymentType
	job.SalaryRange = req.SalaryRange
	// job.IsActive can also be updatable
	if req.ExpiresAt != "" {
		parsedTime, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err == nil {
			job.ExpiresAt = &parsedTime
		} else {
			log.Printf("Warning: Could not parse ExpiresAt '%s' for job update: %v", req.ExpiresAt, err)
		}
	} else {
		job.ExpiresAt = nil // Allow clearing expiration
	}

	if err := h.db.Save(&job).Error; err != nil {
		LogUserAction(h.db, actorUserID, "INST_JOB_UPDATE_FAIL_DB", uint(jobID), "Job", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update job: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "INST_JOB_UPDATE_SUCCESS", uint(jobID), "Job", "Job updated", c)
	return c.Status(fiber.StatusOK).JSON(job)
}

// DeleteJob allows an institution to delete a job (soft delete).
func (h *InstitutionHandler) DeleteJob(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	jobIDStr := c.Params("job_id")
	jobID, err := strconv.ParseUint(jobIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID format"})
	}

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&institutionProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution profile not found."})
	}

	// Soft delete the job, ensuring it belongs to the institution.
	result := h.db.Where("id = ? AND institution_profile_id = ?", uint(jobID), institutionProfile.ID).Delete(&models.Job{})
	if result.Error != nil {
		LogUserAction(h.db, actorUserID, "INST_JOB_DELETE_FAIL_DB", uint(jobID), "Job", result.Error.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete job: " + result.Error.Error()})
	}
	if result.RowsAffected == 0 {
		LogUserAction(h.db, actorUserID, "INST_JOB_DELETE_FAIL_NOTFOUND", uint(jobID), "Job", "Job not found or no permission", c)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found or you do not have permission to delete it."})
	}

	LogUserAction(h.db, actorUserID, "INST_JOB_DELETE_SUCCESS", uint(jobID), "Job", "Job deleted", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Job deleted successfully"})
}

// GetJobApplicants retrieves applicants for a specific job.
func (h *InstitutionHandler) GetJobApplicants(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	jobIDStr := c.Params("job_id")
	jobID, err := strconv.ParseUint(jobIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID format"})
	}

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&institutionProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution profile not found."})
	}

	// Verify the job belongs to this institution
	var job models.Job
	if err := h.db.Where("id = ? AND institution_profile_id = ?", uint(jobID), institutionProfile.ID).First(&job).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found or access denied."})
	}

	var applications []models.JobApplication
	// Preload Educator profile and the User model associated with the Educator
	if err := h.db.Preload("Educator.User").Where("job_id = ?", uint(jobID)).Find(&applications).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve job applicants: " + err.Error()})
	}

	// Transform response to include necessary details
	type ApplicantResponse struct {
		ApplicationID  uint      `json:"application_id"`
		EducatorID     uint      `json:"educator_id"` // User ID of the educator
		EducatorName   string    `json:"educator_name"`
		EducatorEmail  string    `json:"educator_email"`
		Bio            string    `json:"bio"`
		Qualifications string    `json:"qualifications"`
		CoverLetter    string    `json:"cover_letter"`
		ResumeURL      string    `json:"resume_url"`
		AppliedAt      time.Time `json:"applied_at"`
		Status         string    `json:"status"`
	}
	var response []ApplicantResponse
	for _, app := range applications {
		if app.Educator.User.ID == 0 { // Check if User was correctly preloaded
			log.Printf("Warning: Educator User data not loaded for application ID %d, EducatorProfileID %d", app.ID, app.EducatorProfileID)
			// Optionally fetch the user separately if this happens, though Preload should handle it.
		}
		response = append(response, ApplicantResponse{
			ApplicationID:  app.ID,
			EducatorID:     app.Educator.UserID, // This is the User.ID from EducatorProfile.User
			EducatorName:   app.Educator.User.FirstName + " " + app.Educator.User.LastName,
			EducatorEmail:  app.Educator.User.Email,
			Bio:            app.Educator.Bio,
			Qualifications: app.Educator.Qualifications,
			CoverLetter:    app.CoverLetter,
			ResumeURL:      app.ResumeURL,
			AppliedAt:      app.AppliedAt,
			Status:         app.Status,
		})
	}
	LogUserAction(h.db, actorUserID, "INST_JOB_VIEW_APPLICANTS", uint(jobID), "Job", fmt.Sprintf("Viewed %d applicants", len(response)), c)
	return c.Status(fiber.StatusOK).JSON(response)
}

// GetMyJobs retrieves all jobs posted by the logged-in institution.
func (h *InstitutionHandler) GetMyJobs(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&institutionProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution profile not found."})
	}

	var jobs []models.Job
	// Add pagination
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	query := h.db.Where("institution_profile_id = ?", institutionProfile.ID).Order("created_at desc").Offset(offset).Limit(limit)

	if err := query.Find(&jobs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve jobs: " + err.Error()})
	}

	var total int64
	h.db.Model(&models.Job{}).Where("institution_profile_id = ?", institutionProfile.ID).Count(&total)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": jobs,
		"meta": fiber.Map{
			"total":     total,
			"page":      page,
			"limit":     limit,
			"last_page": (total + int64(limit) - 1) / int64(limit),
		},
	})
}
