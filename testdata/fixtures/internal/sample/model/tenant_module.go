package model

import (
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type TenantModule struct {
	TenantID  uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ModuleKey string         `gorm:"type:text;primaryKey"`
	Enabled   bool           `gorm:"not null;default:false"`
	Config    datatypes.JSON `gorm:"type:jsonb;default:'{}'"`

	Tenant Tenant `gorm:"foreignKey:TenantID;references:ID;constraint:OnDelete:CASCADE"`
}

func (TenantModule) TableName() string { return "tenant_modules" }
