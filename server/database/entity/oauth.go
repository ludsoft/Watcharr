package entity

import "time"

// OauthAuthCode is a one-time OAuth2 authorization code issued by the
// built-in provider (feature/oauthp).
type OauthAuthCode struct {
	ID          uint `gorm:"primarykey"`
	CreatedAt   time.Time
	Code        string    `gorm:"uniqueIndex;not null"`
	ClientID    string    `gorm:"not null"`
	RedirectURI string    `gorm:"not null"`
	Scope       string    `gorm:"not null;default:''"`
	UserID      uint      `gorm:"not null"`
	ExpiresAt   time.Time `gorm:"not null"`
	Used        bool      `gorm:"not null;default:false"`
}
