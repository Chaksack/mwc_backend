package handlers

import (
	"mwc_backend/internal/models"
	"mwc_backend/internal/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// ContinentHandler handles continent-related statistics
type ContinentHandler struct {
	db  *gorm.DB
	cfg interface{} // kept minimal; not used yet
}

// NewContinentHandler creates a new ContinentHandler
func NewContinentHandler(db *gorm.DB) *ContinentHandler {
	return &ContinentHandler{db: db}
}

// GetSchoolCountsByContinent returns the count of schools grouped by continent
func (h *ContinentHandler) GetSchoolCountsByContinent(c *fiber.Ctx) error {
	var schools []models.School
	if err := h.db.Find(&schools).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schools"})
	}

	continentCounts := make(map[string]int)
	for _, school := range schools {
		continent := utils.GetContinent(school.CountryCode)
		continentCounts[continent]++
	}

	return c.JSON(fiber.Map{"counts": continentCounts})
}

// ListContinents returns an array of continents with their school counts
func (h *ContinentHandler) ListContinents(c *fiber.Ctx) error {
	var schools []models.School
	if err := h.db.Find(&schools).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schools"})
	}

	counts := make(map[string]int)
	for _, school := range schools {
		continent := utils.GetContinent(school.CountryCode)
		counts[continent]++
	}

	type ContinentCount struct {
		Continent string `json:"continent"`
		Count     int    `json:"count"`
	}

	var list []ContinentCount
	for k, v := range counts {
		list = append(list, ContinentCount{Continent: k, Count: v})
	}

	return c.JSON(fiber.Map{"continents": list})
}
