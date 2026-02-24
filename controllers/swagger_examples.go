package controllers

// Example request/response models for Swagger documentation

// DBPolicyCreateRequest represents the request body for creating a DB policy
type DBPolicyCreateRequest struct {
	CntMgt          uint   `json:"cntmgt" example:"1" description:"Connection Management ID"`
	DBPolicyDefault uint   `json:"dbpolicydefault" example:"1"`
	DBMgt           uint   `json:"dbmgt" example:"1"`
	DBActorMgt      uint   `json:"dbactormgt" example:"1"`
	DBObjectMgt     int    `json:"dbobjectmgt" example:"1"`
	Status          string `json:"status" example:"active"`
	Description     string `json:"description" example:"Policy description"`
}

// DBPolicyCreateResponse represents the response for creating a DB policy
type DBPolicyCreateResponse struct {
	Message string `json:"message" example:"Policy created"`
	ID      uint   `json:"id" example:"123"`
}

// DBPolicyUpdateResponse represents the response for updating a DB policy
type DBPolicyUpdateResponse struct {
	Message string `json:"message" example:"Policy updated"`
}

// DBPolicyDeleteResponse represents the response for deleting a DB policy
type DBPolicyDeleteResponse struct {
	Message string `json:"message" example:"Policy deleted"`
}

// JobStartResponse represents the response when starting a background job
type JobStartResponse struct {
	Message string `json:"message" example:"Background job started: job_12345"`
}

// DBObjectMgtCreateRequest represents the request body for creating a DB object
type DBObjectMgtCreateRequest struct {
	DBMgt       uint   `json:"dbmgt" example:"1"`
	ObjectName  string `json:"objectName" example:"table1"`
	ObjectId    uint   `json:"objectTypeId" example:"1"`
	Description string `json:"description" example:"Table description"`
	Status      string `json:"status" example:"active"`
	SqlParam    string `json:"sqlParam" example:"param1=value1"`
}

// DBObjectMgtCreateResponse represents the response for creating a DB object
type DBObjectMgtCreateResponse struct {
	Message string `json:"message" example:"Object was created successfully"`
	ID      uint   `json:"id" example:"123"`
}

// DBObjectMgtUpdateResponse represents the response for updating a DB object
type DBObjectMgtUpdateResponse struct {
	Message string `json:"message" example:"Object was updated"`
}

// DBObjectMgtDeleteResponse represents the response for deleting a DB object
type DBObjectMgtDeleteResponse struct {
	Message string `json:"message" example:"Object was deleted"`
}

// DBObjectMgtListResponse represents the response for listing DB objects
type DBObjectMgtListResponse struct {
	Message []DBObjectMgtItem `json:"message"`
}

// DBObjectMgtItem represents a single DB object item in the list
type DBObjectMgtItem struct {
	ID          uint   `json:"id" example:"1"`
	DBMgt       uint   `json:"dbmgt" example:"1"`
	ObjectName  string `json:"objectName" example:"table1"`
	ObjectId    uint   `json:"objectTypeId" example:"1"`
	Description string `json:"description" example:"Table description"`
	Status      string `json:"status" example:"active"`
	SqlParam    string `json:"sqlParam" example:"param1=value1"`
}

// ConnectionMgtRequest represents the request body for connection management operations
type ConnectionMgtRequest struct {
	CntMgt uint `json:"cntmgt" example:"1"`
}

// BulkCreateResponse represents the response for bulk creation operations
type BulkCreateResponse struct {
	Message string `json:"message" example:"Inserted 5 new records"`
}

// DBActorMgtCreateRequest represents the request body for creating a DB actor
type DBActorMgtCreateRequest struct {
	CntID       uint   `json:"cntmgt" example:"1" description:"Connection Management ID"`
	DBUser      string `json:"dbuser" example:"testuser"`
	IPAddress   string `json:"ip_address" example:"192.168.1.100"`
	DBClient    string `json:"db_client" example:"mysql_client"`
	OSUser      string `json:"osuser" example:"root"`
	Password    string `json:"dbuserpaswd" example:"password123"`
	Description string `json:"description" example:"Actor description"`
	Status      string `json:"status" example:"active"`
}

// DBActorMgtCreateResponse represents the response for creating a DB actor
type DBActorMgtCreateResponse struct {
	Message string `json:"message" example:"Database Actor was created successfully"`
	ID      uint   `json:"id" example:"123"`
}

// DBActorMgtUpdateRequest represents the request body for updating a DB actor
type DBActorMgtUpdateRequest struct {
	DBUser    string `json:"dbuser" example:"updated_user"`
	IPAddress string `json:"ip_address" example:"192.168.1.101"`
	Password  string `json:"dbuserpaswd" example:"new_password123"`
}

// DBActorMgtUpdateResponse represents the response for updating a DB actor
type DBActorMgtUpdateResponse struct {
	Message string `json:"message" example:"Database Actor was updated"`
}

// DBActorMgtDeleteResponse represents the response for deleting a DB actor
type DBActorMgtDeleteResponse struct {
	Message string `json:"message" example:"Database Actor was deleted"`
}

// DBMgtCreateRequest represents the request body for creating a DB management
type DBMgtCreateRequest struct {
	CntID  uint   `json:"cntmgt" example:"1" description:"Connection Management ID"`
	DbName string `json:"dbname" example:"test_database"`
	DbType string `json:"dbtype" example:"mysql"`
	Status string `json:"status" example:"active"`
}

// DBMgtCreateResponse represents the response for creating a DB management
type DBMgtCreateResponse struct {
	Message string `json:"message" example:"Database was created successfully"`
	ID      uint   `json:"id" example:"123"`
}

// DBMgtDeleteResponse represents the response for deleting a DB management
type DBMgtDeleteResponse struct {
	Message string `json:"message" example:"Database was deleted successfully"`
}

// JobStatusData represents job data in responses
type JobStatusData struct {
	ID        string `json:"id" example:"job_12345"`
	Status    string `json:"status" example:"completed"`
	DBMgtID   uint   `json:"dbmgt_id" example:"1"`
	CreatedAt string `json:"created_at" example:"2023-01-01T00:00:00Z"`
}

// JobStatusSingleResponse represents the response for getting a single job status
type JobStatusSingleResponse struct {
	Success bool          `json:"success" example:"true"`
	Message string        `json:"message" example:"Job status retrieved successfully"`
	Data    JobStatusData `json:"data"`
}

// JobStatusListResponse represents the response for getting all jobs status
type JobStatusListResponse struct {
	Success bool            `json:"success" example:"true"`
	Message string          `json:"message" example:"All jobs status retrieved successfully"`
	Data    []JobStatusData `json:"data"`
}

// JobDeleteResponse represents the response for deleting a job
type JobDeleteResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Job removed from monitoring successfully"`
}

// Error response models for different HTTP status codes
// Based on actual code: utils.ErrorResponse() returns {"error": "message"}

// StandardErrorResponse represents standard error responses from utils.ErrorResponse()
type StandardErrorResponse struct {
	Error string `json:"error" example:"Invalid request body"`
}

// ValidationErrorResponse represents validation errors
type ValidationErrorResponse struct {
	Error string `json:"error" example:"Validation failed: dbuser is required"`
}

// InvalidIDResponse represents invalid ID parameter errors
type InvalidIDResponse struct {
	Error string `json:"error" example:"invalid id"`
}

// InvalidIPResponse represents invalid IP address errors
type InvalidIPResponse struct {
	Error string `json:"error" example:"invalid IP address"`
}

// InvalidConnectionMgtResponse represents invalid connection management ID errors
type InvalidConnectionMgtResponse struct {
	Error string `json:"error" example:"invalid cntmgt"`
}

// InvalidDatabaseMgtResponse represents invalid database management ID errors
type InvalidDatabaseMgtResponse struct {
	Error string `json:"error" example:"invalid dbmgt"`
}

// JobStatusErrorResponse represents job status API errors (different format)
type JobStatusErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Job ID is required"`
}

// JobNotFoundErrorResponse represents job not found errors
type JobNotFoundErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Job not found"`
}

// NotFoundResponse represents 404 Not Found errors (standard format)
type NotFoundResponse struct {
	Error string `json:"error" example:"record not found"`
}

// PolicyNotFoundResponse represents policy not found errors
type PolicyNotFoundResponse struct {
	Error string `json:"error" example:"record not found"`
}

// ObjectNotFoundResponse represents object not found errors
type ObjectNotFoundResponse struct {
	Error string `json:"error" example:"record not found"`
}

// ActorNotFoundResponse represents actor not found errors
type ActorNotFoundResponse struct {
	Error string `json:"error" example:"record not found"`
}

// DatabaseNotFoundResponse represents database not found errors
type DatabaseNotFoundResponse struct {
	Error string `json:"error" example:"record not found"`
}

// InternalServerErrorResponse represents 500 Internal Server Error (standard format)
type InternalServerErrorResponse struct {
	Error string `json:"error" example:"database connection failed"`
}

// DatabaseConnectionErrorResponse represents database connection errors
type DatabaseConnectionErrorResponse struct {
	Error string `json:"error" example:"database connection failed"`
}

// VeloArtifactErrorResponse represents VeloArtifact service errors
type VeloArtifactErrorResponse struct {
	Error string `json:"error" example:"VeloArtifact service unavailable"`
}

// JobProcessingErrorResponse represents job processing errors
type JobProcessingErrorResponse struct {
	Error string `json:"error" example:"failed to start background job"`
}

// PolicyCreationErrorResponse represents policy creation errors
type PolicyCreationErrorResponse struct {
	Error string `json:"error" example:"failed to create policy"`
}

// ObjectCreationErrorResponse represents object creation errors
type ObjectCreationErrorResponse struct {
	Error string `json:"error" example:"failed to create object"`
}

// ActorCreationErrorResponse represents actor creation errors
type ActorCreationErrorResponse struct {
	Error string `json:"error" example:"failed to create actor"`
}

// DatabaseCreationErrorResponse represents database creation errors
type DatabaseCreationErrorResponse struct {
	Error string `json:"error" example:"failed to create database"`
}

// Group Management Swagger Examples

// GroupCreateRequest represents the request body for creating a group
type GroupCreateRequest struct {
	Name           string  `json:"name" example:"MySQL Developers" description:"Group name"`
	Code           string  `json:"code" example:"MYSQL_DEV" description:"Unique group code"`
	Description    *string `json:"description,omitempty" example:"MySQL development team group"`
	DatabaseTypeID *uint   `json:"database_type_id,omitempty" example:"1" description:"Database type ID"`
	GroupType      string  `json:"group_type" example:"CUSTOM" description:"Group type: SYSTEM or CUSTOM"`
	ParentGroupID  *uint   `json:"parent_group_id,omitempty" example:"1" description:"Parent group ID for hierarchy"`
	IsTemplate     bool    `json:"is_template" example:"false" description:"Whether this is a template group"`
	Metadata       *string `json:"metadata,omitempty" example:"{\"department\": \"IT\"}" description:"Additional metadata in JSON format"`
	IsActive       bool    `json:"is_active" example:"true" description:"Whether the group is active"`
}

// GroupCreateResponse represents the response for creating a group
type GroupCreateResponse struct {
	ID             uint    `json:"id" example:"123"`
	Name           string  `json:"name" example:"MySQL Developers"`
	Code           string  `json:"code" example:"MYSQL_DEV"`
	Description    *string `json:"description,omitempty" example:"MySQL development team group"`
	DatabaseTypeID *uint   `json:"database_type_id,omitempty" example:"1"`
	GroupType      string  `json:"group_type" example:"CUSTOM"`
	ParentGroupID  *uint   `json:"parent_group_id,omitempty" example:"1"`
	IsTemplate     bool    `json:"is_template" example:"false"`
	Metadata       *string `json:"metadata,omitempty" example:"{\"department\": \"IT\"}"`
	IsActive       bool    `json:"is_active" example:"true"`
	CreatedAt      string  `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt      string  `json:"updated_at" example:"2024-01-01T00:00:00Z"`
}

// PolicyAssignmentRequest represents the request body for assigning policies to a group
type PolicyAssignmentRequest struct {
	PolicyIDs []uint `json:"policy_ids" description:"List of policy IDs to assign" swaggertype:"array,integer" example:"1,2,3"`
}

// ActorAssignmentRequest represents the request body for assigning actors to a group
type ActorAssignmentRequest struct {
	ActorIDs []uint `json:"actor_ids" description:"List of actor IDs to assign" swaggertype:"array,integer" example:"521,522,523"`
}

// GroupAssignmentRequest represents the request body for bulk assigning groups to a policy
type GroupAssignmentRequest struct {
	GroupIDs []uint `json:"group_ids" description:"List of group IDs to assign" swaggertype:"array,integer" example:"1,2,4"`
}

// GroupAssignmentResponse represents the response for group assignment operations
type GroupAssignmentResponse struct {
	Message string `json:"message" example:"Successfully assigned 3 policies to group"`
}

// GroupBulkAssignmentResponse represents the response for bulk assignment operations
type GroupBulkAssignmentResponse struct {
	Message      string   `json:"message" example:"Policy assigned to 2/3 groups"`
	SuccessCount int      `json:"success_count" example:"2"`
	TotalCount   int      `json:"total_count,omitempty" example:"3"`
	Errors       []string `json:"errors,omitempty" example:"[\"Group 3: group not found\"]"`
}

// GroupDetailsResponse represents comprehensive group information
type GroupDetailsResponse struct {
	Group    GroupCreateResponse       `json:"group"`
	Policies []GroupListPolicyResponse `json:"policies"`
	Actors   []ActorResponse           `json:"actors"`
}

// GroupListPolicyResponse represents a policy in group policy list
type GroupListPolicyResponse struct {
	ID                uint    `json:"id" example:"1"`
	DatabaseTypeID    *uint   `json:"database_type_id,omitempty" example:"1"`
	CategoryID        *uint   `json:"category_id,omitempty" example:"1"`
	Name              string  `json:"name" example:"SELECT Permission"`
	Code              string  `json:"code" example:"SELECT_PERM"`
	Description       *string `json:"description,omitempty" example:"Allow SELECT operations"`
	RiskLevel         string  `json:"risk_level" example:"LOW"`
	IsDDL             bool    `json:"is_ddl" example:"false"`
	IsDML             bool    `json:"is_dml" example:"true"`
	IsSystem          bool    `json:"is_system" example:"false"`
	DBPolicyDefaultID *string `json:"dbpolicydefault_id,omitempty" example:"1,2,3"`
	Metadata          *string `json:"metadata,omitempty" example:"{\"category\": \"read\"}"`
	IsActive          bool    `json:"is_active" example:"true"`
	CreatedAt         string  `json:"created_at" example:"2024-01-01T00:00:00Z"`
}

// ActorResponse represents an actor in group actor list
type ActorResponse struct {
	ID          uint    `json:"id" example:"521"`
	CntID       *uint   `json:"cntid,omitempty" example:"1"`
	DBUser      string  `json:"dbuser" example:"testuser"`
	IPAddress   string  `json:"ip_address" example:"192.168.1.100"`
	DBClient    *string `json:"db_client,omitempty" example:"mysql_client"`
	OSUser      *string `json:"osuser,omitempty" example:"root"`
	Password    *string `json:"password,omitempty" example:"password123"`
	Description *string `json:"description,omitempty" example:"Test database user"`
	Status      *string `json:"status,omitempty" example:"active"`
}

// Group Management Error Responses

// GroupNotFoundResponse represents group not found errors
type GroupNotFoundResponse struct {
	Error string `json:"error" example:"group with id=1 not found"`
}

// InvalidGroupIDResponse represents invalid group ID errors
type InvalidGroupIDResponse struct {
	Error string `json:"error" example:"invalid group ID"`
}

// InvalidPolicyIDResponse represents invalid policy ID errors
type InvalidPolicyIDResponse struct {
	Error string `json:"error" example:"invalid policy ID"`
}

// InvalidActorIDResponse represents invalid actor ID errors
type InvalidActorIDResponse struct {
	Error string `json:"error" example:"invalid actor ID"`
}

// GroupCodeConflictResponse represents group code conflict errors
type GroupCodeConflictResponse struct {
	Error string `json:"error" example:"group with code 'MYSQL_DEV' already exists"`
}

// GroupHasChildrenResponse represents error when trying to delete group with children
type GroupHasChildrenResponse struct {
	Error string `json:"error" example:"cannot delete group with child groups. Found 2 child groups"`
}

// GroupCreationErrorResponse represents group creation errors
type GroupCreationErrorResponse struct {
	Error string `json:"error" example:"failed to create group"`
}

// PolicyAssignmentErrorResponse represents policy assignment errors
type PolicyAssignmentErrorResponse struct {
	Error string `json:"error" example:"failed to assign policies to group"`
}

// ActorAssignmentErrorResponse represents actor assignment errors
type ActorAssignmentErrorResponse struct {
	Error string `json:"error" example:"failed to assign actors to group"`
}

// EmptyListValidationResponse represents empty list validation errors
type EmptyListValidationResponse struct {
	Error string `json:"error" example:"policy IDs list cannot be empty"`
}

// Success Response for deletions
type SuccessResponse struct {
	Message string `json:"message" example:"Group deleted successfully"`
}

// Bulk Policy Update Swagger Models

// BulkPolicyUpdateRequest represents the request body for bulk policy update
type BulkPolicyUpdateRequest struct {
	CntMgtID          uint   `json:"cntmgt_id" example:"1" description:"Connection Management ID" binding:"required"`
	DBMgtID           uint   `json:"dbmgt_id" example:"5" description:"Database Management ID" binding:"required"`
	DBActorMgtID      uint   `json:"dbactormgt_id" example:"3" description:"Database Actor (User) ID" binding:"required"`
	NewPolicyDefaults []uint `json:"new_policy_defaults" swaggertype:"array,integer" example:"10,11,12" description:"List of DBPolicyDefault IDs to apply" binding:"required"`
	NewObjectMgts     []uint `json:"new_object_mgts" swaggertype:"array,integer" example:"100,101,102" description:"List of DBObjectMgt IDs (tables, views, etc.)" binding:"required"`
}

// BulkPolicyUpdateJobResponse represents the response when bulk policy update job starts
type BulkPolicyUpdateJobResponse struct {
	Message string `json:"message" example:"Bulk policy update background job started: F.C4LG5UMHSAPEI0J. Adding 6 policies, removing 3 policies."`
	Status  string `json:"status" example:"job_started"`
}

// GroupAssignmentsUpdateRequest represents request for optimized bulk update
type GroupAssignmentsUpdateRequest struct {
	PolicyIDs []uint `json:"policy_ids" swaggertype:"array,integer" example:"1,2,3" validate:"required"`
	ActorIDs  []uint `json:"actor_ids" swaggertype:"array,integer" example:"521,522,523" validate:"required"`
}

// GroupAssignmentsUpdateResult represents result of optimized bulk update
type GroupAssignmentsUpdateResult struct {
	PoliciesAdded   []uint   `json:"policies_added" swaggertype:"array,integer" example:"1,3"`
	PoliciesRemoved []uint   `json:"policies_removed" swaggertype:"array,integer" example:"2"`
	ActorsAdded     []uint   `json:"actors_added" swaggertype:"array,integer" example:"521,523"`
	ActorsRemoved   []uint   `json:"actors_removed" swaggertype:"array,integer" example:"522"`
	VeloJobsCreated []string `json:"velo_jobs_created" example:"job_1_allow_1735566780,job_1_deny_1735566781"`
	TotalExecutions int      `json:"total_executions" example:"2"`
}

// VeloExecutionError represents a failed VeloArtifact execution
type VeloExecutionError struct {
	ConnectionID uint   `json:"connection_id" example:"2"`
	Operation    string `json:"operation" example:"allow"`
	Error        string `json:"error" example:"connection timeout: failed to connect to client"`
}

// ActorAssignmentResult represents detailed result of actor assignment operation
type ActorAssignmentResult struct {
	ActorsAssigned    []uint               `json:"actors_assigned" swaggertype:"array,integer" example:"10,20,30"`
	AlreadyAssigned   []uint               `json:"already_assigned" swaggertype:"array,integer" example:"5"`
	Success           bool                 `json:"success" example:"true"`
	PartialSuccess    bool                 `json:"partial_success" example:"false"`
	VeloJobsSucceeded []string             `json:"velo_jobs_succeeded" example:"batch_conn_1_group_5_allow_15_commands_1729800000"`
	VeloJobsFailed    []VeloExecutionError `json:"velo_jobs_failed"`
	TotalExecutions   int                  `json:"total_executions" example:"1"`
}

// ActorRemovalResult represents detailed result of actor removal operation
type ActorRemovalResult struct {
	ActorsRemoved     []uint               `json:"actors_removed" swaggertype:"array,integer" example:"10,20"`
	NotAssigned       []uint               `json:"not_assigned" swaggertype:"array,integer" example:"99"`
	Success           bool                 `json:"success" example:"true"`
	PartialSuccess    bool                 `json:"partial_success" example:"false"`
	VeloJobsSucceeded []string             `json:"velo_jobs_succeeded" example:"batch_conn_1_group_5_deny_10_commands_1729800001"`
	VeloJobsFailed    []VeloExecutionError `json:"velo_jobs_failed"`
	TotalExecutions   int                  `json:"total_executions" example:"1"`
}

// Oracle PDB Management Swagger Models

// PDBGetAllRequest represents the request body for synchronizing all PDBs
type PDBGetAllRequest struct {
	CntMgt uint `json:"cntmgt" example:"1" description:"CDB Connection Management ID"`
}

// PDBCreateRequest represents the request body for creating a new Oracle PDB
type PDBCreateRequest struct {
	CntMgt   uint   `json:"cntmgt" example:"1" description:"CDB Connection Management ID"`
	PDBName  string `json:"pdbname" example:"PDB3" description:"Name of the new PDB"`
	SqlParam string `json:"sql_param,omitempty" example:"" description:"Optional hex-encoded SQL parameters"`
}

// PDBCreateResponse represents the response for creating a new Oracle PDB
type PDBCreateResponse struct {
	Message string `json:"message" example:"PDB was created successfully"`
	ID      uint   `json:"id" example:"123"`
}

// PDBUpdateRequest represents the request body for altering an Oracle PDB
type PDBUpdateRequest struct {
	SqlParam string `json:"sql_param" example:"4f50454e" description:"Hex-encoded ALTER PLUGGABLE DATABASE parameters"`
}

// PDBUpdateResponse represents the response for altering an Oracle PDB
type PDBUpdateResponse struct {
	Message string `json:"message" example:"PDB was altered successfully"`
}

// PDBDeleteRequest represents the optional request body for dropping an Oracle PDB
type PDBDeleteRequest struct {
	SqlParam string `json:"sql_param,omitempty" example:"" description:"Optional hex-encoded DROP parameters (e.g., INCLUDING DATAFILES)"`
}

// PDBDeleteResponse represents the response for dropping an Oracle PDB
type PDBDeleteResponse struct {
	Message string `json:"message" example:"PDB was dropped successfully"`
}
