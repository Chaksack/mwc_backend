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
// @Summary Create or update institution profile
// @Description Creates a new institution profile or updates an existing one
// @Tags institution,profile
// @Accept json
// @Produce json
// @Param profile body InstitutionProfileRequest true "Institution profile information"
// @Success 200 {object} models.InstitutionProfile "Profile created or updated successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/profile [post]
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
// @Summary Select a school
// @Description Associates an institution with an existing school from the admin-uploaded list
// @Tags institution,schools
// @Produce json
// @Param school_id path int true "School ID"
// @Success 200 {object} map[string]interface{} "School selected successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Institution profile or school not found"
// @Failure 409 {object} map[string]string "Institution already has a school or school already taken"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/schools/select/{school_id} [put]
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
// @Summary Create a new school
// @Description Creates a new school entry for an institution when not found in admin list
// @Tags institution,schools
// @Accept json
// @Produce json
// @Param school body SchoolUploadData true "School information"
// @Success 201 {object} models.School "School created successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Institution profile not found"
// @Failure 409 {object} map[string]string "Institution already has a school"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/schools [post]
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
	// TODO: Validate req (title, countryCode are important)
	if req.Title == "" || req.CountryCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "School title and country code are required."})
	}

	// Handle nullable fields
	var state string
	if req.State != nil {
		state = *req.State
	}

	var postalCode string
	if req.PostalCode != nil {
		postalCode = *req.PostalCode
	}

	newSchool := models.School{
		Name:            req.Title,
		Address:         req.Address,
		City:            req.City,
		State:           state,
		CountryCode:     req.CountryCode,
		Country:         req.Country,
		ZipCode:         postalCode,
		ContactEmail:    "", // Not available in the new structure
		ContactPhone:    req.Phone,
		Website:         req.Url,
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
// @Summary Post a job opening
// @Description Creates a new job posting for an institution
// @Tags institution,jobs
// @Accept json
// @Produce json
// @Param job body JobRequest true "Job information"
// @Success 201 {object} models.Job "Job posted successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Institution profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/jobs [post]
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
// @Summary Update a job posting
// @Description Updates an existing job posting
// @Tags institution,jobs
// @Accept json
// @Produce json
// @Param job_id path int true "Job ID"
// @Param job body JobRequest true "Updated job information"
// @Success 200 {object} models.Job "Job updated successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid job ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden - can only update own jobs"
// @Failure 404 {object} map[string]string "Job not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/jobs/{job_id} [put]
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
// @Summary Delete a job posting
// @Description Deletes an existing job posting
// @Tags institution,jobs
// @Produce json
// @Param job_id path int true "Job ID"
// @Success 200 {object} map[string]string "Job deleted successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid job ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden - can only delete own jobs"
// @Failure 404 {object} map[string]string "Job not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/jobs/{job_id} [delete]
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
// @Summary Get job applicants
// @Description Retrieves all applicants for a specific job posting
// @Tags institution,jobs
// @Produce json
// @Param job_id path int true "Job ID"
// @Success 200 {array} models.JobApplication "List of job applications with applicant details"
// @Failure 400 {object} map[string]string "Bad request or invalid job ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden - can only view applicants for own jobs"
// @Failure 404 {object} map[string]string "Job not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/jobs/{job_id}/applicants [get]
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

// GetAllJobs retrieves all active jobs in the system.
// @Summary Get all jobs
// @Description Retrieves all active job postings in the system
// @Tags jobs,public
// @Produce json
// @Success 200 {array} models.Job "List of all active jobs"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /jobs [get]
func (h *InstitutionHandler) GetAllJobs(c *fiber.Ctx) error {
	// Add pagination
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var jobs []models.Job
	query := h.db.Where("is_active = ?", true).Order("created_at desc").Offset(offset).Limit(limit)

	if err := query.Find(&jobs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve jobs: " + err.Error()})
	}

	var total int64
	h.db.Model(&models.Job{}).Where("is_active = ?", true).Count(&total)

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

// GetAvailableSchools retrieves schools that can be selected by an institution.
// @Summary Get available schools
// @Description Retrieves a list of schools uploaded by admin that can be selected by an institution
// @Tags institution,schools
// @Produce json
// @Param country_code query string false "Filter schools by country code (e.g., US, UK)"
// @Param page query int false "Page number for pagination" default(1)
// @Param limit query int false "Number of items per page" default(10)
// @Success 200 {object} map[string]interface{} "List of available schools with pagination metadata"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/schools/available [get]
func (h *InstitutionHandler) GetAvailableSchools(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	// Get filter parameters
	countryCode := c.Query("country_code")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	// Build query to get schools uploaded by admin
	query := h.db.Model(&models.School{}).Where("uploaded_by_admin = ?", true)

	// Add country code filter if provided
	if countryCode != "" {
		query = query.Where("country_code = ?", countryCode)
	}

	// Get schools that are not already selected by any institution
	// This uses a subquery to exclude schools that are already linked to institutions
	query = query.Where("id NOT IN (SELECT school_id FROM institution_profiles WHERE school_id IS NOT NULL)")

	// Apply pagination
	query = query.Offset(offset).Limit(limit)

	var schools []models.School
	if err := query.Find(&schools).Error; err != nil {
		LogUserAction(h.db, actorUserID, "INST_SCHOOLS_LIST_FAIL", 0, "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve available schools: " + err.Error()})
	}

	// Count total matching records for pagination metadata
	var total int64
	countQuery := h.db.Model(&models.School{}).Where("uploaded_by_admin = ?", true)
	if countryCode != "" {
		countQuery = countQuery.Where("country_code = ?", countryCode)
	}
	countQuery = countQuery.Where("id NOT IN (SELECT school_id FROM institution_profiles WHERE school_id IS NOT NULL)")
	countQuery.Count(&total)

	LogUserAction(h.db, actorUserID, "INST_SCHOOLS_LIST_SUCCESS", 0, "School", fmt.Sprintf("Listed %d schools", len(schools)), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": schools,
		"meta": fiber.Map{
			"total":     total,
			"page":      page,
			"limit":     limit,
			"last_page": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetMyJobs retrieves all jobs posted by the logged-in institution.
// @Summary Get institution's jobs
// @Description Retrieves all job postings created by the institution
// @Tags institution,jobs
// @Produce json
// @Success 200 {array} models.Job "List of jobs posted by the institution"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Institution profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institution/jobs [get]
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

// InstitutionPublicResponse represents the public details of an institution
type InstitutionPublicResponse struct {
	ID              uint      `json:"id" example:"1"`
	InstitutionName string    `json:"institution_name" example:"Montessori Academy"`
	IsVerified      bool      `json:"is_verified" example:"true"`
	CreatedAt       time.Time `json:"created_at" example:"2023-01-01T12:00:00Z"`
	SchoolLogo      string    `json:"school_logo" example:"https://example.com/school_logo.png"`
	SchoolName      string    `json:"school_name" example:"Montessori Academy Main Campus"`
	About           string    `json:"about" example:"A leading Montessori school dedicated to providing quality education since 1995."`
	ContactInfo     struct {
		Email   string `json:"email" example:"contact@montessoriacademy.example.com"`
		Phone   string `json:"phone" example:"555-123-4567"`
		Website string `json:"website" example:"https://montessoriacademy.example.com"`
		Address string `json:"address" example:"123 Education St, Springfield, IL 62701, USA"`
	} `json:"contact_info"`
}

// GetInstitutionPublicDetails retrieves public details of an institution.
// @Summary Get institution public details
// @Description Retrieves basic public information about an institution
// @Tags institution,public
// @Produce json
// @Param id path int true "Institution ID"
// @Success 200 {object} InstitutionPublicResponse "Institution public details"
// @Failure 404 {object} map[string]string "Institution not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /institutions/{id} [get]
func (h *InstitutionHandler) GetInstitutionPublicDetails(c *fiber.Ctx) error {
	institutionIDStr := c.Params("id")
	institutionID, err := strconv.ParseUint(institutionIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid institution ID format"})
	}

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("id = ?", uint(institutionID)).Preload("User").First(&institutionProfile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve institution: " + err.Error()})
	}

	// Return only public information
	var schoolName, schoolLogo, about string
	var contactEmail, contactPhone, contactWebsite, contactAddress string

	// If school is associated, use its information
	if institutionProfile.School != nil {
		schoolName = institutionProfile.School.Name
		contactEmail = institutionProfile.School.ContactEmail
		contactPhone = institutionProfile.School.ContactPhone
		contactWebsite = institutionProfile.School.Website
		contactAddress = fmt.Sprintf("%s, %s, %s %s, %s", 
			institutionProfile.School.Address, 
			institutionProfile.School.City, 
			institutionProfile.School.State, 
			institutionProfile.School.ZipCode, 
			institutionProfile.School.Country)
	} else {
		// Use institution name as fallback
		schoolName = institutionProfile.InstitutionName
	}

	// For now, these fields might not exist in the database, so we'll use placeholder values
	// In a real implementation, these would come from the database
	schoolLogo = "https://example.com/school_logo.png" // Placeholder
	about = "A leading educational institution dedicated to providing quality education." // Placeholder

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":               institutionProfile.ID,
		"institution_name": institutionProfile.InstitutionName,
		"is_verified":      institutionProfile.IsVerified,
		"created_at":       institutionProfile.CreatedAt,
		"school_logo":      schoolLogo,
		"school_name":      schoolName,
		"about":            about,
		"contact_info": fiber.Map{
			"email":   contactEmail,
			"phone":   contactPhone,
			"website": contactWebsite,
			"address": contactAddress,
		},
	})
}

// UserResponse represents user information in the institution details response
type UserResponse struct {
	ID        uint   `json:"id" example:"42"`
	FirstName string `json:"first_name" example:"John"`
	LastName  string `json:"last_name" example:"Doe"`
	Email     string `json:"email" example:"john.doe@example.com"`
}

// SchoolResponse represents school information in the institution details response
type SchoolResponse struct {
	ID       uint   `json:"id" example:"5"`
	Name     string `json:"name" example:"Montessori Academy Main Campus"`
	Address  string `json:"address" example:"123 Education St"`
	City     string `json:"city" example:"Springfield"`
	State    string `json:"state" example:"IL"`
	ZipCode  string `json:"zip_code" example:"62701"`
	Country  string `json:"country" example:"USA"`
	Phone    string `json:"phone" example:"555-123-4567"`
	Website  string `json:"website" example:"https://montessoriacademy.example.com"`
}

// JobResponse represents job information in the institution details response
type JobResponse struct {
	ID           uint      `json:"id" example:"10"`
	Title        string    `json:"title" example:"Montessori Teacher"`
	Description  string    `json:"description" example:"Looking for an experienced Montessori teacher"`
	Requirements string    `json:"requirements" example:"3+ years experience, Montessori certification"`
	Location     string    `json:"location" example:"Springfield, IL"`
	SalaryRange  string    `json:"salary_range" example:"$45,000 - $60,000"`
	Status       string    `json:"status" example:"open"`
	CreatedAt    time.Time `json:"created_at" example:"2023-02-01T10:00:00Z"`
}

// ReviewResponse represents a review in the institution details response
type ReviewResponse struct {
	ID        uint      `json:"id" example:"15"`
	Rating    int       `json:"rating" example:"5"`
	Comment   string    `json:"comment" example:"Excellent school with dedicated teachers and a nurturing environment."`
	CreatedAt time.Time `json:"created_at" example:"2023-03-10T09:15:00Z"`
	Reviewer  struct {
		Name string `json:"name" example:"Jane Smith"`
	} `json:"reviewer"`
}

// InstitutionDetailsResponse represents the detailed information of an institution
type InstitutionDetailsResponse struct {
	ID              uint           `json:"id" example:"1"`
	InstitutionName string         `json:"institution_name" example:"Montessori Academy"`
	IsVerified      bool           `json:"is_verified" example:"true"`
	CreatedAt       time.Time      `json:"created_at" example:"2023-01-01T12:00:00Z"`
	UpdatedAt       time.Time      `json:"updated_at" example:"2023-01-15T14:30:00Z"`
	User            UserResponse   `json:"user"`
	School          SchoolResponse `json:"school"`
	SchoolLogo      string         `json:"school_logo" example:"https://example.com/school_logo.png"`
	About           string         `json:"about" example:"A leading Montessori school dedicated to providing quality education since 1995."`
	Facts           []string       `json:"facts" example:"['Founded in 1995', 'Over 500 graduates', 'Award-winning curriculum']"`
	Programs        []struct {
		Name        string `json:"name" example:"Early Childhood Program"`
		Description string `json:"description" example:"For children ages 3-6, focusing on sensorial development and practical life skills."`
	} `json:"programs"`
	Activities      []struct {
		Name        string `json:"name" example:"Music and Movement"`
		Description string `json:"description" example:"Weekly music classes incorporating movement and rhythm."`
	} `json:"activities"`
	Philosophy      string         `json:"philosophy" example:"We believe in the Montessori method of education, which emphasizes independence, freedom within limits, and respect for a child's natural psychological, physical, and social development."`
	Map             struct {
		Latitude  float64 `json:"latitude" example:"39.78373"`
		Longitude float64 `json:"longitude" example:"-89.65063"`
		Address   string  `json:"address" example:"123 Education St, Springfield, IL 62701, USA"`
	} `json:"map"`
	Jobs            []JobResponse   `json:"jobs"`
	Reviews         []ReviewResponse `json:"reviews"`
}

// GetInstitutionDetails retrieves detailed information about an institution.
// @Summary Get institution details
// @Description Retrieves detailed information about an institution (requires subscription)
// @Tags institution
// @Produce json
// @Param id path int true "Institution ID"
// @Success 200 {object} InstitutionDetailsResponse "Institution details"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Subscription required"
// @Failure 404 {object} map[string]string "Institution not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /institutions/{id}/details [get]
func (h *InstitutionHandler) GetInstitutionDetails(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Check if user has an active subscription
	var subscription models.Subscription
	err := h.db.Where("user_id = ? AND status = ?", userID, models.SubscriptionActive).First(&subscription).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Active subscription required to view detailed institution information"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check subscription status: " + err.Error()})
	}

	institutionIDStr := c.Params("id")
	institutionID, err := strconv.ParseUint(institutionIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid institution ID format"})
	}

	var institutionProfile models.InstitutionProfile
	if err := h.db.Where("id = ?", uint(institutionID)).Preload("User").Preload("School").Preload("Jobs").First(&institutionProfile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Institution not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve institution: " + err.Error()})
	}

	// Log the action
	LogUserAction(h.db, userID, "VIEW_INSTITUTION_DETAILS", uint(institutionID), "InstitutionProfile", "Viewed institution details", c)

	// Get reviews for this school if available
	var reviews []models.Review
	var schoolID uint
	if institutionProfile.School != nil {
		schoolID = institutionProfile.School.ID
		h.db.Where("school_id = ?", schoolID).Preload("Reviewer").Find(&reviews)
	}

	// Prepare review responses
	var reviewResponses []fiber.Map
	for _, review := range reviews {
		reviewerName := ""
		if review.Reviewer.FirstName != "" || review.Reviewer.LastName != "" {
			reviewerName = fmt.Sprintf("%s %s", review.Reviewer.FirstName, review.Reviewer.LastName)
		} else {
			reviewerName = "Anonymous"
		}

		reviewResponses = append(reviewResponses, fiber.Map{
			"id":         review.ID,
			"rating":     review.Rating,
			"comment":    review.Comment,
			"created_at": review.CreatedAt,
			"reviewer": fiber.Map{
				"name": reviewerName,
			},
		})
	}

	// Prepare location/map data
	var latitude, longitude float64
	var address string
	if institutionProfile.School != nil {
		// In a real implementation, these would come from the database
		// For now, using placeholder values
		latitude = 39.78373  // Placeholder
		longitude = -89.65063 // Placeholder

		address = fmt.Sprintf("%s, %s, %s %s, %s", 
			institutionProfile.School.Address, 
			institutionProfile.School.City, 
			institutionProfile.School.State, 
			institutionProfile.School.ZipCode, 
			institutionProfile.School.Country)
	}

	// For now, these fields might not exist in the database, so we'll use placeholder values
	// In a real implementation, these would come from the database
	schoolLogo := "https://example.com/school_logo.png" // Placeholder
	about := "A leading educational institution dedicated to providing quality education." // Placeholder
	philosophy := "We believe in providing a nurturing environment where students can develop their full potential." // Placeholder

	// Example programs
	programs := []fiber.Map{
		{
			"name": "Early Childhood Program",
			"description": "For children ages 3-6, focusing on sensorial development and practical life skills.",
		},
		{
			"name": "Elementary Program",
			"description": "For children ages 6-12, focusing on academic excellence and social development.",
		},
	}

	// Example activities
	activities := []fiber.Map{
		{
			"name": "Music and Movement",
			"description": "Weekly music classes incorporating movement and rhythm.",
		},
		{
			"name": "Art Studio",
			"description": "Creative expression through various art mediums.",
		},
	}

	// Example facts
	facts := []string{
		"Founded in 1995",
		"Over 500 graduates",
		"Award-winning curriculum",
	}

	// Return detailed information including school, jobs, reviews, and additional fields
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":               institutionProfile.ID,
		"institution_name": institutionProfile.InstitutionName,
		"is_verified":      institutionProfile.IsVerified,
		"created_at":       institutionProfile.CreatedAt,
		"updated_at":       institutionProfile.UpdatedAt,
		"user": fiber.Map{
			"id":         institutionProfile.User.ID,
			"first_name": institutionProfile.User.FirstName,
			"last_name":  institutionProfile.User.LastName,
			"email":      institutionProfile.User.Email,
		},
		"school":      institutionProfile.School,
		"school_logo": schoolLogo,
		"about":       about,
		"facts":       facts,
		"programs":    programs,
		"activities":  activities,
		"philosophy":  philosophy,
		"map": fiber.Map{
			"latitude":  latitude,
			"longitude": longitude,
			"address":   address,
		},
		"jobs":    institutionProfile.Jobs,
		"reviews": reviewResponses,
	})
}
