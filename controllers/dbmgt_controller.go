package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/entity"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var dbMgtSrv = entity.NewDBMgtService()

// SetDBMgtService initializes the database management service instance.
// Used for dependency injection in tests to provide mock implementations.
func SetDBMgtService(s entity.DBMgtService) {
	dbMgtSrv = s
}

// PostDBMgtAll creates all databases for a connection management
// @Summary Create all databases for connection management
// @Description Creates all databases for the specified connection management ID
// @Tags DB Management
// @Accept json
// @Produce json
// @Param params body ConnectionMgtRequest true "Connection Management ID parameters"
// @Success 200 {object} BulkCreateResponse "Databases created successfully with count"
// @Failure 400 {object} StandardErrorResponse "Invalid connection management ID"
// @Failure 500 {object} DatabaseCreationErrorResponse "Internal server error"
// @Router /api/queries/dbmgt/all [post]
func postDBMgtAll(c *gin.Context) {
	var params struct {
		CntMgt uint `json:"cntmgt" validate:"required"`
	}
	if err := c.ShouldBindJSON(&params); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&params); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Synchronizing databases for cntmgt: %d", params.CntMgt)
	changeCount, err := dbMgtSrv.CreateAll(c.Request.Context(), params.CntMgt)
	if err != nil {
		logger.Errorf("Failed to synchronize databases for cntmgt %d: %v", params.CntMgt, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully synchronized %d database changes for cntmgt %d", changeCount, params.CntMgt)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": fmt.Sprintf("Synchronized %d database changes", changeCount),
	})
}

// CreateDBMgt creates a new database management entry
// @Summary Create database management
// @Description Creates a new database management entry with specified parameters
// @Tags DB Management
// @Accept json
// @Produce json
// @Param database body DBMgtCreateRequest true "Database Management object"
// @Success 201 {object} DBMgtCreateResponse "Database created successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} DatabaseCreationErrorResponse "Internal server error"
// @Router /api/queries/dbmgt [post]
func createDBMgt(c *gin.Context) {
	var data models.DBMgt
	if err := c.ShouldBindJSON(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Creating new database: %s", data.DbName)
	newObj, err := dbMgtSrv.Create(c.Request.Context(), data)
	if err != nil {
		logger.Errorf("Failed to create database %s: %v", data.DbName, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully created database %s with ID: %d", newObj.DbName, newObj.ID)
	utils.JSONResponse(c, http.StatusCreated, gin.H{
		"message": "Database was created successfully",
		"id":      newObj.ID,
	})
}

// PUT /api/queries/dbmgt/:id
// func updateDBMgt(c *gin.Context) {
// 	id := c.Param("id")
// 	var data models.DBMgt
// 	if err := c.ShouldBindJSON(&data); err != nil {
// 		utils.ErrorResponse(c, err)
// 		return
// 	}
// 	if err := utils.ValidateStruct(&data); err != nil {
// 		utils.ErrorResponse(c, err)
// 		return
// 	}

// 	updatedObj, err := dbMgtSrv.Update(id, data)
// 	if err != nil {
// 		utils.ErrorResponse(c, err)
// 		return
// 	}
// 	utils.JSONResponse(c, http.StatusOK, updatedObj)
// }

// DeleteDBMgt deletes a database management entry
// @Summary Delete database management
// @Description Deletes an existing database management entry by ID
// @Tags DB Management
// @Accept json
// @Produce json
// @Param id path int true "Database Management ID"
// @Success 200 {object} DBMgtDeleteResponse "Database deleted successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid database management ID"
// @Failure 404 {object} StandardErrorResponse "Database not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/dbmgt/{id} [delete]
func deleteDBMgt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Deleting database with ID: %d", id)
	if err := dbMgtSrv.Delete(c.Request.Context(), utils.MustIntToUint(id)); err != nil {
		logger.Errorf("Failed to delete database with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully deleted database with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "Database was deleted successfully",
	})
}

// RegisterDBMgtRoutes registers HTTP endpoints for database management operations.
func RegisterDBMgtRoutes(rg *gin.RouterGroup) {
	dbmgt := rg.Group("/dbmgt")
	{
		dbmgt.POST("/all", postDBMgtAll)
		dbmgt.POST("", createDBMgt)
		dbmgt.DELETE("/:id", deleteDBMgt)
	}
}
