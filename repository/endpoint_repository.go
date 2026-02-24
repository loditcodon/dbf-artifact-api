package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// EndpointRepository provides data access operations for endpoint records.
type EndpointRepository interface {
	GetByID(tx *gorm.DB, id uint) (*models.Endpoint, error)
	GetByIds(tx *gorm.DB, ids []uint) ([]models.Endpoint, error)
}

type endpointRepository struct {
	db *gorm.DB
}

// NewEndpointRepository creates a new endpoint repository instance.
func NewEndpointRepository() EndpointRepository {
	return &endpointRepository{
		db: config.DB,
	}
}

func (r *endpointRepository) GetByID(tx *gorm.DB, id uint) (*models.Endpoint, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var endpoint models.Endpoint
	if err := db.Where("id = ?", id).First(&endpoint).Error; err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func (r *endpointRepository) GetByIds(tx *gorm.DB, ids []uint) ([]models.Endpoint, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var endpoints []models.Endpoint
	if err := db.Where("id in ?", ids).Find(&endpoints).Error; err != nil {
		return nil, err
	}
	return endpoints, nil
}
