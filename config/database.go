package config

import (
	"fmt"

	"dbfartifactapi/pkg/logger"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// DB is the global GORM database instance used throughout the application.
var DB *gorm.DB

// ConnectDB establishes database connection using GORM with configured MySQL credentials.
func ConnectDB() error {
	logger.Infof("Connecting to database %s@%s:%d/%s", Cfg.DBUser, Cfg.DBHost, Cfg.DBPort, Cfg.DBName)

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		Cfg.DBUser,
		Cfg.DBPass,
		Cfg.DBHost,
		Cfg.DBPort,
		Cfg.DBName,
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Errorf("GORM connection failed: %v", err)
		return err
	}
	logger.Infof("GORM connected successfully to database %s", Cfg.DBName)

	DB = db
	return nil
}
