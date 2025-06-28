package handlers

import (
	"fmt"
	"log"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// ReviewHandler handles review-related requests
type ReviewHandler struct {
	db        *gorm.DB
	mqService queue.MessageQueueService
}

// NewReviewHandler creates a new ReviewHandler
func NewReviewHandler(db *gorm.DB, mqService queue.MessageQueueService) *ReviewHandler {
	return &ReviewHandler{db: db, mqService: mqService}
}

// CreateReviewRequest is the request body for creating a review
type CreateReviewRequest struct {
	SchoolID uint   `json:"school_id" validate:"required"`
	Rating   int    `json:"rating" validate:"required,min=1,max=5"`
	Comment  string `json:"comment" validate:"required,min=10,max=1000"`
}

// CreateReview creates a new review
func (h *ReviewHandler) CreateReview(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only parents and educators can leave reviews
	if user.Role != models.ParentRole && user.Role != models.EducatorRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only parents and educators can leave reviews"})
	}

	// Parse request
	var req CreateReviewRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Rating < 1 || req.Rating > 5 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Rating must be between 1 and 5"})
	}

	if len(req.Comment) < 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Comment must be at least 10 characters"})
	}

	// Check if school exists
	var school models.School
	if err := h.db.First(&school, req.SchoolID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found"})
	}

	// Check if user has already reviewed this school
	var existingReview models.Review
	err := h.db.Where("school_id = ? AND reviewer_id = ?", req.SchoolID, userID).First(&existingReview).Error
	if err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "You have already reviewed this school"})
	} else if err != gorm.ErrRecordNotFound {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing review"})
	}

	// Create review
	review := models.Review{
		SchoolID:   req.SchoolID,
		ReviewerID: userID,
		Rating:     req.Rating,
		Comment:    req.Comment,
		Status:     models.ReviewPending, // All reviews start as pending and need to be approved by an admin
	}

	if err := h.db.Create(&review).Error; err != nil {
		log.Printf("Error creating review: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create review"})
	}

	LogUserAction(h.db, userID, "REVIEW_CREATED", review.ID, "Review", fmt.Sprintf("Review created for school %d with rating %d", req.SchoolID, req.Rating), c)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Review submitted successfully and is pending approval",
		"review": fiber.Map{
			"id":         review.ID,
			"school_id":  review.SchoolID,
			"rating":     review.Rating,
			"comment":    review.Comment,
			"status":     review.Status,
			"created_at": review.CreatedAt,
		},
	})
}

// GetSchoolReviews gets all approved reviews for a school
func (h *ReviewHandler) GetSchoolReviews(c *fiber.Ctx) error {
	schoolID, err := c.ParamsInt("school_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid school ID"})
	}

	// Check if school exists
	var school models.School
	if err := h.db.First(&school, schoolID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "School not found"})
	}

	// Get approved reviews
	var reviews []models.Review
	if err := h.db.Where("school_id = ? AND status = ?", schoolID, models.ReviewApproved).
		Preload("Reviewer", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, first_name, last_name") // Only select necessary fields
		}).
		Order("created_at DESC").
		Find(&reviews).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve reviews"})
	}

	// Calculate average rating
	var totalRating int
	for _, review := range reviews {
		totalRating += review.Rating
	}
	var averageRating float64
	if len(reviews) > 0 {
		averageRating = float64(totalRating) / float64(len(reviews))
	}

	// Format response
	var formattedReviews []fiber.Map
	for _, review := range reviews {
		formattedReviews = append(formattedReviews, fiber.Map{
			"id":         review.ID,
			"rating":     review.Rating,
			"comment":    review.Comment,
			"created_at": review.CreatedAt,
			"reviewer": fiber.Map{
				"id":         review.Reviewer.ID,
				"first_name": review.Reviewer.FirstName,
				"last_name":  review.Reviewer.LastName,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"school_id":      schoolID,
		"average_rating": averageRating,
		"review_count":   len(reviews),
		"reviews":        formattedReviews,
	})
}

// GetUserReviews gets all reviews by the current user
func (h *ReviewHandler) GetUserReviews(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get reviews
	var reviews []models.Review
	if err := h.db.Where("reviewer_id = ?", userID).
		Preload("School").
		Order("created_at DESC").
		Find(&reviews).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve reviews"})
	}

	// Format response
	var formattedReviews []fiber.Map
	for _, review := range reviews {
		formattedReviews = append(formattedReviews, fiber.Map{
			"id":         review.ID,
			"rating":     review.Rating,
			"comment":    review.Comment,
			"status":     review.Status,
			"created_at": review.CreatedAt,
			"school": fiber.Map{
				"id":   review.School.ID,
				"name": review.School.Name,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"reviews": formattedReviews,
	})
}

// UpdateReview updates a review by the current user
func (h *ReviewHandler) UpdateReview(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	reviewID, err := c.ParamsInt("review_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid review ID"})
	}

	// Parse request
	var req CreateReviewRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Rating < 1 || req.Rating > 5 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Rating must be between 1 and 5"})
	}

	if len(req.Comment) < 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Comment must be at least 10 characters"})
	}

	// Get review
	var review models.Review
	if err := h.db.First(&review, reviewID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Review not found"})
	}

	// Check if user is the reviewer
	if review.ReviewerID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You can only update your own reviews"})
	}

	// Update review
	review.Rating = req.Rating
	review.Comment = req.Comment
	review.Status = models.ReviewPending // Reset to pending when updated
	if err := h.db.Save(&review).Error; err != nil {
		log.Printf("Error updating review: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update review"})
	}

	LogUserAction(h.db, userID, "REVIEW_UPDATED", review.ID, "Review", fmt.Sprintf("Review updated for school %d with rating %d", review.SchoolID, req.Rating), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Review updated successfully and is pending approval",
		"review": fiber.Map{
			"id":         review.ID,
			"school_id":  review.SchoolID,
			"rating":     review.Rating,
			"comment":    review.Comment,
			"status":     review.Status,
			"created_at": review.CreatedAt,
		},
	})
}

// DeleteReview deletes a review by the current user
func (h *ReviewHandler) DeleteReview(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	reviewID, err := c.ParamsInt("review_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid review ID"})
	}

	// Get review
	var review models.Review
	if err := h.db.First(&review, reviewID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Review not found"})
	}

	// Check if user is the reviewer
	if review.ReviewerID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You can only delete your own reviews"})
	}

	// Delete review
	if err := h.db.Delete(&review).Error; err != nil {
		log.Printf("Error deleting review: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete review"})
	}

	LogUserAction(h.db, userID, "REVIEW_DELETED", review.ID, "Review", fmt.Sprintf("Review deleted for school %d", review.SchoolID), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Review deleted successfully",
	})
}

// Admin endpoints

// ModerateReview approves or rejects a review (admin only)
func (h *ReviewHandler) ModerateReview(c *fiber.Ctx) error {
	adminID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	reviewID, err := c.ParamsInt("review_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid review ID"})
	}

	// Parse request
	type ModerateRequest struct {
		Status models.ReviewStatus `json:"status" validate:"required,oneof=approved rejected"`
		Notes  string              `json:"notes"`
	}
	var req ModerateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate status
	if req.Status != models.ReviewApproved && req.Status != models.ReviewRejected {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Status must be 'approved' or 'rejected'"})
	}

	// Get review
	var review models.Review
	if err := h.db.First(&review, reviewID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Review not found"})
	}

	// Update review
	review.Status = req.Status
	review.ModeratedBy = &adminID
	now := time.Now()
	review.ModeratedAt = &now
	review.ModeratorNotes = req.Notes

	if err := h.db.Save(&review).Error; err != nil {
		log.Printf("Error moderating review: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to moderate review"})
	}

	LogUserAction(h.db, adminID, "REVIEW_MODERATED", review.ID, "Review", fmt.Sprintf("Review moderated with status %s", req.Status), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": fmt.Sprintf("Review %s successfully", req.Status),
		"review": fiber.Map{
			"id":             review.ID,
			"school_id":      review.SchoolID,
			"reviewer_id":    review.ReviewerID,
			"rating":         review.Rating,
			"comment":        review.Comment,
			"status":         review.Status,
			"moderated_by":   review.ModeratedBy,
			"moderated_at":   review.ModeratedAt,
			"moderator_notes": review.ModeratorNotes,
		},
	})
}

// GetPendingReviews gets all pending reviews (admin only)
func (h *ReviewHandler) GetPendingReviews(c *fiber.Ctx) error {
	// Get pending reviews
	var reviews []models.Review
	if err := h.db.Where("status = ?", models.ReviewPending).
		Preload("School").
		Preload("Reviewer", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, first_name, last_name, email")
		}).
		Order("created_at ASC").
		Find(&reviews).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve pending reviews"})
	}

	// Format response
	var formattedReviews []fiber.Map
	for _, review := range reviews {
		formattedReviews = append(formattedReviews, fiber.Map{
			"id":         review.ID,
			"rating":     review.Rating,
			"comment":    review.Comment,
			"created_at": review.CreatedAt,
			"school": fiber.Map{
				"id":   review.School.ID,
				"name": review.School.Name,
			},
			"reviewer": fiber.Map{
				"id":         review.Reviewer.ID,
				"first_name": review.Reviewer.FirstName,
				"last_name":  review.Reviewer.LastName,
				"email":      review.Reviewer.Email,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"pending_reviews": formattedReviews,
	})
}