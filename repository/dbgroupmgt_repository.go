package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBGroupMgtRepository provides data access operations for database group management records.
type DBGroupMgtRepository interface {
	Create(tx *gorm.DB, group *models.DBGroupMgt) error
	GetAll(tx *gorm.DB) ([]models.DBGroupMgt, error)
	GetByID(tx *gorm.DB, id uint) (*models.DBGroupMgt, error)
	GetByCode(tx *gorm.DB, code string) (*models.DBGroupMgt, error)
	GetByDatabaseType(tx *gorm.DB, databaseTypeID uint) ([]models.DBGroupMgt, error)
	GetActiveGroups(tx *gorm.DB) ([]models.DBGroupMgt, error)
	GetSystemGroups(tx *gorm.DB) ([]models.DBGroupMgt, error)
	GetCustomGroups(tx *gorm.DB) ([]models.DBGroupMgt, error)
	GetChildGroups(tx *gorm.DB, parentGroupID uint) ([]models.DBGroupMgt, error)
	Update(tx *gorm.DB, group *models.DBGroupMgt) error
	Delete(tx *gorm.DB, id uint) error
	Deactivate(tx *gorm.DB, id uint) error
}

type dbGroupMgtRepository struct {
	db *gorm.DB
}

// NewDBGroupMgtRepository creates a new database group management repository instance.
func NewDBGroupMgtRepository() DBGroupMgtRepository {
	return &dbGroupMgtRepository{
		db: config.DB,
	}
}

func (r *dbGroupMgtRepository) Create(tx *gorm.DB, group *models.DBGroupMgt) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Create(group).Error
}

func (r *dbGroupMgtRepository) GetAll(tx *gorm.DB) ([]models.DBGroupMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var groups []models.DBGroupMgt
	if err := db.Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *dbGroupMgtRepository) GetByID(tx *gorm.DB, id uint) (*models.DBGroupMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var group models.DBGroupMgt
	if err := db.Where("id = ?", id).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *dbGroupMgtRepository) GetByCode(tx *gorm.DB, code string) (*models.DBGroupMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var group models.DBGroupMgt
	if err := db.Where("code = ?", code).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *dbGroupMgtRepository) GetByDatabaseType(tx *gorm.DB, databaseTypeID uint) ([]models.DBGroupMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var groups []models.DBGroupMgt
	if err := db.Where("database_type_id = ?", databaseTypeID).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *dbGroupMgtRepository) GetActiveGroups(tx *gorm.DB) ([]models.DBGroupMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var groups []models.DBGroupMgt
	if err := db.Where("is_active = ?", true).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *dbGroupMgtRepository) GetSystemGroups(tx *gorm.DB) ([]models.DBGroupMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var groups []models.DBGroupMgt
	if err := db.Where("group_type = ? AND is_active = ?", "SYSTEM", true).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *dbGroupMgtRepository) GetCustomGroups(tx *gorm.DB) ([]models.DBGroupMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var groups []models.DBGroupMgt
	if err := db.Where("group_type = ? AND is_active = ?", "CUSTOM", true).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *dbGroupMgtRepository) GetChildGroups(tx *gorm.DB, parentGroupID uint) ([]models.DBGroupMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var groups []models.DBGroupMgt
	if err := db.Where("parent_group_id = ? AND is_active = ?", parentGroupID, true).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *dbGroupMgtRepository) Update(tx *gorm.DB, group *models.DBGroupMgt) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Save(group).Error
}

func (r *dbGroupMgtRepository) Delete(tx *gorm.DB, id uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Delete(&models.DBGroupMgt{}, id).Error
}

func (r *dbGroupMgtRepository) Deactivate(tx *gorm.DB, id uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Model(&models.DBGroupMgt{}).Where("id = ?", id).Update("is_active", false).Error
}
