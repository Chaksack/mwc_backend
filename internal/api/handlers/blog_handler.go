package handlers

import (
	"fmt"
	"log"
	"mwc_backend/config"
	"mwc_backend/internal/models"
	"mwc_backend/internal/queue"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gosimple/slug"
	"gorm.io/gorm"
)

// BlogHandler handles blog-related requests
type BlogHandler struct {
	db        *gorm.DB
	cfg       *config.Config
	mqService queue.MessageQueueService
}

// NewBlogHandler creates a new BlogHandler
func NewBlogHandler(db *gorm.DB, cfg *config.Config, mqService queue.MessageQueueService) *BlogHandler {
	return &BlogHandler{db: db, cfg: cfg, mqService: mqService}
}

// CreateBlogPostRequest is the request body for creating a blog post
type CreateBlogPostRequest struct {
	Title       string            `json:"title" validate:"required"`
	Content     string            `json:"content" validate:"required"`
	Excerpt     string            `json:"excerpt"`
	Category    string            `json:"category" validate:"required"`
	Tags        []string          `json:"tags"`
	IsPublished bool              `json:"is_published"`
	IsFeatured  bool              `json:"is_featured"`
	Localizations map[string]map[string]string `json:"localizations"` // Map of language code to localized fields
}

// CreateBlogPost creates a new blog post
// @Summary Create a new blog post
// @Description Creates a new blog post (admin only)
// @Tags blog,admin
// @Accept json
// @Produce json
// @Param request body CreateBlogPostRequest true "Blog post information"
// @Success 201 {object} map[string]interface{} "Blog post created successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins can create blog posts"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /api/v1/admin/blog [post]
func (h *BlogHandler) CreateBlogPost(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins can create blog posts
	if user.Role != models.AdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins can create blog posts"})
	}

	// Parse request
	var req CreateBlogPostRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Title is required"})
	}

	if req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Content is required"})
	}

	if req.Category == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Category is required"})
	}

	// Generate slug from title
	postSlug := slug.Make(req.Title)

	// Check if slug already exists
	var existingPost models.BlogPost
	err := h.db.Where("slug = ?", postSlug).First(&existingPost).Error
	if err == nil {
		// Slug already exists, append a timestamp to make it unique
		postSlug = fmt.Sprintf("%s-%d", postSlug, time.Now().Unix())
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("Error checking existing blog post: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing blog post"})
	}

	// Process localizations
	localizedTitles := make(map[string]string)
	localizedContents := make(map[string]string)
	localizedExcerpts := make(map[string]string)

	for lang, fields := range req.Localizations {
		if title, ok := fields["title"]; ok {
			localizedTitles[lang] = title
		}
		if content, ok := fields["content"]; ok {
			localizedContents[lang] = content
		}
		if excerpt, ok := fields["excerpt"]; ok {
			localizedExcerpts[lang] = excerpt
		}
	}

	// Create blog post
	blogPost := models.BlogPost{
		AuthorID:           userID,
		Title:              req.Title,
		Slug:               postSlug,
		Content:            req.Content,
		Excerpt:            req.Excerpt,
		Category:           req.Category,
		Tags:               req.Tags,
		IsPublished:        req.IsPublished,
		IsFeatured:         req.IsFeatured,
		LocalizedTitles:    localizedTitles,
		LocalizedContents:  localizedContents,
		LocalizedExcerpts:  localizedExcerpts,
	}

	if req.IsPublished {
		now := time.Now()
		blogPost.PublishedAt = &now
	}

	if err := h.db.Create(&blogPost).Error; err != nil {
		log.Printf("Error creating blog post: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create blog post"})
	}

	LogUserAction(h.db, userID, "BLOG_POST_CREATED", blogPost.ID, "BlogPost", fmt.Sprintf("Blog post created: %s", req.Title), c)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Blog post created successfully",
		"blog_post": fiber.Map{
			"id":           blogPost.ID,
			"title":        blogPost.Title,
			"slug":         blogPost.Slug,
			"excerpt":      blogPost.Excerpt,
			"category":     blogPost.Category,
			"tags":         blogPost.Tags,
			"is_published": blogPost.IsPublished,
			"is_featured":  blogPost.IsFeatured,
			"published_at": blogPost.PublishedAt,
			"created_at":   blogPost.CreatedAt,
		},
	})
}

// GetBlogPosts gets all published blog posts
// @Summary Get all published blog posts
// @Description Retrieves all published blog posts with optional filtering
// @Tags blog
// @Produce json
// @Param category query string false "Filter by category"
// @Param tag query string false "Filter by tag"
// @Param language query string false "Language for localized content"
// @Success 200 {object} map[string]interface{} "List of blog posts"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/blog [get]
func (h *BlogHandler) GetBlogPosts(c *fiber.Ctx) error {
	// Parse query parameters
	category := c.Query("category")
	tag := c.Query("tag")
	language := c.Query("language", h.cfg.DefaultLanguage)

	// Build query
	query := h.db.Where("is_published = ?", true)

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if tag != "" {
		query = query.Where("? = ANY(tags)", tag)
	}

	// Get blog posts
	var blogPosts []models.BlogPost
	if err := query.
		Preload("Author", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, first_name, last_name")
		}).
		Order("published_at DESC").
		Find(&blogPosts).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve blog posts"})
	}

	// Format response
	var formattedPosts []fiber.Map
	for _, post := range blogPosts {
		// Get localized fields if available
		title := post.Title
		content := post.Content
		excerpt := post.Excerpt

		if localizedTitle, ok := post.LocalizedTitles[language]; ok && localizedTitle != "" {
			title = localizedTitle
		}

		if localizedContent, ok := post.LocalizedContents[language]; ok && localizedContent != "" {
			content = localizedContent
		}

		if localizedExcerpt, ok := post.LocalizedExcerpts[language]; ok && localizedExcerpt != "" {
			excerpt = localizedExcerpt
		}

		// If no excerpt is provided, generate one from the content
		if excerpt == "" {
			excerpt = generateExcerpt(content, 150)
		}

		formattedPosts = append(formattedPosts, fiber.Map{
			"id":           post.ID,
			"title":        title,
			"slug":         post.Slug,
			"excerpt":      excerpt,
			"category":     post.Category,
			"tags":         post.Tags,
			"published_at": post.PublishedAt,
			"author": fiber.Map{
				"id":         post.Author.ID,
				"first_name": post.Author.FirstName,
				"last_name":  post.Author.LastName,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"blog_posts": formattedPosts,
	})
}

// GetBlogPost gets a specific blog post by slug
// @Summary Get a specific blog post
// @Description Retrieves a specific blog post by its slug
// @Tags blog
// @Produce json
// @Param slug path string true "Blog post slug"
// @Param language query string false "Language for localized content"
// @Success 200 {object} map[string]interface{} "Blog post details"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 404 {object} map[string]string "Blog post not found"
// @Router /api/v1/blog/{slug} [get]
func (h *BlogHandler) GetBlogPost(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Slug is required"})
	}

	language := c.Query("language", h.cfg.DefaultLanguage)

	// Get blog post
	var blogPost models.BlogPost
	if err := h.db.Where("slug = ?", slug).
		Preload("Author", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, first_name, last_name")
		}).
		First(&blogPost).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog post not found"})
	}

	// Check if blog post is published
	if !blogPost.IsPublished {
		// If user is authenticated, check if they are the author or an admin
		userID, ok := c.Locals("user_id").(uint)
		if !ok || userID != blogPost.AuthorID {
			var user models.User
			if !ok || h.db.First(&user, userID).Error != nil || user.Role != models.AdminRole {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog post not found"})
			}
		}
	}

	// Get localized fields if available
	title := blogPost.Title
	content := blogPost.Content
	excerpt := blogPost.Excerpt

	if localizedTitle, ok := blogPost.LocalizedTitles[language]; ok && localizedTitle != "" {
		title = localizedTitle
	}

	if localizedContent, ok := blogPost.LocalizedContents[language]; ok && localizedContent != "" {
		content = localizedContent
	}

	if localizedExcerpt, ok := blogPost.LocalizedExcerpts[language]; ok && localizedExcerpt != "" {
		excerpt = localizedExcerpt
	}

	// If no excerpt is provided, generate one from the content
	if excerpt == "" {
		excerpt = generateExcerpt(content, 150)
	}

	// Increment view count
	if err := h.db.Model(&blogPost).Update("view_count", gorm.Expr("view_count + ?", 1)).Error; err != nil {
		log.Printf("Error updating view count: %v", err)
		// Don't return an error, just log it
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"blog_post": fiber.Map{
			"id":           blogPost.ID,
			"title":        title,
			"slug":         blogPost.Slug,
			"content":      content,
			"excerpt":      excerpt,
			"category":     blogPost.Category,
			"tags":         blogPost.Tags,
			"published_at": blogPost.PublishedAt,
			"view_count":   blogPost.ViewCount + 1, // Include the incremented view count
			"author": fiber.Map{
				"id":         blogPost.Author.ID,
				"first_name": blogPost.Author.FirstName,
				"last_name":  blogPost.Author.LastName,
			},
		},
	})
}

// UpdateBlogPost updates a blog post
// @Summary Update a blog post
// @Description Updates an existing blog post (admin only)
// @Tags blog,admin
// @Accept json
// @Produce json
// @Param post_id path int true "Blog post ID"
// @Param request body CreateBlogPostRequest true "Updated blog post information"
// @Success 200 {object} map[string]interface{} "Blog post updated successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins can update blog posts"
// @Failure 404 {object} map[string]string "Blog post not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /api/v1/admin/blog/{post_id} [put]
func (h *BlogHandler) UpdateBlogPost(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins can update blog posts
	if user.Role != models.AdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins can update blog posts"})
	}

	postID, err := c.ParamsInt("post_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid post ID"})
	}

	// Get blog post
	var blogPost models.BlogPost
	if err := h.db.First(&blogPost, postID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog post not found"})
	}

	// Parse request
	var req CreateBlogPostRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Title is required"})
	}

	if req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Content is required"})
	}

	if req.Category == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Category is required"})
	}

	// Check if title has changed, if so, update the slug
	if req.Title != blogPost.Title {
		newSlug := slug.Make(req.Title)

		// Check if new slug already exists
		var existingPost models.BlogPost
		err := h.db.Where("slug = ? AND id != ?", newSlug, postID).First(&existingPost).Error
		if err == nil {
			// Slug already exists, append a timestamp to make it unique
			newSlug = fmt.Sprintf("%s-%d", newSlug, time.Now().Unix())
		} else if err != gorm.ErrRecordNotFound {
			log.Printf("Error checking existing blog post: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing blog post"})
		}

		blogPost.Slug = newSlug
	}

	// Process localizations
	localizedTitles := make(map[string]string)
	localizedContents := make(map[string]string)
	localizedExcerpts := make(map[string]string)

	for lang, fields := range req.Localizations {
		if title, ok := fields["title"]; ok {
			localizedTitles[lang] = title
		}
		if content, ok := fields["content"]; ok {
			localizedContents[lang] = content
		}
		if excerpt, ok := fields["excerpt"]; ok {
			localizedExcerpts[lang] = excerpt
		}
	}

	// Update blog post
	blogPost.Title = req.Title
	blogPost.Content = req.Content
	blogPost.Excerpt = req.Excerpt
	blogPost.Category = req.Category
	blogPost.Tags = req.Tags
	blogPost.LocalizedTitles = localizedTitles
	blogPost.LocalizedContents = localizedContents
	blogPost.LocalizedExcerpts = localizedExcerpts
	blogPost.IsFeatured = req.IsFeatured

	// Update published status if changed
	if req.IsPublished != blogPost.IsPublished {
		blogPost.IsPublished = req.IsPublished
		if req.IsPublished {
			now := time.Now()
			blogPost.PublishedAt = &now
		}
	}

	if err := h.db.Save(&blogPost).Error; err != nil {
		log.Printf("Error updating blog post: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update blog post"})
	}

	LogUserAction(h.db, userID, "BLOG_POST_UPDATED", blogPost.ID, "BlogPost", fmt.Sprintf("Blog post updated: %s", req.Title), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Blog post updated successfully",
		"blog_post": fiber.Map{
			"id":           blogPost.ID,
			"title":        blogPost.Title,
			"slug":         blogPost.Slug,
			"excerpt":      blogPost.Excerpt,
			"category":     blogPost.Category,
			"tags":         blogPost.Tags,
			"is_published": blogPost.IsPublished,
			"is_featured":  blogPost.IsFeatured,
			"published_at": blogPost.PublishedAt,
			"updated_at":   blogPost.UpdatedAt,
		},
	})
}

// DeleteBlogPost deletes a blog post
// @Summary Delete a blog post
// @Description Deletes an existing blog post (admin only)
// @Tags blog,admin
// @Produce json
// @Param post_id path int true "Blog post ID"
// @Success 200 {object} map[string]string "Blog post deleted successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins can delete blog posts"
// @Failure 404 {object} map[string]string "Blog post not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /api/v1/admin/blog/{post_id} [delete]
func (h *BlogHandler) DeleteBlogPost(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins can delete blog posts
	if user.Role != models.AdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins can delete blog posts"})
	}

	postID, err := c.ParamsInt("post_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid post ID"})
	}

	// Get blog post
	var blogPost models.BlogPost
	if err := h.db.First(&blogPost, postID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog post not found"})
	}

	// Delete blog post
	if err := h.db.Delete(&blogPost).Error; err != nil {
		log.Printf("Error deleting blog post: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete blog post"})
	}

	LogUserAction(h.db, userID, "BLOG_POST_DELETED", blogPost.ID, "BlogPost", fmt.Sprintf("Blog post deleted: %s", blogPost.Title), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Blog post deleted successfully",
	})
}

// GetFeaturedBlogPosts gets all featured blog posts
// @Summary Get featured blog posts
// @Description Retrieves all featured and published blog posts
// @Tags blog
// @Produce json
// @Param language query string false "Language for localized content"
// @Success 200 {object} map[string]interface{} "List of featured blog posts"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/blog/featured [get]
func (h *BlogHandler) GetFeaturedBlogPosts(c *fiber.Ctx) error {
	language := c.Query("language", h.cfg.DefaultLanguage)

	// Get featured blog posts
	var blogPosts []models.BlogPost
	if err := h.db.Where("is_featured = ? AND is_published = ?", true, true).
		Preload("Author", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, first_name, last_name")
		}).
		Order("published_at DESC").
		Find(&blogPosts).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve featured blog posts"})
	}

	// Format response
	var formattedPosts []fiber.Map
	for _, post := range blogPosts {
		// Get localized fields if available
		title := post.Title
		excerpt := post.Excerpt

		if localizedTitle, ok := post.LocalizedTitles[language]; ok && localizedTitle != "" {
			title = localizedTitle
		}

		if localizedExcerpt, ok := post.LocalizedExcerpts[language]; ok && localizedExcerpt != "" {
			excerpt = localizedExcerpt
		}

		// If no excerpt is provided, generate one from the content
		if excerpt == "" {
			content := post.Content
			if localizedContent, ok := post.LocalizedContents[language]; ok && localizedContent != "" {
				content = localizedContent
			}
			excerpt = generateExcerpt(content, 150)
		}

		formattedPosts = append(formattedPosts, fiber.Map{
			"id":           post.ID,
			"title":        title,
			"slug":         post.Slug,
			"excerpt":      excerpt,
			"category":     post.Category,
			"tags":         post.Tags,
			"published_at": post.PublishedAt,
			"author": fiber.Map{
				"id":         post.Author.ID,
				"first_name": post.Author.FirstName,
				"last_name":  post.Author.LastName,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"featured_posts": formattedPosts,
	})
}

// GetBlogCategories gets all blog categories
// @Summary Get all blog categories
// @Description Retrieves all distinct categories from published blog posts
// @Tags blog
// @Produce json
// @Success 200 {object} map[string]interface{} "List of blog categories"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/blog/categories [get]
func (h *BlogHandler) GetBlogCategories(c *fiber.Ctx) error {
	// Get all distinct categories from published blog posts
	var categories []string
	if err := h.db.Model(&models.BlogPost{}).
		Where("is_published = ?", true).
		Distinct().
		Pluck("category", &categories).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve blog categories"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"categories": categories,
	})
}

// GetBlogTags gets all blog tags
// @Summary Get all blog tags
// @Description Retrieves all unique tags from published blog posts
// @Tags blog
// @Produce json
// @Success 200 {object} map[string]interface{} "List of blog tags"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/blog/tags [get]
func (h *BlogHandler) GetBlogTags(c *fiber.Ctx) error {
	// Get all tags from published blog posts
	var blogPosts []models.BlogPost
	if err := h.db.Where("is_published = ?", true).
		Select("tags").
		Find(&blogPosts).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve blog tags"})
	}

	// Collect all unique tags
	tagMap := make(map[string]bool)
	for _, post := range blogPosts {
		for _, tag := range post.Tags {
			tagMap[tag] = true
		}
	}

	// Convert map to slice
	var tags []string
	for tag := range tagMap {
		tags = append(tags, tag)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"tags": tags,
	})
}

// Helper function to generate an excerpt from content
func generateExcerpt(content string, maxLength int) string {
	// Strip HTML tags (simplified approach)
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\r", " ")

	// Truncate to maxLength
	if len(content) > maxLength {
		return content[:maxLength] + "..."
	}
	return content
}
