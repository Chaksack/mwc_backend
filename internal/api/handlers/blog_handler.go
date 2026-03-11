package handlers

import (
    "encoding/json"
    "log"
    "mwc_backend/internal/models"
    "strconv"
    "strings"
    "time"
    "unicode"

    "github.com/gofiber/fiber/v2"
    "gorm.io/gorm"
)

type BlogHandler struct {
	DB *gorm.DB
}

func NewBlogHandler(db *gorm.DB) *BlogHandler {
	return &BlogHandler{DB: db}
}

// Request/Response structs
type CreateBlogRequest struct {
    Title         string   `json:"title" validate:"required"`
    // Slug is optional; if not provided, it will be auto-generated from the title
    Slug          string   `json:"slug"`
    Content       string   `json:"content" validate:"required"`
    Summary       string   `json:"summary"`
    FeaturedImage string   `json:"featured_image"`
    Tags          []string `json:"tags"`
    IsPublished   bool     `json:"is_published"`
    IsFeatured    bool     `json:"is_featured"`
}

type UpdateBlogRequest struct {
	Title         string   `json:"title"`
	Slug          string   `json:"slug"`
	Content       string   `json:"content"`
	Summary       string   `json:"summary"`
	FeaturedImage string   `json:"featured_image"`
	Tags          []string `json:"tags"`
	IsPublished   bool     `json:"is_published"`
	IsFeatured    bool     `json:"is_featured"`
}

type BlogResponse struct {
	ID            uint      `json:"id"`
	Title         string    `json:"title"`
	Slug          string    `json:"slug"`
	Content       string    `json:"content"`
	Summary       string    `json:"summary"`
	FeaturedImage string    `json:"featured_image"`
	Tags          []string  `json:"tags"`
	AuthorID      uint      `json:"author_id"`
	AuthorName    string    `json:"author_name"`
	IsPublished   bool      `json:"is_published"`
	PublishedAt   *time.Time `json:"published_at"`
	ViewCount     int       `json:"view_count"`
	IsFeatured    bool      `json:"is_featured"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CreateBlog creates a new blog post (Admin/SuperAdmin only)
// @Summary Create a new blog post
// @Description Creates a new blog post (Admin/SuperAdmin only)
// @Tags blogs
// @Accept json
// @Produce json
// @Param blog body CreateBlogRequest true "Blog data"
// @Success 201 {object} BlogResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /admin/blogs [post]
func (h *BlogHandler) CreateBlog(c *fiber.Ctx) error {
	// Get user from context
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	var req CreateBlogRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Convert tags to JSON string
	tagsJSON := ""
	if len(req.Tags) > 0 {
		tagsBytes, err := json.Marshal(req.Tags)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid tags format"})
		}
		tagsJSON = string(tagsBytes)
	}

 // Create blog
 blog := models.Blog{
     Title:         req.Title,
     Slug:          req.Slug,
     Content:       req.Content,
     Summary:       req.Summary,
     FeaturedImage: req.FeaturedImage,
     Tags:          tagsJSON,
     AuthorID:      userID,
     IsPublished:   req.IsPublished,
     IsFeatured:    req.IsFeatured,
 }

 // If slug is not provided, generate it from title and ensure uniqueness
 if strings.TrimSpace(blog.Slug) == "" {
     generated, err := h.generateUniqueSlug(req.Title)
     if err != nil {
         log.Printf("Error generating slug: %v", err)
         return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate slug"})
     }
     blog.Slug = generated
 } else {
     // Ensure provided slug is unique; if not, append a suffix to avoid DB unique constraint error
     unique, err := h.ensureUniqueSlug(blog.Slug)
     if err != nil {
         log.Printf("Error ensuring slug uniqueness: %v", err)
         return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to ensure slug uniqueness"})
     }
     blog.Slug = unique
 }

 // Set published date if publishing
 if req.IsPublished {
     now := time.Now()
     blog.PublishedAt = &now
 }

	if err := h.DB.Create(&blog).Error; err != nil {
		log.Printf("Error creating blog: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create blog"})
	}

	// Load author information
	if err := h.DB.Preload("Author").First(&blog, blog.ID).Error; err != nil {
		log.Printf("Error loading blog with author: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load blog"})
	}

	response := h.toBlogResponse(blog)
	return c.Status(fiber.StatusCreated).JSON(response)
}

// GetBlogs retrieves blogs with pagination and filtering
// @Summary Get blogs
// @Description Retrieves blogs with pagination and filtering
// @Tags blogs
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(10)
// @Param published query bool false "Filter by published status"
// @Param featured query bool false "Filter by featured status"
// @Param author_id query int false "Filter by author ID"
// @Param tags query string false "Filter by tags (comma-separated)"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /blogs [get]
func (h *BlogHandler) GetBlogs(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	query := h.DB.Preload("Author")

	// Filters
	if published := c.Query("published"); published != "" {
		if published == "true" {
			query = query.Where("is_published = ?", true)
		} else if published == "false" {
			query = query.Where("is_published = ?", false)
		}
	}

	if featured := c.Query("featured"); featured != "" {
		if featured == "true" {
			query = query.Where("is_featured = ?", true)
		} else if featured == "false" {
			query = query.Where("is_featured = ?", false)
		}
	}

	if authorID := c.Query("author_id"); authorID != "" {
		query = query.Where("author_id = ?", authorID)
	}

	if tags := c.Query("tags"); tags != "" {
		tagList := strings.Split(tags, ",")
		for _, tag := range tagList {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				query = query.Where("tags LIKE ?", "%\""+tag+"\"%")
			}
		}
	}

	// Get total count
	var total int64
	countQuery := query
	if err := countQuery.Model(&models.Blog{}).Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to count blogs"})
	}

	// Get blogs
	var blogs []models.Blog
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&blogs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve blogs"})
	}

	// Convert to response
	var responses []BlogResponse
	for _, blog := range blogs {
		responses = append(responses, h.toBlogResponse(blog))
	}

	return c.JSON(fiber.Map{
		"blogs": responses,
		"pagination": fiber.Map{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetBlogBySlug retrieves a single blog by slug
// @Summary Get blog by slug
// @Description Retrieves a single blog by slug and increments view count
// @Tags blogs
// @Produce json
// @Param slug path string true "Blog slug"
// @Success 200 {object} BlogResponse
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /blogs/{slug} [get]
func (h *BlogHandler) GetBlogBySlug(c *fiber.Ctx) error {
	slug := c.Params("slug")

	var blog models.Blog
	if err := h.DB.Preload("Author").Where("slug = ?", slug).First(&blog).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve blog"})
	}

	// Increment view count
	h.DB.Model(&blog).Update("view_count", gorm.Expr("view_count + 1"))

	response := h.toBlogResponse(blog)
	return c.JSON(response)
}

// UpdateBlog updates an existing blog post (Admin/SuperAdmin only)
// @Summary Update blog post
// @Description Updates an existing blog post (Admin/SuperAdmin only)
// @Tags blogs
// @Accept json
// @Produce json
// @Param id path int true "Blog ID"
// @Param blog body UpdateBlogRequest true "Blog data"
// @Success 200 {object} BlogResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /admin/blogs/{id} [put]
func (h *BlogHandler) UpdateBlog(c *fiber.Ctx) error {
	blogID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid blog ID"})
	}

	var req UpdateBlogRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Find existing blog
	var blog models.Blog
	if err := h.DB.First(&blog, blogID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find blog"})
	}

	// Update fields
	updates := make(map[string]interface{})
	
 if req.Title != "" {
        updates["title"] = req.Title
        // If slug not explicitly provided but title changed, regenerate slug based on new title (keeping uniqueness)
        if strings.TrimSpace(req.Slug) == "" {
            if newSlug, err := h.generateUniqueSlug(req.Title); err == nil {
                updates["slug"] = newSlug
            } else {
                log.Printf("Error generating slug on update: %v", err)
            }
        }
    }
    if req.Slug != "" {
        // Ensure uniqueness for provided slug
        if unique, err := h.ensureUniqueSlug(req.Slug); err == nil {
            updates["slug"] = unique
        } else {
            log.Printf("Error ensuring slug uniqueness on update: %v", err)
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to ensure slug uniqueness"})
        }
    }
	if req.Content != "" {
		updates["content"] = req.Content
	}
	if req.Summary != "" {
		updates["summary"] = req.Summary
	}
	if req.FeaturedImage != "" {
		updates["featured_image"] = req.FeaturedImage
	}
	if len(req.Tags) > 0 {
		tagsJSON, err := json.Marshal(req.Tags)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid tags format"})
		}
		updates["tags"] = string(tagsJSON)
	}
	
	updates["is_published"] = req.IsPublished
	updates["is_featured"] = req.IsFeatured

	// Set/unset published date
	if req.IsPublished && blog.PublishedAt == nil {
		now := time.Now()
		updates["published_at"] = &now
	} else if !req.IsPublished {
		updates["published_at"] = nil
	}

	if err := h.DB.Model(&blog).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update blog"})
	}

	// Load updated blog with author
	if err := h.DB.Preload("Author").First(&blog, blogID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load updated blog"})
	}

	response := h.toBlogResponse(blog)
	return c.JSON(response)
}

// DeleteBlog deletes a blog post (Admin/SuperAdmin only)
// @Summary Delete blog post
// @Description Deletes a blog post (Admin/SuperAdmin only)
// @Tags blogs
// @Param id path int true "Blog ID"
// @Success 204
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /admin/blogs/{id} [delete]
func (h *BlogHandler) DeleteBlog(c *fiber.Ctx) error {
	blogID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid blog ID"})
	}

	// Check if blog exists
	var blog models.Blog
	if err := h.DB.First(&blog, blogID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find blog"})
	}

	// Delete blog
	if err := h.DB.Delete(&blog).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete blog"})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// Helper function to convert Blog model to response
func (h *BlogHandler) toBlogResponse(blog models.Blog) BlogResponse {
	var tags []string
	if blog.Tags != "" {
		json.Unmarshal([]byte(blog.Tags), &tags)
	}

	authorName := ""
	if blog.Author.FirstName != "" || blog.Author.LastName != "" {
		authorName = strings.TrimSpace(blog.Author.FirstName + " " + blog.Author.LastName)
	}
	if authorName == "" {
		authorName = blog.Author.Email
	}

	return BlogResponse{
		ID:            blog.ID,
		Title:         blog.Title,
		Slug:          blog.Slug,
		Content:       blog.Content,
		Summary:       blog.Summary,
		FeaturedImage: blog.FeaturedImage,
		Tags:          tags,
		AuthorID:      blog.AuthorID,
		AuthorName:    authorName,
		IsPublished:   blog.IsPublished,
		PublishedAt:   blog.PublishedAt,
		ViewCount:     blog.ViewCount,
		IsFeatured:    blog.IsFeatured,
		CreatedAt:     blog.CreatedAt,
		UpdatedAt:     blog.UpdatedAt,
	}
}

// generateUniqueSlug creates a URL-friendly slug from the given title and ensures uniqueness in the database.
func (h *BlogHandler) generateUniqueSlug(title string) (string, error) {
    base := slugify(title)
    if base == "" {
        base = "post"
    }
    // Try base, then base-2, base-3, ... until unique
    slug := base
    i := 2
    for {
        var count int64
        if err := h.DB.Model(&models.Blog{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
            return "", err
        }
        if count == 0 {
            return slug, nil
        }
        slug = base + "-" + strconv.Itoa(i)
        i++
        // safety cap
        if i > 10000 {
            return "", fiber.NewError(fiber.StatusInternalServerError, "could not generate unique slug")
        }
    }
}

// ensureUniqueSlug ensures the provided slug is unique; if taken, appends numeric suffix.
func (h *BlogHandler) ensureUniqueSlug(proposed string) (string, error) {
    proposed = slugify(proposed)
    if proposed == "" {
        proposed = "post"
    }
    if unique, err := h.generateUniqueSlug(proposed); err == nil && unique == proposed {
        return proposed, nil
    }
    // If proposed already exists, append suffixes
    base := proposed
    slug := base
    i := 2
    for {
        var count int64
        if err := h.DB.Model(&models.Blog{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
            return "", err
        }
        if count == 0 {
            return slug, nil
        }
        slug = base + "-" + strconv.Itoa(i)
        i++
        if i > 10000 {
            return "", fiber.NewError(fiber.StatusInternalServerError, "could not ensure unique slug")
        }
    }
}

// slugify converts a string into a URL-friendly slug.
func slugify(s string) string {
    s = strings.ToLower(strings.TrimSpace(s))
    // Replace spaces and underscores with hyphens
    s = strings.ReplaceAll(s, "_", "-")
    s = strings.ReplaceAll(s, " ", "-")
    // Remove invalid characters, keep alphanumerics and hyphens
    var b strings.Builder
    prevHyphen := false
    for _, r := range s {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
            b.WriteRune(r)
            prevHyphen = false
        } else if r == '-' {
            if !prevHyphen {
                b.WriteRune('-')
                prevHyphen = true
            }
        } else if unicode.IsLetter(r) || unicode.IsDigit(r) {
            // For unicode letters/digits, skip or convert to ASCII if possible; here we skip to keep minimal
        } else {
            if !prevHyphen {
                b.WriteRune('-')
                prevHyphen = true
            }
        }
    }
    res := b.String()
    res = strings.Trim(res, "-")
    // Collapse multiple hyphens
    for strings.Contains(res, "--") {
        res = strings.ReplaceAll(res, "--", "-")
    }
    return res
}