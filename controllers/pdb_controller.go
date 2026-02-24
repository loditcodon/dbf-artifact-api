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

var pdbSrv = services.NewPDBService()

// SetPDBService initializes the PDB management service instance.
// Used for dependency injection in tests to provide mock implementations.
func SetPDBService(s services.PDBService) {
	pdbSrv = s
}

// PostPDBAll synchronizes all PDBs for an Oracle CDB connection
// @Summary Synchronize all PDBs for CDB connection
// @Description Queries DBA_PDBS on the remote Oracle server and synchronizes local PDB records
// @Tags PDB Management
// @Accept json
// @Produce json
// @Param params body PDBGetAllRequest true "CDB Connection Management ID"
// @Success 200 {object} BulkCreateResponse "PDBs synchronized successfully with count"
// @Failure 400 {object} StandardErrorResponse "Invalid connection management ID"
// @Failure 500 {object} DatabaseCreationErrorResponse "Internal server error"
// @Router /api/queries/pdb/all [post]
func postPDBAll(c *gin.Context) {
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

	logger.Debugf("Synchronizing PDBs for CDB cntmgt: %d", params.CntMgt)
	changeCount, err := pdbSrv.GetAll(c.Request.Context(), params.CntMgt)
	if err != nil {
		logger.Errorf("Failed to synchronize PDBs for cntmgt %d: %v", params.CntMgt, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully synchronized %d PDB changes for cntmgt %d", changeCount, params.CntMgt)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": fmt.Sprintf("Synchronized %d PDB changes", changeCount),
	})
}

// CreatePDB creates a new Oracle Pluggable Database
// @Summary Create PDB
// @Description Creates a new Oracle Pluggable Database on the remote server and registers it locally
// @Tags PDB Management
// @Accept json
// @Produce json
// @Param pdb body PDBCreateRequest true "PDB creation parameters"
// @Success 201 {object} PDBCreateResponse "PDB created successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} DatabaseCreationErrorResponse "Internal server error"
// @Router /api/queries/pdb [post]
func createPDB(c *gin.Context) {
	var req services.PDBCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&req); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Creating new PDB: %s for CDB cntmgt: %d", req.PDBName, req.CntMgt)
	newObj, err := pdbSrv.Create(c.Request.Context(), req)
	if err != nil {
		logger.Errorf("Failed to create PDB %s: %v", req.PDBName, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully created PDB %s with ID: %d", req.PDBName, newObj.ID)
	utils.JSONResponse(c, http.StatusCreated, gin.H{
		"message": "PDB was created successfully",
		"id":      newObj.ID,
	})
}

// UpdatePDB alters an existing Oracle Pluggable Database
// @Summary Alter PDB
// @Description Executes ALTER PLUGGABLE DATABASE on the remote Oracle server
// @Tags PDB Management
// @Accept json
// @Produce json
// @Param id path int true "PDB Connection Management ID"
// @Param pdb body PDBUpdateRequest true "PDB alter parameters"
// @Success 200 {object} PDBUpdateResponse "PDB altered successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid ID or request body"
// @Failure 404 {object} StandardErrorResponse "PDB not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/pdb/{id} [put]
func updatePDB(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	var req services.PDBUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, err)
		return
	}
	if err := utils.ValidateStruct(&req); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Altering PDB with ID: %d", id)
	if err := pdbSrv.Update(c.Request.Context(), utils.MustIntToUint(id), req); err != nil {
		logger.Errorf("Failed to alter PDB with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully altered PDB with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "PDB was altered successfully",
	})
}

// DeletePDB drops an Oracle Pluggable Database
// @Summary Drop PDB
// @Description Drops a Pluggable Database on the remote Oracle server and removes local record
// @Tags PDB Management
// @Accept json
// @Produce json
// @Param id path int true "PDB Connection Management ID"
// @Param params body PDBDeleteRequest false "Optional DROP parameters"
// @Success 200 {object} PDBDeleteResponse "PDB dropped successfully"
// @Failure 400 {object} StandardErrorResponse "Invalid PDB connection management ID"
// @Failure 404 {object} StandardErrorResponse "PDB not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/pdb/{id} [delete]
func deletePDB(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	// Optional sql_param for DROP command options (e.g., hex-encoded "INCLUDING DATAFILES")
	var params struct {
		SqlParam string `json:"sql_param"`
	}
	// Body is optional for DELETE, ignore bind errors
	_ = c.ShouldBindJSON(&params)

	logger.Debugf("Dropping PDB with ID: %d", id)
	if err := pdbSrv.Delete(c.Request.Context(), utils.MustIntToUint(id), params.SqlParam); err != nil {
		logger.Errorf("Failed to drop PDB with ID %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}
	logger.Infof("Successfully dropped PDB with ID: %d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "PDB was dropped successfully",
	})
}

// RegisterPDBRoutes registers HTTP endpoints for Oracle PDB management operations.
func RegisterPDBRoutes(rg *gin.RouterGroup) {
	pdb := rg.Group("/pdb")
	{
		pdb.POST("/all", postPDBAll)
		pdb.POST("", createPDB)
		pdb.PUT("/:id", updatePDB)
		pdb.DELETE("/:id", deletePDB)
	}
}
