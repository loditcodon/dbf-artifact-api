package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBGroupListPoliciesRepository provides data access operations for group policy list records.
type DBGroupListPoliciesRepository interface {
	GetAll(tx *gorm.DB) ([]models.DBGroupListPolicies, error)
	GetByID(tx *gorm.DB, id uint) (*models.DBGroupListPolicies, error)
	GetByCode(tx *gorm.DB, code string) (*models.DBGroupListPolicies, error)
	GetActiveByDatabaseType(tx *gorm.DB, databaseTypeID uint) ([]models.DBGroupListPolicies, error)
}

type dbGroupListPoliciesRepository struct {
	db *gorm.DB
}

// NewDBGroupListPoliciesRepository creates a new group policy list repository instance.
func NewDBGroupListPoliciesRepository() DBGroupListPoliciesRepository {
	return &dbGroupListPoliciesRepository{
		db: config.DB,
	}
}

func (r *dbGroupListPoliciesRepository) GetAll(tx *gorm.DB) ([]models.DBGroupListPolicies, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policies []models.DBGroupListPolicies
	if err := db.Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}

func (r *dbGroupListPoliciesRepository) GetByID(tx *gorm.DB, id uint) (*models.DBGroupListPolicies, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policy models.DBGroupListPolicies
	if err := db.Where("id = ?", id).First(&policy).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func (r *dbGroupListPoliciesRepository) GetByCode(tx *gorm.DB, code string) (*models.DBGroupListPolicies, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policy models.DBGroupListPolicies
	if err := db.Where("code = ?", code).First(&policy).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func (r *dbGroupListPoliciesRepository) GetActiveByDatabaseType(tx *gorm.DB, databaseTypeID uint) ([]models.DBGroupListPolicies, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policies []models.DBGroupListPolicies
	if err := db.Where("database_type_id = ? AND is_active = ?", databaseTypeID, true).Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}
