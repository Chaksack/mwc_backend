package handlers

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"strconv"
)

type EducatorHandler struct {
	db        *gorm.DB
	mqService queue.MessageQueueService
}

func NewEducatorHandler(db *gorm.DB, mq queue.MessageQueueService) *EducatorHandler {
	return &EducatorHandler{db: db, mqService: mq}
}

type EducatorProfileRequest struct {
	Bio            string `json:"bio"`
	Qualifications string `json:"qualifications"`
	Experience     string `json:"experience"`
}

type JobApplicationRequest struct {
	CoverLetter string `json:"cover_letter"`
	ResumeURL   string `json:"resume_url" validate:"omitempty,url"` // Optional, but if provided, must be URL
}

func (h *EducatorHandler) CreateOrUpdateEducatorProfile(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	req := new(EducatorProfileRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate req

	var profile models.EducatorProfile
	err := h.db.Where("user_id = ?", actorUserID).First(&profile).Error
	isNewProfile := false
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			isNewProfile = true
			profile.UserID = actorUserID
		} else {
			LogUserAction(h.db, actorUserID, "EDU_PROFILE_FETCH_FAIL", actorUserID, "EducatorProfile", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
		}
	}

	profile.Bio = req.Bio
	profile.Qualifications = req.Qualifications
	profile.Experience = req.Experience

	if err := h.db.Save(&profile).Error; err != nil {
		actionType := "EDU_PROFILE_UPDATE_FAIL"
		if isNewProfile {
			actionType = "EDU_PROFILE_CREATE_FAIL"
		}
		LogUserAction(h.db, actorUserID, actionType, profile.ID, "EducatorProfile", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save educator profile: " + err.Error()})
	}

	actionType := "EDU_PROFILE_UPDATE_SUCCESS"
	if isNewProfile {
		actionType = "EDU_PROFILE_CREATE_SUCCESS"
	}
	LogUserAction(h.db, actorUserID, actionType, profile.ID, "EducatorProfile", "Profile saved", c)
	return c.Status(fiber.StatusOK).JSON(profile)
}

// SearchSchools allows educators (and parents) to search for schools.
func (h *EducatorHandler) SearchSchools(c *fiber.Ctx) error {
	// This handler is identical to GetPublicSchools, can be aliased or refactored.
	// For now, just calling the shared one.
	return GetPublicSchools(h.db)(c)
}

// SaveSchool allows an educator to save a school to their list.
func (h *EducatorHandler) SaveSchool(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("school_id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	var educatorProfile models.EducatorProfile
	if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&educatorProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Educator profile not found."})
	}

	var school models.School
	if err := h.db.First(&school, uint(schoolID)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found."})
	}

	// Check if already saved
	for _, savedSchool := range educatorProfile.SavedSchools {
		if savedSchool.ID == uint(schoolID) {
			LogUserAction(h.db, actorUserID, "EDU_SCHOOL_SAVE_FAIL_ALREADY_SAVED", uint(schoolID), "School", "School already saved", c)
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "School already saved."})
		}
	}

	if err := h.db.Model(&educatorProfile).Association("SavedSchools").Append(&school); err != nil {
		LogUserAction(h.db, actorUserID, "EDU_SCHOOL_SAVE_FAIL_DB", uint(schoolID), "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save school: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "EDU_SCHOOL_SAVE_SUCCESS", uint(schoolID), "School", "School saved", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "School saved successfully."})
}

// DeleteSavedSchool allows an educator to remove a school from their saved list.
func (h *EducatorHandler) DeleteSavedSchool(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("school_id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	var educatorProfile models.EducatorProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&educatorProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Educator profile not found."})
	}

	var school models.School
	if err := h.db.First(&school, uint(schoolID)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found."})
	}

	if err := h.db.Model(&educatorProfile).Association("SavedSchools").Delete(&school); err != nil {
		LogUserAction(h.db, actorUserID, "EDU_SCHOOL_UNSAVE_FAIL_DB", uint(schoolID), "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete saved school: " + err.Error()})
	}
	// GORM's Delete for associations might not return error if item wasn't associated.
	// Check RowsAffected if precise feedback is needed.

	LogUserAction(h.db, actorUserID, "EDU_SCHOOL_UNSAVE_SUCCESS", uint(schoolID), "School", "School unsaved", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Saved school deleted successfully."})
}

// GetSavedSchools retrieves the educator's saved schools.
func (h *EducatorHandler) GetSavedSchools(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	var educatorProfile models.EducatorProfile
	if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&educatorProfile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Educator profile not found."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(educatorProfile.SavedSchools)
}

// ApplyForJob allows an educator to apply for a job.
func (h *EducatorHandler) ApplyForJob(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)
	jobIDStr := c.Params("job_id")
	jobID, err := strconv.ParseUint(jobIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID format"})
	}

	var educatorProfile models.EducatorProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&educatorProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Educator profile not found. Please complete your profile first."})
	}

	var job models.Job
	if err := h.db.Where("id = ? AND is_active = ?", uint(jobID), true).First(&job).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Active job not found."})
	}

	// Check if already applied
	var existingApplication models.JobApplication
	errCheck := h.db.Where("job_id = ? AND educator_profile_id = ?", uint(jobID), educatorProfile.ID).First(&existingApplication).Error
	if errCheck == nil { // Application found
		LogUserAction(h.db, actorUserID, "EDU_JOB_APPLY_FAIL_ALREADY_APPLIED", uint(jobID), "Job", "Already applied", c)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "You have already applied for this job."})
	}
	if errCheck != gorm.ErrRecordNotFound { // Some other DB error
		LogUserAction(h.db, actorUserID, "EDU_JOB_APPLY_FAIL_DB_CHECK", uint(jobID), "Job", errCheck.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error checking existing application: " + errCheck.Error()})
	}

	req := new(JobApplicationRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate req (e.g. ResumeURL if provided)

	application := models.JobApplication{
		JobID:             uint(jobID),
		EducatorProfileID: educatorProfile.ID,
		CoverLetter:       req.CoverLetter,
		ResumeURL:         req.ResumeURL,
		Status:            "pending", // Initial status
	}

	if err := h.db.Create(&application).Error; err != nil {
		LogUserAction(h.db, actorUserID, "EDU_JOB_APPLY_FAIL_DB_CREATE", uint(jobID), "JobApplication", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to submit application: " + err.Error()})
	}

	LogUserAction(h.db, actorUserID, "EDU_JOB_APPLY_SUCCESS", uint(jobID), "JobApplication", "Application submitted", c)
	// TODO: Notify institution about new application (e.g., via email or RabbitMQ)
	return c.Status(fiber.StatusCreated).JSON(application)
}

// GetAppliedJobs retrieves all jobs an educator has applied for.
func (h *EducatorHandler) GetAppliedJobs(c *fiber.Ctx) error {
	actorUserID, _ := c.Locals("user_id").(uint)

	var educatorProfile models.EducatorProfile
	if err := h.db.Where("user_id = ?", actorUserID).First(&educatorProfile).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Educator profile not found."})
	}

	var applications []models.JobApplication
	// Preload Job details, and within Job, the InstitutionProfile and its User (for institution name) and School
	if err := h.db.
		Preload("Job.InstitutionProfile.User").
		Preload("Job.InstitutionProfile.School").
		Where("educator_profile_id = ?", educatorProfile.ID).
		Order("created_at desc").
		Find(&applications).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve applied jobs: " + err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(applications)
}
