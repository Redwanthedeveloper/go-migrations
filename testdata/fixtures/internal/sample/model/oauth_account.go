package model

import (
	"time"

	"github.com/google/uuid"
)

// OAuthAccount exercises composite index discovery: Provider and
// ProviderUserID share a uniqueIndex name, forming a single multi-column index.
type OAuthAccount struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index:idx_oauth_accounts_user_id"`
	Provider       string    `gorm:"type:text;not null;uniqueIndex:idx_oauth_accounts_provider_user"`
	ProviderUserID string    `gorm:"type:text;not null;uniqueIndex:idx_oauth_accounts_provider_user"`
	CreatedAt      time.Time `gorm:"type:timestamptz;not null;default:now()"`
}

func (OAuthAccount) TableName() string { return "oauth_accounts" }
