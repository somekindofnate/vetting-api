package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/somekindofnate/vetting-api/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// CreateNewAccount sets up a new customer and returns their first plaintext API key
func CreateNewAccount(db *gorm.DB, email, rawPassword, orgName string) (string, error) {
	// 1. Hash the human password (SLOW - protects against database leaks)
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(rawPassword), 12)
	if err != nil {
		return "", err
	}

	// 2. Generate the machine API Key (FAST hash - protects against DB leaks without latency)
	plaintextKey, keyHash, prefix, err := generateAPIKey("live")
	if err != nil {
		return "", err
	}

	// 3. Database Transaction: All or nothing
	err = db.Transaction(func(tx *gorm.DB) error {
		org := models.Organization{Name: orgName}
		if err := tx.Create(&org).Error; err != nil {
			return err
		}

		user := models.User{
			OrganizationID: org.ID,
			Email:          email,
			PasswordHash:   string(hashBytes),
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		apiKey := models.APIKey{
			OrganizationID: org.ID,
			Environment:    "live",
			Prefix:         prefix,
			KeyHash:        keyHash,
		}
		if err := tx.Create(&apiKey).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	// Return the plaintext key so the dashboard can show it to the user exactly ONCE.
	return plaintextKey, nil
}

// Internal helper
func generateAPIKey(environment string) (plaintextKey, hash, prefix string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", err
	}

	secret := hex.EncodeToString(bytes)

	prefix = "vett_test_"
	if environment == "live" {
		prefix = "vett_live_"
	}

	plaintextKey = fmt.Sprintf("%s%s", prefix, secret)
	hashBytes := sha256.Sum256([]byte(plaintextKey))
	hash = hex.EncodeToString(hashBytes[:])

	return plaintextKey, hash, prefix, nil
}
