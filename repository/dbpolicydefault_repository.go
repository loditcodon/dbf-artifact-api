package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBPolicyDefaultRepository provides data access operations for policy template records.
type DBPolicyDefaultRepository interface {
	GetAll(tx *gorm.DB) ([]models.DBPolicyDefault, error)
	GetById(tx *gorm.DB, id uint) (*models.DBPolicyDefault, error)
}

type dbPolicyDefaultRepository struct {
	db *gorm.DB
}

// NewDBPolicyDefaultRepository creates a new policy template repository instance.
func NewDBPolicyDefaultRepository() DBPolicyDefaultRepository {
	return &dbPolicyDefaultRepository{
		db: config.DB,
	}
}

func (r *dbPolicyDefaultRepository) GetAll(tx *gorm.DB) ([]models.DBPolicyDefault, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbPolicyDefaults []models.DBPolicyDefault
	if err := db.Model(models.DBPolicyDefault{}).Find(&dbPolicyDefaults).Error; err != nil {
		return nil, err
	}
	return dbPolicyDefaults, nil
}

func (r *dbPolicyDefaultRepository) GetById(tx *gorm.DB, id uint) (*models.DBPolicyDefault, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbPolicyDefault models.DBPolicyDefault
	if err := db.Model(models.DBPolicyDefault{}).Where("id = ?", id).First(&dbPolicyDefault).Error; err != nil {
		return nil, err
	}
	return &dbPolicyDefault, nil
}
