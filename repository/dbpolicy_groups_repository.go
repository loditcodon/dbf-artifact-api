package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBPolicyGroupsRepository provides data access operations for policy-group mapping records.
type DBPolicyGroupsRepository interface {
	Create(tx *gorm.DB, policyGroup *models.DBPolicyGroups) error
	GetAll(tx *gorm.DB) ([]models.DBPolicyGroups, error)
	GetByID(tx *gorm.DB, id uint) (*models.DBPolicyGroups, error)
	GetByGroupID(tx *gorm.DB, groupID uint) ([]models.DBPolicyGroups, error)
	GetByPolicyID(tx *gorm.DB, policyID uint) ([]models.DBPolicyGroups, error)
	GetActivePoliciesByGroupID(tx *gorm.DB, groupID uint) ([]models.DBPolicyGroups, error)
	Update(tx *gorm.DB, policyGroup *models.DBPolicyGroups) error
	Delete(tx *gorm.DB, id uint) error
	DeactivateByGroupIDAndPolicyID(tx *gorm.DB, groupID, policyID uint) error
}

type dbPolicyGroupsRepository struct {
	db *gorm.DB
}

// NewDBPolicyGroupsRepository creates a new policy-group mapping repository instance.
func NewDBPolicyGroupsRepository() DBPolicyGroupsRepository {
	return &dbPolicyGroupsRepository{
		db: config.DB,
	}
}

func (r *dbPolicyGroupsRepository) Create(tx *gorm.DB, policyGroup *models.DBPolicyGroups) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Create(policyGroup).Error
}

func (r *dbPolicyGroupsRepository) GetAll(tx *gorm.DB) ([]models.DBPolicyGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policyGroups []models.DBPolicyGroups
	if err := db.Find(&policyGroups).Error; err != nil {
		return nil, err
	}
	return policyGroups, nil
}

func (r *dbPolicyGroupsRepository) GetByID(tx *gorm.DB, id uint) (*models.DBPolicyGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policyGroup models.DBPolicyGroups
	if err := db.Where("id = ?", id).First(&policyGroup).Error; err != nil {
		return nil, err
	}
	return &policyGroup, nil
}

func (r *dbPolicyGroupsRepository) GetByGroupID(tx *gorm.DB, groupID uint) ([]models.DBPolicyGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policyGroups []models.DBPolicyGroups
	if err := db.Where("group_id = ?", groupID).Find(&policyGroups).Error; err != nil {
		return nil, err
	}
	return policyGroups, nil
}

func (r *dbPolicyGroupsRepository) GetByPolicyID(tx *gorm.DB, policyID uint) ([]models.DBPolicyGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policyGroups []models.DBPolicyGroups
	if err := db.Where("dbgroup_listpolicies_id = ?", policyID).Find(&policyGroups).Error; err != nil {
		return nil, err
	}
	return policyGroups, nil
}

func (r *dbPolicyGroupsRepository) GetActivePoliciesByGroupID(tx *gorm.DB, groupID uint) ([]models.DBPolicyGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var policyGroups []models.DBPolicyGroups
	// query := db.Where("group_id = ? AND is_active = ?", groupID, true)
	query := db.Where("group_id = ?", groupID)

	// Filter by valid date range
	query = query.Where("valid_from <= NOW() AND (valid_until IS NULL OR valid_until >= NOW())")

	if err := query.Find(&policyGroups).Error; err != nil {
		return nil, err
	}
	return policyGroups, nil
}

func (r *dbPolicyGroupsRepository) Update(tx *gorm.DB, policyGroup *models.DBPolicyGroups) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Save(policyGroup).Error
}

func (r *dbPolicyGroupsRepository) Delete(tx *gorm.DB, id uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Delete(&models.DBPolicyGroups{}, id).Error
}

func (r *dbPolicyGroupsRepository) DeactivateByGroupIDAndPolicyID(tx *gorm.DB, groupID, policyID uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Where("group_id = ? AND dbgroup_listpolicies_id = ?", groupID, policyID).
		Delete(&models.DBPolicyGroups{}).Error
}
