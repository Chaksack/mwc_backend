package handlers

import (
	"encoding/json"
	"fmt" // For LogUserAction details
	"log"
	"mime/multipart"
	"mwc_backend/config"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"mwc_backend/internal/utils"
	"os"
	"strconv" // For parsing IDs
	"strings" // For string operations like ToUpper
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/price"
	"github.com/stripe/stripe-go/v72/product"
	"github.com/stripe/stripe-go/v72/sub"
)

// AdminHandler handles admin-specific requests.
type AdminHandler struct {
	db        *gorm.DB
	mqService queue.MessageQueueService
	cfg       *config.Config
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(db *gorm.DB, mq queue.MessageQueueService, cfg *config.Config) *AdminHandler {
	// Initialize Stripe key once if provided in config
	if cfg != nil && cfg.StripeSecretKey != "" {
		stripe.Key = cfg.StripeSecretKey
	}
	return &AdminHandler{db: db, mqService: mq, cfg: cfg}
}

// SchoolUploadData represents the structure of a school in the JSON file.
type SchoolUploadData struct {
	Rank              int     `json:"rank"`
	Title             string  `json:"title" validate:"required"`
	Price             *string `json:"price"`
	CategoryName      string  `json:"categoryName"`
	Address           string  `json:"address"`
	Neighborhood      *string `json:"neighborhood"`
	Street            string  `json:"street"`
	City              string  `json:"city"`
	PostalCode        *string `json:"postalCode"`
	State             *string `json:"state"`
	CountryCode       string  `json:"countryCode" validate:"required"`
	Country           string  `json:"country" validate:"required"`
	Phone             string  `json:"phone"`
	PhoneUnformatted  string  `json:"phoneUnformatted"`
	ClaimThisBusiness bool    `json:"claimThisBusiness"`
	Location          struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"location"`
	PermanentlyClosed bool     `json:"permanentlyClosed"`
	TemporarilyClosed bool     `json:"temporarilyClosed"`
	PlaceID           string   `json:"placeId"`
	Categories        []string `json:"categories"`
	Fid               string   `json:"fid"`
	Cid               string   `json:"cid"`
	ReviewsCount      int      `json:"reviewsCount"`
	ImagesCount       int      `json:"imagesCount"`
	ImageCategories   []string `json:"imageCategories"`
	ScrapedAt         string   `json:"scrapedAt"`
	GoogleFoodUrl     *string  `json:"googleFoodUrl"`
	HotelAds          []string `json:"hotelAds"`
	OpeningHours      []struct {
		Day   string `json:"day"`
		Hours string `json:"hours"`
	} `json:"openingHours"`
	PeopleAlsoSearch interface{} `json:"peopleAlsoSearch"`
	PlacesTags       interface{} `json:"placesTags"`
	ReviewsTags      interface{} `json:"reviewsTags"`
	AdditionalInfo   struct {
		Accessibility []struct {
			Key   string `json:"key"`
			Value bool   `json:"value"`
		} `json:"Accessibility"`
	} `json:"additionalInfo"`
	GasPrices       []string `json:"gasPrices"`
	Url             string   `json:"url"`
	SearchPageUrl   string   `json:"searchPageUrl"`
	SearchString    string   `json:"searchString"`
	Language        string   `json:"language"`
	IsAdvertisement bool     `json:"isAdvertisement"`
	ImageUrl        string   `json:"imageUrl"`
	Kgmid           string   `json:"kgmid"`
}

// BatchUploadSchools handles batch uploading of schools from a JSON file.
// @Summary Batch upload schools
// @Description Upload multiple schools from a JSON file
// @Tags admin,schools
// @Accept multipart/form-data
// @Produce json
// @Param schools_file formData file true "JSON file containing school data"
// @Param countryCode query string false "ISO country code (e.g., US, UK, CA) to filter schools by country"
// @Param country query string false "Country name to filter schools by country"
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

	// Get the country parameter if provided
	country := c.Query("country")

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

	var operationErrors []string
	var createdCount int64 = 0
	var updatedCount int64 = 0

	for _, data := range schoolsData {
		// If countryCode is provided, use it as a default or filter by it
		if countryCode != "" {
			if data.CountryCode == "" {
				// Use the provided countryCode as a default value
				data.CountryCode = countryCode
			} else if strings.ToUpper(data.CountryCode) != strings.ToUpper(countryCode) {
				// Filter out schools that don't match the provided countryCode
				continue
			}
		}

		// If country is provided, use it as a default or filter by it
		if country != "" {
			if data.Country == "" {
				// Use the provided country as a default value
				data.Country = country
			} else if strings.ToLower(data.Country) != strings.ToLower(country) {
				// Filter out schools that don't match the provided country
				continue
			}
		}

		// Validate required fields
		if data.Title == "" {
			operationErrors = append(operationErrors, fmt.Sprintf("School at index %d: Title is required", data.Rank))
			continue
		}
		if data.CountryCode == "" {
			operationErrors = append(operationErrors, fmt.Sprintf("School at index %d: CountryCode is required", data.Rank))
			continue
		}
		if data.Country == "" {
			operationErrors = append(operationErrors, fmt.Sprintf("School at index %d: Country is required", data.Rank))
			continue
		}

		// Map the new data structure to the School model
		var state string
		if data.State != nil {
			state = *data.State
		}

		var postalCode string
		if data.PostalCode != nil {
			postalCode = *data.PostalCode
		}

		school := models.School{
			Name:            data.Title,
			Address:         data.Address,
			City:            data.City,
			State:           state,
			CountryCode:     data.CountryCode,
			Country:         data.Country,
			ZipCode:         postalCode,
			ContactEmail:    "", // Not available in the new structure
			ContactPhone:    data.Phone,
			Website:         data.Url,
			Latitude:        data.Location.Lat,
			Longitude:       data.Location.Lng,
			SearchString:    data.SearchString,
			SearchPageUrl:   data.SearchPageUrl,
			UploadedByAdmin: true,
			CreatedByUserID: &adminUserID,
		}

		// Try to find an existing school to update (match by name + country_code, and city when available)
		var existingSchool models.School
		query := h.db.Where("LOWER(name) = LOWER(?) AND country_code = ?", data.Title, data.CountryCode)
		if data.City != "" {
			query = query.Where("LOWER(city) = LOWER(?)", data.City)
		}
		err := query.First(&existingSchool).Error
		if err == nil {
			// Update existing record - preserve CreatedByUserID if already set
			existingSchool.Name = school.Name
			existingSchool.Address = school.Address
			existingSchool.City = school.City
			existingSchool.State = school.State
			existingSchool.Country = school.Country
			existingSchool.CountryCode = school.CountryCode
			existingSchool.ZipCode = school.ZipCode
			existingSchool.ContactPhone = school.ContactPhone
			existingSchool.Website = school.Website
			existingSchool.Latitude = school.Latitude
			existingSchool.Longitude = school.Longitude
			existingSchool.SearchString = school.SearchString
			existingSchool.SearchPageUrl = school.SearchPageUrl
			existingSchool.UploadedByAdmin = true

			if err := h.db.Save(&existingSchool).Error; err != nil {
				operationErrors = append(operationErrors, fmt.Sprintf("Failed to update existing school '%s': %s", data.Title, err.Error()))
				continue
			}
			updatedCount++
			continue
		}
		if err != gorm.ErrRecordNotFound {
			operationErrors = append(operationErrors, fmt.Sprintf("DB error searching for school '%s': %s", data.Title, err.Error()))
			continue
		}

		// Not found - create new record
		if err := h.db.Create(&school).Error; err != nil {
			operationErrors = append(operationErrors, fmt.Sprintf("Failed to create school '%s': %s", data.Title, err.Error()))
			continue
		}
		createdCount++
	}

	actionDetail := map[string]interface{}{
		"file_name":       file.Filename,
		"attempted_count": len(schoolsData),
		"created_count":   createdCount,
		"updated_count":   updatedCount,
		"errors":          operationErrors,
	}

	// Add countryCode to action log if it was provided
	if countryCode != "" {
		actionDetail["countryCode"] = countryCode
	}

	// Add country to action log if it was provided
	if country != "" {
		actionDetail["country"] = country
	}
	detailJson, _ := json.Marshal(actionDetail)
	LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_BATCH_UPLOAD_COMPLETE", 0, "System", string(detailJson), c)

	if len(operationErrors) > 0 {
		response := fiber.Map{
			"message":       "Batch upload partially completed with errors.",
			"created_count": createdCount,
			"updated_count": updatedCount,
			"errors":        operationErrors,
		}

		// Add countryCode to response if it was provided
		if countryCode != "" {
			response["countryCode"] = countryCode
		}

		// Add country to response if it was provided
		if country != "" {
			response["country"] = country
		}

		return c.Status(fiber.StatusMultiStatus).JSON(response)
	}

	response := fiber.Map{
		"message":       "Schools batch uploaded successfully.",
		"created_count": createdCount,
		"updated_count": updatedCount,
	}

	// Add countryCode to response if it was provided
	if countryCode != "" {
		response["countryCode"] = countryCode
	}

	// Add country to response if it was provided
	if country != "" {
		response["country"] = country
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

	// Map the new data structure to the School model
	school.Name = updateData.Title
	school.Address = updateData.Address
	school.City = updateData.City

	// Handle nullable fields
	if updateData.State != nil {
		school.State = *updateData.State
	} else {
		school.State = ""
	}

	school.CountryCode = updateData.CountryCode

	if updateData.PostalCode != nil {
		school.ZipCode = *updateData.PostalCode
	} else {
		school.ZipCode = ""
	}

	// ContactEmail is not available in the new structure
	school.ContactEmail = ""
	school.ContactPhone = updateData.Phone
	school.Website = updateData.Url
	school.SearchString = updateData.SearchString
	school.SearchPageUrl = updateData.SearchPageUrl
	// Update latitude/longitude if provided
	school.Latitude = updateData.Location.Lat
	school.Longitude = updateData.Location.Lng
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
	Role models.UserRole `json:"role" validate:"required,oneof=institution montessori_professional parent training_center admin"`
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

// ManualSchoolCreationRequest represents the structure for manually creating a school or training center
type ManualSchoolCreationRequest struct {
	Name         string                `json:"name" validate:"required"`
	Category     models.SchoolCategory `json:"category" validate:"required,oneof=school training_center"`
	Address      string                `json:"address"`
	City         string                `json:"city"`
	State        string                `json:"state"`
	CountryCode  string                `json:"country_code" validate:"required"`
	Country      string                `json:"country" validate:"required"`
	ZipCode      string                `json:"zip_code"`
	ContactEmail string                `json:"contact_email" validate:"omitempty,email"`
	ContactPhone string                `json:"contact_phone"`
	Website      string                `json:"website" validate:"omitempty,url"`
}

// CreateSchool allows admin to manually create a school or training center.
// @Summary Create a school or training center
// @Description Manually creates a new school or training center (admin only)
// @Tags admin,schools
// @Accept json
// @Produce json
// @Param school body ManualSchoolCreationRequest true "School or training center information"
// @Success 201 {object} models.School "School created successfully"
// @Failure 400 {object} map[string]string "Bad request or validation error"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/schools/create [post]
func (h *AdminHandler) CreateSchool(c *fiber.Ctx) error {
	adminUserID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User ID not found in token"})
	}

	var req ManualSchoolCreationRequest
	if err := c.BodyParser(&req); err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_CREATE_FAIL_PARSE", 0, "System", "Failed to parse request: "+err.Error(), c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body: " + err.Error()})
	}

	// Validate required fields
	if req.Name == "" {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_CREATE_FAIL_VALIDATION", 0, "System", "School name is required", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "School name is required"})
	}

	if req.CountryCode == "" {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_CREATE_FAIL_VALIDATION", 0, "System", "Country code is required", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Country code is required"})
	}

	if req.Country == "" {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_CREATE_FAIL_VALIDATION", 0, "System", "Country is required", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Country is required"})
	}

	// Validate category
	if req.Category != models.SchoolCategorySchool && req.Category != models.SchoolCategoryTrainingCenter {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_CREATE_FAIL_VALIDATION", 0, "System", "Invalid category", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Category must be either 'school' or 'training_center'"})
	}

	// Create the school
	school := models.School{
		Name:            req.Name,
		Category:        req.Category,
		Address:         req.Address,
		City:            req.City,
		State:           req.State,
		CountryCode:     strings.ToUpper(req.CountryCode), // Ensure uppercase
		Country:         req.Country,
		ZipCode:         req.ZipCode,
		ContactEmail:    req.ContactEmail,
		ContactPhone:    req.ContactPhone,
		Website:         req.Website,
		UploadedByAdmin: true,
		CreatedByUserID: &adminUserID, // Set the admin who created it
	}

	if err := h.db.Create(&school).Error; err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_CREATE_FAIL_DB", 0, "School", "Failed to create school: "+err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create school: " + err.Error()})
	}

	LogUserAction(h.db, adminUserID, "ADMIN_SCHOOL_CREATE_SUCCESS", school.ID, "School", fmt.Sprintf("Manually created %s: %s", req.Category, req.Name), c)
	return c.Status(fiber.StatusCreated).JSON(school)
}

// CreateTrainingCenter is an alias for CreateSchool with training_center category validation.
// @Summary Create a training center
// @Description Manually creates a new training center (admin only) - same as creating a school with training_center category
// @Tags admin,schools,training-centers
// @Accept json
// @Produce json
// @Param training_center body ManualSchoolCreationRequest true "Training center information"
// @Success 201 {object} models.School "Training center created successfully"
// @Failure 400 {object} map[string]string "Bad request or validation error"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/training-centers/create [post]
func (h *AdminHandler) CreateTrainingCenter(c *fiber.Ctx) error {
	// Parse the request first to validate category
	var req ManualSchoolCreationRequest
	if err := c.BodyParser(&req); err != nil {
		adminUserID, _ := c.Locals("user_id").(uint)
		LogUserAction(h.db, adminUserID, "ADMIN_TRAINING_CENTER_CREATE_FAIL_PARSE", 0, "System", "Failed to parse request: "+err.Error(), c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body: " + err.Error()})
	}

	// Force category to be training_center
	req.Category = models.SchoolCategoryTrainingCenter

	// Re-encode the request and call CreateSchool
	c.Request().SetBody(nil) // Clear the body
	if err := c.JSON(req); err != nil {
		adminUserID, _ := c.Locals("user_id").(uint)
		LogUserAction(h.db, adminUserID, "ADMIN_TRAINING_CENTER_CREATE_FAIL_ENCODE", 0, "System", "Failed to re-encode request: "+err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Internal error processing request"})
	}

	// Call the main CreateSchool method
	return h.CreateSchool(c)
}

type CreateAdminRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8"`
	FirstName string `json:"first_name" validate:"required"`
	LastName  string `json:"last_name" validate:"required"`
}

// CreateAdmin allows super admin to create new admin users.
// @Summary Create an admin user
// @Description Creates a new admin user (super admin only)
// @Tags admin,users
// @Accept json
// @Produce json
// @Param admin body CreateAdminRequest true "Admin user information"
// @Success 201 {object} models.User "Admin user created successfully"
// @Failure 400 {object} map[string]string "Bad request or validation error"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 409 {object} map[string]string "Email already exists"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/admins [post]
func (h *AdminHandler) CreateAdmin(c *fiber.Ctx) error {
	superAdminUserID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User ID not found in token"})
	}

	var req CreateAdminRequest
	if err := c.BodyParser(&req); err != nil {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_PARSE", 0, "System", "Failed to parse request: "+err.Error(), c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body: " + err.Error()})
	}

	// Validate required fields
	if req.Email == "" {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_VALIDATION", 0, "System", "Email is required", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email is required"})
	}

	if req.Password == "" {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_VALIDATION", 0, "System", "Password is required", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Password is required"})
	}

	if len(req.Password) < 8 {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_VALIDATION", 0, "System", "Password must be at least 8 characters", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Password must be at least 8 characters"})
	}

	if req.FirstName == "" {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_VALIDATION", 0, "System", "First name is required", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "First name is required"})
	}

	if req.LastName == "" {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_VALIDATION", 0, "System", "Last name is required", c)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Last name is required"})
	}

	// Check if email already exists
	var existingUser models.User
	if err := h.db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_EMAIL_EXISTS", 0, "System", "Email already exists: "+req.Email, c)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Email already exists"})
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_HASH", 0, "System", "Failed to hash password: "+err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to process password"})
	}

	// Create the admin user
	adminUser := models.User{
		Email:         req.Email,
		PasswordHash:  string(hashedPassword),
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Role:          models.AdminRole,
		IsActive:      true,
		EmailVerified: true, // Admin doesn't require email verification
	}

	if err := h.db.Create(&adminUser).Error; err != nil {
		LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_FAIL_DB", 0, "User", "Failed to create admin user: "+err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create admin user: " + err.Error()})
	}

	// Remove password hash from response
	adminUser.PasswordHash = ""

	LogUserAction(h.db, superAdminUserID, "ADMIN_CREATE_SUCCESS", adminUser.ID, "User", fmt.Sprintf("Created admin user: %s %s (%s)", req.FirstName, req.LastName, req.Email), c)
	return c.Status(fiber.StatusCreated).JSON(adminUser)
}

// UpdateAdminProfileRequest is the request body for admin profile updates.
type UpdateAdminProfileRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// UpdateAdminProfile updates admin user profile including profile picture
// @Summary Update admin profile
// @Description Updates the current admin's profile information including optional profile picture
// @Tags admin,profile
// @Accept multipart/form-data
// @Produce json
// @Param first_name formData string false "First name"
// @Param last_name formData string false "Last name"
// @Param email formData string false "Email address"
// @Param profile_picture formData file false "Profile picture file"
// @Success 200 {object} models.User "Profile updated successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 409 {object} map[string]string "Email already exists"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/profile [put]
func (h *AdminHandler) UpdateAdminProfile(c *fiber.Ctx) error {
	adminUserID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User ID not found in token"})
	}

	// Get the current admin user
	var user models.User
	if err := h.db.Preload("ProfilePictures").First(&user, adminUserID).Error; err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_FAIL_FETCH", adminUserID, "User", "Failed to fetch user: "+err.Error(), c)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	// Verify the user is actually an admin
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "This endpoint is only for admin users"})
	}

	// Parse form data
	firstName := c.FormValue("first_name")
	lastName := c.FormValue("last_name")
	email := c.FormValue("email")

	// Update fields if provided
	updated := false
	if firstName != "" && firstName != user.FirstName {
		user.FirstName = firstName
		updated = true
	}
	if lastName != "" && lastName != user.LastName {
		user.LastName = lastName
		updated = true
	}
	if email != "" && email != user.Email {
		// Check if email already exists for another user
		var existingUser models.User
		if err := h.db.Where("email = ? AND id != ?", email, adminUserID).First(&existingUser).Error; err == nil {
			LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_FAIL_EMAIL_EXISTS", adminUserID, "User", "Email already exists: "+email, c)
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Email already exists"})
		}
		user.Email = email
		updated = true
	}

	// Handle profile picture upload if provided
	fileHeader, err := c.FormFile("profile_picture")
	if err == nil && fileHeader != nil {
		// Ensure uploads directory exists
		uploadDir := "./uploads/admin_profiles"
		if err := ensureDir(uploadDir); err != nil {
			LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_FAIL_DIR", adminUserID, "System", "Failed to create upload directory: "+err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create upload directory"})
		}

		// Save file
		dst := fmt.Sprintf("%s/%d_%s", uploadDir, adminUserID, fileHeader.Filename)
		if err := c.SaveFile(fileHeader, dst); err != nil {
			LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_FAIL_SAVE_FILE", adminUserID, "System", "Failed to save file: "+err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save file: " + err.Error()})
		}

		urlPath := "/uploads/admin_profiles/" + fmt.Sprintf("%d_%s", adminUserID, fileHeader.Filename)

		// Set any existing pictures as non-primary
		if err := h.db.Model(&models.UserProfilePicture{}).Where("user_id = ?", adminUserID).Update("is_primary", false).Error; err != nil {
			LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_WARN_PRIMARY", adminUserID, "UserProfilePicture", "Failed to update existing pictures: "+err.Error(), c)
		}

		// Create new profile picture record
		picture := models.UserProfilePicture{
			UserID:    adminUserID,
			URL:       urlPath,
			FileName:  fileHeader.Filename,
			IsPrimary: true,
		}

		if err := h.db.Create(&picture).Error; err != nil {
			LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_FAIL_PICTURE", adminUserID, "UserProfilePicture", "Failed to save picture record: "+err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save picture record: " + err.Error()})
		}

		updated = true
	}

	// Save user updates if any
	if updated {
		if err := h.db.Save(&user).Error; err != nil {
			LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_FAIL_SAVE", adminUserID, "User", "Failed to update user: "+err.Error(), c)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update profile: " + err.Error()})
		}
	}

	// Reload user with profile pictures
	if err := h.db.Preload("ProfilePictures").First(&user, adminUserID).Error; err != nil {
		LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_FAIL_RELOAD", adminUserID, "User", "Failed to reload user: "+err.Error(), c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reload profile"})
	}

	// Remove password hash from response
	user.PasswordHash = ""

	LogUserAction(h.db, adminUserID, "ADMIN_PROFILE_UPDATE_SUCCESS", adminUserID, "User", "Profile updated successfully", c)
	return c.Status(fiber.StatusOK).JSON(user)
}

// Dynamic Subscription Plan Management

type CreateSubscriptionPlanRequest struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Price           float64  `json:"price"`
	Currency        string   `json:"currency"`
	BillingCycle    string   `json:"billing_cycle"`
	Features        []string `json:"features"`
	AllowedRoles    []string `json:"allowed_roles"`
	StripePriceID   string   `json:"stripe_price_id,omitempty"`
	StripeLookupKey string   `json:"stripe_lookup_key,omitempty"`
}

// CreateSubscriptionPlan creates a new dynamic subscription plan
// @Summary Create subscription plan
// @Description Creates a new dynamic subscription plan (admin only)
// @Tags admin,subscriptions
// @Accept json
// @Produce json
// @Param request body CreateSubscriptionPlanRequest true "Subscription plan information"
// @Success 201 {object} map[string]interface{} "Subscription plan created successfully"
// @Failure 400 {object} map[string]string "Bad request or validation error"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/subscription-plans [post]
func (h *AdminHandler) CreateSubscriptionPlan(c *fiber.Ctx) error {
	var req CreateSubscriptionPlanRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate required fields
	if req.Name == "" || req.Price <= 0 {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Name and price are required",
		})
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}
	if req.BillingCycle == "" {
		req.BillingCycle = "monthly"
	}

	// Get current user ID from JWT context
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Convert features and roles to JSON strings
	featuresJSON, _ := json.Marshal(req.Features)
	rolesJSON, _ := json.Marshal(req.AllowedRoles)

	// Optionally create Stripe Product + Price when Price ID not provided
	stripePriceID := strings.TrimSpace(req.StripePriceID)
	// Normalize placeholder or invalid Stripe price IDs from client (e.g., "string")
	if strings.EqualFold(stripePriceID, "string") || (stripePriceID != "" && !strings.HasPrefix(strings.ToLower(stripePriceID), "price_")) {
		log.Printf("Ignoring invalid stripe_price_id value '%s'; will attempt to create a new price in Stripe", stripePriceID)
		stripePriceID = ""
	}
	var stripeProductID string
	var usedLookupKey string
	if stripePriceID == "" {
		// Initialize Stripe key from config or env
		if stripe.Key == "" {
			if h.cfg != nil && h.cfg.StripeSecretKey != "" {
				stripe.Key = h.cfg.StripeSecretKey
			} else {
				stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
			}
		}
		if stripe.Key == "" {
			log.Printf("STRIPE_SECRET_KEY not configured; proceeding without Stripe price creation")
		} else {
			// Create Product
			prod, pErr := product.New(&stripe.ProductParams{Name: stripe.String(req.Name)})
			if pErr != nil {
				log.Printf("Failed to create Stripe product: %v", pErr)
				LogUserAction(h.db, c.Locals("user_id").(uint), "ADMIN_SUB_PLAN_STRIPE_PRODUCT_FAIL", 0, "Stripe", pErr.Error(), c)
			} else {
				stripeProductID = prod.ID
				interval := "month"
				if strings.ToLower(req.BillingCycle) == "annual" || strings.ToLower(req.BillingCycle) == "yearly" || strings.ToLower(req.BillingCycle) == "year" {
					interval = "year"
				}
				unitAmount := int64(req.Price * 100)
				// Determine lookup key: prefer request value; else generate from single allowed role
				lookupKey := strings.TrimSpace(req.StripeLookupKey)
				if lookupKey == "" && len(req.AllowedRoles) == 1 {
					lookupKey = utils.GenerateRoleLookupKey(models.UserRole(strings.ToLower(req.AllowedRoles[0])))
				}
				params := &stripe.PriceParams{
					Currency:   stripe.String(strings.ToLower(req.Currency)),
					UnitAmount: stripe.Int64(unitAmount),
					Product:    stripe.String(prod.ID),
					Recurring: &stripe.PriceRecurringParams{
						Interval: stripe.String(interval),
					},
				}
				if lookupKey != "" {
					params.LookupKey = stripe.String(lookupKey)
					usedLookupKey = lookupKey
				}
				pr, prErr := price.New(params)
				if prErr != nil {
					log.Printf("Failed to create Stripe price: %v", prErr)
					LogUserAction(h.db, c.Locals("user_id").(uint), "ADMIN_SUB_PLAN_STRIPE_PRICE_FAIL", 0, "Stripe", prErr.Error(), c)
				} else {
					stripePriceID = pr.ID
					LogUserAction(h.db, c.Locals("user_id").(uint), "ADMIN_SUB_PLAN_STRIPE_CREATED", 0, "Stripe", fmt.Sprintf("product_id=%s price_id=%s", stripeProductID, stripePriceID), c)
				}
			}
		}
	}

	// Create subscription plan
	plan := models.DynamicSubscriptionPlan{
		Name:            req.Name,
		Description:     req.Description,
		Price:           req.Price,
		Currency:        req.Currency,
		BillingCycle:    req.BillingCycle,
		Features:        string(featuresJSON),
		AllowedRoles:    string(rolesJSON),
		StripePriceID:   stripePriceID,
		CreatedByUserID: userID,
		IsActive:        true,
	}

	if err := h.db.Create(&plan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Error creating subscription plan",
			"error":   err.Error(),
		})
	}

	// Create role mappings
	for _, roleStr := range req.AllowedRoles {
		mapping := models.RoleSubscriptionMapping{
			Role:               models.UserRole(roleStr),
			SubscriptionPlanID: plan.ID,
		}
		h.db.Create(&mapping)
	}

	return c.Status(201).JSON(fiber.Map{
		"success":           true,
		"message":           "Subscription plan created successfully",
		"data":              plan,
		"stripe_product_id": stripeProductID,
		"stripe_price_id":   stripePriceID,
		"stripe_lookup_key": usedLookupKey,
	})
}

// GetSubscriptionPlans retrieves all subscription plans
// @Summary Get all subscription plans
// @Description Retrieves all dynamic subscription plans (admin only)
// @Tags admin,subscriptions
// @Produce json
// @Success 200 {object} map[string]interface{} "Subscription plans retrieved successfully"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/subscription-plans [get]
func (h *AdminHandler) GetSubscriptionPlans(c *fiber.Ctx) error {
	var plans []models.DynamicSubscriptionPlan

	if err := h.db.Preload("CreatedBy").Find(&plans).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Error retrieving subscription plans",
			"error":   err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"data":    plans,
	})
}

type UpdateSubscriptionPlanRequest struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Price         float64  `json:"price"`
	Currency      string   `json:"currency"`
	BillingCycle  string   `json:"billing_cycle"`
	Features      []string `json:"features"`
	AllowedRoles  []string `json:"allowed_roles"`
	IsActive      bool     `json:"is_active"`
	StripePriceID string   `json:"stripe_price_id,omitempty"`
}

// UpdateSubscriptionPlan updates an existing subscription plan
// @Summary Update subscription plan
// @Description Updates an existing dynamic subscription plan (admin only)
// @Tags admin,subscriptions
// @Accept json
// @Produce json
// @Param id path int true "Subscription plan ID"
// @Param request body UpdateSubscriptionPlanRequest true "Updated subscription plan information"
// @Success 200 {object} map[string]interface{} "Subscription plan updated successfully"
// @Failure 400 {object} map[string]string "Bad request or validation error"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Subscription plan not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/subscription-plans/{id} [put]
func (h *AdminHandler) UpdateSubscriptionPlan(c *fiber.Ctx) error {
	planID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid plan ID",
		})
	}

	var req UpdateSubscriptionPlanRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	var plan models.DynamicSubscriptionPlan
	if err := h.db.First(&plan, planID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Subscription plan not found",
		})
	}

	// Convert features and roles to JSON strings
	featuresJSON, _ := json.Marshal(req.Features)
	rolesJSON, _ := json.Marshal(req.AllowedRoles)

	// Update plan
	plan.Name = req.Name
	plan.Description = req.Description
	plan.Price = req.Price
	plan.Currency = req.Currency
	plan.BillingCycle = req.BillingCycle
	plan.Features = string(featuresJSON)
	plan.AllowedRoles = string(rolesJSON)
	plan.IsActive = req.IsActive
	if req.StripePriceID != "" {
		plan.StripePriceID = req.StripePriceID
	}

	if err := h.db.Save(&plan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Error updating subscription plan",
			"error":   err.Error(),
		})
	}

	// Update role mappings
	h.db.Where("subscription_plan_id = ?", plan.ID).Delete(&models.RoleSubscriptionMapping{})
	for _, roleStr := range req.AllowedRoles {
		mapping := models.RoleSubscriptionMapping{
			Role:               models.UserRole(roleStr),
			SubscriptionPlanID: plan.ID,
		}
		h.db.Create(&mapping)
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"message": "Subscription plan updated successfully",
		"data":    plan,
	})
}

// DeleteSubscriptionPlan deletes a subscription plan
// @Summary Delete subscription plan
// @Description Deletes a dynamic subscription plan if not in use (admin only)
// @Tags admin,subscriptions
// @Produce json
// @Param id path int true "Subscription plan ID"
// @Success 200 {object} map[string]interface{} "Subscription plan deleted successfully"
// @Failure 400 {object} map[string]string "Cannot delete subscription plan that is currently in use"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Subscription plan not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/subscription-plans/{id} [delete]
func (h *AdminHandler) DeleteSubscriptionPlan(c *fiber.Ctx) error {
	planID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid plan ID",
		})
	}

	var plan models.DynamicSubscriptionPlan
	if err := h.db.First(&plan, planID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Subscription plan not found",
		})
	}

	// Check if plan is in use
	var subscriptionCount int64
	h.db.Model(&models.Subscription{}).Where("dynamic_plan_id = ?", planID).Count(&subscriptionCount)
	if subscriptionCount > 0 {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Cannot delete subscription plan that is currently in use",
		})
	}

	// Delete role mappings first
	h.db.Where("subscription_plan_id = ?", planID).Delete(&models.RoleSubscriptionMapping{})

	// Delete plan
	if err := h.db.Delete(&plan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Error deleting subscription plan",
			"error":   err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"message": "Subscription plan deleted successfully",
	})
}

// GetRoleSubscriptionMappings retrieves subscription plans for a specific role
// @Summary Get role subscription mappings
// @Description Retrieves subscription plans available for a specific role (admin only)
// @Tags admin,subscriptions
// @Produce json
// @Param role query string true "User role to filter by" Enums(parent,montessori_professional,institution,training_center)
// @Success 200 {object} map[string]interface{} "Role subscription mappings retrieved successfully"
// @Failure 400 {object} map[string]string "Role parameter is required"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/role-subscriptions [get]
func (h *AdminHandler) GetRoleSubscriptionMappings(c *fiber.Ctx) error {
	role := c.Query("role")
	if role == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Role parameter is required",
		})
	}

	var mappings []models.RoleSubscriptionMapping
	if err := h.db.Preload("SubscriptionPlan").Where("role = ?", role).Find(&mappings).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Error retrieving role subscription mappings",
			"error":   err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"data":    mappings,
	})
}

type AssignUserSubscriptionRequest struct {
	UserID             uint `json:"user_id"`
	SubscriptionPlanID uint `json:"subscription_plan_id"`
	DurationMonths     int  `json:"duration_months"`
}

// AssignUserSubscription assigns a subscription plan to a user
// @Summary Assign subscription to user
// @Description Assigns a subscription plan to a specific user (admin only)
// @Tags admin,subscriptions
// @Accept json
// @Produce json
// @Param request body AssignUserSubscriptionRequest true "Subscription assignment information"
// @Success 200 {object} map[string]interface{} "Subscription assigned successfully"
// @Failure 400 {object} map[string]string "Bad request or user role not allowed for this plan"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "User or subscription plan not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/assign-subscription [post]
func (h *AdminHandler) AssignUserSubscription(c *fiber.Ctx) error {
	var req AssignUserSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate user exists
	var user models.User
	if err := h.db.First(&user, req.UserID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "User not found",
		})
	}

	// Validate subscription plan exists
	var plan models.DynamicSubscriptionPlan
	if err := h.db.First(&plan, req.SubscriptionPlanID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Subscription plan not found",
		})
	}

	// Ensure plan has a Stripe Price ID
	if strings.TrimSpace(plan.StripePriceID) == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Selected plan is not linked to Stripe (missing stripe_price_id)",
		})
	}

	// Check if user role is allowed for this plan
	var allowedRoles []string
	_ = json.Unmarshal([]byte(plan.AllowedRoles), &allowedRoles)
	roleAllowed := false
	for _, role := range allowedRoles {
		if role == string(user.Role) {
			roleAllowed = true
			break
		}
	}
	if !roleAllowed {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "User role is not allowed for this subscription plan",
		})
	}

	// Initialize Stripe secret if needed
	if stripe.Key == "" {
		stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	}

	// Find existing local subscription (latest)
	var subscription models.Subscription
	err := h.db.Where("user_id = ?", req.UserID).Order("created_at DESC").First(&subscription).Error

	// Determine or create Stripe customer ID
	stripeCustomerID := subscription.StripeCustomerID
	if stripeCustomerID == "" {
		// Try reuse a previous customer's ID if exists in other subs
		var anySub models.Subscription
		if e := h.db.Where("user_id = ? AND stripe_customer_id <> ''", req.UserID).Order("created_at DESC").First(&anySub).Error; e == nil {
			stripeCustomerID = anySub.StripeCustomerID
		}
	}
	if stripeCustomerID == "" && stripe.Key != "" {
		custParams := &stripe.CustomerParams{Email: stripe.String(user.Email), Name: stripe.String(fmt.Sprintf("%s %s", user.FirstName, user.LastName))}
		custParams.AddMetadata("user_id", fmt.Sprintf("%d", user.ID))
		cust, cerr := customer.New(custParams)
		if cerr != nil {
			log.Printf("Admin assign subscription: failed creating Stripe customer: %v", cerr)
		} else {
			stripeCustomerID = cust.ID
		}
	}

	now := time.Now()

	// Helper to map Stripe subscription to local fields
	mapFromStripe := func(s *stripe.Subscription, local *models.Subscription) {
		if s == nil || local == nil {
			return
		}
		local.StripeSubscriptionID = s.ID
		local.StripeCustomerID = s.Customer.ID
		// status
		switch s.Status {
		case "active", "trialing":
			local.Status = models.SubscriptionActive
		default:
			local.Status = models.SubscriptionInactive
		}
		if s.CurrentPeriodEnd > 0 {
			local.EndDate = time.Unix(s.CurrentPeriodEnd, 0)
		}
		local.AutoRenew = !s.CancelAtPeriodEnd
	}

	if err == gorm.ErrRecordNotFound {
		// Create new local subscription record and Stripe subscription
		newSub := models.Subscription{
			UserID:           req.UserID,
			Plan:             models.SubscriptionPlan("dynamic"),
			DynamicPlanID:    &req.SubscriptionPlanID,
			Status:           models.SubscriptionActive,
			StartDate:        now,
			EndDate:          now, // will be updated from Stripe below
			AutoRenew:        true,
			StripeCustomerID: stripeCustomerID,
		}
		// Create Stripe subscription if possible
		if stripe.Key != "" && plan.StripePriceID != "" {
			params := &stripe.SubscriptionParams{
				Customer: stripe.String(stripeCustomerID),
				Items:    []*stripe.SubscriptionItemsParams{{Price: stripe.String(plan.StripePriceID)}},
			}
			params.AddMetadata("user_id", fmt.Sprintf("%d", user.ID))
			params.AddMetadata("plan_id", fmt.Sprintf("%d", plan.ID))
			ss, sErr := sub.New(params)
			if sErr != nil {
				log.Printf("Admin assign subscription: failed to create Stripe subscription: %v", sErr)
			} else {
				mapFromStripe(ss, &newSub)
			}
		}
		if e := h.db.Create(&newSub).Error; e != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to create subscription", "error": e.Error()})
		}
		return c.Status(200).JSON(fiber.Map{"success": true, "message": "Subscription assigned successfully", "data": newSub})
	} else if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to query subscription", "error": err.Error()})
	}

	// Update existing subscription: if has Stripe subscription, switch price; otherwise create one
	if subscription.StripeSubscriptionID != "" && stripe.Key != "" {
		// Get Stripe subscription to obtain item ID
		ss, gErr := sub.Get(subscription.StripeSubscriptionID, nil)
		if gErr != nil {
			log.Printf("Admin assign subscription: failed to fetch Stripe subscription: %v", gErr)
		} else if len(ss.Items.Data) > 0 {
			itemID := ss.Items.Data[0].ID
			uParams := &stripe.SubscriptionParams{
				Items: []*stripe.SubscriptionItemsParams{{
					ID:    stripe.String(itemID),
					Price: stripe.String(plan.StripePriceID),
				}},
			}
			uss, uErr := sub.Update(subscription.StripeSubscriptionID, uParams)
			if uErr != nil {
				log.Printf("Admin assign subscription: failed to update Stripe subscription: %v", uErr)
			} else {
				mapFromStripe(uss, &subscription)
			}
		}
	} else if stripe.Key != "" {
		// No Stripe sub yet; create one
		params := &stripe.SubscriptionParams{
			Customer: stripe.String(stripeCustomerID),
			Items:    []*stripe.SubscriptionItemsParams{{Price: stripe.String(plan.StripePriceID)}},
		}
		params.AddMetadata("user_id", fmt.Sprintf("%d", user.ID))
		params.AddMetadata("plan_id", fmt.Sprintf("%d", plan.ID))
		ss, sErr := sub.New(params)
		if sErr != nil {
			log.Printf("Admin assign subscription: failed to create Stripe subscription: %v", sErr)
		} else {
			mapFromStripe(ss, &subscription)
		}
	}

	// Update local record fields
	subscription.DynamicPlanID = &req.SubscriptionPlanID
	subscription.Plan = models.SubscriptionPlan("dynamic")
	subscription.StartDate = subscription.StartDate
	if subscription.EndDate.IsZero() {
		// Fallback: set a nominal end date based on DurationMonths if Stripe did not set
		subscription.EndDate = now.AddDate(0, req.DurationMonths, 0)
	}
	if subscription.StripeSubscriptionID == "" {
		// If Stripe didn't create, keep autorenew off to avoid confusion
		subscription.AutoRenew = false
	}
	if e := h.db.Save(&subscription).Error; e != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update subscription", "error": e.Error()})
	}

	return c.Status(200).JSON(fiber.Map{"success": true, "message": "Subscription assigned successfully", "data": subscription})
}
