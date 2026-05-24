package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Organization represents the billing and structural parent
type Organization struct {
	ID        string `gorm:"type:char(36);primaryKey"`
	Name      string `gorm:"type:varchar(255);not null"`
	Tier      string `gorm:"type:varchar(50);default:'sandbox'"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Relationships
	Users   []User   // An org has many users
	APIKeys []APIKey // An org has many API keys
}

// User represents a human dashboard login
type User struct {
	ID             string `gorm:"type:char(36);primaryKey"`
	OrganizationID string `gorm:"type:char(36);not null;index"`
	Email          string `gorm:"type:varchar(255);uniqueIndex;not null"`
	PasswordHash   string `gorm:"type:varchar(255);not null"` // Hashed via bcrypt
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// APIKey represents a machine-to-machine integration token
type APIKey struct {
	ID             string `gorm:"type:char(36);primaryKey"`
	OrganizationID string `gorm:"type:char(36);not null;index"`
	Environment    string `gorm:"type:varchar(20);default:'live'"`       // 'test' or 'live'
	Prefix         string `gorm:"type:varchar(20);not null"`             // e.g., "vett_live_"
	KeyHash        string `gorm:"type:varchar(64);uniqueIndex;not null"` // SHA-256 hash
	IsActive       bool   `gorm:"default:true"`
	CreatedAt      time.Time
	ExpiresAt      *time.Time
}

// --- GORM Hooks to auto-generate UUIDs ---

func (org *Organization) BeforeCreate(tx *gorm.DB) error {
	org.ID = uuid.New().String()
	return nil
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	u.ID = uuid.New().String()
	return nil
}

func (k *APIKey) BeforeCreate(tx *gorm.DB) error {
	k.ID = uuid.New().String()
	return nil
}
