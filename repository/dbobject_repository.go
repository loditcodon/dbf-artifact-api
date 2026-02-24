package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBObjectRepository provides data access operations for database object type records.
type DBObjectRepository interface {
	GetAll() ([]models.DBObject, error)
}

type dbObjectRepository struct {
	db *gorm.DB
}

// NewDBObjectRepository creates a new database object type repository instance.
func NewDBObjectRepository() DBObjectRepository {
	return &dbObjectRepository{
		db: config.DB,
	}
}

func (r *dbObjectRepository) GetAll() ([]models.DBObject, error) {
	var dbObjects []models.DBObject
	if err := r.db.Model(models.DBObject{}).Find(&dbObjects).Error; err != nil {
		return nil, err
	}
	return dbObjects, nil
}
