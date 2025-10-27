package handlers

import (
	"mwc_backend/internal/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// SavedSchoolHandler provides unified CRUD for saved schools across user roles.
// Currently supports Parent and Montessori Professional roles.
// Routes are mounted under /api/v1/me/schools/saved
//
// Note: Data storage remains in role-specific many2many tables defined in models.
// This handler abstracts that difference behind a single API for clients.

type SavedSchoolHandler struct {
	db *gorm.DB
}

func NewSavedSchoolHandler(db *gorm.DB) *SavedSchoolHandler {
	return &SavedSchoolHandler{db: db}
}

// SaveSchool saves a school for the current user regardless of role (parent or montessori professional).
// @Summary Save a school (unified)
// @Description Adds a school to the current user's saved schools list (supports parent and montessori professional)
// @Tags users,schools
// @Produce json
// @Param school_id path int true "School ID"
// @Success 200 {object} map[string]string "School saved successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "User profile or school not found"
// @Failure 409 {object} map[string]string "School already saved"
// @Failure 422 {object} map[string]string "Role not supported for saved schools"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /me/schools/saved/{school_id} [post]
func (h *SavedSchoolHandler) SaveSchool(c *fiber.Ctx) error {
	actorUserID, ok := c.Locals("user_id").(uint)
	if !ok || actorUserID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Load user and role
	var user models.User
	if err := h.db.First(&user, actorUserID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	schoolIDStr := c.Params("school_id")
	schoolID64, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}
	schoolID := uint(schoolID64)

	var school models.School
	if err := h.db.First(&school, schoolID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found"})
	}

	switch user.Role {
	case models.ParentRole:
		var parentProfile models.ParentProfile
		if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&parentProfile).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Parent profile not found."})
		}
		for _, s := range parentProfile.SavedSchools {
			if s.ID == schoolID {
				LogUserAction(h.db, actorUserID, "USER_SCHOOL_SAVE_DUPLICATE", schoolID, "School", "Already saved", c)
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "School already saved."})
			}
		}
		if err := h.db.Model(&parentProfile).Association("SavedSchools").Append(&school); err != nil {
			LogUserAction(h.db, actorUserID, "USER_SCHOOL_SAVE_FAIL_DB", schoolID, "School", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save school: " + err.Error()})
		}
		LogUserAction(h.db, actorUserID, "USER_SCHOOL_SAVE_SUCCESS", schoolID, "School", "School saved", c)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "School saved successfully."})

	case models.MontessoriProfessionalRole:
		var prof models.MontessoriProfessionalProfile
		if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&prof).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Montessori Professional profile not found."})
		}
		for _, s := range prof.SavedSchools {
			if s.ID == schoolID {
				LogUserAction(h.db, actorUserID, "USER_SCHOOL_SAVE_DUPLICATE", schoolID, "School", "Already saved", c)
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "School already saved."})
			}
		}
		if err := h.db.Model(&prof).Association("SavedSchools").Append(&school); err != nil {
			LogUserAction(h.db, actorUserID, "USER_SCHOOL_SAVE_FAIL_DB", schoolID, "School", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save school: " + err.Error()})
		}
		LogUserAction(h.db, actorUserID, "USER_SCHOOL_SAVE_SUCCESS", schoolID, "School", "School saved", c)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "School saved successfully."})
	default:
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "This user role does not support saved schools."})
	}
}

// DeleteSavedSchool removes a school from the current user's saved list.
// @Summary Unsave a school (unified)
// @Description Removes a school from the current user's saved schools list (supports parent and montessori professional)
// @Tags users,schools
// @Produce json
// @Param school_id path int true "School ID"
// @Success 200 {object} map[string]string "Saved school deleted successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "User profile or school not found"
// @Failure 422 {object} map[string]string "Role not supported for saved schools"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /me/schools/saved/{school_id} [delete]
func (h *SavedSchoolHandler) DeleteSavedSchool(c *fiber.Ctx) error {
	actorUserID, ok := c.Locals("user_id").(uint)
	if !ok || actorUserID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var user models.User
	if err := h.db.First(&user, actorUserID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	schoolIDStr := c.Params("school_id")
	sid, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}
	schoolID := uint(sid)

	var school models.School
	if err := h.db.First(&school, schoolID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found."})
	}

	switch user.Role {
	case models.ParentRole:
		var parentProfile models.ParentProfile
		if err := h.db.Where("user_id = ?", actorUserID).First(&parentProfile).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Parent profile not found."})
		}
		if err := h.db.Model(&parentProfile).Association("SavedSchools").Delete(&school); err != nil {
			LogUserAction(h.db, actorUserID, "USER_SCHOOL_UNSAVE_FAIL_DB", schoolID, "School", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete saved school: " + err.Error()})
		}
		LogUserAction(h.db, actorUserID, "USER_SCHOOL_UNSAVE_SUCCESS", schoolID, "School", "School unsaved", c)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Saved school deleted successfully."})
	case models.MontessoriProfessionalRole:
		var prof models.MontessoriProfessionalProfile
		if err := h.db.Where("user_id = ?", actorUserID).First(&prof).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Montessori Professional profile not found."})
		}
		if err := h.db.Model(&prof).Association("SavedSchools").Delete(&school); err != nil {
			LogUserAction(h.db, actorUserID, "USER_SCHOOL_UNSAVE_FAIL_DB", schoolID, "School", err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete saved school: " + err.Error()})
		}
		LogUserAction(h.db, actorUserID, "USER_SCHOOL_UNSAVE_SUCCESS", schoolID, "School", "School unsaved", c)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Saved school deleted successfully."})
	default:
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "This user role does not support saved schools."})
	}
}

// GetSavedSchools returns the current user's saved schools.
// @Summary Get saved schools (unified)
// @Description Retrieves the list of schools saved by the current user (supports parent and montessori professional)
// @Tags users,schools
// @Produce json
// @Success 200 {array} models.School "List of saved schools"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "User profile not found"
// @Failure 422 {object} map[string]string "Role not supported for saved schools"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /me/schools/saved [get]
func (h *SavedSchoolHandler) GetSavedSchools(c *fiber.Ctx) error {
	actorUserID, ok := c.Locals("user_id").(uint)
	if !ok || actorUserID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var user models.User
	if err := h.db.First(&user, actorUserID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	switch user.Role {
	case models.ParentRole:
		var parentProfile models.ParentProfile
		if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&parentProfile).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Parent profile not found."})
		}
		return c.Status(fiber.StatusOK).JSON(parentProfile.SavedSchools)
	case models.MontessoriProfessionalRole:
		var prof models.MontessoriProfessionalProfile
		if err := h.db.Preload("SavedSchools").Where("user_id = ?", actorUserID).First(&prof).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Montessori Professional profile not found."})
		}
		return c.Status(fiber.StatusOK).JSON(prof.SavedSchools)
	default:
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "This user role does not support saved schools."})
	}
}
