package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"mwc_backend/config"
	"mwc_backend/internal/api/middleware"
	"mwc_backend/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// PersonaE2ETestSuite tests complete user journeys for each persona
type PersonaE2ETestSuite struct {
	suite.Suite
	db           *gorm.DB
	app          *fiber.App
	cfg          *config.Config
	mqService    *MockMessageQueueService
	emailService *MockEmailService
}

func (suite *PersonaE2ETestSuite) SetupSuite() {
	// Create in-memory database with shared cache
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	suite.Require().NoError(err)
	suite.db = db

	// Run migrations
	err = db.AutoMigrate(
		&models.User{},
		&models.InstitutionProfile{},
		&models.School{},
		&models.Job{},
		&models.JobApplication{},
		&models.MontessoriProfessionalProfile{},
		&models.MontessoriJobPreference{},
		&models.ParentProfile{},
		&models.Review{},
		&models.DynamicSubscriptionPlan{},
		&models.RoleSubscriptionMapping{},
		&models.Subscription{},
		&models.ActionLog{},
	)
	suite.Require().NoError(err)

	// Setup config
	suite.cfg = &config.Config{
		JWTSecret:          "test-secret-key-for-e2e-tests",
		JwtExpirationHours: 72,
	}

	// Setup mock services
	suite.mqService = &MockMessageQueueService{}
	// Setup mock to return false for IsInitialized to avoid RabbitMQ-specific code
	suite.mqService.On("IsInitialized").Return(false)
	suite.emailService = &MockEmailService{}
	// Configure email service mocks to avoid unexpected calls during registration
	suite.emailService.On("SendEmail", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	suite.emailService.On("SendHTMLEmail", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	suite.emailService.On("SendEmailVerification", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	suite.emailService.On("SendPasswordResetEmail", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	// Setup Fiber app with middleware and routes
	suite.app = fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Setup handlers
	authHandler := NewAuthHandler(db, suite.cfg, suite.emailService, suite.mqService)
	institutionHandler := NewInstitutionHandler(db, suite.mqService, suite.emailService)
	professionalHandler := NewMontessoriProfessionalHandler(db, suite.mqService, suite.emailService)

	// For ParentHandler, pass nil for mqService since it tries to cast to *queue.RabbitMQService
	// which would panic with our mock. The parent handler gracefully handles nil mqService.
	parentHandler := NewParentHandler(db, nil, suite.emailService)

	// Auth middleware
	authMw := middleware.Protected(suite.cfg.JWTSecret)

	// Public routes
	apiV1 := suite.app.Group("/api/v1")
	apiV1.Post("/register", authHandler.Register)
	apiV1.Post("/login", authHandler.Login)

	// Institution routes
	instRoutes := apiV1.Group("/institution", authMw, middleware.RoleAuth(models.InstitutionRole, models.TrainingCenterRole))
	instRoutes.Post("/profile", institutionHandler.CreateOrUpdateInstitutionProfile)
	instRoutes.Post("/schools", institutionHandler.CreateSchool)
	instRoutes.Post("/jobs", institutionHandler.PostJob)
	instRoutes.Get("/jobs", institutionHandler.GetMyJobs)
	instRoutes.Put("/jobs/:job_id", institutionHandler.UpdateJob)
	instRoutes.Delete("/jobs/:job_id", institutionHandler.DeleteJob)
	instRoutes.Get("/jobs/:job_id/applicants", institutionHandler.GetJobApplicants)

	// Montessori Professional routes
	profRoutes := apiV1.Group("/montessori-professional", authMw, middleware.RoleAuth(models.MontessoriProfessionalRole))
	profRoutes.Post("/profile", professionalHandler.CreateOrUpdateMontessoriProfessionalProfile)
	profRoutes.Put("/job-preferences", professionalHandler.UpsertJobPreference)
	profRoutes.Post("/jobs/:job_id/apply", professionalHandler.ApplyForJob)
	profRoutes.Get("/jobs/applied", professionalHandler.GetAppliedJobs)

	// Parent routes
	parentRoutes := apiV1.Group("/parent", authMw, middleware.RoleAuth(models.ParentRole))
	parentRoutes.Post("/profile", parentHandler.CreateOrUpdateParentProfile)
	parentRoutes.Post("/schools/save/:school_id", parentHandler.SaveSchool)
	parentRoutes.Get("/schools/saved", parentHandler.GetSavedSchools)
}

func (suite *PersonaE2ETestSuite) TearDownSuite() {
	sqlDB, _ := suite.db.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}
}

func (suite *PersonaE2ETestSuite) SetupTest() {
	// Clean all tables before each test
	suite.db.Exec("DELETE FROM job_applications")
	suite.db.Exec("DELETE FROM jobs")
	suite.db.Exec("DELETE FROM schools")
	suite.db.Exec("DELETE FROM montessori_job_preferences")
	suite.db.Exec("DELETE FROM montessori_professional_profiles")
	suite.db.Exec("DELETE FROM parent_profiles")
	suite.db.Exec("DELETE FROM institution_profiles")
	suite.db.Exec("DELETE FROM reviews")
	suite.db.Exec("DELETE FROM users")
}

// Helper to create and verify a user
func (suite *PersonaE2ETestSuite) registerUser(email, password, firstName, lastName string, role models.UserRole, institutionName string) string {
	registerReq := map[string]interface{}{
		"email":      email,
		"password":   password,
		"first_name": firstName,
		"last_name":  lastName,
		"role":       role,
	}
	if institutionName != "" {
		registerReq["institution_name"] = institutionName
	}

	body, _ := json.Marshal(registerReq)
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := suite.app.Test(req, -1)
	suite.Require().NoError(err)
	suite.Require().Equal(fiber.StatusCreated, resp.StatusCode, "Registration failed")

	// Manually verify email
	var user models.User
	suite.db.Where("email = ?", email).First(&user)
	user.EmailVerified = true
	suite.db.Save(&user)

	return suite.loginUser(email, password)
}

// Helper to login and get JWT token
func (suite *PersonaE2ETestSuite) loginUser(email, password string) string {
	loginReq := map[string]string{
		"email":    email,
		"password": password,
	}
	body, _ := json.Marshal(loginReq)
	req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := suite.app.Test(req, -1)
	suite.Require().NoError(err)
	suite.Require().Equal(fiber.StatusOK, resp.StatusCode, "Login failed")

	var loginResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&loginResp)
	return loginResp["token"].(string)
}

// Helper to make authenticated requests
func (suite *PersonaE2ETestSuite) authRequest(method, path, token string, body interface{}) (*http.Response, error) {
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)

	return suite.app.Test(req, -1)
}

// Helper to make multipart/form-data requests
func (suite *PersonaE2ETestSuite) authMultipartRequest(path, token string, fields map[string]string) (*http.Response, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	w.Close()

	req := httptest.NewRequest("POST", path, &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", token)
	return suite.app.Test(req, -1)
}

// =============================================================================
// INSTITUTION PERSONA E2E FLOW
// =============================================================================

func (suite *PersonaE2ETestSuite) TestInstitutionPersona_EndToEndFlow() {
	suite.T().Log("🏫 Testing Institution Persona - Complete End-to-End Flow")

	// Step 1: Register and Login
	token := suite.registerUser(
		"montessori.school@example.com",
		"SecurePass123!",
		"Maria",
		"Smith",
		models.InstitutionRole,
		"Montessori Learning Academy",
	)
	assert.NotEmpty(suite.T(), token)
	suite.T().Log("✅ Step 1: Institution registered and logged in")

	// Step 2: Create Institution Profile (multipart with school linking)
	resp, err := suite.authMultipartRequest("/api/v1/institution/profile", token, map[string]string{
		"institution_name":    "Montessori Learning Academy",
		"school_name":         "Montessori Academy Main Campus",
		"school_city":         "San Francisco",
		"school_state":        "CA",
		"school_country_code": "US",
		"school_category":     "school",
	})
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
	suite.T().Log("✅ Step 2: Institution profile created and school linked")

	// Step 3: Skip explicit school creation since linked in profile
	suite.T().Log("✅ Step 3: Skipped school creation (already linked)")

	// Step 4: Post a Job
	jobReq := map[string]interface{}{
		"title":           "Lead Montessori Teacher",
		"description":     "Seeking experienced AMI/AMS certified teacher",
		"location":        "San Francisco, CA",
		"employment_type": "full_time",
		"salary_range":    "$55,000 - $85,000",
	}
	resp, err = suite.authRequest("POST", "/api/v1/institution/jobs", token, jobReq)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusCreated, resp.StatusCode)
	suite.T().Log("✅ Step 4: Job posted")

	// Step 5: View My Jobs
	resp, err = suite.authRequest("GET", "/api/v1/institution/jobs", token, nil)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	var jobs []models.Job
	json.NewDecoder(resp.Body).Decode(&jobs)
	assert.GreaterOrEqual(suite.T(), len(jobs), 1, "Should have at least one job")
	suite.T().Log(fmt.Sprintf("✅ Step 5: Retrieved %d job(s)", len(jobs)))

	// Step 6: Update a Job
	if len(jobs) > 0 {
		jobID := jobs[0].ID
		updateReq := map[string]interface{}{
			"title":       "Senior Lead Montessori Teacher",
			"salary_max":  95000,
			"description": "Updated: Looking for highly experienced educator",
		}
		resp, err = suite.authRequest("PUT", fmt.Sprintf("/api/v1/institution/jobs/%d", jobID), token, updateReq)
		suite.Require().NoError(err)
		assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
		suite.T().Log("✅ Step 6: Job updated")
	}

	suite.T().Log("✅✅✅ Institution Persona - ALL STEPS PASSED")
}

// =============================================================================
// MONTESSORI PROFESSIONAL PERSONA E2E FLOW
// =============================================================================

func (suite *PersonaE2ETestSuite) TestMontessoriProfessional_EndToEndFlow() {
	suite.T().Log("👨‍🏫 Testing Montessori Professional Persona - Complete End-to-End Flow")

	// Setup: Create an institution with a job posting first
	institutionToken := suite.registerUser(
		"hiring.school@example.com",
		"SecurePass123!",
		"John",
		"Doe",
		models.InstitutionRole,
		"Test Montessori School",
	)

	// Create institution profile and job
	var institutionUser models.User
	suite.db.Where("email = ?", "hiring.school@example.com").First(&institutionUser)

	institutionProfile := models.InstitutionProfile{
		UserID:          institutionUser.ID,
		InstitutionName: "Test Montessori School",
	}
	suite.db.Create(&institutionProfile)

	job := models.Job{
		InstitutionProfileID: institutionProfile.ID,
		Title:                "Montessori Teacher Position",
		Description:          "Great opportunity for passionate educator",
		Location:             "Boston, MA, US",
		EmploymentType:       "full_time",
		SalaryRange:          "$45,000 - $75,000",
		IsActive:             true,
	}
	suite.db.Create(&job)
	suite.T().Log("✅ Setup: Institution and job created")

	// Step 1: Register and Login as Professional
	token := suite.registerUser(
		"teacher@example.com",
		"SecurePass123!",
		"Alice",
		"Johnson",
		models.MontessoriProfessionalRole,
		"",
	)
	assert.NotEmpty(suite.T(), token)
	suite.T().Log("✅ Step 1: Montessori Professional registered and logged in")

	// Step 2: Create Professional Profile
	profileReq := map[string]interface{}{
		"bio":                 "Experienced Montessori educator with 10 years teaching children ages 3-6",
		"country_code":        "US",
		"city":                "Boston",
		"state":               "MA",
		"certifications":      "AMI, AMS",
		"years_of_experience": 10,
		"looking_for_job":     true,
	}
	resp, err := suite.authRequest("POST", "/api/v1/montessori-professional/profile", token, profileReq)
	suite.Require().NoError(err)
	assert.True(suite.T(), resp.StatusCode == fiber.StatusOK || resp.StatusCode == fiber.StatusCreated)
	suite.T().Log("✅ Step 2: Professional profile created")

	// Step 3: Set Job Preferences
	preferenceReq := map[string]interface{}{
		"preferred_locations": "Boston, Cambridge, Somerville",
		"job_types":           "full_time",
		"experience_level":    "mid",
		"min_salary":          45000,
		"max_salary":          75000,
	}
	resp, err = suite.authRequest("PUT", "/api/v1/montessori-professional/job-preferences", token, preferenceReq)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
	suite.T().Log("✅ Step 3: Job preferences set")

	// Step 4: Job search is handled through GetAllJobs - skip for now
	suite.T().Log("✅ Step 4: Skipped job search (tested via GetAllJobs)")

	// Step 5: Apply for Job
	resp, err = suite.authRequest("POST", fmt.Sprintf("/api/v1/montessori-professional/jobs/%d/apply", job.ID), token, nil)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusCreated, resp.StatusCode)
	suite.T().Log("✅ Step 5: Applied for job")

	// Step 6: View Applied Jobs
	resp, err = suite.authRequest("GET", "/api/v1/montessori-professional/jobs/applied", token, nil)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	var appliedJobs []models.JobApplication
	json.NewDecoder(resp.Body).Decode(&appliedJobs)
	assert.GreaterOrEqual(suite.T(), len(appliedJobs), 1, "Should have at least one applied job")
	suite.T().Log(fmt.Sprintf("✅ Step 6: Retrieved %d applied job(s)", len(appliedJobs)))

	// Step 7: Institution views applicants
	resp, err = suite.authRequest("GET", fmt.Sprintf("/api/v1/institution/jobs/%d/applicants", job.ID), institutionToken, nil)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	var applicants []models.JobApplication
	json.NewDecoder(resp.Body).Decode(&applicants)
	assert.Equal(suite.T(), 1, len(applicants), "Institution should see 1 applicant")
	suite.T().Log("✅ Step 7: Institution viewed applicants")

	suite.T().Log("✅✅✅ Montessori Professional Persona - ALL STEPS PASSED")
}

// =============================================================================
// PARENT PERSONA E2E FLOW
// =============================================================================

func (suite *PersonaE2ETestSuite) TestParentPersona_EndToEndFlow() {
	suite.T().Log("👨‍👩‍👧 Testing Parent Persona - Complete End-to-End Flow")

	// Setup: Create a school
	school := models.School{
		Name:        "Excellent Montessori School",
		CountryCode: "US",
		City:        "New York",
		State:       "NY",
	}
	suite.db.Create(&school)

	// Step 1: Register and Login as Parent
	token := suite.registerUser(
		"parent@example.com",
		"SecurePass123!",
		"Bob",
		"Williams",
		models.ParentRole,
		"",
	)
	assert.NotEmpty(suite.T(), token)
	suite.T().Log("✅ Step 1: Parent registered and logged in")

	// Step 2: Create Parent Profile
	profileReq := map[string]interface{}{
		"country_code":       "US",
		"city":               "New York",
		"state":              "NY",
		"children_ages":      "5,7",
		"is_public":          true,
		"looking_for_school": true,
	}
	resp, err := suite.authRequest("POST", "/api/v1/parent/profile", token, profileReq)
	suite.Require().NoError(err)
	assert.True(suite.T(), resp.StatusCode == fiber.StatusOK || resp.StatusCode == fiber.StatusCreated)
	suite.T().Log("✅ Step 2: Parent profile created")

	// Step 3: Save a School
	resp, err = suite.authRequest("POST", fmt.Sprintf("/api/v1/parent/schools/save/%d", school.ID), token, nil)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusCreated, resp.StatusCode)
	suite.T().Log("✅ Step 3: School saved")

	// Step 4: View Saved Schools
	resp, err = suite.authRequest("GET", "/api/v1/parent/schools/saved", token, nil)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	var savedSchools []models.School
	json.NewDecoder(resp.Body).Decode(&savedSchools)
	assert.GreaterOrEqual(suite.T(), len(savedSchools), 1, "Should have at least one saved school")
	suite.T().Log(fmt.Sprintf("✅ Step 4: Retrieved %d saved school(s)", len(savedSchools)))

	suite.T().Log("✅✅✅ Parent Persona - ALL STEPS PASSED")
}

// =============================================================================
// TRAINING CENTER PERSONA E2E FLOW
// =============================================================================

func (suite *PersonaE2ETestSuite) TestTrainingCenterPersona_EndToEndFlow() {
	suite.T().Log("🎓 Testing Training Center Persona - Complete End-to-End Flow")

	// Step 1: Register and Login as Training Center
	token := suite.registerUser(
		"training@example.com",
		"SecurePass123!",
		"Sarah",
		"Miller",
		models.TrainingCenterRole,
		"Advanced Montessori Training Institute",
	)
	assert.NotEmpty(suite.T(), token)
	suite.T().Log("✅ Step 1: Training Center registered and logged in")

	// Step 2: Create Training Center Profile
	profileReq := map[string]interface{}{
		"institution_name": "Advanced Montessori Training Institute",
		"country_code":     "GB",
		"city":             "London",
		"description":      "Premier Montessori teacher training center",
		"website":          "https://amti.edu",
	}
	resp, err := suite.authRequest("POST", "/api/v1/institution/profile", token, profileReq)
	suite.Require().NoError(err)
	assert.True(suite.T(), resp.StatusCode == fiber.StatusOK || resp.StatusCode == fiber.StatusCreated)
	suite.T().Log("✅ Step 2: Training Center profile created")

	// Step 3: Post Training Position
	jobReq := map[string]interface{}{
		"title":             "Montessori Trainer - AMI Program",
		"description":       "Lead trainer for AMI certification courses",
		"job_type":          "full_time",
		"experience_level":  "expert",
		"country_code":      "GB",
		"city":              "London",
		"salary_min":        60000,
		"salary_max":        95000,
		"application_email": "careers@amti.edu",
	}
	resp, err = suite.authRequest("POST", "/api/v1/institution/jobs", token, jobReq)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusCreated, resp.StatusCode)
	suite.T().Log("✅ Step 3: Training position posted")

	// Step 4: View Posted Jobs
	resp, err = suite.authRequest("GET", "/api/v1/institution/jobs", token, nil)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	var jobs []models.Job
	json.NewDecoder(resp.Body).Decode(&jobs)
	assert.GreaterOrEqual(suite.T(), len(jobs), 1, "Should have at least one job")
	suite.T().Log(fmt.Sprintf("✅ Step 4: Retrieved %d job(s)", len(jobs)))

	suite.T().Log("✅✅✅ Training Center Persona - ALL STEPS PASSED")
}

// Run the test suite
func TestPersonaE2ETestSuite(t *testing.T) {
	suite.Run(t, new(PersonaE2ETestSuite))
}
