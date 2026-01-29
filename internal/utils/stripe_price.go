package utils

import (
	"crypto/rand"
	"math/big"
	"strings"

	"mwc_backend/internal/models"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/price"
)

// GenerateRoleLookupKey creates a stable-looking lookup key for a role with a random suffix.
// Example outputs: institution_xse3421, prof_xser4312, parent_xsde3421, training_xse3421
// Note: This does not create a Stripe Price. It only generates a string you may
// attach as Price.LookupKey when creating a price in Stripe.
func GenerateRoleLookupKey(role models.UserRole) string {
	prefix := roleLookupPrefix(role)
	return prefix + "_" + randomAlphaNum(8)
}

// roleLookupPrefix maps system roles to short prefixes used in lookup keys.
func roleLookupPrefix(role models.UserRole) string {
	switch role {
	case models.InstitutionRole:
		return "institution"
	case models.SchoolRole:
		// Treat school the same as institution for lookup keys
		return "institution"
	case models.MontessoriProfessionalRole:
		return "prof"
	case models.ParentRole:
		return "parent"
	case models.TrainingCenterRole:
		return "training"
	case models.AdminRole:
		return "admin"
	case models.SuperAdminRole:
		return "superadmin"
	default:
		return strings.ToLower(string(role))
	}
}

// randomAlphaNum returns a random lowercase alphanumeric string of given length.
func randomAlphaNum(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	var sb strings.Builder
	for i := 0; i < n; i++ {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		sb.WriteByte(letters[idx.Int64()])
	}
	return sb.String()
}

// ResolveStripePriceIDFromLookupKey fetches the Stripe Price ID using a given lookup key.
// It searches active prices with an exact match on the lookup key and returns the first match.
func ResolveStripePriceIDFromLookupKey(lookupKey string) (string, error) {
	params := &stripe.PriceListParams{Active: stripe.Bool(true)}
	params.Filters.AddFilter("lookup_keys[]", "", lookupKey)
	it := price.List(params)
	for it.Next() {
		pr := it.Price()
		if pr != nil {
			return pr.ID, nil
		}
	}
	if err := it.Err(); err != nil {
		return "", err
	}
	return "", nil
}
