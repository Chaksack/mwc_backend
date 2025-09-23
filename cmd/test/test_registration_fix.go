package main

import (
	"fmt"
	"mwc_backend/internal/models"
)

func main() {
	// Test that the longest role name fits within the new constraint
	roles := []models.UserRole{
		models.AdminRole,                  // "admin" (5 chars)
		models.InstitutionRole,            // "institution" (11 chars)
		models.MontessoriProfessionalRole, // "montessori_professional" (23 chars)
		models.TrainingCenterRole,         // "training_center" (15 chars)
		models.ParentRole,                 // "parent" (6 chars)
	}

	fmt.Println("Testing role lengths against new VARCHAR(30) constraint:")
	for _, role := range roles {
		roleStr := string(role)
		fmt.Printf("Role: %-25s Length: %d chars (fits in VARCHAR(30): %t)\n", 
			roleStr, len(roleStr), len(roleStr) <= 30)
	}
	
	fmt.Println("\nThe fix should resolve the VARCHAR constraint violation error.")
	fmt.Println("All role names now fit within the VARCHAR(30) limit.")
}