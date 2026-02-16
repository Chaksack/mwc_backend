package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"mime/multipart"
	"mwc_backend/internal/models"
	"mwc_backend/internal/utils"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	Title          string   `json:"title" validate:"required"`
	Slug           string   `json:"slug" validate:"required"`
	Content        string   `json:"content" validate:"required"`
	Summary        string   `json:"summary"`
	FeaturedImage  string   `json:"featured_image"`
	Images         []string `json:"images"`
	ThumbnailImage string   `json:"thumbnail_image"`
	HeaderImage    string   `json:"header_image"`
	YouTubeURL     string   `json:"youtube_url"`
	Tags           []string `json:"tags"`
	IsPublished    bool     `json:"is_published"`
	IsFeatured     bool     `json:"is_featured"`
}

type UpdateBlogRequest struct {
	Title          *string   `json:"title"`
	Slug           *string   `json:"slug"`
	Content        *string   `json:"content"`
	Summary        *string   `json:"summary"`
	FeaturedImage  *string   `json:"featured_image"`
	Images         *[]string `json:"images"`
	ThumbnailImage *string   `json:"thumbnail_image"`
	HeaderImage    *string   `json:"header_image"`
	YouTubeURL     *string   `json:"youtube_url"`
	Tags           *[]string `json:"tags"`
	IsPublished    *bool     `json:"is_published"`
	IsFeatured     *bool     `json:"is_featured"`
}

type BlogResponse struct {
	ID             uint       `json:"id"`
	Title          string     `json:"title"`
	Slug           string     `json:"slug"`
	Content        string     `json:"content"`
	Summary        string     `json:"summary"`
	FeaturedImage  string     `json:"featured_image"`
	Images         []string   `json:"images"`
	ThumbnailImage string     `json:"thumbnail_image"`
	HeaderImage    string     `json:"header_image"`
	YouTubeURL     string     `json:"youtube_url"`
	Tags           []string   `json:"tags"`
	AuthorID       uint       `json:"author_id"`
	AuthorName     string     `json:"author_name"`
	IsPublished    bool       `json:"is_published"`
	PublishedAt    *time.Time `json:"published_at"`
	ViewCount      int        `json:"view_count"`
	IsFeatured     bool       `json:"is_featured"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type BlogContentMediaResponse struct {
	URL       string `json:"url"`
	RawURL    string `json:"raw_url"`
	HTML      string `json:"html"`
	Storage   string `json:"storage"`
	ObjectKey string `json:"object_key"`
	FileName  string `json:"file_name"`
}

type BlogContentYouTubeRequest struct {
	YouTubeURL string `json:"youtube_url"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Title      string `json:"title"`
}

type BlogContentYouTubeResponse struct {
	WatchURL string `json:"watch_url"`
	EmbedURL string `json:"embed_url"`
	HTML     string `json:"html"`
}

func parseStringSlice(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var out []string
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

func multipartValue(form *multipart.Form, key string) (string, bool) {
	if form == nil {
		return "", false
	}
	vals, ok := form.Value[key]
	if !ok || len(vals) == 0 {
		return "", false
	}
	return vals[0], true
}

func multipartFileHeaders(form *multipart.Form, key string) []*multipart.FileHeader {
	if form == nil {
		return nil
	}
	if form.File == nil {
		return nil
	}
	if v, ok := form.File[key]; ok && len(v) > 0 {
		return v
	}
	return nil
}

// UploadBlogContentImage uploads a single image for inline insertion into a WYSIWYG editor.
// @Summary Upload inline blog image
// @Description Uploads an image and returns an accessible URL plus an <img> snippet that can be inserted into blog content.
// @Tags blogs
// @Accept multipart/form-data
// @Produce json
// @Param image formData file true "Image file"
// @Param alt formData string false "Alt text"
// @Success 200 {object} BlogContentMediaResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /admin/blogs/content-images [post]
func (h *BlogHandler) UploadBlogContentImage(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	fh, err := c.FormFile("image")
	if err != nil {
		// Some editors use "file".
		fh, err = c.FormFile("file")
		if err != nil {
			// Some clients use "featured_image".
			fh, err = c.FormFile("featured_image")
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "image is required"})
			}
		}
	}

	f, err := fh.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to open uploaded file"})
	}
	defer f.Close()

	media, _, err := utils.SaveUploadedFile(
		context.Background(),
		fh.Filename,
		fh.Header.Get("Content-Type"),
		f,
		"./uploads/blog_content_images",
		"/uploads/blog_content_images",
		"blogs/content-images",
		userID,
	)
	if err != nil {
		log.Printf("Error uploading blog content image: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to upload image"})
	}

	alt := strings.TrimSpace(c.FormValue("alt"))
	resolved := utils.ResolveMediaURL(context.Background(), media.URL, media.Storage, media.ObjectKey)
	html := "<img src=\"" + resolved + "\""
	if alt != "" {
		html += " alt=\"" + alt + "\""
	}
	html += " loading=\"lazy\" />"

	return c.JSON(BlogContentMediaResponse{
		URL:       resolved,
		RawURL:    media.URL,
		HTML:      html,
		Storage:   media.Storage,
		ObjectKey: media.ObjectKey,
		FileName:  media.FileName,
	})
}

// UploadBlogContentYouTube returns a safe YouTube <iframe> snippet for inline insertion into a WYSIWYG editor.
// @Summary Generate inline YouTube embed
// @Description Normalizes a YouTube link and returns an <iframe> snippet that is compatible with blog content sanitization.
// @Tags blogs
// @Accept json
// @Produce json
// @Param payload body BlogContentYouTubeRequest true "YouTube payload"
// @Success 200 {object} BlogContentYouTubeResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /admin/blogs/content-youtube [post]
func (h *BlogHandler) UploadBlogContentYouTube(c *fiber.Ctx) error {
	var req BlogContentYouTubeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	normalized, err := normalizeYouTubeURL(req.YouTubeURL)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if normalized == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "youtube_url is required"})
	}

	u, err := url.Parse(normalized)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid youtube_url"})
	}
	videoID := strings.TrimSpace(u.Query().Get("v"))
	if videoID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid youtube_url"})
	}

	width := req.Width
	height := req.Height
	if width <= 0 {
		width = 560
	}
	if height <= 0 {
		height = 315
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "YouTube video player"
	}

	embedURL := "https://www.youtube.com/embed/" + videoID
	rawHTML := fmt.Sprintf(
		`<iframe width="%d" height="%d" src="%s" title="%s" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe>`,
		width,
		height,
		embedURL,
		html.EscapeString(title),
	)

	safeHTML := utils.SanitizeBlogContent(rawHTML)
	if strings.TrimSpace(safeHTML) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "YouTube embed not allowed"})
	}

	return c.JSON(BlogContentYouTubeResponse{
		WatchURL: normalized,
		EmbedURL: embedURL,
		HTML:     safeHTML,
	})
}

func normalizeYouTubeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	host := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	if host != "youtube.com" && host != "m.youtube.com" && host != "youtu.be" {
		return "", errors.New("youtube_url must be a youtube.com or youtu.be link")
	}
	if host == "youtu.be" {
		id := strings.Trim(strings.TrimPrefix(u.Path, "/"), " ")
		if id == "" {
			return "", errors.New("invalid youtu.be link")
		}
		return "https://www.youtube.com/watch?v=" + id, nil
	}
	// youtube.com variants: /watch?v=..., /embed/<id>, /shorts/<id>
	pathLower := strings.ToLower(u.Path)
	if pathLower == "/watch" {
		vid := u.Query().Get("v")
		if vid == "" {
			return "", errors.New("youtube.com/watch link missing v parameter")
		}
		return "https://www.youtube.com/watch?v=" + vid, nil
	}
	if strings.HasPrefix(pathLower, "/embed/") {
		id := strings.TrimPrefix(u.Path, "/embed/")
		if id == "" {
			return "", errors.New("invalid youtube embed link")
		}
		return "https://www.youtube.com/watch?v=" + id, nil
	}
	if strings.HasPrefix(pathLower, "/shorts/") {
		id := strings.TrimPrefix(u.Path, "/shorts/")
		if id == "" {
			return "", errors.New("invalid youtube shorts link")
		}
		return "https://www.youtube.com/watch?v=" + id, nil
	}
	// Accept other youtube.com links as-is
	return u.String(), nil
}

func containsString(list []string, value string) bool {
	for _, v := range list {
		if strings.TrimSpace(v) == strings.TrimSpace(value) {
			return true
		}
	}
	return false
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
	var images []string
	thumbnailImage := ""
	headerImage := ""
	youtubeURL := ""

	contentType := strings.ToLower(strings.TrimSpace(c.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid multipart form"})
		}

		if v, ok := multipartValue(form, "title"); ok {
			req.Title = v
		}
		if v, ok := multipartValue(form, "slug"); ok {
			req.Slug = v
		}
		if v, ok := multipartValue(form, "content"); ok {
			req.Content = v
		}
		if v, ok := multipartValue(form, "summary"); ok {
			req.Summary = v
		}
		if v, ok := multipartValue(form, "featured_image"); ok {
			req.FeaturedImage = v
		}

		if v, ok := multipartValue(form, "youtube_url"); ok {
			parsed, err := normalizeYouTubeURL(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			youtubeURL = parsed
		}

		if v, ok := multipartValue(form, "tags"); ok {
			tags, err := parseStringSlice(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid tags format"})
			}
			req.Tags = tags
		}

		if v, ok := multipartValue(form, "is_published"); ok {
			b, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid is_published"})
			}
			req.IsPublished = b
		}
		if v, ok := multipartValue(form, "is_featured"); ok {
			b, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid is_featured"})
			}
			req.IsFeatured = b
		}

		if v, ok := multipartValue(form, "images"); ok {
			parsed, err := parseStringSlice(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid images format"})
			}
			images = append(images, parsed...)
		}
		// Accept common HTML form field name too.
		if v, ok := multipartValue(form, "images[]"); ok {
			parsed, err := parseStringSlice(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid images format"})
			}
			images = append(images, parsed...)
		}

		if v, ok := multipartValue(form, "thumbnail_image"); ok {
			thumbnailImage = strings.TrimSpace(v)
		}
		if v, ok := multipartValue(form, "header_image"); ok {
			headerImage = strings.TrimSpace(v)
		}

		// Upload files (images)
		fileHeaders := append([]*multipart.FileHeader{}, multipartFileHeaders(form, "images")...)
		fileHeaders = append(fileHeaders, multipartFileHeaders(form, "images[]")...)
		for _, fh := range fileHeaders {
			f, err := fh.Open()
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to open uploaded file"})
			}
			media, _, err := utils.SaveUploadedFile(
				context.Background(),
				fh.Filename,
				fh.Header.Get("Content-Type"),
				f,
				"./uploads/blog_images",
				"/uploads/blog_images",
				"blogs/images",
				userID,
			)
			_ = f.Close()
			if err != nil {
				log.Printf("Error uploading blog image: %v", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to upload image"})
			}
			images = append(images, media.URL)
		}

		if v, ok := multipartValue(form, "thumbnail_index"); ok {
			idx, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || idx < 0 || idx >= len(images) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid thumbnail_index"})
			}
			thumbnailImage = images[idx]
		}
		if v, ok := multipartValue(form, "header_index"); ok {
			idx, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || idx < 0 || idx >= len(images) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid header_index"})
			}
			headerImage = images[idx]
		}
	} else {
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}
		images = append(images, req.Images...)
		thumbnailImage = strings.TrimSpace(req.ThumbnailImage)
		headerImage = strings.TrimSpace(req.HeaderImage)
		parsed, err := normalizeYouTubeURL(req.YouTubeURL)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		youtubeURL = parsed
	}

	// Backward compatibility: featured_image as the single provided image.
	if len(images) == 0 && strings.TrimSpace(req.FeaturedImage) != "" {
		images = []string{strings.TrimSpace(req.FeaturedImage)}
	}
	if thumbnailImage == "" && strings.TrimSpace(req.FeaturedImage) != "" {
		thumbnailImage = strings.TrimSpace(req.FeaturedImage)
	}
	if headerImage == "" && strings.TrimSpace(req.FeaturedImage) != "" {
		headerImage = strings.TrimSpace(req.FeaturedImage)
	}

	// Basic validation
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Slug) == "" || strings.TrimSpace(req.Content) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "title, slug, and content are required"})
	}

	// Allow WYSIWYG editor HTML but sanitize it for safety.
	req.Content = utils.SanitizeBlogContent(req.Content)
	if len(images) > 0 {
		if thumbnailImage == "" || headerImage == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail_image and header_image are required when images are provided"})
		}
		if !containsString(images, thumbnailImage) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail_image must be one of images"})
		}
		if !containsString(images, headerImage) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "header_image must be one of images"})
		}
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

	imagesJSON := ""
	if len(images) > 0 {
		b, err := json.Marshal(images)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid images format"})
		}
		imagesJSON = string(b)
	}

	// Create blog
	blog := models.Blog{
		Title:          req.Title,
		Slug:           req.Slug,
		Content:        req.Content,
		Summary:        req.Summary,
		FeaturedImage:  thumbnailImage,
		Images:         imagesJSON,
		ThumbnailImage: thumbnailImage,
		HeaderImage:    headerImage,
		YouTubeURL:     youtubeURL,
		Tags:           tagsJSON,
		AuthorID:       userID,
		IsPublished:    req.IsPublished,
		IsFeatured:     req.IsFeatured,
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

	// Find existing blog
	var blog models.Blog
	if err := h.DB.First(&blog, blogID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find blog"})
	}

	var req UpdateBlogRequest
	updates := make(map[string]interface{})

	contentType := strings.ToLower(strings.TrimSpace(c.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid multipart form"})
		}

		// Use Value map presence to decide updates.
		if v, ok := multipartValue(form, "title"); ok {
			updates["title"] = v
		}
		if v, ok := multipartValue(form, "slug"); ok {
			updates["slug"] = v
		}
		if v, ok := multipartValue(form, "content"); ok {
			updates["content"] = v
		}
		if v, ok := multipartValue(form, "summary"); ok {
			updates["summary"] = v
		}
		if v, ok := multipartValue(form, "featured_image"); ok {
			updates["featured_image"] = v
		}

		if v, ok := multipartValue(form, "youtube_url"); ok {
			parsed, err := normalizeYouTubeURL(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			updates["you_tube_url"] = parsed
		}

		if v, ok := multipartValue(form, "tags"); ok {
			tags, err := parseStringSlice(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid tags format"})
			}
			b, err := json.Marshal(tags)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid tags format"})
			}
			updates["tags"] = string(b)
		}
		if v, ok := multipartValue(form, "is_published"); ok {
			b, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid is_published"})
			}
			updates["is_published"] = b
		}
		if v, ok := multipartValue(form, "is_featured"); ok {
			b, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid is_featured"})
			}
			updates["is_featured"] = b
		}

		// Images: can replace or append
		replaceImages := false
		imagesTouched := false
		if v, ok := multipartValue(form, "replace_images"); ok {
			b, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid replace_images"})
			}
			replaceImages = b
			imagesTouched = true
		}

		var currentImages []string
		if !replaceImages {
			if blog.Images != "" {
				_ = json.Unmarshal([]byte(blog.Images), &currentImages)
			}
		}
		if v, ok := multipartValue(form, "images"); ok {
			parsed, err := parseStringSlice(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid images format"})
			}
			currentImages = parsed
			imagesTouched = true
		}
		if v, ok := multipartValue(form, "images[]"); ok {
			parsed, err := parseStringSlice(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid images format"})
			}
			currentImages = parsed
			imagesTouched = true
		}

		fileHeaders := append([]*multipart.FileHeader{}, multipartFileHeaders(form, "images")...)
		fileHeaders = append(fileHeaders, multipartFileHeaders(form, "images[]")...)
		if len(fileHeaders) > 0 {
			imagesTouched = true
		}
		for _, fh := range fileHeaders {
			f, err := fh.Open()
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to open uploaded file"})
			}
			media, _, err := utils.SaveUploadedFile(
				context.Background(),
				fh.Filename,
				fh.Header.Get("Content-Type"),
				f,
				"./uploads/blog_images",
				"/uploads/blog_images",
				"blogs/images",
				uint(blogID),
			)
			_ = f.Close()
			if err != nil {
				log.Printf("Error uploading blog image: %v", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to upload image"})
			}
			currentImages = append(currentImages, media.URL)
		}

		thumbnail := blog.ThumbnailImage
		header := blog.HeaderImage
		if v, ok := multipartValue(form, "thumbnail_image"); ok {
			thumbnail = strings.TrimSpace(v)
			imagesTouched = true
		}
		if v, ok := multipartValue(form, "header_image"); ok {
			header = strings.TrimSpace(v)
			imagesTouched = true
		}
		if v, ok := multipartValue(form, "thumbnail_index"); ok {
			idx, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || idx < 0 || idx >= len(currentImages) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid thumbnail_index"})
			}
			thumbnail = currentImages[idx]
			imagesTouched = true
		}
		if v, ok := multipartValue(form, "header_index"); ok {
			idx, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || idx < 0 || idx >= len(currentImages) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid header_index"})
			}
			header = currentImages[idx]
			imagesTouched = true
		}

		if imagesTouched && len(currentImages) == 0 {
			// Explicitly cleared images; ensure we don't store stale thumbnail/header.
			updates["images"] = "[]"
			updates["thumbnail_image"] = ""
			updates["header_image"] = ""
			updates["featured_image"] = ""
		} else if len(currentImages) > 0 {
			if strings.TrimSpace(thumbnail) == "" || strings.TrimSpace(header) == "" {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail_image and header_image are required when images are provided"})
			}
			if !containsString(currentImages, thumbnail) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail_image must be one of images"})
			}
			if !containsString(currentImages, header) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "header_image must be one of images"})
			}
			b, err := json.Marshal(currentImages)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid images format"})
			}
			updates["images"] = string(b)
			updates["thumbnail_image"] = thumbnail
			updates["header_image"] = header
			updates["featured_image"] = thumbnail
		}
	} else {
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		if req.Title != nil {
			updates["title"] = *req.Title
		}
		if req.Slug != nil {
			updates["slug"] = *req.Slug
		}
		if req.Content != nil {
			updates["content"] = utils.SanitizeBlogContent(*req.Content)
		}
		if req.Summary != nil {
			updates["summary"] = *req.Summary
		}
		if req.FeaturedImage != nil {
			updates["featured_image"] = *req.FeaturedImage
		}
		if req.YouTubeURL != nil {
			parsed, err := normalizeYouTubeURL(*req.YouTubeURL)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			updates["you_tube_url"] = parsed
		}
		if req.Tags != nil {
			b, err := json.Marshal(*req.Tags)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid tags format"})
			}
			updates["tags"] = string(b)
		}
		if req.IsPublished != nil {
			updates["is_published"] = *req.IsPublished
		}
		if req.IsFeatured != nil {
			updates["is_featured"] = *req.IsFeatured
		}
		if req.Images != nil {
			images := *req.Images
			b, err := json.Marshal(images)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid images format"})
			}
			updates["images"] = string(b)

			thumb := blog.ThumbnailImage
			head := blog.HeaderImage
			if req.ThumbnailImage != nil {
				thumb = strings.TrimSpace(*req.ThumbnailImage)
			}
			if req.HeaderImage != nil {
				head = strings.TrimSpace(*req.HeaderImage)
			}
			if len(images) == 0 {
				// Explicitly cleared images; clear selections too.
				updates["thumbnail_image"] = ""
				updates["header_image"] = ""
				updates["featured_image"] = ""
			} else {
				if strings.TrimSpace(thumb) == "" || strings.TrimSpace(head) == "" {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail_image and header_image are required when images are provided"})
				}
				if !containsString(images, thumb) {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail_image must be one of images"})
				}
				if !containsString(images, head) {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "header_image must be one of images"})
				}
				updates["thumbnail_image"] = thumb
				updates["header_image"] = head
				updates["featured_image"] = thumb
			}
		} else {
			if req.ThumbnailImage != nil {
				updates["thumbnail_image"] = strings.TrimSpace(*req.ThumbnailImage)
				updates["featured_image"] = strings.TrimSpace(*req.ThumbnailImage)
			}
			if req.HeaderImage != nil {
				updates["header_image"] = strings.TrimSpace(*req.HeaderImage)
			}
		}
	}

	// Set/unset published date only if published status is being updated.
	if v, ok := updates["is_published"]; ok {
		isPublished, _ := v.(bool)
		if isPublished && blog.PublishedAt == nil {
			now := time.Now()
			updates["published_at"] = &now
		} else if !isPublished {
			updates["published_at"] = nil
		}
	}

	if len(updates) == 0 {
		// No changes.
		if err := h.DB.Preload("Author").First(&blog, blogID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load updated blog"})
		}
		return c.JSON(h.toBlogResponse(blog))
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

// UploadBlogImages uploads one or more blog images and appends them to the blog's image list (Admin/SuperAdmin only).
// If the blog has images, it must have both thumbnail and header image selected.
// @Summary Upload blog images
// @Description Uploads one or more images for a blog post. Images are stored in S3 when configured (S3_BUCKET).
// @Tags blogs
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "Blog ID"
// @Param images formData file true "Images (repeat the field to upload multiple)"
// @Param replace_images formData bool false "If true, replace existing images"
// @Param thumbnail_index formData int false "0-based index in the final images array"
// @Param header_index formData int false "0-based index in the final images array"
// @Param thumbnail_image formData string false "Explicit thumbnail URL (must be in images)"
// @Param header_image formData string false "Explicit header URL (must be in images)"
// @Success 200 {object} BlogResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /admin/blogs/{id}/images [post]
func (h *BlogHandler) UploadBlogImages(c *fiber.Ctx) error {
	// Actor user id (for naming keys).
	userID, ok := c.Locals("user_id").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	blogID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid blog ID"})
	}

	// Find existing blog
	var blog models.Blog
	if err := h.DB.First(&blog, blogID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find blog"})
	}

	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid multipart form"})
	}

	replaceImages := false
	if v, ok := multipartValue(form, "replace_images"); ok {
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid replace_images"})
		}
		replaceImages = b
	}

	var currentImages []string
	if !replaceImages && strings.TrimSpace(blog.Images) != "" {
		_ = json.Unmarshal([]byte(blog.Images), &currentImages)
	}

	fileHeaders := append([]*multipart.FileHeader{}, multipartFileHeaders(form, "images")...)
	fileHeaders = append(fileHeaders, multipartFileHeaders(form, "images[]")...)
	if len(fileHeaders) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "images is required"})
	}

	for _, fh := range fileHeaders {
		f, err := fh.Open()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to open uploaded file"})
		}
		media, _, err := utils.SaveUploadedFile(
			context.Background(),
			fh.Filename,
			fh.Header.Get("Content-Type"),
			f,
			"./uploads/blog_images",
			"/uploads/blog_images",
			"blogs/images",
			userID,
		)
		_ = f.Close()
		if err != nil {
			log.Printf("Error uploading blog image: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to upload image"})
		}
		currentImages = append(currentImages, media.URL)
	}

	thumbnail := strings.TrimSpace(blog.ThumbnailImage)
	header := strings.TrimSpace(blog.HeaderImage)
	if replaceImages {
		thumbnail = ""
		header = ""
	}

	if v, ok := multipartValue(form, "thumbnail_index"); ok {
		idx, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || idx < 0 || idx >= len(currentImages) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid thumbnail_index"})
		}
		thumbnail = currentImages[idx]
	}
	if v, ok := multipartValue(form, "header_index"); ok {
		idx, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || idx < 0 || idx >= len(currentImages) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid header_index"})
		}
		header = currentImages[idx]
	}

	if v, ok := multipartValue(form, "thumbnail_image"); ok {
		thumbnail = strings.TrimSpace(v)
	}
	if v, ok := multipartValue(form, "header_image"); ok {
		header = strings.TrimSpace(v)
	}

	if len(currentImages) > 0 {
		if thumbnail == "" || header == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail_image and header_image are required when images are provided"})
		}
		if !containsString(currentImages, thumbnail) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail_image must be one of images"})
		}
		if !containsString(currentImages, header) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "header_image must be one of images"})
		}
	}

	imagesJSON, err := json.Marshal(currentImages)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid images format"})
	}

	updates := map[string]interface{}{
		"images":          string(imagesJSON),
		"thumbnail_image": thumbnail,
		"header_image":    header,
		"featured_image":  thumbnail,
	}

	if err := h.DB.Model(&blog).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update blog images"})
	}

	if err := h.DB.Preload("Author").First(&blog, blogID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load updated blog"})
	}

	return c.JSON(h.toBlogResponse(blog))
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
		_ = json.Unmarshal([]byte(blog.Tags), &tags)
	}
	var images []string
	if blog.Images != "" {
		_ = json.Unmarshal([]byte(blog.Images), &images)
	}

	// Resolve media URLs for clients.
	ctx := context.Background()
	for i := range images {
		images[i] = utils.ResolveMediaURL(ctx, images[i], "s3", "")
	}
	thumb := utils.ResolveMediaURL(ctx, blog.ThumbnailImage, "s3", "")
	header := utils.ResolveMediaURL(ctx, blog.HeaderImage, "s3", "")
	featured := blog.FeaturedImage
	if strings.TrimSpace(featured) == "" {
		featured = thumb
	} else {
		featured = utils.ResolveMediaURL(ctx, featured, "s3", "")
	}

	authorName := ""
	if blog.Author.FirstName != "" || blog.Author.LastName != "" {
		authorName = strings.TrimSpace(blog.Author.FirstName + " " + blog.Author.LastName)
	}
	if authorName == "" {
		authorName = blog.Author.Email
	}

	return BlogResponse{
		ID:             blog.ID,
		Title:          blog.Title,
		Slug:           blog.Slug,
		Content:        blog.Content,
		Summary:        blog.Summary,
		FeaturedImage:  featured,
		Images:         images,
		ThumbnailImage: thumb,
		HeaderImage:    header,
		YouTubeURL:     blog.YouTubeURL,
		Tags:           tags,
		AuthorID:       blog.AuthorID,
		AuthorName:     authorName,
		IsPublished:    blog.IsPublished,
		PublishedAt:    blog.PublishedAt,
		ViewCount:      blog.ViewCount,
		IsFeatured:     blog.IsFeatured,
		CreatedAt:      blog.CreatedAt,
		UpdatedAt:      blog.UpdatedAt,
	}
}
