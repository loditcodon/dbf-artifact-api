package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var sessionSrv = services.NewSessionService()

// SetSessionService initializes the session service instance.
func SetSessionService(srv services.SessionService) {
	sessionSrv = srv
}

// KillSessionRequest represents the request body for killing a session
type KillSessionRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

// KillSession kills a database session
// @Summary Kill database session
// @Description Kills a specific database session using connection management ID and session ID
// @Tags Session Management
// @Accept json
// @Produce json
// @Param cntid path int true "Connection Management ID"
// @Param killSessionRequest body KillSessionRequest true "Session ID to kill"
// @Success 200 {object} JobStartResponse "Kill session command executed successfully for session ID on connection ID"
// @Failure 400 {object} StandardErrorResponse "Invalid request parameters"
// @Failure 500 {object} JobProcessingErrorResponse "Internal server error"
// @Router /api/queries/session/kill/{cntid} [post]
func killSession(c *gin.Context) {
	cntid, err := strconv.Atoi(c.Param("cntid"))
	if err != nil {
		logger.Errorf("Invalid cntid parameter: %v", err)
		utils.ErrorResponse(c, fmt.Errorf("invalid cntid"))
		return
	}

	var req KillSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf("Invalid request body: %v", err)
		utils.ErrorResponse(c, fmt.Errorf("invalid request body: %v", err))
		return
	}

	logger.Infof("Kill session request: cntid=%d, session_id=%s", cntid, req.SessionID)

	jobMessage, err := sessionSrv.KillSession(c.Request.Context(), uint(cntid), req.SessionID)
	if err != nil {
		logger.Errorf("Failed to kill session for cntid %d, session_id %s: %v", cntid, req.SessionID, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully started kill session job for cntid %d, session_id %s: %s", cntid, req.SessionID, jobMessage)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": jobMessage,
	})
}

// RegisterSessionRoutes registers HTTP endpoints for session management operations.
func RegisterSessionRoutes(rg *gin.RouterGroup) {
	session := rg.Group("/session")
	{
		session.POST("/kill/:cntid", killSession)
	}
}
