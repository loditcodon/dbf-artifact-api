package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBActorMgtRepository provides data access operations for database actor management records.
type DBActorMgtRepository interface {
	Create(tx *gorm.DB, dbActorMgt *models.DBActorMgt) error
	GetByID(tx *gorm.DB, id uint) (*models.DBActorMgt, error)
	GetAllByCntID(tx *gorm.DB, cntID uint) ([]*models.DBActorMgt, error)
	CountByCntIDAndDBUserAndIP(tx *gorm.DB, cntID uint, dbUser, ip string) (int64, error)
	GetByCntMgt(tx *gorm.DB, cntId uint) ([]models.DBActorMgt, error)
	GetByIds(tx *gorm.DB, ids []uint) ([]models.DBActorMgt, error)
}

type dbActorMgtRepository struct {
	db *gorm.DB
}

// NewDBActorMgtRepository creates a new database actor management repository instance.
func NewDBActorMgtRepository() DBActorMgtRepository {
	return &dbActorMgtRepository{
		db: config.DB,
	}
}

func (r *dbActorMgtRepository) Create(tx *gorm.DB, dbActorMgt *models.DBActorMgt) error {
	db := tx
	if db == nil {
		db = r.db
	}
	if err := db.Create(dbActorMgt).Error; err != nil {
		return err
	}
	return nil
}

func (r *dbActorMgtRepository) GetByID(tx *gorm.DB, id uint) (*models.DBActorMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbActorMgt models.DBActorMgt
	if err := db.First(&dbActorMgt, id).Error; err != nil {
		return nil, err
	}
	return &dbActorMgt, nil
}

func (r *dbActorMgtRepository) CountByCntIDAndDBUserAndIP(tx *gorm.DB, cntID uint, dbUser, ip string) (int64, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var count int64
	if err := db.Model(&models.DBActorMgt{}).
		Where("cntid = ? AND dbuser = ? AND ip_address = ?", cntID, dbUser, ip).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *dbActorMgtRepository) GetByCntMgt(tx *gorm.DB, cntId uint) ([]models.DBActorMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbActorMgts []models.DBActorMgt
	if err := db.Model(models.DBActorMgt{}).Where("cntid = ?", cntId).Find(&dbActorMgts).Error; err != nil {
		return nil, err
	}
	return dbActorMgts, nil
}

func (r *dbActorMgtRepository) GetAllByCntID(tx *gorm.DB, cntID uint) ([]*models.DBActorMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbActorMgts []*models.DBActorMgt
	if err := db.Model(&models.DBActorMgt{}).
		Where("cntid = ?", cntID).
		Find(&dbActorMgts).Error; err != nil {
		return nil, err
	}
	return dbActorMgts, nil
}

func (r *dbActorMgtRepository) GetByIds(tx *gorm.DB, ids []uint) ([]models.DBActorMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbActorMgts []models.DBActorMgt
	if err := db.Model(models.DBActorMgt{}).Where("id in ?", ids).Find(&dbActorMgts).Error; err != nil {
		return nil, err
	}
	return dbActorMgts, nil
}
