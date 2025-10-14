package handlers

import (
	"fmt"
	"mwc_backend/internal/models"
	"mwc_backend/internal/utils"
	"strconv"

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
	// Load all schools
	var schools []models.School
	if err := h.db.Find(&schools).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schools"})
	}

	// Build school and training center counts per continent
	schoolCounts := make(map[string]int)
	trainingCounts := make(map[string]int)
	// Keep map of school ID -> country code for later job mapping
	schoolCountry := make(map[uint]string)
	for _, s := range schools {
		continent := utils.GetContinent(s.CountryCode)
		schoolCountry[s.ID] = s.CountryCode
		if s.Category == models.SchoolCategorySchool {
			schoolCounts[continent]++
		} else if s.Category == models.SchoolCategoryTrainingCenter {
			trainingCounts[continent]++
		}
	}

	// Load jobs and associate them via InstitutionProfile -> SchoolID -> country
	var jobs []models.Job
	if err := h.db.Preload("InstitutionProfile").Find(&jobs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch jobs"})
	}

	jobCounts := make(map[string]int)
	for _, j := range jobs {
		inst := j.InstitutionProfile
		if inst.SchoolID != nil {
			if country, ok := schoolCountry[*inst.SchoolID]; ok {
				continent := utils.GetContinent(country)
				jobCounts[continent]++
				continue
			}
		}
		// If no linked school, try to use institution profile's school data if eager loaded
		if inst.School != nil {
			continent := utils.GetContinent(inst.School.CountryCode)
			jobCounts[continent]++
			continue
		}
		// Fallback: attribute to Unknown
		jobCounts["Unknown"]++
	}

	// Prepare display order and central coordinates
	type LatLng struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	}

	displayOrder := []struct {
		ID     string
		Key    string
		Label  string
		LatLng LatLng
	}{
		{"1", "North America", "North America", LatLng{54.5260, -105.2551}},
		{"2", "South America", "South America", LatLng{-14.2350, -51.9253}},
		{"3", "Europe", "Europe", LatLng{54.5260, 15.2551}},
		{"4", "Africa", "Africa", LatLng{1.6508, 17.6791}},
		{"5", "Asia", "Asia", LatLng{34.0479, 100.6197}},
		{"6", "Oceania", "Australia", LatLng{-25.2744, 133.7751}},
		{"7", "Antarctica", "Antarctica", LatLng{-82.8628, 135.0000}},
	}

	// Build output list following sample formatting
	type ContinentSummary struct {
		ID             string `json:"id"`
		Continent      string `json:"continent"`
		Schools        string `json:"schools"`
		TrainingCenter string `json:"trainingCenter"`
		Jobs           string `json:"jobs"`
		LatLng         LatLng `json:"latlng"`
	}

	var resp []ContinentSummary
	for _, item := range displayOrder {
		key := item.Key
		// Normalize Oceania -> if our utils returns "Oceania", map to "Australia" label shown
		schoolsCount := schoolCounts[key]
		trainingCount := trainingCounts[key]
		jobsCount := jobCounts[key]

		// If key is "Oceania" but label is "Australia",
		// also accept counts under "Oceania" or "Australia" variants
		if key == "Oceania" {
			if v, ok := schoolCounts["Australia"]; ok {
				schoolsCount += v
			}
			if v, ok := trainingCounts["Australia"]; ok {
				trainingCount += v
			}
			if v, ok := jobCounts["Australia"]; ok {
				jobsCount += v
			}
		}

		// format numbers with commas
		schoolsStr := fmt.Sprintf("%s Schools", formatNumber(schoolsCount))
		trainingStr := fmt.Sprintf("%s Training Centers", formatNumber(trainingCount))
		jobsStr := fmt.Sprintf("%s Job Openings", formatNumber(jobsCount))
		if item.Key == "Antarctica" {
			// override for Antarctica sample
			if jobsCount == 0 {
				jobsStr = "Few Research Openings"
			}
		}

		resp = append(resp, ContinentSummary{
			ID:             item.ID,
			Continent:      item.Label,
			Schools:        schoolsStr,
			TrainingCenter: trainingStr,
			Jobs:           jobsStr,
			LatLng:         item.LatLng,
		})
	}

	return c.JSON(fiber.Map{"continents": resp})
}

// formatNumber returns integer formatted with commas (e.g., 1200 -> "1,200")
func formatNumber(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	// insert commas
	out := ""
	rem := len(s) % 3
	if rem > 0 {
		out = s[:rem]
	}
	for i := rem; i < len(s); i += 3 {
		if len(out) > 0 {
			out += ","
		}
		out += s[i : i+3]
	}
	return out
}
