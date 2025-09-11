package handlers

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"strconv"
)

type MontessoriProfessionalHandler struct {
	db        *gorm.DB
	mqService queue.MessageQueueService
}

func NewMontessoriProfessionalHandler(db *gorm.DB, mq queue.MessageQueueService) *MontessoriProfessionalHandler {
	return &MontessoriProfessionalHandler{db: db, mqService: mq}
}

type MontessoriProfessionalProfileRequest struct {
	Bio            string `json:"bio"`
	Qualifications string `json:"qualifications"`
	Experience     string `json:"experience"`
}

type JobApplicationRequest struct {
	CoverLetter string `json:"cover_letter" form:"cover_letter"`
	// ResumeURL is now handled as a file upload, not as a URL string
}

// CreateOrUpdateMontessoriProfessionalProfile creates or updates a montessori professional's profile.
// @Summary Create or update montessori professional profile
// @Description Creates a new montessori professional profile or updates an existing one
// @Tags montessori-professional,profile
// @Accept json
// @Produce json
// @Param profile body MontessoriProfessionalProfileRequest true "Montessori Professional profile information"
// @Success 200 {object} models.MontessoriProfessionalProfile "Profile created or updated successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /montessori-professional/profile [post]
func (h *MontessoriProfessionalHandler) CreateOrUpdateMontessoriProfessionalProfile(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	req := new(MontessoriProfessionalProfileRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate req

	var profile models.MontessoriProfessionalProfile
	err := h.db.Where("user_id = ?", actorUserID).First(&profile).Error
	isNewProfile := false
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			isNewProfile = true
			profile.UserID = actorUserID
		} else {
			LogUserAction(h.db, actorUserID, "MONT_PROF_PROFILE_FETCH_FAIL", actorUserID, "MontessoriProfessionalProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
		}
	}

	profile.Bio = req.Bio
	profile.Qualifications = req.Qualifications
	profile.Experience = req.Experience

	if err := h.db.Save(&profile).Error; err != nil {
		actionType := "MONT_PROF_PROFILE_UPDATE_FAIL"
		if isNewProfile {
			actionType = "MONT_PROF_PROFILE_CREATE_FAIL"
		}
		LogUserAction(h.db, actorUserID, actionType, profile.ID, "MontessoriProfessionalProfile", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save montessori professional profile: " + err.Error()})
	}

	actionType := "MONT_PROF_PROFILE_UPDATE_SUCCESS"
	if isNewProfile {
		actionType = "MONT_PROF_PROFILE_CREATE_SUCCESS"
	}
	LogUserAction(h.db, actorUserID, actionType, profile.ID, "MontessoriProfessionalProfile", "Profile saved", c)
	return c.Status(fiber.StatusOK).JSON(profile)
}

// SearchSchools allows montessori professionals (and parents) to search for schools.
// @Summary Search for schools
// @Description Search for schools with various filters and pagination
// @Tags montessori-professional,schools
// @Produce json
// @Param name query string false "Filter by school name"
// @Param city query string false "Filter by city"
// @Param country_code query string false "Filter by country code"
// @Param category query string false "Filter by category (school or training_center)"
// @Param page query int false "Page number for pagination" default(1)
// @Param limit query int false "Number of items per page" default(10)
// @Success 200 {object} map[string]interface{} "List of schools with pagination metadata"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /montessori-professional/schools/search [get]
func (h *MontessoriProfessionalHandler) SearchSchools(c *fiber.Ctx) error {
	// This handler is identical to GetPublicSchools, can be aliased or refactored.
	// For now, just calling the shared one.
	return GetPublicSchools(h.db)(c)
}

// SaveSchool allows a montessori professional to save a school to their list.
// @Summary Save a school
// @Description Adds a school to the montessori professional's saved schools list
// @Tags montessori-professional,schools
// @Produce json
// @Param school_id path int true "School ID"
// @Success 200 {object} map[string]string "School saved successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Montessori Professional profile or school not found"
// @Failure 409 {object} map[string]string "School already saved"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /montessori-professional/schools/save/{school_id} [post]
func (h *MontessoriProfessionalHandler) SaveSchool(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("school_id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	var professionalProfile models.MontessoriProfessionalProfile
	if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&professionalProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Montessori Professional profile not found."})
	}

	var school models.School
	if err := h.db.First(&school, uint(schoolID)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found."})
	}

	// Check if already saved
	for _, savedSchool := range professionalProfile.SavedSchools {
		if savedSchool.ID == uint(schoolID) {
			LogUserAction(h.db, actorUserID, "MONT_PROF_SCHOOL_SAVE_FAIL_ALREADY_SAVED", uint(schoolID), "School", "School already saved", c)
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "School already saved."})
		}
	}

	if err := h.db.Model(&professionalProfile).Association("SavedSchools").Append(&school); err != nil {
		LogUserAction(h.db, actorUserID, "MONT_PROF_SCHOOL_SAVE_FAIL_DB", uint(schoolID), "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save school: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "MONT_PROF_SCHOOL_SAVE_SUCCESS", uint(schoolID), "School", "School saved", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "School saved successfully."})
}

// DeleteSavedSchool allows a montessori professional to remove a school from their saved list.
// @Summary Delete a saved school
// @Description Removes a school from the montessori professional's saved schools list
// @Tags montessori-professional,schools
// @Produce json
// @Param school_id path int true "School ID"
// @Success 200 {object} map[string]string "School removed successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Montessori Professional profile or school not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /montessori-professional/schools/save/{school_id} [delete]
func (h *MontessoriProfessionalHandler) DeleteSavedSchool(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("school_id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	var professionalProfile models.MontessoriProfessionalProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&professionalProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Montessori Professional profile not found."})
	}

	var school models.School
	if err := h.db.First(&school, uint(schoolID)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found."})
	}

	if err := h.db.Model(&professionalProfile).Association("SavedSchools").Delete(&school); err != nil {
		LogUserAction(h.db, actorUserID, "MONT_PROF_SCHOOL_UNSAVE_FAIL_DB", uint(schoolID), "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete saved school: " + err.Error()})
	}
	// GORM's Delete for associations might not return error if item wasn't associated.
	// Check RowsAffected if precise feedback is needed.

	LogUserAction(h.db, actorUserID, "MONT_PROF_SCHOOL_UNSAVE_SUCCESS", uint(schoolID), "School", "School unsaved", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Saved school deleted successfully."})
}

// GetSavedSchools retrieves the montessori professional's saved schools.
// @Summary Get saved schools
// @Description Retrieves the list of schools saved by the montessori professional
// @Tags montessori-professional,schools
// @Produce json
// @Success 200 {array} models.School "List of saved schools"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Montessori Professional profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /montessori-professional/schools/saved [get]
func (h *MontessoriProfessionalHandler) GetSavedSchools(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	var professionalProfile models.MontessoriProfessionalProfile
	if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&professionalProfile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Montessori Professional profile not found."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(professionalProfile.SavedSchools)
}

// ApplyForJob allows a montessori professional to apply for a job.
// @Summary Apply for a job
// @Description Allows a montessori professional to submit an application for a job posting
// @Tags montessori-professional,jobs
// @Accept multipart/form-data
// @Produce json
// @Param job_id path int true "Job ID"
// @Param cover_letter formData string false "Cover letter for the application"
// @Param resume formData file false "Resume file (PDF, DOC, DOCX)"
// @Success 200 {object} map[string]string "Application submitted successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid job ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Montessori Professional profile or job not found"
// @Failure 409 {object} map[string]string "Already applied for this job"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /montessori-professional/jobs/{job_id}/apply [post]
func (h *MontessoriProfessionalHandler) ApplyForJob(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	jobIDStr := c.Params("job_id")
	jobID, err := strconv.ParseUint(jobIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID format"})
	}

	var professionalProfile models.MontessoriProfessionalProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&professionalProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Montessori Professional profile not found. Please complete your profile first."})
	}

	var job models.Job
	if err := h.db.First(&job, uint(jobID)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found."})
	}

	// Check if already applied
	var existingApplication models.JobApplication
	err = h.db.Where("job_id = ? AND montessori_professional_profile_id = ?", uint(jobID), professionalProfile.ID).First(&existingApplication).Error
	if err == nil {
		LogUserAction(h.db, actorUserID, "MONT_PROF_JOB_APPLY_FAIL_ALREADY_APPLIED", uint(jobID), "Job", "Already applied", c)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "You have already applied for this job."})
	} else if err != gorm.ErrRecordNotFound {
		LogUserAction(h.db, actorUserID, "MONT_PROF_JOB_APPLY_FAIL_DB_CHECK", uint(jobID), "Job", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error checking existing applications: " + err.Error()})
	}

	// Parse form
	coverLetter := c.FormValue("cover_letter")

	// Handle resume upload (if provided)
	resumeURL := ""
	file, err := c.FormFile("resume")
	if err == nil && file != nil {
		// TODO: Implement file upload logic (save to disk/cloud and get URL)
		// For now, just use the filename as placeholder
		resumeURL = "/uploads/resumes/" + file.Filename
		// In a real implementation, you'd save the file and return the actual URL
	}

	application := models.JobApplication{
		JobID:                           uint(jobID),
		MontessoriProfessionalProfileID: professionalProfile.ID,
		CoverLetter:                     coverLetter,
		ResumeURL:                       resumeURL,
		Status:                          "pending",
	}

	if err := h.db.Create(&application).Error; err != nil {
		LogUserAction(h.db, actorUserID, "MONT_PROF_JOB_APPLY_FAIL_CREATE", uint(jobID), "JobApplication", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to submit application: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "MONT_PROF_JOB_APPLY_SUCCESS", uint(jobID), "JobApplication", "Application submitted", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Application submitted successfully."})
}

// GetAppliedJobs retrieves jobs the montessori professional has applied for.
// @Summary Get applied jobs
// @Description Retrieves the list of jobs the montessori professional has applied for
// @Tags montessori-professional,jobs
// @Produce json
// @Success 200 {array} models.JobApplication "List of job applications"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Montessori Professional profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /montessori-professional/jobs/applied [get]
func (h *MontessoriProfessionalHandler) GetAppliedJobs(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	var professionalProfile models.MontessoriProfessionalProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&professionalProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Montessori Professional profile not found."})
	}

	var applications []models.JobApplication
	if err := h.db.Preload("Job").Preload("Job.InstitutionProfile").Where("montessori_professional_profile_id = ?", professionalProfile.ID).Find(&applications).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve applications: " + err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(applications)
}