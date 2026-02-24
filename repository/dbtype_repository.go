package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBTypeRepository provides data access operations for database type records.
type DBTypeRepository interface {
	GetAll(tx *gorm.DB) ([]models.DBType, error)
}

type dbTypeRepository struct {
	db *gorm.DB
}

// NewDBTypeRepository creates a new database type repository instance.
func NewDBTypeRepository() DBTypeRepository {
	return &dbTypeRepository{
		db: config.DB,
	}
}

func (r *dbTypeRepository) GetAll(tx *gorm.DB) ([]models.DBType, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbTypes []models.DBType
	if err := db.Find(&dbTypes).Error; err != nil {
		return nil, err
	}
	return dbTypes, nil
}
