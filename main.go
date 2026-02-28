package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"dbfartifactapi/bootstrap"
	"dbfartifactapi/config"
	"dbfartifactapi/controllers"
	_ "dbfartifactapi/docs"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services"
	"dbfartifactapi/services/entity"
	"dbfartifactapi/services/fileops"
	"dbfartifactapi/services/group"
	"dbfartifactapi/services/job"
	"dbfartifactapi/services/pdb"
	"dbfartifactapi/services/policy"
	"dbfartifactapi/services/session"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           dbfartifactapi
// @version         1.0
// @description     DBF Artifact API

// @BasePath  /api

func main() {
	// logger.Init("/var/log/dbf/dbfartifactapi.log")
	// 1) Load config
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("LoadConfig error: %v", err)
	}

	// 2) Connect DB (GORM)
	if err := config.ConnectDB(); err != nil {
		log.Fatalf("ConnectDB error: %v", err)
	}
	if config.DB == nil {
		log.Fatal("Database is nil after ConnectDB")
	}

	if err := bootstrap.LoadData(); err != nil {
		log.Fatalf("Load data error: %v", err)
	}

	controllers.SetDBActorMgtService(entity.NewDBActorMgtService())
	controllers.SetDBMgtService(entity.NewDBMgtService())
	controllers.SetDBObjectMgtService(entity.NewDBObjectMgtService())
	controllers.SetDBPolicyService(policy.NewDBPolicyService())
	controllers.SetSessionService(session.NewSessionService())
	controllers.SetPolicyComplianceService(services.NewPolicyComplianceService())
	controllers.SetGroupManagementService(group.NewGroupManagementService())
	controllers.SetBackupService(fileops.NewBackupService())
	controllers.SetUploadService(fileops.NewUploadService())
	controllers.SetDownloadService(fileops.NewDownloadService())
	controllers.SetPDBService(pdb.NewPDBService())

	// 3) Init structured logger with config
	logLevel := logger.ParseLogLevel(config.Cfg.LogLevel)
	logger.InitWithConfig(
		config.Cfg.LogFile,
		logLevel,
		config.Cfg.LogMaxSize,
		config.Cfg.LogMaxBackups,
		config.Cfg.LogMaxAge,
		config.Cfg.LogCompress,
	)
	logger.Infof("Starting DBF Artifact API with log level: %s", config.Cfg.LogLevel)

	// 4) Setup Gin
	router := gin.Default()
	router.Use(utils.LoggerMiddleware())

	v1 := router.Group("/api")
	{
		queries := v1.Group("/queries")
		{
			controllers.RegisterDBActorMgtRoutes(queries)
			controllers.RegisterDBMgtRoutes(queries)
			controllers.RegisterDBObjectMgtRoutes(queries)
			controllers.RegisterDBPolicyRoutes(queries)
			controllers.RegisterSessionRoutes(queries)
			controllers.RegisterPolicyComplianceRoutes(queries)
			controllers.RegisterConnectionTestRoutes(queries)
			controllers.RegisterGroupManagementRoutes(queries)
			controllers.RegisterBackupRoutes(queries)
			controllers.RegisterUploadRoutes(queries)
			controllers.RegisterDownloadRoutes(queries)
			controllers.RegisterPDBRoutes(queries)
		}

		// Job status monitoring routes
		controllers.RegisterJobStatusRoutes(v1)
	}

	// 5) Swagger route
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 6) Setup graceful shutdown
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Infof("Received shutdown signal, stopping job monitor service...")

		// Stop job monitor service
		jobMonitor := job.GetJobMonitorService()
		jobMonitor.Stop()

		logger.Infof("Application shutdown complete")
		os.Exit(0)
	}()

	// 7) Run
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	logger.Infof("Starting server at port %s", port)
	router.Run("0.0.0.0:" + port)
}
