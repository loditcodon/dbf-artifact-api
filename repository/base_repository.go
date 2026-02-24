package repository

import (
	"dbfartifactapi/config"

	"gorm.io/gorm"
)

// BaseRepository provides transaction management capabilities for database operations.
type BaseRepository interface {
	Begin() *gorm.DB
	// Commit(tx *gorm.DB) error
	// Rollback(tx *gorm.DB)
}

type baseRepository struct {
	db *gorm.DB
}

// NewBaseRepository creates a new base repository instance with database connection.
func NewBaseRepository() BaseRepository {
	return &baseRepository{
		db: config.DB,
	}
}

func (r *baseRepository) Begin() *gorm.DB {
	return r.db.Begin()
}

// func (r *baseRepository) Commit(tx *gorm.DB) error {
// 	return tx.Commit().Error
// }

// func (r *baseRepository) Rollback(tx *gorm.DB) {
// 	tx.Rollback()
// }
