package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var dbObjectMgtSrv = services.NewDBObjectMgtService()

// SetDBObjectMgtService initializes the database object management service instance.
func SetDBObjectMgtService(srv services.DBObjectMgtService) {
	dbObjectMgtSrv = srv
}

// GetDBObjectMgtByDbMgtId gets all database objects for a specific database management
// @Summary Get database objects by database management ID
// @Description Retrieves all database objects associated with the specified database management ID
// @Tags DB Object Management
// @Accept json
// @Produce json
// @Param id path int true "Database Management ID"
// @Success 200 {object} DBObjectMgtListResponse "List of database objects"
// @Failure 400 {object} StandardErrorResponse "Invalid database management ID"
// @Failure 500 {object} DatabaseConnectionErrorResponse "Internal server error"
// @Router /api/queries/dbobjectmgt/dbmgt/{id} [get]
func getDBObjectMgtByDbMgtId(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	logger.Debugf("Getting objects by dbmgt ID: %d", id)
	data, err := dbObjectMgtSrv.GetByDbMgtId(c.Request.Context(), utils.MustIntToUint(id))
	if err != nil {
		logger.Errorf("Failed to get objects by dbmgt ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully retrieved %d objects for dbmgt ID: %d", len(data), id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": data,
	})
}

// GetDBObjectMgtByCntMgtId gets all database objects for a specific connection management
// @Summary Get database objects by connection management ID
// @Description Retrieves all database objects associated with the specified connection management ID
// @Tags DB Object Management
// @Accept json
// @Produce json
// @Param id path int true "Connection Management ID"
// @Success 200 {object} DBObjectMgtListResponse "List of database objects"
// @Failure 400 {object} StandardErrorResponse "Invalid connection management ID"
// @Failure 500 {object} DatabaseConnectionErrorResponse "Internal server error"
// @Router /api/queries/dbobjectmgt/cntmgt/{id} [get]
func getDBObjectMgtByCntMgtId(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	logger.Debugf("Getting objects by cntmgt ID: %d", id)
	data, err := dbObjectMgtSrv.GetByCntMgtId(c.Request.Context(), utils.MustIntToUint(id))
	if err != nil {
		logger.Errorf("Failed to get objects by cntmgt ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully retrieved %d objects for cntmgt ID: %d", len(data), id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": data,
	})
}

// CreateDBObjectMgt creates a new database object management entry
// @Summary Create database object management
// @Description Creates a new database object management entry with specified parameters
// @Tags DB Object Management
// @Accept json
// @Produce json
// @Param object body DBObjectMgtCreateRequest true "Database Object Management object"
// @Success 201 {object} DBObjectMgtCreateResponse "Object created successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} ObjectCreationErrorResponse "Internal server error"
// @Router /api/queries/dbobjectmgt [post]
func createDBObjectMgt(c *gin.Context) {
	var data models.DBObjectMgt
	if err := c.ShouldBindJSON(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Creating new object: %+v", data)
	newObj, err := dbObjectMgtSrv.Create(c.Request.Context(), data)
	if err != nil {
		logger.Errorf("Failed to create object: %v", err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully created object with ID: %d", newObj.ID)
	utils.JSONResponse(c, http.StatusCreated, gin.H{
		"message": "Object was created successfully",
		"id":      newObj.ID,
	})
}

// UpdateDBObjectMgt updates an existing database object management entry
// @Summary Update database object management
// @Description Updates an existing database object management entry by ID
// @Tags DB Object Management
// @Accept json
// @Produce json
// @Param id path int true "Object Management ID"
// @Param object body DBObjectMgtCreateRequest true "Updated Database Object Management object"
// @Success 200 {object} DBObjectMgtUpdateResponse "Object updated successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid ID or request body"
// @Failure 404 {object} StandardErrorResponse "Object not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/dbobjectmgt/{id} [put]
func updateDBObjectMgt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid id"))
	}
	var data models.DBObjectMgt
	if err := c.ShouldBindJSON(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Updating object with ID: %d", id)
	_, err = dbObjectMgtSrv.Update(c.Request.Context(), utils.MustIntToUint(id), data)
	if err != nil {
		logger.Errorf("Failed to update object with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully updated object with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "Object was updated",
	})
}

// DeleteDBObjectMgt deletes a database object management entry
// @Summary Delete database object management
// @Description Deletes an existing database object management entry by ID
// @Tags DB Object Management
// @Accept json
// @Produce json
// @Param id path int true "Object Management ID"
// @Success 200 {object} DBObjectMgtDeleteResponse "Object deleted successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid object management ID"
// @Failure 404 {object} StandardErrorResponse "Object not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/dbobjectmgt/{id} [delete]
func deleteDBObjectMgt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid id"))
	}

	logger.Debugf("Deleting object with ID: %d", id)
	if err := dbObjectMgtSrv.Delete(c.Request.Context(), utils.MustIntToUint(id)); err != nil {
		logger.Errorf("Failed to delete object with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully deleted object with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "Object was deleted",
	})
}

// RegisterDBObjectMgtRoutes registers HTTP endpoints for database object management operations.
func RegisterDBObjectMgtRoutes(rg *gin.RouterGroup) {
	dbobjectmgt := rg.Group("/dbobjectmgt")
	{
		// dbobjectmgt.GET("", getDBObjectMgt)
		dbobjectmgt.GET("/dbmgt/:id", getDBObjectMgtByDbMgtId)
		dbobjectmgt.GET("/cntmgt/:id", getDBObjectMgtByCntMgtId)
		dbobjectmgt.POST("", createDBObjectMgt)
		dbobjectmgt.PUT("/:id", updateDBObjectMgt)
		dbobjectmgt.DELETE("/:id", deleteDBObjectMgt)
	}
}
