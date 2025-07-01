package handlers

import (
	"encoding/json"
	"fmt" // For LogUserAction details
	"log"
	"mime/multipart"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"strconv" // For parsing IDs
	"strings" // For string operations like ToUpper

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// AdminHandler handles admin-specific requests.
type AdminHandler struct {
	db        *gorm.DB
	mqService queue.MessageQueueService
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(db *gorm.DB, mq queue.MessageQueueService) *AdminHandler {
	return &AdminHandler{db: db, mqService: mq}
}

// SchoolUploadData represents the structure of a school in the JSON file.
type SchoolUploadData struct {
	Name         string `json:"name" validate:"required"`
	Address      string `json:"address"`
	City         string `json:"city"`
	State        string `json:"state"`
	CountryCode  string `json:"country_code" validate:"required"`
	ZipCode      string `json:"zip_code"`
	ContactEmail string `json:"contact_email" validate:"email"`
	ContactPhone string `json:"contact_phone"`
	Website      string `json:"website" validate:"url"`
}

// BatchUploadSchools handles batch uploading of schools from a JSON file.
// @Summary Batch upload schools
// @Description Upload multiple schools from a JSON file
// @Tags admin,schools
// @Accept multipart/form-data
// @Produce json
// @Param schools_file formData file true "JSON file containing school data"
// @Param countryCode query string false "ISO country code (e.g., US, UK, CA) to filter schools by country"
// @Param country_code_filter query string false "Alternative parameter name for ISO country code filter (same as countryCode)"
// @Success 200 {object} map[string]interface{} "Schools uploaded successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/schools/batch-upload [post]
func (h *AdminHandler) BatchUploadSchools(c *fiber.Ctx) error {
	adminUserID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User ID not found in token"})
	}

	// Get the countryCode parameter if provided
	countryCode := c.Query("countryCode")

	// Also check for country_code_filter parameter for compatibility
	if countryCode == "" {
		countryCode = c.Query("country_code_filter")
	}

	file, err := c.FormFile("schools_file")
	if err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_BATCH_UPLOAD_FAIL_FILE", 0, "System", "Failed to get file: "+err.Error(), c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to get file: " + err.Error()})
	}

	if file.Header.Get("Content-Type") != "application/json" {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_BATCH_UPLOAD_FAIL_TYPE", 0, "System", "Invalid file type", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid file type. Only JSON is accepted."})
	}

	openedFile, err := file.Open()
	if err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_BATCH_UPLOAD_FAIL_OPEN", 0, "System", "Failed to open file: "+err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to open file: " + err.Error()})
	}
	defer func(openedFile multipart.File) {
		err := openedFile.Close()
		if err != nil {
			log.Printf("Error closing uploaded file: %v", err)
		}
	}(openedFile)

	var schoolsData []SchoolUploadData
	if err := json.NewDecoder(openedFile).Decode(&schoolsData); err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_BATCH_UPLOAD_FAIL_PARSE", 0, "System", "Failed to parse JSON: "+err.Error(), c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse JSON file: " + err.Error()})
	}

	if len(schoolsData) == 0 {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_BATCH_UPLOAD_FAIL_EMPTY", 0, "System", "No school data in file", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "No school data found in the file."})
	}

	var schoolsToCreate []models.School
	// var createdCount int // Not needed if using GORM's return
	var operationErrors []string

	for _, data := range schoolsData {
		// If countryCode is provided, filter schools by country code
		// Use case-insensitive comparison for country codes
		if countryCode != "" && strings.ToUpper(data.CountryCode) != strings.ToUpper(countryCode) {
			continue
		}

		// TODO: Add validation for each SchoolUploadData item
		// validate := validator.New()
		// if err := validate.Struct(data); err != nil { ... }
		school := models.School{
			Name:            data.Name,
			Address:         data.Address,
			City:            data.City,
			State:           data.State,
			CountryCode:     data.CountryCode,
			ZipCode:         data.ZipCode,
			ContactEmail:    data.ContactEmail,
			ContactPhone:    data.ContactPhone,
			Website:         data.Website,
			UploadedByAdmin: true,
			CreatedByUserID: &adminUserID,
		}
		schoolsToCreate = append(schoolsToCreate, school)
	}

	var createdCount int64 = 0
	if len(schoolsToCreate) > 0 {
		result := h.db.Create(&schoolsToCreate) // GORM creates records and populates their IDs
		if result.Error != nil {
			operationErrors = append(operationErrors, "Failed to batch insert schools: "+result.Error.Error())
			log.Printf("Error batch inserting schools: %v", result.Error)
		}
		createdCount = result.RowsAffected
	}

	actionDetail := map[string]interface{}{
		"file_name":       file.Filename,
		"attempted_count": len(schoolsData),
		"created_count":   createdCount,
		"errors":          operationErrors,
	}

	// Add countryCode to action log if it was provided
	if countryCode != "" {
		actionDetail["countryCode"] = countryCode
	}
	detailJson, _ := json.Marshal(actionDetail)
	LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_BATCH_UPLOAD_COMPLETE", 0, "System", string(detailJson), c)

	if len(operationErrors) > 0 {
		response := fiber.Map{
			"message":       "Batch upload partially completed with errors.",
			"created_count": createdCount,
			"errors":        operationErrors,
		}

		// Add countryCode to response if it was provided
		if countryCode != "" {
			response["countryCode"] = countryCode
		}

		return c.Status(fiber.StatusMultiStatus).JSON(response)
	}

	response := fiber.Map{
		"message":       "Schools batch uploaded successfully.",
		"created_count": createdCount,
	}

	// Add countryCode to response if it was provided
	if countryCode != "" {
		response["countryCode"] = countryCode
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

// UpdateSchool updates an existing school.
// @Summary Update school information
// @Description Updates an existing school's information
// @Tags admin,schools
// @Accept json
// @Produce json
// @Param id path int true "School ID"
// @Param school body SchoolUploadData true "Updated school information"
// @Success 200 {object} models.School "School updated successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "School not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/schools/{id} [put]
func (h *AdminHandler) UpdateSchool(c *fiber.Ctx) error {
	adminUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	var school models.School
	if err := h.db.First(&school, uint(schoolID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}

	var updateData SchoolUploadData
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON: " + err.Error()})
	}
	// TODO: Validate updateData

	school.Name = updateData.Name
	school.Address = updateData.Address
	school.City = updateData.City
	school.State = updateData.State
	school.CountryCode = updateData.CountryCode
	school.ZipCode = updateData.ZipCode
	school.ContactEmail = updateData.ContactEmail
	school.ContactPhone = updateData.ContactPhone
	school.Website = updateData.Website
	// school.UploadedByAdmin remains true, or could be updatable
	// school.CreatedByUserID should ideally not change, or track updater

	if err := h.db.Save(&school).Error; err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_UPDATE_FAIL", uint(schoolID), "School", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update school: " + err.Error()})
	}

	LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_UPDATE_SUCCESS", uint(schoolID), "School", "School updated successfully", c)
	return c.Status(fiber.StatusOK).JSON(school)
}

// GetSchoolsByCountry retrieves schools filtered by country code.
// @Summary Get schools by country
// @Description Retrieves a list of schools filtered by country code with pagination
// @Tags admin,schools
// @Produce json
// @Param country_code query string true "Country code (e.g., US, UK)"
// @Param page query int false "Page number for pagination" default(1)
// @Param limit query int false "Number of items per page" default(10)
// @Success 200 {object} map[string]interface{} "List of schools with pagination metadata"
// @Failure 400 {object} map[string]string "Bad request - missing country_code"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/schools [get]
func (h *AdminHandler) GetSchoolsByCountry(c *fiber.Ctx) error {
	countryCode := c.Query("country_code")
	if countryCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "country_code query parameter is required"})
	}

	var schools []models.School
	// Add pagination
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	query := h.db.Where("country_code = ?", countryCode).Offset(offset).Limit(limit)

	if err := query.Find(&schools).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}

	var total int64
	h.db.Model(&models.School{}).Where("country_code = ?", countryCode).Count(&total)

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

// DeleteSchool deletes a school by ID.
// @Summary Delete a school
// @Description Deletes a school by its ID
// @Tags admin,schools
// @Produce json
// @Param id path int true "School ID"
// @Success 200 {object} map[string]string "School deleted successfully"
// @Failure 400 {object} map[string]string "Bad request or invalid school ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "School not found"
// @Failure 409 {object} map[string]string "Conflict - school linked to institutions"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/schools/{id} [delete]
func (h *AdminHandler) DeleteSchool(c *fiber.Ctx) error {
	adminUserID, _ := c.Locals("user_id").(uint)
	schoolIDStr := c.Params("id")
	schoolID, err := strconv.ParseUint(schoolIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID format"})
	}

	// Check if any institution is linked to this school
	var institutionProfileCount int64
	h.db.Model(&models.InstitutionProfile{}).Where("school_id = ?", schoolID).Count(&institutionProfileCount)
	if institutionProfileCount > 0 {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_DELETE_FAIL_LINKED", uint(schoolID), "School", "School linked to institution(s)", c)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": fmt.Sprintf("Cannot delete school. It is currently linked to %d institution(s).", institutionProfileCount)})
	}

	// GORM's default Delete is a soft delete if gorm.DeletedAt field exists in the model.
	// School model has gorm.Model, so it supports soft delete.
	result := h.db.Delete(&models.School{}, uint(schoolID))
	if result.Error != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_DELETE_FAIL_DB", uint(schoolID), "School", result.Error.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete school: " + result.Error.Error()})
	}
	if result.RowsAffected == 0 {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_DELETE_FAIL_NOTFOUND", uint(schoolID), "School", "School not found", c)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found or already deleted."})
	}

	LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_DELETE_SUCCESS", uint(schoolID), "School", "School deleted successfully", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "School deleted successfully"})
}

// GetAllUsers retrieves all users (admin only).
// @Summary Get all users
// @Description Retrieves a list of all users with pagination
// @Tags admin,users
// @Produce json
// @Param page query int false "Page number for pagination" default(1)
// @Param limit query int false "Number of items per page" default(10)
// @Success 200 {object} map[string]interface{} "List of users with pagination metadata"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/users [get]
func (h *AdminHandler) GetAllUsers(c *fiber.Ctx) error {
	var users []models.User
	// Preload profiles for more detailed user info if needed, e.g., h.db.Preload("InstitutionProfile").Find(&users)
	// Add pagination
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	if err := h.db.Offset(offset).Limit(limit).Order("created_at desc").Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve users: " + err.Error()})
	}

	var total int64
	h.db.Model(&models.User{}).Count(&total)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": users,
		"meta": fiber.Map{
			"total":     total,
			"page":      page,
			"limit":     limit,
			"last_page": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// UserStatusUpdateRequest for updating user's active status
type UserStatusUpdateRequest struct {
	IsActive bool `json:"is_active"`
}

// UpdateUserStatus allows admin to activate/deactivate a user.
// @Summary Update user active status
// @Description Activates or deactivates a user account
// @Tags admin,users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param status body UserStatusUpdateRequest true "User status update information"
// @Success 200 {object} map[string]interface{} "User status updated successfully"
// @Failure 400 {object} map[string]string "Bad request, invalid user ID, or admin trying to change own status"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/users/{id}/status [put]
func (h *AdminHandler) UpdateUserStatus(c *fiber.Ctx) error {
	adminUserID, _ := c.Locals("user_id").(uint)
	targetUserIDStr := c.Params("id")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID format"})
	}

	if uint(targetUserID) == adminUserID {
		LogUserAction(h.db, adminUserID, "ADMIN_USER_STATUS_FAIL_SELF", uint(targetUserID), "User", "Admin tried to change own status", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Admin cannot change their own active status."})
	}

	var req UserStatusUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body: " + err.Error()})
	}

	var user models.User
	if err := h.db.First(&user, uint(targetUserID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error: " + err.Error()})
	}

	user.IsActive = req.IsActive
	if err := h.db.Save(&user).Error; err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_USER_STATUS_FAIL_DB", uint(targetUserID), "User", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update user status: " + err.Error()})
	}

	status := "activated"
	if !req.IsActive {
		status = "deactivated"
	}
	LogUserAction(h.db, adminUserID, "ADMIN_USER_STATUS_SUCCESS", uint(targetUserID), "User", fmt.Sprintf("User %s", status), c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": fmt.Sprintf("User %s successfully.", status), "user": user})
}

// UserRoleUpdateRequest for updating user's role
type UserRoleUpdateRequest struct {
	Role models.UserRole `json:"role" validate:"required,oneof=institution educator parent training_center admin"`
}

// UpdateUserRole allows admin to change a user's role.
// @Summary Update user role
// @Description Changes a user's role (admin only)
// @Tags admin,users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param role body UserRoleUpdateRequest true "User role update information"
// @Success 200 {object} map[string]interface{} "User role updated successfully"
// @Failure 400 {object} map[string]string "Bad request, invalid user ID, or admin trying to change own role"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/users/{id}/role [put]
func (h *AdminHandler) UpdateUserRole(c *fiber.Ctx) error {
	adminUserID, _ := c.Locals("user_id").(uint)
	targetUserIDStr := c.Params("id")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID format"})
	}

	if uint(targetUserID) == adminUserID {
		LogUserAction(h.db, adminUserID, "ADMIN_USER_ROLE_FAIL_SELF", uint(targetUserID), "User", "Admin tried to change own role", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Admin cannot change their own role via this endpoint."})
	}

	var req UserRoleUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body: " + err.Error()})
	}
	// TODO: Validate req.Role to ensure it's a valid role

	var user models.User
	if err := h.db.First(&user, uint(targetUserID)).Error; err != nil {
		// ... (error handling as above)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	// Caution: Changing roles can have significant implications.
	// May need to handle associated profiles (e.g., delete old profile, create new one if structure differs).
	// For simplicity, this example only changes the role field.
	// A more robust implementation would involve a transaction and profile management.
	oldRole := user.Role
	user.Role = req.Role
	if err := h.db.Save(&user).Error; err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_USER_ROLE_FAIL_DB", uint(targetUserID), "User", err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update user role: " + err.Error()})
	}

	LogUserAction(h.db, adminUserID, "ADMIN_USER_ROLE_SUCCESS", uint(targetUserID), "User", fmt.Sprintf("User role changed from %s to %s", oldRole, req.Role), c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "User role updated successfully.", "user": user})
}

// DeleteUser allows admin to delete a user (soft delete).
// @Summary Delete a user
// @Description Soft deletes a user account (admin only)
// @Tags admin,users
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} map[string]string "User deleted successfully"
// @Failure 400 {object} map[string]string "Bad request, invalid user ID, or admin trying to delete themselves"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/users/{id} [delete]
func (h *AdminHandler) DeleteUser(c *fiber.Ctx) error {
	adminUserID, _ := c.Locals("user_id").(uint)
	targetUserIDStr := c.Params("id")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID format"})
	}

	if uint(targetUserID) == adminUserID {
		LogUserAction(h.db, adminUserID, "ADMIN_USER_DELETE_FAIL_SELF", uint(targetUserID), "User", "Admin tried to delete self", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Admin cannot delete themselves."})
	}

	// Soft delete. GORM handles this if `gorm.Model` is used.
	// Associated profiles might need cascading deletes or manual cleanup depending on constraints.
	// The `constraint:OnUpdate:CASCADE,OnDelete:SET NULL` in User model for profiles handles this by setting UserID to NULL.
	result := h.db.Delete(&models.User{}, uint(targetUserID))
	if result.Error != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_USER_DELETE_FAIL_DB", uint(targetUserID), "User", result.Error.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete user: " + result.Error.Error()})
	}
	if result.RowsAffected == 0 {
		LogUserAction(h.db, adminUserID, "ADMIN_USER_DELETE_FAIL_NOTFOUND", uint(targetUserID), "User", "User not found", c)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found or already deleted."})
	}

	LogUserAction(h.db, adminUserID, "ADMIN_USER_DELETE_SUCCESS", uint(targetUserID), "User", "User deleted successfully", c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "User deleted successfully."})
}

// GetActionLogs retrieves action logs (admin only).
// @Summary Get action logs
// @Description Retrieves a list of action logs with pagination and filtering
// @Tags admin,logs
// @Produce json
// @Param page query int false "Page number for pagination" default(1)
// @Param limit query int false "Number of items per page" default(20)
// @Param user_id query int false "Filter logs by user ID"
// @Param action_type query string false "Filter logs by action type"
// @Success 200 {object} map[string]interface{} "List of action logs with pagination metadata"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/action-logs [get]
func (h *AdminHandler) GetActionLogs(c *fiber.Ctx) error {
	var logs []models.ActionLog
	// Add pagination and filtering as needed
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20")) // Default limit
	offset := (page - 1) * limit

	query := h.db.Model(&models.ActionLog{}).Preload("User") // Preload User for context

	// Optional filters
	if userIDFilter := c.Query("user_id"); userIDFilter != "" {
		uid, err := strconv.ParseUint(userIDFilter, 10, 32)
		if err == nil {
			query = query.Where("user_id = ?", uint(uid))
		}
	}
	if actionTypeFilter := c.Query("action_type"); actionTypeFilter != "" {
		query = query.Where("LOWER(action_type) LIKE LOWER(?)", "%"+actionTypeFilter+"%")
	}

	if err := query.Order("performed_at desc").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve action logs: " + err.Error()})
	}

	var total int64
	// Apply same filters for count
	countQuery := h.db.Model(&models.ActionLog{})
	if userIDFilter := c.Query("user_id"); userIDFilter != "" {
		uid, err := strconv.ParseUint(userIDFilter, 10, 32)
		if err == nil {
			countQuery = countQuery.Where("user_id = ?", uint(uid))
		}
	}
	if actionTypeFilter := c.Query("action_type"); actionTypeFilter != "" {
		countQuery = countQuery.Where("LOWER(action_type) LIKE LOWER(?)", "%"+actionTypeFilter+"%")
	}
	countQuery.Count(&total)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": logs,
		"meta": fiber.Map{
			"total":     total,
			"page":      page,
			"limit":     limit,
			"last_page": (total + int64(limit) - 1) / int64(limit),
		},
	})
}
