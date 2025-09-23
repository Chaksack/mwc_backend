package models

import (
	"time"

	"gorm.io/gorm"
)

// GormModel is a struct that includes common fields for all models
// @Description GORM model with common fields
// @Schema models.GormModel
type GormModel struct {
	ID        uint           `json:"id" gorm:"primarykey" example:"1"`
	CreatedAt time.Time      `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt time.Time      `json:"updated_at" example:"2023-01-01T00:00:00Z"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index" swaggertype:"string" format:"date-time" example:"2023-01-01T00:00:00Z"`
}

// UserRole defines the type for user roles
type UserRole string

// SubscriptionPlan defines the type for subscription plans
type SubscriptionPlan string

// SubscriptionStatus defines the type for subscription status
type SubscriptionStatus string

// ReviewStatus defines the type for review status
type ReviewStatus string

// SchoolCategory defines the type for school category
type SchoolCategory string

const (
	SuperAdminRole             UserRole = "superadmin"
	AdminRole                  UserRole = "admin"
	InstitutionRole            UserRole = "institution"
	MontessoriProfessionalRole UserRole = "montessori_professional"
	TrainingCenterRole         UserRole = "training_center"
	ParentRole                 UserRole = "parent"
)

const (
	FreePlan    SubscriptionPlan = "free"
	MonthlyPlan SubscriptionPlan = "monthly"
	AnnualPlan  SubscriptionPlan = "annual"
)

const (
	SubscriptionActive   SubscriptionStatus = "active"
	SubscriptionInactive SubscriptionStatus = "inactive"
	SubscriptionCanceled SubscriptionStatus = "canceled"
)

const (
	ReviewPending  ReviewStatus = "pending"
	ReviewApproved ReviewStatus = "approved"
	ReviewRejected ReviewStatus = "rejected"
)

const (
	SchoolCategorySchool         SchoolCategory = "school"
	SchoolCategoryTrainingCenter SchoolCategory = "training_center"
)

// User represents a user in the system
// @Description User information
// @Schema models.User
type User struct {
	GormModel           // Use GormModel for Swagger documentation
	Email        string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"` // Store hashed passwords only
	FirstName    string
	LastName     string
	Role         UserRole `gorm:"type:varchar(30);not null"`
	IsActive     bool     `gorm:"default:true"`
	LastLogin    *time.Time
	
	// Email verification fields
	EmailVerified             bool       `gorm:"default:false"`
	VerificationToken         *string    `gorm:"uniqueIndex"` // Token for email verification
	VerificationTokenExpiry   *time.Time // Expiry time for verification token

	// Password reset fields
	PasswordResetToken        *string    `gorm:"uniqueIndex"` // Token for password reset
	PasswordResetTokenExpiry  *time.Time // Expiry time for password reset token

	// Relationships (depending on role)
	InstitutionProfile            *InstitutionProfile            `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // For Institution/TrainingCenter
	MontessoriProfessionalProfile *MontessoriProfessionalProfile `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // For Montessori Professional
	ParentProfile                 *ParentProfile                 `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // For Parent
}

// School represents a school
// @Description School information
// @Schema models.School
type School struct {
	GormModel                      // Use GormModel for Swagger documentation
	Name            string         `gorm:"not null"`
	Category        SchoolCategory `gorm:"type:varchar(20);not null;default:'school';index"` // For filtering by category
	Address         string
	City            string
	State           string
	CountryCode     string `gorm:"index;not null"` // For searching by country
	Country         string // Full country name
	ZipCode         string
	ContactEmail    string
	ContactPhone    string
	Website         string
	SearchString    string // Search query used to find this school
	SearchPageUrl   string // URL of the search page where this school was found
	UploadedByAdmin bool   `gorm:"default:false"` // True if uploaded by admin batch
	CreatedByUserID *uint  // Pointer to allow NULL if uploaded by admin initially
	User            *User  `gorm:"foreignKey:CreatedByUserID"`
	Member          bool   `gorm:"default:false"` // True if an institution/training center has selected this school
	Hiring          bool   `gorm:"default:false"` // True if the associated institution has active job postings
}

// InstitutionProfile for Institution and Training Center users
// @Description Institution or Training Center profile information
// @Schema models.InstitutionProfile
type InstitutionProfile struct {
	GormModel               // Use GormModel for Swagger documentation
	UserID           uint   `gorm:"uniqueIndex;not null"` // Foreign key to User table
	User             User   // Eager load user details if needed
	InstitutionName  string `gorm:"not null"`
	SchoolID         *uint  `gorm:"uniqueIndex"` // A school can be mapped to only one institution/training center
	School           *School
	VerificationDocs string // Path to verification documents
	IsVerified       bool   `gorm:"default:false"`
	Jobs             []Job  `gorm:"foreignKey:InstitutionProfileID"`
}

// MontessoriProfessionalProfile for Montessori Professional users
// @Description Montessori Professional profile information
// @Schema models.MontessoriProfessionalProfile
type MontessoriProfessionalProfile struct {
	GormModel           // Use GormModel for Swagger documentation
	UserID         uint `gorm:"uniqueIndex;not null"`
	User           User // Eager load user details
	Bio            string
	Qualifications string
	Experience     string
	SavedSchools   []*School        `gorm:"many2many:montessori_professional_saved_schools;"`
	Applications   []JobApplication `gorm:"foreignKey:MontessoriProfessionalProfileID"`
}

// ParentProfile for Parent users
// @Description Parent profile information
// @Schema models.ParentProfile
type ParentProfile struct {
	GormModel                        // Use GormModel for Swagger documentation
	UserID           uint            `gorm:"uniqueIndex;not null"`
	User             User            // Eager load user details
	ProfileVisibility string         `gorm:"default:'public'"` // "public" or "private"
	ParentAge        int             // Age of the parent
	SavedSchools     []*School       `gorm:"many2many:parent_saved_schools;"`
	Schools          []*School       `gorm:"many2many:parent_children_schools;"` // Schools the parent's children attend
}

// Job posted by an Institution or Training Center
// @Description Job posting information
// @Schema models.Job
type Job struct {
	GormModel                               // Use GormModel for Swagger documentation
	InstitutionProfileID uint               `gorm:"not null"` // Links to InstitutionProfile
	InstitutionProfile   InstitutionProfile // Eager load institution profile
	Title                string             `gorm:"not null"`
	Description          string             `gorm:"type:text"`
	Location             string
	EmploymentType       string // e.g., Full-time, Part-time
	SalaryRange          string
	PostedAt             time.Time `gorm:"autoCreateTime"`
	ExpiresAt            *time.Time
	IsActive             bool             `gorm:"default:true"`
	Applications         []JobApplication `gorm:"foreignKey:JobID"`
}

// JobApplication by a Montessori Professional
// @Description Job application information
// @Schema models.JobApplication
type JobApplication struct {
	GormModel
	JobID                           uint `gorm:"not null"`
	Job                             Job
	MontessoriProfessionalProfileID uint   `gorm:"not null"` // Links to MontessoriProfessionalProfile
	CoverLetter                     string `gorm:"type:text"`
	ResumeURL                       string
	AppliedAt                       time.Time                     `gorm:"autoCreateTime"`
	Status                          string                        `gorm:"default:'pending'"` // e.g., pending, viewed, shortlisted, rejected
	MontessoriProfessional          MontessoriProfessionalProfile `gorm:"foreignKey:MontessoriProfessionalProfileID"`
}

// Message between Parents
// @Description Message information
// @Schema models.Message
type Message struct {
	GormModel
	SenderID    uint      `gorm:"not null"`
	RecipientID uint      `gorm:"not null"`
	Content     string    `gorm:"type:text;not null"`
	SentAt      time.Time `gorm:"autoCreateTime"`
	ReadAt      *time.Time
	IsRead      bool `gorm:"default:false;index"` // Index for faster querying of unread messages
	Sender      User `gorm:"foreignKey:SenderID"`
	Recipient   User `gorm:"foreignKey:RecipientID"`
}

// ActionLog for admin to track user actions
// @Description Action log information
// @Schema models.ActionLog
type ActionLog struct {
	GormModel
	UserID      *uint     `gorm:"index"` // User who performed the action (can be nil for system actions)
	User        *User     `gorm:"foreignKey:UserID"`
	ActionType  string    // e.g., "SCHOOL_CREATE", "JOB_POST", "USER_REGISTER"
	TargetID    uint      // e.g., ID of the school created, job posted
	TargetType  string    // e.g., "School", "Job"
	Details     string    `gorm:"type:text"` // JSON string or textual details
	PerformedAt time.Time `gorm:"autoCreateTime"`
	IPAddress   string
	UserAgent   string
}

// Event represents an event posted by a school or training center
// @Description Event information
// @Schema models.Event
type Event struct {
	GormModel
	CreatorID       uint               `gorm:"not null;index"` // User who created the event
	Creator         User               `gorm:"foreignKey:CreatorID"`
	InstitutionID   uint               `gorm:"not null;index"` // Institution that hosts the event
	Institution     InstitutionProfile `gorm:"foreignKey:InstitutionID"`
	Title           string             `gorm:"not null"`
	Description     string             `gorm:"type:text"`
	StartDate       time.Time          `gorm:"not null"`
	EndDate         time.Time          `gorm:"not null"`
	Location        string
	VirtualEvent    bool      `gorm:"default:false"`
	VirtualEventURL string    // URL for virtual events
	EventType       string    // e.g., "Workshop", "Open House", "Conference"
	Audience        string    // e.g., "Parents", "Educators", "All"
	PublishedAt     time.Time `gorm:"index"`
	IsPublished     bool      `gorm:"default:false"`
	IsFeatured      bool      `gorm:"default:false"`
	MaxAttendees    int       // Maximum number of attendees, 0 for unlimited
	// I18n support
	LocalizedTitles       map[string]string `gorm:"type:jsonb"` // e.g., {"en": "Title", "es": "Título"}
	LocalizedDescriptions map[string]string `gorm:"type:jsonb"` // e.g., {"en": "Description", "es": "Descripción"}
}

// BlogCategory represents a blog category
// @Description Blog category information
// @Schema models.BlogCategory
type BlogCategory struct {
	GormModel
	Name        string `gorm:"uniqueIndex;not null"`
	Slug        string `gorm:"uniqueIndex;not null"` // URL-friendly version of the name
	Description string `gorm:"type:text"`
	PostCount   int    `gorm:"default:0"` // Count of posts in this category
}

// BlogTag represents a blog tag
// @Description Blog tag information
// @Schema models.BlogTag
type BlogTag struct {
	GormModel
	Name      string `gorm:"uniqueIndex;not null"`
	Slug      string `gorm:"uniqueIndex;not null"` // URL-friendly version of the name
	PostCount int    `gorm:"default:0"`            // Count of posts with this tag
}

// BlogPost represents a blog post or article
// @Description Blog post information
// @Schema models.BlogPost
type BlogPost struct {
	GormModel
	AuthorID    uint       `gorm:"not null;index"` // User who wrote the post
	Author      User       `gorm:"foreignKey:AuthorID"`
	Title       string     `gorm:"not null"`
	Slug        string     `gorm:"uniqueIndex;not null"` // URL-friendly version of the title
	Content     string     `gorm:"type:text;not null"`
	Excerpt     string     `gorm:"type:text"`
	PublishedAt *time.Time `gorm:"index"`
	IsPublished bool       `gorm:"default:false"`
	IsFeatured  bool       `gorm:"default:false"`
	ViewCount   int        `gorm:"default:0"`
	Category    string     `gorm:"index"`
	Tags        []string   `gorm:"type:text[]"`
	// I18n support
	LocalizedTitles   map[string]string `gorm:"type:jsonb"` // e.g., {"en": "Title", "es": "Título"}
	LocalizedContents map[string]string `gorm:"type:jsonb"` // e.g., {"en": "Content", "es": "Contenido"}
	LocalizedExcerpts map[string]string `gorm:"type:jsonb"` // e.g., {"en": "Excerpt", "es": "Extracto"}
}

// Subscription represents a premium subscription
// @Description Subscription information
// @Schema models.Subscription
type Subscription struct {
	GormModel
	UserID               uint               `gorm:"not null;index"` // User who has the subscription
	User                 User               `gorm:"foreignKey:UserID"`
	Plan                 SubscriptionPlan   `gorm:"type:varchar(20);not null"`
	Status               SubscriptionStatus `gorm:"type:varchar(20);not null"`
	StartDate            time.Time          `gorm:"not null"`
	EndDate              time.Time          `gorm:"not null"`
	AutoRenew            bool               `gorm:"default:true"`
	StripeCustomerID     string             `gorm:"index"`
	StripeSubscriptionID string             `gorm:"index"`
	CancelledAt          *time.Time
	CancellationReason   string
}

// Review represents a review of a school
// @Description Review information
// @Schema models.Review
type Review struct {
	GormModel
	SchoolID       uint         `gorm:"not null;index"` // School being reviewed
	School         School       `gorm:"foreignKey:SchoolID"`
	ReviewerID     uint         `gorm:"not null;index"` // User who wrote the review
	Reviewer       User         `gorm:"foreignKey:ReviewerID"`
	Rating         int          `gorm:"not null"` // 1-5 stars
	Comment        string       `gorm:"type:text"`
	Status         ReviewStatus `gorm:"type:varchar(20);not null;default:'pending'"`
	ModeratedBy    *uint        // Admin who moderated the review
	ModeratedAt    *time.Time
	ModeratorNotes string `gorm:"type:text"` // Notes from the moderator
}

// AutoMigrate runs GORM's auto migration.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&School{},
		&InstitutionProfile{},
		&MontessoriProfessionalProfile{},
		&ParentProfile{},
		&Job{},
		&JobApplication{},
		&Message{},
		&ActionLog{},
		&Event{},
		&BlogCategory{},
		&BlogTag{},
		&BlogPost{},
		&Subscription{},
		&Review{},
	)
}
