package models

import (
	"time"

	"gorm.io/gorm"
)

// UserRole defines the type for user roles
type UserRole string

// SubscriptionPlan defines the type for subscription plans
type SubscriptionPlan string

// SubscriptionStatus defines the type for subscription status
type SubscriptionStatus string

// ReviewStatus defines the type for review status
type ReviewStatus string

const (
	AdminRole          UserRole = "admin"
	InstitutionRole    UserRole = "institution"
	EducatorRole       UserRole = "educator"
	TrainingCenterRole UserRole = "training_center"
	ParentRole         UserRole = "parent"
)

const (
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

// User represents a user in the system
type User struct {
	gorm.Model
	Email        string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"` // Store hashed passwords only
	FirstName    string
	LastName     string
	Role         UserRole `gorm:"type:varchar(20);not null"`
	IsActive     bool     `gorm:"default:true"`
	LastLogin    *time.Time

	// Relationships (depending on role)
	InstitutionProfile *InstitutionProfile `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // For Institution/TrainingCenter
	EducatorProfile    *EducatorProfile    `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // For Educator
	ParentProfile      *ParentProfile      `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // For Parent
}

// School represents a school
type School struct {
	gorm.Model
	Name            string `gorm:"not null"`
	Address         string
	City            string
	State           string
	CountryCode     string `gorm:"index;not null"` // For searching by country
	ZipCode         string
	ContactEmail    string
	ContactPhone    string
	Website         string
	UploadedByAdmin bool  `gorm:"default:false"` // True if uploaded by admin batch
	CreatedByUserID *uint // Pointer to allow NULL if uploaded by admin initially
	User            *User `gorm:"foreignKey:CreatedByUserID"`
}

// InstitutionProfile for Institution and Training Center users
type InstitutionProfile struct {
	gorm.Model
	UserID           uint   `gorm:"uniqueIndex;not null"` // Foreign key to User table
	User             User   // Eager load user details if needed
	InstitutionName  string `gorm:"not null"`
	SchoolID         *uint  `gorm:"uniqueIndex"` // A school can be mapped to only one institution/training center
	School           *School
	VerificationDocs string // Path to verification documents
	IsVerified       bool   `gorm:"default:false"`
	Jobs             []Job  `gorm:"foreignKey:InstitutionProfileID"`
}

// EducatorProfile for Educator users
type EducatorProfile struct {
	gorm.Model
	UserID         uint `gorm:"uniqueIndex;not null"`
	User           User // Eager load user details
	Bio            string
	Qualifications string
	Experience     string
	SavedSchools   []*School        `gorm:"many2many:educator_saved_schools;"`
	Applications   []JobApplication `gorm:"foreignKey:EducatorProfileID"`
}

// ParentProfile for Parent users
type ParentProfile struct {
	gorm.Model
	UserID       uint      `gorm:"uniqueIndex;not null"`
	User         User      // Eager load user details
	SavedSchools []*School `gorm:"many2many:parent_saved_schools;"`
	// Other parent-specific fields
}

// Job posted by an Institution or Training Center
type Job struct {
	gorm.Model
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

// JobApplication by an Educator
type JobApplication struct {
	gorm.Model
	JobID             uint `gorm:"not null"`
	Job               Job
	EducatorProfileID uint   `gorm:"not null"` // Links to EducatorProfile
	CoverLetter       string `gorm:"type:text"`
	ResumeURL         string
	AppliedAt         time.Time       `gorm:"autoCreateTime"`
	Status            string          `gorm:"default:'pending'"` // e.g., pending, viewed, shortlisted, rejected
	Educator          EducatorProfile `gorm:"foreignKey:EducatorProfileID"`
}

// Message between Parents
type Message struct {
	gorm.Model
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
type ActionLog struct {
	gorm.Model
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
type Event struct {
	gorm.Model
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

// BlogPost represents a blog post or article
type BlogPost struct {
	gorm.Model
	AuthorID    uint   `gorm:"not null;index"` // User who wrote the post
	Author      User   `gorm:"foreignKey:AuthorID"`
	Title       string `gorm:"not null"`
	Slug        string `gorm:"uniqueIndex;not null"` // URL-friendly version of the title
	Content     string `gorm:"type:text;not null"`
	Excerpt     string `gorm:"type:text"`
	PublishedAt *time.Time `gorm:"index"`
	IsPublished bool       `gorm:"default:false"`
	IsFeatured  bool       `gorm:"default:false"`
	ViewCount   int        `gorm:"default:0"`
	Category    string     `gorm:"index"`
	Tags        []string   `gorm:"type:text[]"`
	// I18n support
	LocalizedTitles    map[string]string `gorm:"type:jsonb"` // e.g., {"en": "Title", "es": "Título"}
	LocalizedContents  map[string]string `gorm:"type:jsonb"` // e.g., {"en": "Content", "es": "Contenido"}
	LocalizedExcerpts  map[string]string `gorm:"type:jsonb"` // e.g., {"en": "Excerpt", "es": "Extracto"}
}

// Subscription represents a premium subscription
type Subscription struct {
	gorm.Model
	UserID             uint               `gorm:"not null;index"` // User who has the subscription
	User               User               `gorm:"foreignKey:UserID"`
	Plan               SubscriptionPlan   `gorm:"type:varchar(20);not null"`
	Status             SubscriptionStatus `gorm:"type:varchar(20);not null"`
	StartDate          time.Time          `gorm:"not null"`
	EndDate            time.Time          `gorm:"not null"`
	AutoRenew          bool               `gorm:"default:true"`
	StripeCustomerID   string             `gorm:"index"`
	StripeSubscriptionID string           `gorm:"index"`
	CancelledAt        *time.Time
	CancellationReason string
}

// Review represents a review of a school
type Review struct {
	gorm.Model
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
		&EducatorProfile{},
		&ParentProfile{},
		&Job{},
		&JobApplication{},
		&Message{},
		&ActionLog{},
		&Event{},
		&BlogPost{},
		&Subscription{},
		&Review{},
	)
}
