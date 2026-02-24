package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/utils"
	"fmt"

	"gorm.io/gorm"
)

// DBObjectMgtRepository provides data access operations for database object management records.
type DBObjectMgtRepository interface {
	GetById(tx *gorm.DB, id uint) (*models.DBObjectMgt, error)
	GetByDbMgtId(db *gorm.DB, id uint) ([]models.DBObjectMgt, error)
	GetByCntMgtId(tx *gorm.DB, id uint) ([]models.DBObjectMgt, error)
	CountByDbIdAndObjTypeAndObjName(tx *gorm.DB, dbId, objType uint, objName string) (int64, error)
	GetByObjectId(tx *gorm.DB, objectId uint) ([]models.DBObjectMgt, error)
	GetByObjectIdAndDbMgt(tx *gorm.DB, objectId, dbmgt uint) ([]models.DBObjectMgt, error)
	GetByObjectIdAndDbMgtInt(tx *gorm.DB, objectId int, dbmgt uint) ([]models.DBObjectMgt, error)
	GetByDbMgtAndObjectId(tx *gorm.DB, dbmgt, objectId uint) ([]models.DBObjectMgt, error)
	GetByIds(tx *gorm.DB, ids []uint) ([]models.DBObjectMgt, error)
}

type dbObjectMgtRepository struct {
	db *gorm.DB
}

// NewDBObjectMgtRepository creates a new database object management repository instance.
func NewDBObjectMgtRepository() DBObjectMgtRepository {
	return &dbObjectMgtRepository{
		db: config.DB,
	}
}

func (r *dbObjectMgtRepository) GetById(tx *gorm.DB, id uint) (*models.DBObjectMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbObjectMgt models.DBObjectMgt
	if err := db.Table(models.DBObjectMgt{}.TableName()).Where("id = ?", id).First(&dbObjectMgt).Error; err != nil {
		return nil, err
	}
	return &dbObjectMgt, nil
}

func (r *dbObjectMgtRepository) GetByDbMgtId(tx *gorm.DB, id uint) ([]models.DBObjectMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbObjectMgt []models.DBObjectMgt
	if err := db.Table(models.DBObjectMgt{}.TableName()).Where("dbid = ?", id).Find(&dbObjectMgt).Error; err != nil {
		return nil, err
	}
	return dbObjectMgt, nil
}

func (r *dbObjectMgtRepository) GetByCntMgtId(tx *gorm.DB, id uint) ([]models.DBObjectMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbObjectMgt []models.DBObjectMgt
	if err := db.Table(models.DBObjectMgt{}.TableName()+" as obj").
		Joins("join dbmgt as db on obj.dbid = db.id").
		Joins("join cntmgt as cnt on db.cnt_id = cnt.id").
		Where("cnt.id = ?", id).
		Find(&dbObjectMgt).Error; err != nil {
		return nil, err
	}
	return dbObjectMgt, nil
}

func (r *dbObjectMgtRepository) CountByDbIdAndObjTypeAndObjName(tx *gorm.DB, dbId, objType uint, objName string) (int64, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var count int64
	query := db.Model(models.DBObjectMgt{})
	if dbId != 0 {
		query = query.Where("dbid = ?", dbId)
	}
	if err := query.
		Where("objectname = ? and dbobject_id = ?", objName, objType).
		Count(&count).Error; err != nil {
		return 0, nil
	}
	return count, nil
}

func (r *dbObjectMgtRepository) GetByObjectId(tx *gorm.DB, objectId uint) ([]models.DBObjectMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbObjectMgts []models.DBObjectMgt
	if err := db.Table(models.DBObjectMgt{}.TableName()).Where("dbobject_id = ?", objectId).
		Order("id ASC").Find(&dbObjectMgts).Error; err != nil {
		return nil, err
	}
	return dbObjectMgts, nil
}

func (r *dbObjectMgtRepository) GetByObjectIdAndDbMgt(tx *gorm.DB, objectId, dbmgt uint) ([]models.DBObjectMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbObjectMgts []models.DBObjectMgt
	if err := db.Table(models.DBObjectMgt{}.TableName()).Where("dbobject_id = ? and dbid = ?", objectId, dbmgt).
		Order("id ASC").Find(&dbObjectMgts).Error; err != nil {
		return nil, err
	}
	return dbObjectMgts, nil
}

// GetByObjectIdAndDbMgtInt handles int objectId parameter for -1 wildcard support
// TODO: Implement wildcard logic for objectId = -1
func (r *dbObjectMgtRepository) GetByObjectIdAndDbMgtInt(tx *gorm.DB, objectId int, dbmgt uint) ([]models.DBObjectMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}

	if objectId == -1 {
		// TODO: Implement wildcard logic - return all objects for this dbmgt
		// For now, delegate to GetByDbMgtId
		return r.handleWildcardObjectQuery(db, dbmgt)
	}

	// Normal case: convert to uint and use existing method
	return r.GetByObjectIdAndDbMgt(db, utils.MustIntToUint(objectId), dbmgt)
}

// handleWildcardObjectQuery processes objectId = -1 case
// TODO: Add your custom logic here for handling wildcard object queries
func (r *dbObjectMgtRepository) handleWildcardObjectQuery(tx *gorm.DB, dbmgt uint) ([]models.DBObjectMgt, error) {
	// PLACEHOLDER: Add your implementation here
	// This should return all relevant objects for the database management instance
	// Example implementation:
	// return r.GetByDbMgtId(tx, dbmgt)

	var dbObjectMgts []models.DBObjectMgt
	logger.Warnf("Wildcard object query (objectId = -1) not fully implemented for dbmgt=%d", dbmgt)
	return dbObjectMgts, fmt.Errorf("wildcard object query (objectId = -1) requires custom implementation")
}

// GetByDbMgtAndObjectId gets objects by database management ID and object type for synchronization
func (r *dbObjectMgtRepository) GetByDbMgtAndObjectId(tx *gorm.DB, dbmgt, objectId uint) ([]models.DBObjectMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbObjectMgts []models.DBObjectMgt
	if err := db.Table(models.DBObjectMgt{}.TableName()).Where("dbid = ? and dbobject_id = ?", dbmgt, objectId).
		Find(&dbObjectMgts).Error; err != nil {
		return nil, err
	}
	return dbObjectMgts, nil
}

func (r *dbObjectMgtRepository) GetByIds(tx *gorm.DB, ids []uint) ([]models.DBObjectMgt, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbObjectMgts []models.DBObjectMgt
	if err := db.Table(models.DBObjectMgt{}.TableName()).Where("id in ?", ids).Find(&dbObjectMgts).Error; err != nil {
		return nil, err
	}
	return dbObjectMgts, nil
}
