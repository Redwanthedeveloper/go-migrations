package model

import (
	"time"

	"github.com/google/uuid"
)

type Tenant struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string     `gorm:"type:text;not null"`
	Slug      string     `gorm:"type:text;not null;uniqueIndex:idx_tenants_slug"`
	PlanID    *uuid.UUID `gorm:"type:uuid"`
	Status    string     `gorm:"type:text;not null;default:'active'"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:now()"`

	Plan *Plan `gorm:"foreignKey:PlanID;references:ID"`
}

func (Tenant) TableName() string { return "tenants" }
