package models

import (
	"time"

	"gorm.io/gorm"
)

// UserRole defines the type for user roles
type UserRole string

const (
	AdminRole          UserRole = "admin"
	InstitutionRole    UserRole = "institution"
	EducatorRole       UserRole = "educator"
	TrainingCenterRole UserRole = "training_center"
	ParentRole         UserRole = "parent"
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
	)
}
