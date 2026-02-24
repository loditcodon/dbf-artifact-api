package controllers

import (
	"net/http"
	"strconv"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services"

	"github.com/gin-gonic/gin"
)

// ConnectionTestController handles database connection testing operations.
type ConnectionTestController struct {
	connectionTestService services.ConnectionTestService
}

// NewConnectionTestController creates a new connection test controller instance.
func NewConnectionTestController() *ConnectionTestController {
	return &ConnectionTestController{
		connectionTestService: services.NewConnectionTestService(),
	}
}

// TestConnection tests database connection status
// @Summary Test database connection
// @Description Tests connection to database and updates status in the system
// @Tags Connection
// @Accept json
// @Produce json
// @Param id path int true "Connection Management ID"
// @Success 200 {object} map[string]interface{} "Connection test successful"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 404 {object} map[string]interface{} "Connection not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/queries/connection/test/{id} [post]
func (ctrl *ConnectionTestController) TestConnection(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		logger.Errorf("Invalid connection ID parameter: %s, error: %v", idParam, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid connection ID",
			"message": "Connection ID must be a positive integer",
		})
		return
	}

	logger.Infof("Testing connection for ID: %d", id)

	// Execute connection test
	result, err := ctrl.connectionTestService.TestConnection(c.Request.Context(), uint(id))
	if err != nil {
		logger.Errorf("Connection test failed for ID %d: %v", id, err)

		// Determine appropriate HTTP status code based on error type
		if err.Error() == "connection with id="+idParam+" not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Connection not found",
				"message": err.Error(),
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Connection test failed",
				"message": err.Error(),
			})
		}
		return
	}

	logger.Infof("Connection test completed for ID %d", id)

	// Return success response with test result
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Connection test completed successfully",
		"data": gin.H{
			"connection_id": id,
			"test_result":   result,
		},
	})
}

// RegisterConnectionTestRoutes registers connection test routes
func RegisterConnectionTestRoutes(rg *gin.RouterGroup) {
	connectionController := NewConnectionTestController()

	connection := rg.Group("/connection")
	{
		connection.POST("/test/:id", connectionController.TestConnection)
	}
}
