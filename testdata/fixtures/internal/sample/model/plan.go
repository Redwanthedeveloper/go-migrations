package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Plan struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string         `gorm:"type:text;not null"`
	Limits    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time      `gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"type:timestamptz;autoUpdateTime"`
}

func (Plan) TableName() string { return "plans" }
