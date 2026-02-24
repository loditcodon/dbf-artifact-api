package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// DBMgtRepository provides data access operations for database management records.
type DBMgtRepository interface {
	GetByID(tx *gorm.DB, id uint) (*models.DBMgt, error)
	GetByCntMgtId(tx *gorm.DB, id uint) ([]models.DBMgt, error)
	GetByIds(tx *gorm.DB, ids []uint) ([]models.DBMgt, error)
	GetAllByCntIDAndDBType(tx *gorm.DB, cntID uint, dbType string) ([]*models.DBMgt, error)
	CountByCntIdAndDBNameAndDBType(tx *gorm.DB, cntId uint, dbName, dbType string) (int64, error)
}

type dbMgtRepository struct {
	db *gorm.DB
}

// NewDBMgtRepository creates a new database management repository instance.
func NewDBMgtRepository() DBMgtRepository {
	return &dbMgtRepository{
		db: config.DB,
	}
}

func (r *dbMgtRepository) GetByID(tx *gorm.DB, id uint) (*models.DBMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbMgt models.DBMgt
	if err := db.Table(models.DBMgt{}.TableName()).Where("id = ?", id).First(&dbMgt).Error; err != nil {
		return nil, err
	}
	return &dbMgt, nil
}

func (r *dbMgtRepository) GetByCntMgtId(tx *gorm.DB, id uint) ([]models.DBMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbMgts []models.DBMgt
	if err := db.Table(models.DBMgt{}.TableName()).Where("cnt_id = ?", id).Find(&dbMgts).Error; err != nil {
		return nil, err
	}
	return dbMgts, nil
}

func (r *dbMgtRepository) GetByIds(tx *gorm.DB, ids []uint) ([]models.DBMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbMgts []models.DBMgt
	if err := db.Model(models.DBMgt{}).Where("id in ?", ids).Find(&dbMgts).Error; err != nil {
		return nil, err
	}
	return dbMgts, nil
}

func (r *dbMgtRepository) GetAllByCntIDAndDBType(tx *gorm.DB, cntID uint, dbType string) ([]*models.DBMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbMgts []*models.DBMgt
	if err := db.Table(models.DBMgt{}.TableName()).
		Where("cnt_id = ? AND dbtype = ?", cntID, dbType).
		Find(&dbMgts).Error; err != nil {
		return nil, err
	}
	return dbMgts, nil
}

func (r *dbMgtRepository) CountByCntIdAndDBNameAndDBType(tx *gorm.DB, cntId uint, dbName, dbType string) (int64, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var count int64
	if err := db.Model(&models.DBMgt{}).
		Where("cnt_id = ? AND dbname = ? AND dbtype = ?", cntId, dbName, dbType).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
