package dto

import "dbfartifactapi/models"

// BulkPolicyUpdateRequest represents the request payload for bulk policy update
type BulkPolicyUpdateRequest struct {
	CntMgtID          uint   `json:"cntmgt_id" binding:"required"`
	DBMgtID           uint   `json:"dbmgt_id" binding:"required"`
	DBActorMgtID      uint   `json:"dbactormgt_id" binding:"required"`
	NewPolicyDefaults []uint `json:"new_policy_defaults" binding:"required"`
	NewObjectMgts     []uint `json:"new_object_mgts" binding:"required"`
}

// BulkPolicyUpdateResponse represents the response after bulk policy update job is started
type BulkPolicyUpdateResponse struct {
	JobID           string `json:"job_id"`
	Message         string `json:"message"`
	PoliciesAdded   int    `json:"policies_added"`
	PoliciesRemoved int    `json:"policies_removed"`
	Success         bool   `json:"success"`
}

// PolicyCombination represents a unique combination of policy default and object
type PolicyCombination struct {
	PolicyDefaultID uint
	ObjectMgtID     uint
	DBPolicyID      uint
}

// BulkPolicyUpdateJobContext contains context data for bulk policy update job completion
type BulkPolicyUpdateJobContext struct {
	CntMgtID        uint                     `json:"cnt_mgt_id"`
	DBMgtID         uint                     `json:"dbmgt_id"`
	DBActorMgtID    uint                     `json:"dbactor_mgt_id"`
	PolicesToAdd    []PolicyCombination      `json:"policies_to_add"`
	PolicesToRemove []PolicyCombination      `json:"policies_to_remove"`
	DBMgt           *models.DBMgt            `json:"dbmgt"`
	CMT             *models.CntMgt           `json:"cmt"`
	Actor           *models.DBActorMgt       `json:"actor"`
	EndpointID      uint                     `json:"endpoint_id"`
	CommandMap      map[string]CommandDetail `json:"command_map"`
}

// CommandDetail stores details about a command to be executed
type CommandDetail struct {
	SQL               string            `json:"sql"`
	Action            string            `json:"action"`
	PolicyCombination PolicyCombination `json:"policy_combination"`
}
