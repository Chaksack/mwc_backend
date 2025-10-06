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
	Title         string                       `json:"title" validate:"required"`
	Content       string                       `json:"content" validate:"required"`
	Excerpt       string                       `json:"excerpt"`
	Category      string                       `json:"category" validate:"required"`
	Tags          []string                     `json:"tags"`
	IsPublished   bool                         `json:"is_published"`
	IsFeatured    bool                         `json:"is_featured"`
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
// @Failure 403 {object} map[string]string "Only admins and superadmins can create blog posts"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog [post]
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

	// Only admins and superadmins can create blog posts
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can create blog posts"})
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

	// Initialize tags to empty slice if nil to prevent database errors
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	// Create blog post
	blogPost := models.BlogPost{
		AuthorID:          userID,
		Title:             req.Title,
		Slug:              postSlug,
		Content:           req.Content,
		Excerpt:           req.Excerpt,
		Category:          req.Category,
		Tags:              tags,
		IsPublished:       req.IsPublished,
		IsFeatured:        req.IsFeatured,
		LocalizedTitles:   localizedTitles,
		LocalizedContents: localizedContents,
		LocalizedExcerpts: localizedExcerpts,
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
// @Router /blog [get]
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
// @Router /blog/{slug} [get]
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
// @Failure 403 {object} map[string]string "Only admins and superadmins can update blog posts"
// @Failure 404 {object} map[string]string "Blog post not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/{post_id} [put]
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
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can update blog posts"})
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

	// Initialize tags to empty slice if nil to prevent database errors
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	// Update blog post
	blogPost.Title = req.Title
	blogPost.Content = req.Content
	blogPost.Excerpt = req.Excerpt
	blogPost.Category = req.Category
	blogPost.Tags = tags
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
// @Failure 403 {object} map[string]string "Only admins and superadmins can delete blog posts"
// @Failure 404 {object} map[string]string "Blog post not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/{post_id} [delete]
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
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can delete blog posts"})
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
// @Router /blog/featured [get]
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
// @Router /blog/categories [get]
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
// @Router /blog/tags [get]
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

// --- Category Management Functions (Admin Only) ---

// CreateBlogCategoryRequest represents the request structure for creating a blog category
type CreateBlogCategoryRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

// UpdateBlogCategoryRequest represents the request structure for updating a blog category
type UpdateBlogCategoryRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

// CreateBlogCategory creates a new blog category (admin only)
// @Summary Create a new blog category
// @Description Creates a new blog category. Only admins can create categories.
// @Tags admin,blog
// @Accept json
// @Produce json
// @Param request body CreateBlogCategoryRequest true "Category information"
// @Success 201 {object} map[string]interface{} "Category created successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins and superadmins can create categories"
// @Failure 409 {object} map[string]string "Category already exists"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/categories [post]
func (h *BlogHandler) CreateBlogCategory(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins and superadmins can create categories
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can create categories"})
	}

	// Parse request
	var req CreateBlogCategoryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Category name is required"})
	}

	// Generate slug from name
	categorySlug := slug.Make(req.Name)

	// Check if category with same name or slug already exists
	var existingCategory models.BlogCategory
	err := h.db.Where("name = ? OR slug = ?", req.Name, categorySlug).First(&existingCategory).Error
	if err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Category with this name already exists"})
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("Error checking existing category: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing category"})
	}

	// Create category
	category := models.BlogCategory{
		Name:        req.Name,
		Slug:        categorySlug,
		Description: req.Description,
		PostCount:   0,
	}

	if err := h.db.Create(&category).Error; err != nil {
		log.Printf("Error creating blog category: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create category"})
	}

	LogUserAction(h.db, userID, "BLOG_CATEGORY_CREATED", category.ID, "BlogCategory", fmt.Sprintf("Blog category created: %s", req.Name), c)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Category created successfully",
		"category": fiber.Map{
			"id":          category.ID,
			"name":        category.Name,
			"slug":        category.Slug,
			"description": category.Description,
			"post_count":  category.PostCount,
			"created_at":  category.CreatedAt,
		},
	})
}

// GetBlogCategoriesAdmin gets all blog categories for admin
// @Summary Get all blog categories (admin)
// @Description Retrieves all blog categories with detailed information for admin
// @Tags admin,blog
// @Produce json
// @Success 200 {object} map[string]interface{} "List of blog categories"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins and superadmins can access this endpoint"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/categories [get]
func (h *BlogHandler) GetBlogCategoriesAdmin(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins and superadmins can access this endpoint
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can access this endpoint"})
	}

	var categories []models.BlogCategory
	if err := h.db.Order("name ASC").Find(&categories).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve categories"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"categories": categories,
	})
}

// UpdateBlogCategory updates a blog category (admin only)
// @Summary Update a blog category
// @Description Updates an existing blog category. Only admins can update categories.
// @Tags admin,blog
// @Accept json
// @Produce json
// @Param category_id path int true "Category ID"
// @Param request body UpdateBlogCategoryRequest true "Updated category information"
// @Success 200 {object} map[string]interface{} "Category updated successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins and superadmins can update categories"
// @Failure 404 {object} map[string]string "Category not found"
// @Failure 409 {object} map[string]string "Category name already exists"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/categories/{category_id} [put]
func (h *BlogHandler) UpdateBlogCategory(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins and superadmins can update categories
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can update categories"})
	}

	// Get category ID from URL params
	categoryID := c.Params("category_id")
	if categoryID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Category ID is required"})
	}

	// Parse request
	var req UpdateBlogCategoryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Category name is required"})
	}

	// Find existing category
	var category models.BlogCategory
	if err := h.db.First(&category, categoryID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Category not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve category"})
	}

	// Generate new slug from name
	newSlug := slug.Make(req.Name)

	// Check if another category with same name or slug already exists (excluding current category)
	var existingCategory models.BlogCategory
	err := h.db.Where("(name = ? OR slug = ?) AND id != ?", req.Name, newSlug, category.ID).First(&existingCategory).Error
	if err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Another category with this name already exists"})
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("Error checking existing category: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing category"})
	}

	// Update category
	category.Name = req.Name
	category.Slug = newSlug
	category.Description = req.Description

	if err := h.db.Save(&category).Error; err != nil {
		log.Printf("Error updating blog category: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update category"})
	}

	LogUserAction(h.db, userID, "BLOG_CATEGORY_UPDATED", category.ID, "BlogCategory", fmt.Sprintf("Blog category updated: %s", req.Name), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Category updated successfully",
		"category": fiber.Map{
			"id":          category.ID,
			"name":        category.Name,
			"slug":        category.Slug,
			"description": category.Description,
			"post_count":  category.PostCount,
			"updated_at":  category.UpdatedAt,
		},
	})
}

// DeleteBlogCategory deletes a blog category (admin only)
// @Summary Delete a blog category
// @Description Deletes a blog category. Only admins can delete categories.
// @Tags admin,blog
// @Produce json
// @Param category_id path int true "Category ID"
// @Success 200 {object} map[string]string "Category deleted successfully"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins and superadmins can delete categories"
// @Failure 404 {object} map[string]string "Category not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/categories/{category_id} [delete]
func (h *BlogHandler) DeleteBlogCategory(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins and superadmins can delete categories
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can delete categories"})
	}

	// Get category ID from URL params
	categoryID := c.Params("category_id")
	if categoryID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Category ID is required"})
	}

	// Find existing category
	var category models.BlogCategory
	if err := h.db.First(&category, categoryID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Category not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve category"})
	}

	// Delete category
	if err := h.db.Delete(&category).Error; err != nil {
		log.Printf("Error deleting blog category: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete category"})
	}

	LogUserAction(h.db, userID, "BLOG_CATEGORY_DELETED", category.ID, "BlogCategory", fmt.Sprintf("Blog category deleted: %s", category.Name), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Category deleted successfully",
	})
}

// --- Tag Management Functions (Admin Only) ---

// CreateBlogTagRequest represents the request structure for creating a blog tag
type CreateBlogTagRequest struct {
	Name string `json:"name" validate:"required"`
}

// UpdateBlogTagRequest represents the request structure for updating a blog tag
type UpdateBlogTagRequest struct {
	Name string `json:"name" validate:"required"`
}

// CreateBlogTag creates a new blog tag (admin only)
// @Summary Create a new blog tag
// @Description Creates a new blog tag. Only admins can create tags.
// @Tags admin,blog
// @Accept json
// @Produce json
// @Param request body CreateBlogTagRequest true "Tag information"
// @Success 201 {object} map[string]interface{} "Tag created successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins and superadmins can create tags"
// @Failure 409 {object} map[string]string "Tag already exists"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/tags [post]
func (h *BlogHandler) CreateBlogTag(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins can create tags
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can create tags"})
	}

	// Parse request
	var req CreateBlogTagRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Tag name is required"})
	}

	// Generate slug from name
	tagSlug := slug.Make(req.Name)

	// Check if tag with same name or slug already exists
	var existingTag models.BlogTag
	err := h.db.Where("name = ? OR slug = ?", req.Name, tagSlug).First(&existingTag).Error
	if err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Tag with this name already exists"})
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("Error checking existing tag: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing tag"})
	}

	// Create tag
	tag := models.BlogTag{
		Name:      req.Name,
		Slug:      tagSlug,
		PostCount: 0,
	}

	if err := h.db.Create(&tag).Error; err != nil {
		log.Printf("Error creating blog tag: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create tag"})
	}

	LogUserAction(h.db, userID, "BLOG_TAG_CREATED", tag.ID, "BlogTag", fmt.Sprintf("Blog tag created: %s", req.Name), c)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Tag created successfully",
		"tag": fiber.Map{
			"id":         tag.ID,
			"name":       tag.Name,
			"slug":       tag.Slug,
			"post_count": tag.PostCount,
			"created_at": tag.CreatedAt,
		},
	})
}

// GetBlogTagsAdmin gets all blog tags for admin
// @Summary Get all blog tags (admin)
// @Description Retrieves all blog tags with detailed information for admin
// @Tags admin,blog
// @Produce json
// @Success 200 {object} map[string]interface{} "List of blog tags"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins and superadmins can access this endpoint"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/tags [get]
func (h *BlogHandler) GetBlogTagsAdmin(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins can access this endpoint
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can access this endpoint"})
	}

	var tags []models.BlogTag
	if err := h.db.Order("name ASC").Find(&tags).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve tags"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"tags": tags,
	})
}

// UpdateBlogTag updates a blog tag (admin only)
// @Summary Update a blog tag
// @Description Updates an existing blog tag. Only admins can update tags.
// @Tags admin,blog
// @Accept json
// @Produce json
// @Param tag_id path int true "Tag ID"
// @Param request body UpdateBlogTagRequest true "Updated tag information"
// @Success 200 {object} map[string]interface{} "Tag updated successfully"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins and superadmins can update tags"
// @Failure 404 {object} map[string]string "Tag not found"
// @Failure 409 {object} map[string]string "Tag name already exists"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/tags/{tag_id} [put]
func (h *BlogHandler) UpdateBlogTag(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins can update tags
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can update tags"})
	}

	// Get tag ID from URL params
	tagID := c.Params("tag_id")
	if tagID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Tag ID is required"})
	}

	// Parse request
	var req UpdateBlogTagRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Validate request
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Tag name is required"})
	}

	// Find existing tag
	var tag models.BlogTag
	if err := h.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Tag not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve tag"})
	}

	// Generate new slug from name
	newSlug := slug.Make(req.Name)

	// Check if another tag with same name or slug already exists (excluding current tag)
	var existingTag models.BlogTag
	err := h.db.Where("(name = ? OR slug = ?) AND id != ?", req.Name, newSlug, tag.ID).First(&existingTag).Error
	if err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Another tag with this name already exists"})
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("Error checking existing tag: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing tag"})
	}

	// Update tag
	tag.Name = req.Name
	tag.Slug = newSlug

	if err := h.db.Save(&tag).Error; err != nil {
		log.Printf("Error updating blog tag: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update tag"})
	}

	LogUserAction(h.db, userID, "BLOG_TAG_UPDATED", tag.ID, "BlogTag", fmt.Sprintf("Blog tag updated: %s", req.Name), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Tag updated successfully",
		"tag": fiber.Map{
			"id":         tag.ID,
			"name":       tag.Name,
			"slug":       tag.Slug,
			"post_count": tag.PostCount,
			"updated_at": tag.UpdatedAt,
		},
	})
}

// DeleteBlogTag deletes a blog tag (admin only)
// @Summary Delete a blog tag
// @Description Deletes a blog tag. Only admins can delete tags.
// @Tags admin,blog
// @Produce json
// @Param tag_id path int true "Tag ID"
// @Success 200 {object} map[string]string "Tag deleted successfully"
// @Failure 401 {object} map[string]string "User not authenticated"
// @Failure 403 {object} map[string]string "Only admins and superadmins can delete tags"
// @Failure 404 {object} map[string]string "Tag not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /admin/blog/tags/{tag_id} [delete]
func (h *BlogHandler) DeleteBlogTag(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	// Get user role
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Only admins can delete tags
	if user.Role != models.AdminRole && user.Role != models.SuperAdminRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins and superadmins can delete tags"})
	}

	// Get tag ID from URL params
	tagID := c.Params("tag_id")
	if tagID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Tag ID is required"})
	}

	// Find existing tag
	var tag models.BlogTag
	if err := h.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Tag not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve tag"})
	}

	// Delete tag
	if err := h.db.Delete(&tag).Error; err != nil {
		log.Printf("Error deleting blog tag: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete tag"})
	}

	LogUserAction(h.db, userID, "BLOG_TAG_DELETED", tag.ID, "BlogTag", fmt.Sprintf("Blog tag deleted: %s", tag.Name), c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Tag deleted successfully",
	})
}
