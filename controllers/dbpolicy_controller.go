package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/services/policy"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var dbPolicySrv = policy.NewDBPolicyService()

// SetDBPolicyService initializes the database policy service instance.
func SetDBPolicyService(srv policy.DBPolicyService) {
	dbPolicySrv = srv
}

// GetDBPolicyByCntMgt generates database policies for all databases under a connection management
// @Summary Generate policies for connection management
// @Description Starts background job to generate database policies for all databases under specified connection management ID
// @Tags DB Policy
// @Accept json
// @Produce json
// @Param cntmgt path int true "Connection Management ID"
// @Success 200 {object} JobStartResponse "Background job started message with job ID"
// @Failure 400 {object} StandardErrorResponse "Invalid connection management ID"
// @Failure 500 {object} JobProcessingErrorResponse "Internal server error"
// @Router /api/queries/dbpolicy/cntmgt/{cntmgt} [get]
func getDBPolicyByCntMgt(c *gin.Context) {
	cntmgt, err := strconv.Atoi(c.Param("cntmgt"))
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid cntmgt"))
		return
	}
	logger.Debugf("Getting policies by cntmgt: %d", cntmgt)
	jobMessage, err := dbPolicySrv.GetByCntMgt(c.Request.Context(), uint(cntmgt))
	if err != nil {
		logger.Errorf("Failed to get policies by cntmgt %d: %v", cntmgt, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully started policy job for cntmgt %d: %s", cntmgt, jobMessage)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": jobMessage,
	})
}

// CreateDBPolicy creates a new database policy
// @Summary Create database policy
// @Description Creates a new database policy with specified parameters
// @Tags DB Policy
// @Accept json
// @Produce json
// @Param policy body DBPolicyCreateRequest true "Database Policy object"
// @Success 201 {object} DBPolicyCreateResponse "Policy created successfully with ID"
// @Failure 400 {object} StandardErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} PolicyCreationErrorResponse "Internal server error"
// @Router /api/queries/dbpolicy [post]
func createDBPolicy(c *gin.Context) {
	var data models.DBPolicy
	if err := c.ShouldBindJSON(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Creating new policy: %+v", data)
	newObj, err := dbPolicySrv.Create(c.Request.Context(), data)
	if err != nil {
		logger.Errorf("Failed to create policy: %v", err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully created policy with ID: %d", newObj.ID)
	utils.JSONResponse(c, http.StatusCreated, gin.H{
		"message": "Policy created",
		"id":      newObj.ID,
	})
}

// UpdateDBPolicy updates an existing database policy
// @Summary Update database policy
// @Description Updates an existing database policy by ID
// @Tags DB Policy
// @Accept json
// @Produce json
// @Param id path int true "Policy ID"
// @Param policy body DBPolicyCreateRequest true "Updated Database Policy object"
// @Success 200 {object} DBPolicyUpdateResponse "Policy updated successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid ID or request body"
// @Failure 404 {object} StandardErrorResponse "Policy not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/dbpolicy/{id} [put]
func updateDBPolicy(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid id"))
	}
	var data models.DBPolicy
	if err := c.ShouldBindJSON(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&data); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Updating policy with ID: %d", id)
	_, err = dbPolicySrv.Update(c.Request.Context(), utils.MustIntToUint(id), data)
	if err != nil {
		logger.Errorf("Failed to update policy with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully updated policy with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "Policy updated",
	})
}

// DeleteDBPolicy deletes an existing database policy
// @Summary Delete database policy
// @Description Deletes an existing database policy by ID. Revokes permissions via SqlUpdateDeny before removing policy record.
// @Tags DB Policy
// @Accept json
// @Produce json
// @Param id path int true "Policy ID"
// @Success 200 {object} DBPolicyDeleteResponse "Policy deleted successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid ID"
// @Failure 404 {object} StandardErrorResponse "Policy not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/dbpolicy/{id} [delete]
func deleteDBPolicy(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid id"))
		return
	}

	logger.Debugf("Deleting policy with ID: %d", id)
	err = dbPolicySrv.Delete(c.Request.Context(), utils.MustIntToUint(id))
	if err != nil {
		logger.Errorf("Failed to delete policy with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully deleted policy with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "Policy deleted",
	})
}

// BulkDeleteDBPolicyRequest represents request body for bulk policy deletion
type BulkDeleteDBPolicyRequest struct {
	PolicyIDs []uint `json:"policy_ids" binding:"required,min=1"`
}

// BulkDeleteDBPolicyResponse represents response for bulk policy deletion
type BulkDeleteDBPolicyResponse struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message"`
	DeletedCount int      `json:"deleted_count"`
	FailedIDs    []uint   `json:"failed_ids,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

// BulkDeleteDBPolicies deletes multiple database policies
// @Summary Bulk delete database policies
// @Description Deletes multiple database policies by IDs. Revokes permissions via SqlUpdateDeny for enabled policies before removing records. Returns count of successfully deleted policies and list of failed IDs if any.
// @Tags DB Policy
// @Accept json
// @Produce json
// @Param request body BulkDeleteDBPolicyRequest true "Policy IDs to delete"
// @Success 200 {object} BulkDeleteDBPolicyResponse "Bulk delete completed with success/failure details"
// @Failure 400 {object} StandardErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/dbpolicy/bulkdelete [post]
func bulkDeleteDBPolicies(c *gin.Context) {
	var req BulkDeleteDBPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf("Failed to bind bulk delete request: %v", err)
		utils.ErrorResponse(c, fmt.Errorf("invalid request body: %v", err))
		return
	}

	if err := utils.ValidateStruct(&req); err != nil {
		logger.Errorf("Bulk delete request validation failed: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Bulk delete request for %d policies: %v", len(req.PolicyIDs), req.PolicyIDs)

	deletedCount, failedIDs, errors := dbPolicySrv.BulkDelete(c.Request.Context(), req.PolicyIDs)

	response := BulkDeleteDBPolicyResponse{
		Success:      len(failedIDs) == 0,
		DeletedCount: deletedCount,
		FailedIDs:    failedIDs,
		Errors:       errors,
	}

	if response.Success {
		response.Message = fmt.Sprintf("Successfully deleted %d policies", deletedCount)
		logger.Infof("Bulk delete completed: deleted=%d", deletedCount)
	} else {
		response.Message = fmt.Sprintf("Deleted %d policies, failed %d policies", deletedCount, len(failedIDs))
		logger.Warnf("Bulk delete partially completed: deleted=%d, failed=%d", deletedCount, len(failedIDs))
	}

	utils.JSONResponse(c, http.StatusOK, response)
}

// BulkUpdatePoliciesByActor performs bulk policy update for a specific database actor
// @Summary Bulk update database policies
// @Description Compares existing policies with desired state (Cartesian product of policy_defaults × objects) and executes changes via VeloArtifact background job. Only updates database after successful remote execution to ensure atomic consistency.
// @Tags DB Policy
// @Accept json
// @Produce json
// @Param request body BulkPolicyUpdateRequest true "Bulk policy update request with actor, policy defaults, and objects"
// @Success 200 {object} BulkPolicyUpdateJobResponse "Background job started with job ID and change summary"
// @Failure 400 {object} StandardErrorResponse "Invalid request body or validation error"
// @Failure 404 {object} NotFoundResponse "Referenced entity (cntmgt, dbmgt, actor, policy, object) not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error during job creation or VeloArtifact execution"
// @Router /api/queries/dbpolicy/bulkupdate [post]
func bulkUpdatePoliciesByActor(c *gin.Context) {
	var req dto.BulkPolicyUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf("Failed to bind bulk policy update request: %v", err)
		utils.ErrorResponse(c, fmt.Errorf("invalid request body: %v", err))
		return
	}

	if err := utils.ValidateStruct(&req); err != nil {
		logger.Errorf("Bulk policy update request validation failed: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Bulk policy update request: cntmgt_id=%d, dbmgt_id=%d, actor_id=%d, policy_defaults=%v, objects=%v",
		req.CntMgtID, req.DBMgtID, req.DBActorMgtID, req.NewPolicyDefaults, req.NewObjectMgts)

	jobMessage, err := dbPolicySrv.BulkUpdatePoliciesByActor(c.Request.Context(), req)
	if err != nil {
		logger.Errorf("Failed to start bulk policy update job: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Bulk policy update job started successfully: %s", jobMessage)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": jobMessage,
		"status":  "job_started",
	})
}

// RegisterDBPolicyRoutes registers HTTP endpoints for database policy operations.
func RegisterDBPolicyRoutes(rg *gin.RouterGroup) {
	dbpolicy := rg.Group("/dbpolicy")
	{
		dbpolicy.GET("/cntmgt/:cntmgt", getDBPolicyByCntMgt)
		dbpolicy.POST("", createDBPolicy)
		dbpolicy.POST("/bulkupdate", bulkUpdatePoliciesByActor)
		dbpolicy.POST("/bulkdelete", bulkDeleteDBPolicies)
		dbpolicy.PUT("/:id", updateDBPolicy)
		dbpolicy.DELETE("/:id", deleteDBPolicy)
	}
}
