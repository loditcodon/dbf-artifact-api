package controllers

import (
	"net/http"
	"strconv"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/compliance"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var policyComplianceSrv = compliance.NewPolicyComplianceService()

// SetPolicyComplianceService initializes the policy compliance service instance.
func SetPolicyComplianceService(srv compliance.PolicyComplianceService) {
	policyComplianceSrv = srv
}

// StartPolicyComplianceCheck starts policy compliance check for a connection
// @Summary Start policy compliance check
// @Description Initiates policy compliance check for the specified connection management ID
// @Tags Policy Compliance
// @Accept json
// @Produce json
// @Param id path int true "Connection Management ID"
// @Success 200 {object} map[string]string "Policy compliance check started"
// @Failure 400 {object} map[string]string "Invalid connection management ID"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/queries/policy-compliance/start/{id} [post]
func startPolicyComplianceCheck(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Starting policy compliance check for cntmgt ID: %d", id)
	result, err := policyComplianceSrv.StartCheck(c.Request.Context(), utils.MustIntToUint(id))
	if err != nil {
		logger.Errorf("Failed to start policy compliance check for cntmgt ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully started policy compliance check for cntmgt ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": result,
	})
}

// RegisterPolicyComplianceRoutes registers HTTP endpoints for policy compliance operations.
func RegisterPolicyComplianceRoutes(rg *gin.RouterGroup) {
	policyCompliance := rg.Group("/policy-compliance")
	{
		policyCompliance.POST("/start/:id", startPolicyComplianceCheck)
	}
}
