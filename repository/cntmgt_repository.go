package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// CntMgtRepository provides data access operations for connection management records.
type CntMgtRepository interface {
	GetCntMgtByID(tx *gorm.DB, id uint) (*models.CntMgt, error)
	GetCntMgtByDbMgtId(tx *gorm.DB, dbmgtId uint) (*models.CntMgt, error)
	GetByDbmgts(tx *gorm.DB, dbmgts []uint) ([]models.CntMgt, error)
	UpdateStatus(tx *gorm.DB, id uint, status string) error
	Create(tx *gorm.DB, cntMgt *models.CntMgt) error
	GetByParentConnectionID(tx *gorm.DB, parentID uint) ([]models.CntMgt, error)
	DeleteByID(tx *gorm.DB, id uint) error
	CountByParentIDAndCntName(tx *gorm.DB, parentID uint, cntName string) (int64, error)
}

type cntMgtRepository struct {
	db *gorm.DB
}

// NewCntMgtRepository creates a new connection management repository instance.
func NewCntMgtRepository() CntMgtRepository {
	return &cntMgtRepository{
		db: config.DB,
	}
}

func (r *cntMgtRepository) GetCntMgtByID(tx *gorm.DB, id uint) (*models.CntMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}

	var cmt models.CntMgt
	if err := db.Where("id = ?", id).First(&cmt).Error; err != nil {
		return nil, err
	}
	return &cmt, nil
}

func (r *cntMgtRepository) GetCntMgtByDbMgtId(tx *gorm.DB, dbmgtId uint) (*models.CntMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var cmt models.CntMgt
	if err := db.Table(models.CntMgt{}.TableName()+" as cnt").
		Joins("join dbmgt as db on db.cnt_id = cnt.id").
		Where("db.id = ?", dbmgtId).
		Find(&cmt).Error; err != nil {
		return nil, err
	}
	return &cmt, nil
}

func (r *cntMgtRepository) GetByDbmgts(tx *gorm.DB, dbmgts []uint) ([]models.CntMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var cmts []models.CntMgt
	if err := db.Table(models.CntMgt{}.TableName()+" as cnt").
		Joins("JOIN dbmgt AS db ON db.cnt_id = cnt.id").
		Where("db.id IN ?", dbmgts).
		Group("cnt.id").
		Find(&cmts).Error; err != nil {
		return nil, err
	}

	return cmts, nil
}

func (r *cntMgtRepository) UpdateStatus(tx *gorm.DB, id uint, status string) error {
	db := tx
	if db == nil {
		db = r.db
	}

	if err := db.Model(&models.CntMgt{}).Where("id = ?", id).Update("status", status).Error; err != nil {
		return err
	}
	return nil
}

func (r *cntMgtRepository) Create(tx *gorm.DB, cntMgt *models.CntMgt) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Create(cntMgt).Error
}

func (r *cntMgtRepository) GetByParentConnectionID(tx *gorm.DB, parentID uint) ([]models.CntMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var cmts []models.CntMgt
	if err := db.Where("parent_connection_id = ?", parentID).Find(&cmts).Error; err != nil {
		return nil, err
	}
	return cmts, nil
}

func (r *cntMgtRepository) DeleteByID(tx *gorm.DB, id uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.Delete(&models.CntMgt{}, id).Error
}

func (r *cntMgtRepository) CountByParentIDAndCntName(tx *gorm.DB, parentID uint, cntName string) (int64, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var count int64
	if err := db.Model(&models.CntMgt{}).Where("parent_connection_id = ? AND cntname = ?", parentID, cntName).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
