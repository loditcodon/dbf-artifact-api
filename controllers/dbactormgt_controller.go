package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

// dbActorMgtSrv is the global database actor management service instance.
var dbActorMgtSrv = services.NewDBActorMgtService()

// SetDBActorMgtService initializes the database actor management service instance.
func SetDBActorMgtService(s services.DBActorMgtService) {
	dbActorMgtSrv = s
}

// PostDBActorMgtAll creates all database actors for a connection management
// @Summary Create all database actors for connection management
// @Description Creates all database actors for the specified connection management ID
// @Tags DB Actor Management
// @Accept json
// @Produce json
// @Param params body ConnectionMgtRequest true "Connection Management ID parameters"
// @Success 200 {object} BulkCreateResponse "Actors created successfully with count"
// @Failure 400 {object} StandardErrorResponse "Invalid connection management ID"
// @Failure 500 {object} ActorCreationErrorResponse "Internal server error"
// @Router /api/queries/dbactormgt/all [post]
func postDBActorMgtAll(c *gin.Context) {
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

	logger.Debugf("Synchronizing database users for cntmgt: %d", params.CntMgt)
	insertedCount, err := dbActorMgtSrv.CreateAll(c.Request.Context(), params.CntMgt)
	if err != nil {
		logger.Errorf("Failed to synchronize database users for cntmgt %d: %v", params.CntMgt, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully synchronized %d database user changes for cntmgt %d", insertedCount, params.CntMgt)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": fmt.Sprintf("Synchronized %d database user changes", insertedCount),
	})
}

// CreateDBActorMgt creates a new database actor management entry
// @Summary Create database actor management
// @Description Creates a new database actor management entry with specified parameters
// @Tags DB Actor Management
// @Accept json
// @Produce json
// @Param actor body DBActorMgtCreateRequest true "Database Actor Management object"
// @Success 201 {object} DBActorMgtCreateResponse "Actor created successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid request body, validation error, or invalid IP address"
// @Failure 500 {object} ActorCreationErrorResponse "Internal server error"
// @Router /api/queries/dbactormgt [post]
func createDBActorMgt(c *gin.Context) {
	var req dto.DBActorMgtCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&req); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	// Get connection type to determine validation rules
	cntMgtRepo := repository.NewCntMgtRepository()
	cmt, err := cntMgtRepo.GetCntMgtByID(nil, req.CntMgt)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("cntmgt id=%d not found: %v", req.CntMgt, err))
		return
	}

	// MySQL requires ip_address, Oracle does not
	if strings.ToLower(cmt.CntType) != "oracle" {
		if req.IPAddress == "" {
			utils.ErrorResponse(c, fmt.Errorf("ip_address is required for %s database type", cmt.CntType))
			return
		}
		if !utils.IsValidMySQLHost(req.IPAddress) {
			utils.ErrorResponse(c, fmt.Errorf("invalid MySQL host format: must be %%, localhost, valid IPv4/IPv6 address, or hostname"))
			return
		}
	}

	// Map DTO to model
	data := models.DBActorMgt{
		CntID:       req.CntMgt,
		DBUser:      req.DBUser,
		Password:    req.Password,
		IPAddress:   req.IPAddress,
		DBClient:    req.DBClient,
		OSUser:      req.OSUser,
		Description: req.Description,
		Status:      req.Status,
	}

	logger.Debugf("Creating new database user: %+v", data)
	newObj, err := dbActorMgtSrv.Create(c.Request.Context(), data)
	if err != nil {
		logger.Errorf("Failed to create database user: %v", err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully created database user with ID: %d", newObj.ID)
	utils.JSONResponse(c, http.StatusCreated, gin.H{
		"message": "Database Actor was created successfully",
		"id":      newObj.ID,
	})
}

// UpdateDBActorMgt updates an existing database actor management entry
// @Summary Update database actor management
// @Description Updates an existing database actor management entry by ID
// @Tags DB Actor Management
// @Accept json
// @Produce json
// @Param id path int true "Actor Management ID"
// @Param actor body DBActorMgtUpdateRequest true "Updated Database Actor Management object"
// @Success 200 {object} DBActorMgtUpdateResponse "Actor updated successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid ID or request body"
// @Failure 404 {object} StandardErrorResponse "Actor not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/dbactormgt/{id} [put]
func updateDBActorMgt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	var data dto.DBActorMgtUpdate
	if err := c.ShouldBindJSON(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	// Validate IP address if provided for update
	if data.IPAddress != "" && !utils.IsValidMySQLHost(data.IPAddress) {
		utils.ErrorResponse(c, fmt.Errorf("invalid MySQL host format: must be %%, localhost, valid IPv4/IPv6 address, or hostname"))
		return
	}

	logger.Debugf("Updating database user with ID: %d", id)
	_, err = dbActorMgtSrv.Update(c.Request.Context(), utils.MustIntToUint(id), data)
	if err != nil {
		logger.Errorf("Failed to update database user with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully updated database user with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "Database Actor was updated",
	})
}

// DeleteDBActorMgt deletes a database actor management entry
// @Summary Delete database actor management
// @Description Deletes an existing database actor management entry by ID
// @Tags DB Actor Management
// @Accept json
// @Produce json
// @Param id path int true "Actor Management ID"
// @Success 200 {object} DBActorMgtDeleteResponse "Actor deleted successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid actor management ID"
// @Failure 404 {object} StandardErrorResponse "Actor not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/dbactormgt/{id} [delete]
func deleteDBActorMgt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Deleting database user with ID: %d", id)
	if err := dbActorMgtSrv.Delete(c.Request.Context(), utils.MustIntToUint(id)); err != nil {
		logger.Errorf("Failed to delete database user with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully deleted database user with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "Database Actor was deleted",
	})
}

// RegisterDBActorMgtRoutes registers HTTP endpoints for database actor management operations.
func RegisterDBActorMgtRoutes(rg *gin.RouterGroup) {
	dbactormgt := rg.Group("/dbactormgt")
	{
		dbactormgt.POST("/all", postDBActorMgtAll)
		dbactormgt.POST("", createDBActorMgt)
		dbactormgt.PUT("/:id", updateDBActorMgt)
		dbactormgt.DELETE("/:id", deleteDBActorMgt)
	}
}
