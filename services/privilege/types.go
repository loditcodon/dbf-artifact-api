package privilege

import (
	"dbfartifactapi/models"

	"gorm.io/gorm"
)

// PolicyEvaluator provides policy evaluation capabilities needed by privilege handlers.
// Implemented by dbPolicyService in services/ to break circular dependency.
type PolicyEvaluator interface {
	// IsPolicyAllowed checks if query result matches allow/deny criteria
	IsPolicyAllowed(output, resAllow, resDeny string) bool
	// ExtractResultValue extracts scalar result from query output
	ExtractResultValue(result []map[string]interface{}) string
	// GetPolicyDefaultsMap returns the full map of policy defaults loaded at startup
	GetPolicyDefaultsMap() map[uint]models.DBPolicyDefault
	// GetDBActorMgts retrieves database actors for a connection management ID
	GetDBActorMgts(tx *gorm.DB, cntID uint) ([]*models.DBActorMgt, error)
	// GetDBObjectsByObjectIdAndDbMgt retrieves database objects by object type and database
	GetDBObjectsByObjectIdAndDbMgt(tx *gorm.DB, objectID int, dbMgtID uint) ([]models.DBObjectMgt, error)
}

// PolicyInput represents processed policy template data.
// Exported to allow cross-package access from privilege handlers.
type PolicyInput struct {
	Policydf models.DBPolicyDefault
	ActorId  uint
	ObjectId int
	DbmgtId  int    // Database management ID, -1 for all databases
	FinalSQL string // Store the processed SQL to avoid re-processing
}

// QueryResult represents a single query execution result from dbfAgentAPI.
// Used by both MySQL and Oracle privilege handlers.
type QueryResult struct {
	QueryKey    string          `json:"query_key"`
	Query       string          `json:"query"`
	Status      string          `json:"status"`
	Result      [][]interface{} `json:"result"`
	ExecuteTime string          `json:"execute_time"`
	DurationMs  int             `json:"duration_ms"`
}

// NewPolicyEvaluatorFunc is a factory function type for creating PolicyEvaluator instances.
// Registered by services/ at init time to break circular dependency.
type NewPolicyEvaluatorFunc func() PolicyEvaluator

// RetrieveJobResultsFunc fetches job results from VeloArtifact via agent API.
// Registered by services/ at init time to break circular dependency.
type RetrieveJobResultsFunc func(jobID string, ep *models.Endpoint) ([]QueryResult, error)

// GetEndpointForJobFunc retrieves endpoint by ID for job processing.
// Registered by services/ at init time to break circular dependency.
type GetEndpointForJobFunc func(jobID string, endpointID uint) (*models.Endpoint, error)

// PrivilegeSessionJobContext contains context data for MySQL privilege session job completion.
// Passed to completion handler when dbfAgentAPI background job finishes.
type PrivilegeSessionJobContext struct {
	CntMgtID      uint                `json:"cnt_mgt_id"`
	DbMgts        []models.DBMgt      `json:"db_mgts"`
	DbActorMgts   []models.DBActorMgt `json:"db_actor_mgts"`
	CMT           *models.CntMgt      `json:"cmt"`
	EndpointID    uint                `json:"endpoint_id"`
	SessionID     string              `json:"session_id"`
	PrivilegeFile string              `json:"privilege_file"`
}
