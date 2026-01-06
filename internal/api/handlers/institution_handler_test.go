package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"mwc_backend/internal/models"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockMessageQueueService is a mock implementation of the MessageQueueService interface
type MockMessageQueueService struct {
	mock.Mock
}

func (m *MockMessageQueueService) Publish(ctx context.Context, exchange, routingKey string, body []byte, delayMilliseconds int32) error {
	args := m.Called(ctx, exchange, routingKey, body, delayMilliseconds)
	return args.Error(0)
}

func (m *MockMessageQueueService) Consume(queueName, consumerTag string, handler func(delivery amqp.Delivery) error) error {
	args := m.Called(queueName, consumerTag, handler)
	return args.Error(0)
}

func (m *MockMessageQueueService) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockMessageQueueService) DeclareDelayedMessageExchangeAndQueue(exchangeName, queueName, deadLetterExchange, deadLetterRoutingKey string) error {
	args := m.Called(exchangeName, queueName, deadLetterExchange, deadLetterRoutingKey)
	return args.Error(0)
}

func (m *MockMessageQueueService) DeclareExchange(name, kind string, durable, autoDelete, internal, noWait bool, argsTable amqp.Table) error {
	args := m.Called(name, kind, durable, autoDelete, internal, noWait, argsTable)
	return args.Error(0)
}

func (m *MockMessageQueueService) DeclareQueue(name string, durable, autoDelete, exclusive, noWait bool, argsTable amqp.Table) (amqp.Queue, error) {
	args := m.Called(name, durable, autoDelete, exclusive, noWait, argsTable)
	return args.Get(0).(amqp.Queue), args.Error(1)
}

func (m *MockMessageQueueService) BindQueue(queueName, routingKey, exchangeName string, noWait bool, argsTable amqp.Table) error {
	args := m.Called(queueName, routingKey, exchangeName, noWait, argsTable)
	return args.Error(0)
}

func (m *MockMessageQueueService) IsInitialized() bool {
	args := m.Called()
	return args.Bool(0)
}

// MockEmailService is a mock implementation of the EmailService interface
type MockEmailService struct {
	mock.Mock
}

func (m *MockEmailService) SendEmail(to, subject, body string) error {
	args := m.Called(to, subject, body)
	return args.Error(0)
}

func (m *MockEmailService) SendHTMLEmail(to, subject, htmlBody string) error {
	args := m.Called(to, subject, htmlBody)
	return args.Error(0)
}

func (m *MockEmailService) SendEmailVerification(to, verificationLink string) error {
	args := m.Called(to, verificationLink)
	return args.Error(0)
}

func (m *MockEmailService) SendPasswordResetEmail(to, resetLink string) error {
	args := m.Called(to, resetLink)
	return args.Error(0)
}

// InstitutionHandlerTestSuite defines the test suite
type InstitutionHandlerTestSuite struct {
	suite.Suite
	db           *gorm.DB
	handler      *InstitutionHandler
	app          *fiber.App
	mqService    *MockMessageQueueService
	emailService *MockEmailService
}

// SetupSuite runs before all tests in the suite
func (suite *InstitutionHandlerTestSuite) SetupSuite() {
	// Setup in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(suite.T(), err)

	// Auto-migrate all models
	err = db.AutoMigrate(
		&models.User{},
		&models.InstitutionProfile{},
		&models.School{},
		&models.Job{},
		&models.JobApplication{},
		&models.ActionLog{},
		&models.Event{},
		&models.MontessoriProfessionalProfile{},
		&models.MontessoriJobPreference{},
	)
	assert.NoError(suite.T(), err)

	suite.db = db
}

// SetupTest runs before each test
func (suite *InstitutionHandlerTestSuite) SetupTest() {
	// Initialize mock services
	suite.mqService = new(MockMessageQueueService)
	suite.emailService = new(MockEmailService)

	// Initialize handler
	suite.handler = NewInstitutionHandler(suite.db, suite.mqService, suite.emailService)

	// Initialize Fiber app
	suite.app = fiber.New()
}

// TearDownTest runs after each test
func (suite *InstitutionHandlerTestSuite) TearDownTest() {
	// Clean up database tables
	suite.db.Exec("DELETE FROM users")
	suite.db.Exec("DELETE FROM institution_profiles")
	suite.db.Exec("DELETE FROM schools")
	suite.db.Exec("DELETE FROM jobs")
	suite.db.Exec("DELETE FROM job_applications")
	suite.db.Exec("DELETE FROM action_logs")
	suite.db.Exec("DELETE FROM events")
	suite.db.Exec("DELETE FROM montessori_professional_profiles")
	suite.db.Exec("DELETE FROM montessori_job_preferences")
}

// TearDownSuite runs after all tests
func (suite *InstitutionHandlerTestSuite) TearDownSuite() {
	sqlDB, _ := suite.db.DB()
	sqlDB.Close()
}

// Helper function to create a test user
func (suite *InstitutionHandlerTestSuite) createTestUser(role models.UserRole) *models.User {
	user := &models.User{
		Email:         fmt.Sprintf("test-%s-%d@example.com", role, time.Now().UnixNano()),
		PasswordHash:  "hashed_password",
		FirstName:     "Test",
		LastName:      "User",
		Role:          role,
		IsActive:      true,
		EmailVerified: true,
	}
	result := suite.db.Create(user)
	assert.NoError(suite.T(), result.Error)
	return user
}

// Helper function to create a test school
func (suite *InstitutionHandlerTestSuite) createTestSchool() *models.School {
	school := &models.School{
		Name:        fmt.Sprintf("Test School %d", time.Now().UnixNano()),
		Address:     "123 Test St",
		City:        "Test City",
		State:       "Test State",
		Country:     "Test Country",
		CountryCode: "US",
		ZipCode:     "12345",
		Category:    models.SchoolCategorySchool,
		Member:      false,
	}
	result := suite.db.Create(school)
	assert.NoError(suite.T(), result.Error)
	return school
}

// Helper function to create institution profile
func (suite *InstitutionHandlerTestSuite) createTestInstitutionProfile(userID uint, schoolID *uint) *models.InstitutionProfile {
	profile := &models.InstitutionProfile{
		UserID:          userID,
		InstitutionName: "Test Institution",
		SchoolID:        schoolID,
	}
	result := suite.db.Create(profile)
	assert.NoError(suite.T(), result.Error)
	return profile
}

// TestCreateOrUpdateInstitutionProfile_Create tests creating a new institution profile
func (suite *InstitutionHandlerTestSuite) TestCreateOrUpdateInstitutionProfile_Create() {
	user := suite.createTestUser(models.InstitutionRole)

	// Create multipart form
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	writer.WriteField("institution_name", "New Institution")
	writer.WriteField("school_name", "New School")
	writer.WriteField("school_address", "456 New St")
	writer.WriteField("school_city", "New City")
	writer.WriteField("school_country_code", "US")
	writer.Close()

	req := httptest.NewRequest("POST", "/institution/profile", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	suite.app.Post("/institution/profile", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("user_role", user.Role)
		return suite.handler.CreateOrUpdateInstitutionProfile(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Verify profile was created
	var profile models.InstitutionProfile
	result := suite.db.Where("user_id = ?", user.ID).First(&profile)
	assert.NoError(suite.T(), result.Error)
	assert.Equal(suite.T(), "New Institution", profile.InstitutionName)
}

// TestCreateOrUpdateInstitutionProfile_Update tests updating an existing profile
func (suite *InstitutionHandlerTestSuite) TestCreateOrUpdateInstitutionProfile_Update() {
	user := suite.createTestUser(models.InstitutionRole)
	suite.createTestInstitutionProfile(user.ID, nil)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	writer.WriteField("institution_name", "Updated Institution")
	writer.Close()

	req := httptest.NewRequest("POST", "/institution/profile", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	suite.app.Post("/institution/profile", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("user_role", user.Role)
		return suite.handler.CreateOrUpdateInstitutionProfile(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Verify profile was updated
	var profile models.InstitutionProfile
	suite.db.Where("user_id = ?", user.ID).First(&profile)
	assert.Equal(suite.T(), "Updated Institution", profile.InstitutionName)
}

// TestCreateOrUpdateInstitutionProfile_MissingUserID tests error when user ID not found
func (suite *InstitutionHandlerTestSuite) TestCreateOrUpdateInstitutionProfile_MissingUserID() {
	req := httptest.NewRequest("POST", "/institution/profile", nil)

	suite.app.Post("/institution/profile", suite.handler.CreateOrUpdateInstitutionProfile)

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusUnauthorized, resp.StatusCode)
}

// TestCreateOrUpdateInstitutionProfile_MissingInstitutionName tests validation
func (suite *InstitutionHandlerTestSuite) TestCreateOrUpdateInstitutionProfile_MissingInstitutionName() {
	user := suite.createTestUser(models.InstitutionRole)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	writer.WriteField("institution_name", "")
	writer.Close()

	req := httptest.NewRequest("POST", "/institution/profile", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	suite.app.Post("/institution/profile", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("user_role", user.Role)
		return suite.handler.CreateOrUpdateInstitutionProfile(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusBadRequest, resp.StatusCode)
}

// TestSelectSchool tests selecting a school for an institution
func (suite *InstitutionHandlerTestSuite) TestSelectSchool() {
	user := suite.createTestUser(models.InstitutionRole)
	profile := suite.createTestInstitutionProfile(user.ID, nil)
	school := suite.createTestSchool()

	req := httptest.NewRequest("PUT", fmt.Sprintf("/schools/select/%d", school.ID), nil)

	suite.app.Put("/schools/select/:school_id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		return suite.handler.SelectSchool(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Verify school was selected
	suite.db.First(&profile, profile.ID)
	assert.NotNil(suite.T(), profile.SchoolID)
	assert.Equal(suite.T(), school.ID, *profile.SchoolID)
}

// TestSelectSchool_NoProfile tests error when institution has no profile
func (suite *InstitutionHandlerTestSuite) TestSelectSchool_NoProfile() {
	user := suite.createTestUser(models.InstitutionRole)
	school := suite.createTestSchool()

	req := httptest.NewRequest("PUT", fmt.Sprintf("/schools/select/%d", school.ID), nil)

	suite.app.Put("/schools/select/:school_id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		return suite.handler.SelectSchool(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusNotFound, resp.StatusCode)
}

// TestCreateSchool tests creating a new school
func (suite *InstitutionHandlerTestSuite) TestCreateSchool() {
	user := suite.createTestUser(models.InstitutionRole)
	// Create profile first since CreateSchool requires it
	suite.createTestInstitutionProfile(user.ID, nil)

	schoolReq := map[string]interface{}{
		"name":         "New School",
		"address":      "789 School Ave",
		"city":         "School City",
		"country_code": "US",
	}
	body, _ := json.Marshal(schoolReq)

	req := httptest.NewRequest("POST", "/schools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	suite.app.Post("/schools", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("user_role", user.Role)
		return suite.handler.CreateSchool(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusCreated, resp.StatusCode)

	// Verify school was created
	var school models.School
	result := suite.db.Where("name = ?", "New School").First(&school)
	assert.NoError(suite.T(), result.Error)
}

// TestCreateSchool_MissingRequiredFields tests validation
func (suite *InstitutionHandlerTestSuite) TestCreateSchool_MissingRequiredFields() {
	user := suite.createTestUser(models.InstitutionRole)
	// Create profile first since CreateSchool requires it
	suite.createTestInstitutionProfile(user.ID, nil)

	schoolReq := map[string]interface{}{
		"name": "New School",
		// Missing country_code
	}
	body, _ := json.Marshal(schoolReq)

	req := httptest.NewRequest("POST", "/schools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	suite.app.Post("/schools", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("user_role", user.Role)
		return suite.handler.CreateSchool(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusBadRequest, resp.StatusCode)
}

// TestPostJob tests creating a new job posting
func (suite *InstitutionHandlerTestSuite) TestPostJob() {
	user := suite.createTestUser(models.InstitutionRole)
	school := suite.createTestSchool()
	schoolID := school.ID
	suite.createTestInstitutionProfile(user.ID, &schoolID)

	suite.mqService.On("Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	jobReq := JobRequest{
		Title:          "Math Teacher",
		Description:    "Teaching mathematics",
		Location:       "Test City",
		EmploymentType: "full-time",
		SalaryRange:    "$50,000 - $70,000",
	}
	body, _ := json.Marshal(jobReq)

	req := httptest.NewRequest("POST", "/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	suite.app.Post("/jobs", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		return suite.handler.PostJob(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusCreated, resp.StatusCode)

	// Verify job was created
	var job models.Job
	result := suite.db.Where("title = ?", "Math Teacher").First(&job)
	assert.NoError(suite.T(), result.Error)
	assert.NotZero(suite.T(), job.InstitutionProfileID)
}

// TestPostJob_NoProfile tests error when institution has no profile
func (suite *InstitutionHandlerTestSuite) TestPostJob_NoProfile() {
	user := suite.createTestUser(models.InstitutionRole)

	jobReq := JobRequest{
		Title:       "Math Teacher",
		Description: "Teaching mathematics",
	}
	body, _ := json.Marshal(jobReq)

	req := httptest.NewRequest("POST", "/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	suite.app.Post("/jobs", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		return suite.handler.PostJob(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusNotFound, resp.StatusCode)
}

// TestUpdateJob tests updating an existing job
func (suite *InstitutionHandlerTestSuite) TestUpdateJob() {
	user := suite.createTestUser(models.InstitutionRole)
	school := suite.createTestSchool()
	schoolID := school.ID
	profile := suite.createTestInstitutionProfile(user.ID, &schoolID)

	// Create initial job
	job := &models.Job{
		Title:                "Old Title",
		Description:          "Old Description",
		InstitutionProfileID: profile.ID,
		Location:             "Old Location",
	}
	suite.db.Create(job)

	updateReq := map[string]interface{}{
		"title":       "Updated Title",
		"description": "Updated Description",
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest("PUT", fmt.Sprintf("/jobs/%d", job.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	suite.app.Put("/jobs/:job_id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		return suite.handler.UpdateJob(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Verify job was updated
	suite.db.First(job, job.ID)
	assert.Equal(suite.T(), "Updated Title", job.Title)
	assert.Equal(suite.T(), "Updated Description", job.Description)
}

// TestUpdateJob_Unauthorized tests updating someone else's job
func (suite *InstitutionHandlerTestSuite) TestUpdateJob_Unauthorized() {
	user1 := suite.createTestUser(models.InstitutionRole)
	user2 := suite.createTestUser(models.InstitutionRole)
	profile1 := suite.createTestInstitutionProfile(user1.ID, nil)

	job := &models.Job{
		Title:                "Test Job",
		Description:          "Test Description",
		InstitutionProfileID: profile1.ID,
	}
	suite.db.Create(job)

	updateReq := map[string]interface{}{
		"title": "Updated Title",
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest("PUT", fmt.Sprintf("/jobs/%d", job.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	suite.app.Put("/jobs/:job_id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user2.ID) // Different user
		return suite.handler.UpdateJob(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusForbidden, resp.StatusCode)
}

// TestDeleteJob tests deleting a job
func (suite *InstitutionHandlerTestSuite) TestDeleteJob() {
	user := suite.createTestUser(models.InstitutionRole)
	profile := suite.createTestInstitutionProfile(user.ID, nil)

	job := &models.Job{
		Title:                "Test Job",
		Description:          "Test Description",
		InstitutionProfileID: profile.ID,
	}
	suite.db.Create(job)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/jobs/%d", job.ID), nil)

	suite.app.Delete("/jobs/:job_id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		return suite.handler.DeleteJob(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Verify job was soft deleted
	var deletedJob models.Job
	result := suite.db.Unscoped().First(&deletedJob, job.ID)
	assert.NoError(suite.T(), result.Error)
	assert.NotNil(suite.T(), deletedJob.DeletedAt)
}

// TestGetInstitutionPublicDetails tests retrieving public institution details
func (suite *InstitutionHandlerTestSuite) TestGetInstitutionPublicDetails() {
	user := suite.createTestUser(models.InstitutionRole)
	school := suite.createTestSchool()
	schoolID := school.ID
	profile := suite.createTestInstitutionProfile(user.ID, &schoolID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/institutions/%d", profile.ID), nil)

	suite.app.Get("/institutions/:id", suite.handler.GetInstitutionPublicDetails)

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response models.InstitutionProfile
	json.Unmarshal(body, &response)

	assert.Equal(suite.T(), profile.InstitutionName, response.InstitutionName)
}

// TestGetInstitutionPublicDetails_NotFound tests error when institution not found
func (suite *InstitutionHandlerTestSuite) TestGetInstitutionPublicDetails_NotFound() {
	req := httptest.NewRequest("GET", "/institutions/99999", nil)

	suite.app.Get("/institutions/:id", suite.handler.GetInstitutionPublicDetails)

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusNotFound, resp.StatusCode)
}

// TestSearchInstitutions tests searching for institutions
func (suite *InstitutionHandlerTestSuite) TestSearchInstitutions() {
	// Create test institutions
	for i := 0; i < 3; i++ {
		user := suite.createTestUser(models.InstitutionRole)
		profile := &models.InstitutionProfile{
			UserID:          user.ID,
			InstitutionName: fmt.Sprintf("SearchTest Institution %d", i),
		}
		suite.db.Create(profile)
	}

	req := httptest.NewRequest("GET", "/institutions/search?q=SearchTest", nil)

	suite.app.Get("/institutions/search", suite.handler.SearchInstitutions)

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response map[string]interface{}
	json.Unmarshal(body, &response)

	institutions := response["institutions"].([]interface{})
	assert.GreaterOrEqual(suite.T(), len(institutions), 3)
}

// TestSearchInstitutions_EmptyQuery tests search with empty query
func (suite *InstitutionHandlerTestSuite) TestSearchInstitutions_EmptyQuery() {
	req := httptest.NewRequest("GET", "/institutions/search", nil)

	suite.app.Get("/institutions/search", suite.handler.SearchInstitutions)

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusBadRequest, resp.StatusCode)
}

// TestCreateOrUpdateInstitutionProfile_TrainingCenter tests training center role
func (suite *InstitutionHandlerTestSuite) TestCreateOrUpdateInstitutionProfile_TrainingCenter() {
	user := suite.createTestUser(models.TrainingCenterRole)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	writer.WriteField("institution_name", "Training Center")
	writer.WriteField("school_name", "Training School")
	writer.WriteField("school_country_code", "US")
	writer.WriteField("school_category", "training_center")
	writer.Close()

	req := httptest.NewRequest("POST", "/institution/profile", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	suite.app.Post("/institution/profile", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("user_role", user.Role)
		return suite.handler.CreateOrUpdateInstitutionProfile(c)
	})

	resp, err := suite.app.Test(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Verify school category is training_center
	var profile models.InstitutionProfile
	suite.db.Preload("School").Where("user_id = ?", user.ID).First(&profile)
	if profile.School != nil {
		assert.Equal(suite.T(), models.SchoolCategoryTrainingCenter, profile.School.Category)
	}
}

// TestCreateOrUpdateInstitutionProfile_WithProfilePicture tests profile picture upload
func (suite *InstitutionHandlerTestSuite) TestCreateOrUpdateInstitutionProfile_WithProfilePicture() {
	user := suite.createTestUser(models.InstitutionRole)

	// Create a temporary test directory
	tmpDir := "./uploads/institution_profiles"
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll("./uploads")

	// Create multipart form with file
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	writer.WriteField("institution_name", "Test Institution")

	// Create a fake file
	fileWriter, _ := writer.CreateFormFile("profile_picture", "test.jpg")
	fileWriter.Write([]byte("fake image data"))
	writer.Close()

	req := httptest.NewRequest("POST", "/institution/profile", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	suite.app.Post("/institution/profile", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("user_role", user.Role)
		return suite.handler.CreateOrUpdateInstitutionProfile(c)
	})

	resp, err := suite.app.Test(req, -1) // -1 disables timeout
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	// Verify profile picture URL was saved
	var profile models.InstitutionProfile
	suite.db.Where("user_id = ?", user.ID).First(&profile)
	assert.NotEmpty(suite.T(), profile.ProfilePictureURL)
}

// Run the test suite
func TestInstitutionHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(InstitutionHandlerTestSuite))
}
