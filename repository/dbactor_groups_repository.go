package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBActorGroupsRepository provides data access operations for actor-group membership records.
type DBActorGroupsRepository interface {
	Create(tx *gorm.DB, actorGroup *models.DBActorGroups) error
	GetAll(tx *gorm.DB) ([]models.DBActorGroups, error)
	GetByID(tx *gorm.DB, id uint) (*models.DBActorGroups, error)
	GetByActorID(tx *gorm.DB, actorID uint) ([]models.DBActorGroups, error)
	GetByGroupID(tx *gorm.DB, groupID uint) ([]models.DBActorGroups, error)
	GetActiveGroupsByActorID(tx *gorm.DB, actorID uint) ([]models.DBActorGroups, error)
	GetActiveActorsByGroupID(tx *gorm.DB, groupID uint) ([]models.DBActorGroups, error)
	Update(tx *gorm.DB, actorGroup *models.DBActorGroups) error
	Delete(tx *gorm.DB, id uint) error
	DeactivateByActorIDAndGroupID(tx *gorm.DB, actorID, groupID uint) error
}

type dbActorGroupsRepository struct {
	db *gorm.DB
}

// NewDBActorGroupsRepository creates a new actor-group membership repository instance.
func NewDBActorGroupsRepository() DBActorGroupsRepository {
	return &dbActorGroupsRepository{
		db: config.DB,
	}
}

func (r *dbActorGroupsRepository) Create(tx *gorm.DB, actorGroup *models.DBActorGroups) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Create(actorGroup).Error
}

func (r *dbActorGroupsRepository) GetAll(tx *gorm.DB) ([]models.DBActorGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var actorGroups []models.DBActorGroups
	if err := db.Find(&actorGroups).Error; err != nil {
		return nil, err
	}
	return actorGroups, nil
}

func (r *dbActorGroupsRepository) GetByID(tx *gorm.DB, id uint) (*models.DBActorGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var actorGroup models.DBActorGroups
	if err := db.Where("id = ?", id).First(&actorGroup).Error; err != nil {
		return nil, err
	}
	return &actorGroup, nil
}

func (r *dbActorGroupsRepository) GetByActorID(tx *gorm.DB, actorID uint) ([]models.DBActorGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var actorGroups []models.DBActorGroups
	if err := db.Where("actor_id = ?", actorID).Find(&actorGroups).Error; err != nil {
		return nil, err
	}
	return actorGroups, nil
}

func (r *dbActorGroupsRepository) GetByGroupID(tx *gorm.DB, groupID uint) ([]models.DBActorGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var actorGroups []models.DBActorGroups
	if err := db.Where("group_id = ?", groupID).Find(&actorGroups).Error; err != nil {
		return nil, err
	}
	return actorGroups, nil
}

func (r *dbActorGroupsRepository) GetActiveGroupsByActorID(tx *gorm.DB, actorID uint) ([]models.DBActorGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var actorGroups []models.DBActorGroups
	// query := db.Where("actor_id = ? AND is_active = ?", actorID, true)
	query := db.Where("actor_id = ?", actorID)

	// Filter by valid date range
	query = query.Where("valid_from <= NOW() AND (valid_until IS NULL OR valid_until >= NOW())")

	if err := query.Find(&actorGroups).Error; err != nil {
		return nil, err
	}
	return actorGroups, nil
}

func (r *dbActorGroupsRepository) GetActiveActorsByGroupID(tx *gorm.DB, groupID uint) ([]models.DBActorGroups, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var actorGroups []models.DBActorGroups
	// query := db.Where("group_id = ? AND is_active = ?", groupID, true)
	query := db.Where("group_id = ?", groupID)

	// Filter by valid date range
	query = query.Where("valid_from <= NOW() AND (valid_until IS NULL OR valid_until >= NOW())")

	if err := query.Find(&actorGroups).Error; err != nil {
		return nil, err
	}
	return actorGroups, nil
}

func (r *dbActorGroupsRepository) Update(tx *gorm.DB, actorGroup *models.DBActorGroups) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Save(actorGroup).Error
}

func (r *dbActorGroupsRepository) Delete(tx *gorm.DB, id uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Delete(&models.DBActorGroups{}, id).Error
}

func (r *dbActorGroupsRepository) DeactivateByActorIDAndGroupID(tx *gorm.DB, actorID, groupID uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Where("actor_id = ? AND group_id = ?", actorID, groupID).
		Delete(&models.DBActorGroups{}).Error
}
