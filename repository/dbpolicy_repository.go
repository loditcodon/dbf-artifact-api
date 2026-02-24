package repository

import (
	"dbfartifactapi/config"
	"dbfartifactapi/models"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// DBPolicyRepository provides data access operations for database policy records.
type DBPolicyRepository interface {
	GetByDbMgt(tx *gorm.DB, dbmgt uint) ([]models.DBPolicy, error)
	GetByCntMgt(tx *gorm.DB, cntmgt uint) ([]models.DBPolicy, error)
	GetById(tx *gorm.DB, id uint) (*models.DBPolicy, error)
	GetPoliciesByActorAndScope(tx *gorm.DB, cntMgtID, dbMgtID, actorMgtID uint) ([]models.DBPolicy, error)
	BulkDelete(tx *gorm.DB, policyIDs []uint) error
	BulkCreate(tx *gorm.DB, policies []models.DBPolicy) error
	DeleteByActorAndPolicyDefaults(tx *gorm.DB, cntMgtID, actorID uint, policyDefaultIDs []uint) error
	BulkCreateWithDuplicateCheck(tx *gorm.DB, policies []models.DBPolicy) (int, error)
}

type dbPolicyRepository struct {
	db *gorm.DB
}

// NewDBPolicyRepository creates a new database policy repository instance.
func NewDBPolicyRepository() DBPolicyRepository {
	return &dbPolicyRepository{
		db: config.DB,
	}
}

func (r *dbPolicyRepository) GetByDbMgt(tx *gorm.DB, dbmgt uint) ([]models.DBPolicy, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbPolicies []models.DBPolicy
	if err := db.Model(models.DBPolicy{}).Where("dbmgt_id = ?", dbmgt).Find(&dbPolicies).Error; err != nil {
		return nil, err
	}
	return dbPolicies, nil
}

func (r *dbPolicyRepository) GetByCntMgt(tx *gorm.DB, cntmgt uint) ([]models.DBPolicy, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbPolicies []models.DBPolicy
	if err := db.Table(models.DBPolicy{}.TableName()+" as po").
		Joins("join dbmgt db on db.id = po.dbmgt_id").
		Joins("join cntmgt cnt on cnt.id = db.cnt_id").
		Where("db.cnt_id = ?", cntmgt).Find(&dbPolicies).Error; err != nil {
		return nil, err
	}
	return dbPolicies, nil
}

func (r *dbPolicyRepository) GetById(tx *gorm.DB, id uint) (*models.DBPolicy, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbPolicy models.DBPolicy
	if err := db.Model(models.DBPolicy{}).Where("id = ?", id).First(&dbPolicy).Error; err != nil {
		return nil, err
	}
	return &dbPolicy, nil
}

// GetPoliciesByActorAndScope retrieves all policies for a specific actor within a database scope.
// Used for calculating policy diffs during bulk updates.
func (r *dbPolicyRepository) GetPoliciesByActorAndScope(tx *gorm.DB, cntMgtID, dbMgtID, actorMgtID uint) ([]models.DBPolicy, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	var dbPolicies []models.DBPolicy
	if err := db.Model(models.DBPolicy{}).
		Where("cnt_id = ? AND dbmgt_id = ? AND actor_id = ?", cntMgtID, dbMgtID, actorMgtID).
		Find(&dbPolicies).Error; err != nil {
		return nil, err
	}
	return dbPolicies, nil
}

// BulkDelete removes multiple policies atomically.
// Critical for maintaining consistency when revoking permissions via bulk operations.
func (r *dbPolicyRepository) BulkDelete(tx *gorm.DB, policyIDs []uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	if len(policyIDs) == 0 {
		return nil
	}
	if err := db.Delete(&models.DBPolicy{}, policyIDs).Error; err != nil {
		return err
	}
	return nil
}

// BulkCreate inserts multiple policies atomically.
// Critical for maintaining consistency when granting permissions via bulk operations.
func (r *dbPolicyRepository) BulkCreate(tx *gorm.DB, policies []models.DBPolicy) error {
	db := tx
	if db == nil {
		db = r.db
	}
	if len(policies) == 0 {
		return nil
	}
	if err := db.Create(&policies).Error; err != nil {
		return err
	}
	return nil
}

// DeleteByActorAndPolicyDefaults deletes policies matching actor and policy default IDs.
// Used for cleaning up actor-wide policies when removing actors from groups.
func (r *dbPolicyRepository) DeleteByActorAndPolicyDefaults(tx *gorm.DB, cntMgtID, actorID uint, policyDefaultIDs []uint) error {
	db := tx
	if db == nil {
		db = r.db
	}
	if len(policyDefaultIDs) == 0 {
		return nil
	}
	// Delete policies matching: cnt_id, actor_id, and dbpolicydefault_id IN list
	// Không quan tâm dbmgt_id và object_id
	if err := db.Where("cnt_id = ? AND actor_id = ? AND dbpolicydefault_id IN ?",
		cntMgtID, actorID, policyDefaultIDs).Delete(&models.DBPolicy{}).Error; err != nil {
		return err
	}
	return nil
}

// BulkCreateWithDuplicateCheck inserts policies, skipping duplicates.
// Returns count of successfully inserted records.
func (r *dbPolicyRepository) BulkCreateWithDuplicateCheck(tx *gorm.DB, policies []models.DBPolicy) (int, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	if len(policies) == 0 {
		return 0, nil
	}

	insertedCount := 0
	for _, policy := range policies {
		// Check if policy already exists (silent mode to reduce log noise)
		var existing models.DBPolicy
		err := db.Session(&gorm.Session{Logger: db.Logger.LogMode(gormlogger.Silent)}).
			Where("cnt_id = ? AND actor_id = ? AND object_id = ? AND dbmgt_id = ? AND dbpolicydefault_id = ?",
				policy.CntMgt, policy.DBActorMgt, policy.DBObjectMgt, policy.DBMgt, policy.DBPolicyDefault).
			First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// Record doesn't exist, insert it
			if err := db.Create(&policy).Error; err != nil {
				return insertedCount, err
			}
			insertedCount++
		} else if err != nil {
			// Other error occurred
			return insertedCount, err
		}
		// If record exists (err == nil), skip silently
	}

	return insertedCount, nil
}
