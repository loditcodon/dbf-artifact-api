package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBActorRepository provides data access operations for database actor records.
type DBActorRepository interface {
	GetAll(tx *gorm.DB) ([]models.DBActor, error)
	GetByDBType(tx *gorm.DB, dbType string) (*models.DBActor, error)
}

type dbActorRepository struct {
	db *gorm.DB
}

// NewDBActorRepository creates a new database actor repository instance.
func NewDBActorRepository() DBActorRepository {
	return &dbActorRepository{
		db: config.DB,
	}
}

func (r *dbActorRepository) GetAll(tx *gorm.DB) ([]models.DBActor, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbActors []models.DBActor
	if err := db.Find(&dbActors).Error; err != nil {
		return nil, err
	}
	return dbActors, nil
}

func (r *dbActorRepository) GetByDBType(tx *gorm.DB, dbType string) (*models.DBActor, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbActor models.DBActor
	if err := db.Where("dbtype = ?", dbType).First(&dbActor).Error; err != nil {
		return nil, err
	}
	return &dbActor, nil
}
