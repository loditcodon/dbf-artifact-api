package oracle

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dbfartifactapi/config"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/job"
	"dbfartifactapi/services/privilege"
	"dbfartifactapi/utils"

	"gorm.io/gorm"
)

// processedOraclePrivilegeJobs tracks Oracle jobs that have already been processed to prevent duplicate execution
var processedOraclePrivilegeJobs sync.Map

// policyInput represents a processed Oracle policy query ready for execution.
type policyInput struct {
	policydf models.DBPolicyDefault
	actorId  uint
	objectId int
	dbmgtId  int
	finalSQL string
}

// policyClassification categorizes Oracle policy templates by execution order.
type policyClassification struct {
	superPrivileges     []models.DBPolicyDefault
	actionWidePrivs     []models.DBPolicyDefault
	objectSpecificPrivs []models.DBPolicyDefault
}

// grantedActionsCache tracks which actions have been granted to which actors.
type grantedActionsCache struct {
	mu      sync.RWMutex
	granted map[uint]map[int]bool // actorID -> actionID -> granted
}

// allowedPolicyResults tracks which policies were allowed for each actor.
type allowedPolicyResults struct {
	mu            sync.Mutex
	actorPolicies map[uint]map[uint]bool // actorID -> policyDefaultID -> true
}

// superPrivilegeActors tracks actors that have been granted super privileges.
type superPrivilegeActors struct {
	mu     sync.RWMutex
	actors map[uint]bool
}

func newGrantedActionsCache() *grantedActionsCache {
	return &grantedActionsCache{
		granted: make(map[uint]map[int]bool),
	}
}

func (c *grantedActionsCache) markGranted(actorID uint, actionID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.granted[actorID] == nil {
		c.granted[actorID] = make(map[int]bool)
	}
	c.granted[actorID][actionID] = true
}

func (c *grantedActionsCache) isGranted(actorID uint, actionID int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if actions, ok := c.granted[actorID]; ok {
		return actions[actionID]
	}
	return false
}

func newAllowedPolicyResults() *allowedPolicyResults {
	return &allowedPolicyResults{
		actorPolicies: make(map[uint]map[uint]bool),
	}
}

func (a *allowedPolicyResults) recordAllowed(actorID uint, policyDefaultID uint) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.actorPolicies[actorID] == nil {
		a.actorPolicies[actorID] = make(map[uint]bool)
	}
	a.actorPolicies[actorID][policyDefaultID] = true
}

func newSuperPrivilegeActors() *superPrivilegeActors {
	return &superPrivilegeActors{
		actors: make(map[uint]bool),
	}
}

func (s *superPrivilegeActors) markSuperPrivilege(actorID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actors[actorID] = true
}

func (s *superPrivilegeActors) hasSuperPrivilege(actorID uint) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.actors[actorID]
}

// groupListPolicy maps a list policy ID to its set of required policy default IDs.
type groupListPolicy struct {
	listPolicyID     uint
	policyDefaultIDs map[uint]bool
}

// CreateOraclePrivilegeSessionCompletionHandler creates callback for Oracle privilege session job completion.
// Handles both notification-based and VeloArtifact polling completion flows.
func CreateOraclePrivilegeSessionCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing Oracle privilege session completion for job %s, status: %s", jobID, statusResp.Status)

		// Extract context data from job
		contextData, ok := jobInfo.ContextData["oracle_privilege_session_context"]
		if !ok {
			return fmt.Errorf("missing oracle privilege session context data for job %s", jobID)
		}

		// Process completed jobs
		if statusResp.Status == "completed" {
			return processOraclePrivilegeSessionResults(jobID, contextData, statusResp, jobInfo)
		}

		logger.Errorf("Oracle privilege session job %s failed", jobID)
		return fmt.Errorf("oracle privilege session job failed: %s", statusResp.Message)
	}
}

// processOraclePrivilegeSessionResults processes the results of Oracle privilege data loading job.
// Routes to notification-based or VeloArtifact polling processing based on context.
func processOraclePrivilegeSessionResults(jobID string, contextData interface{}, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing Oracle privilege session results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	// Extract OraclePrivilegeSessionJobContext from contextData
	sessionContext, ok := contextData.(*OraclePrivilegeSessionJobContext)
	if !ok {
		return fmt.Errorf("invalid oracle privilege session context data for job %s", jobID)
	}

	// Check if notification-based completion
	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processOraclePrivilegeSessionFromNotification(jobID, sessionContext, notificationData, statusResp)
	}

	// Legacy VeloArtifact polling flow
	return processOraclePrivilegeSessionFromVeloArtifact(jobID, sessionContext, statusResp)
}

// processOraclePrivilegeSessionFromNotification handles Oracle privilege session processing from notification.
func processOraclePrivilegeSessionFromNotification(jobID string, sessionContext *OraclePrivilegeSessionJobContext, notificationData interface{}, statusResp *job.StatusResponse) error {
	logger.Infof("Processing Oracle privilege session from notification for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Extract notification data
	notification, ok := notificationData.(map[string]interface{})
	if !ok {
		err := fmt.Errorf("invalid notification data format for oracle job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	fileName, ok := notification["fileName"].(string)
	if !ok {
		err := fmt.Errorf("missing fileName in notification data for oracle job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	md5Hash, ok := notification["md5Hash"].(string)
	if !ok {
		err := fmt.Errorf("missing md5Hash in notification data for oracle job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	success, ok := notification["success"].(bool)
	if !ok || !success {
		err := fmt.Errorf("oracle job %s was not successful according to notification", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Processing notification-based Oracle privilege data: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Construct local file path
	localFilePath := fmt.Sprintf("%s/%s/%s", config.Cfg.NotificationFileDir, jobID, md5Hash)
	logger.Infof("Processing Oracle privilege data from notification file: %s", localFilePath)

	// Parse privilege data results
	privilegeData, err := parseOraclePrivilegeDataFile(localFilePath)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse notification oracle privilege data for job %s: %v", jobID, err)
		jobMonitor.FailJobAfterProcessing(jobID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	logger.Infof("Successfully parsed %d Oracle privilege table results from notification for job %s", len(privilegeData), jobID)

	// Process policies with loaded privilege data
	totalPolicies, err := createOraclePoliciesWithPrivilegeData(jobID, sessionContext, privilegeData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Oracle privilege session completed successfully - created %d policies", totalPolicies)
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark oracle job as completed: %v", err)
	}

	logger.Infof("Oracle privilege session notification handler executed successfully for job %s - created %d policies", jobID, totalPolicies)
	return nil
}

// processOraclePrivilegeSessionFromVeloArtifact handles Oracle privilege session processing via VeloArtifact polling.
func processOraclePrivilegeSessionFromVeloArtifact(jobID string, sessionContext *OraclePrivilegeSessionJobContext, statusResp *job.StatusResponse) error {
	logger.Infof("Processing Oracle privilege session from VeloArtifact polling for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Get endpoint information
	ep, err := privilege.GetEndpointForJob(jobID, sessionContext.EndpointID)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Retrieve privilege data results
	privilegeData, err := retrieveOraclePrivilegeDataResults(jobID, ep)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Successfully retrieved %d Oracle privilege table results via VeloArtifact for job %s", len(privilegeData), jobID)

	// Process policies with loaded privilege data
	totalPolicies, err := createOraclePoliciesWithPrivilegeData(jobID, sessionContext, privilegeData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Oracle privilege session completed successfully - created %d policies", totalPolicies)
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark oracle job as completed: %v", err)
	}

	logger.Infof("Oracle privilege session VeloArtifact handler executed successfully for job %s - created %d policies", jobID, totalPolicies)
	return nil
}

// parseOraclePrivilegeDataFile reads and parses Oracle privilege data results file.
func parseOraclePrivilegeDataFile(filePath string) ([]privilege.QueryResult, error) {
	logger.Debugf("Reading Oracle privilege data file: %s", filePath)

	// Read file content
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read oracle privilege data file %s: %v", filePath, err)
	}

	// Parse JSON content
	var resultsData []privilege.QueryResult
	if err := json.Unmarshal(fileData, &resultsData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from oracle file %s: %v", filePath, err)
	}

	logger.Debugf("Successfully parsed Oracle privilege data file with %d table results", len(resultsData))
	return resultsData, nil
}

// retrieveOraclePrivilegeDataResults gets Oracle privilege data from VeloArtifact.
func retrieveOraclePrivilegeDataResults(jobID string, ep *models.Endpoint) ([]privilege.QueryResult, error) {
	return privilege.RetrieveJobResults(jobID, ep)
}

// createOraclePoliciesWithPrivilegeData creates policies using three-pass execution strategy for Oracle databases.
// Uses same go-mysql-server approach as MySQL - creates Oracle privilege tables and executes DBPolicyDefault queries.
// Pass 1: Super privileges - actor_id=-1, object_id=-1, dbmgt_id=-1
// Pass 2: Action-wide privileges (DBGroupListPolicies) - object_id=-1, dbmgt_id=-1
// Pass 3: Object-specific privileges - normal policies with specific objects/databases
func createOraclePoliciesWithPrivilegeData(jobID string, sessionContext *OraclePrivilegeSessionJobContext, privilegeData []privilege.QueryResult) (int, error) {
	// Idempotency check: prevent duplicate processing from notification + polling
	if _, alreadyProcessed := processedOraclePrivilegeJobs.LoadOrStore(jobID, true); alreadyProcessed {
		logger.Warnf("Oracle job %s already processed, skipping duplicate execution", jobID)
		return 0, nil
	}

	logger.Infof("Creating Oracle policies from privilege data for job %s (conn_type=%s)",
		jobID, sessionContext.ConnType.String())

	// Parse privilege data into structured types
	oraclePrivData, err := parseOraclePrivilegeResults(privilegeData, sessionContext.ConnType)
	if err != nil {
		return 0, fmt.Errorf("failed to parse oracle privilege results: %w", err)
	}

	logger.Infof("Parsed Oracle privilege data: sys_privs=%d, tab_privs=%d, role_privs=%d, pwfile_users=%d, cdb_sys_privs=%d",
		len(oraclePrivData.SysPrivs), len(oraclePrivData.TabPrivs), len(oraclePrivData.RolePrivs),
		len(oraclePrivData.PwFileUsers), len(oraclePrivData.CdbSysPrivs))

	// Create in-memory Oracle privilege session (uses go-mysql-server with Oracle tables)
	ctx := context.Background()
	session, err := NewOraclePrivilegeSession(ctx, sessionContext.SessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to create oracle privilege session: %w", err)
	}
	defer session.Close()

	// Load Oracle privilege data into in-memory tables
	if err := session.LoadOraclePrivilegeDataFromResults(oraclePrivData); err != nil {
		return 0, fmt.Errorf("failed to load oracle privilege data: %w", err)
	}

	logger.Infof("Oracle privilege data loaded into session %s, classifying policy templates", sessionContext.SessionID)

	// Create query log file for tracking (only if enabled)
	var logFile *os.File
	if config.Cfg.EnableMySQLPrivilegeQueryLogging {
		logFileName := fmt.Sprintf("oracle_privilege_queries_%s_%s.log", sessionContext.SessionID, time.Now().Format("20060102_150405"))
		logFilePath := filepath.Join(config.Cfg.DBFWebTempDir, logFileName)
		logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logger.Warnf("Failed to create oracle query log file %s: %v", logFilePath, err)
			logFile = nil
		} else {
			defer logFile.Close()
			logger.Infof("Oracle query log file created: %s", logFilePath)
			logFile.WriteString(fmt.Sprintf("=== Oracle Privilege Session Query Log ===\n"))
			logFile.WriteString(fmt.Sprintf("Job ID: %s\n", jobID))
			logFile.WriteString(fmt.Sprintf("Session ID: %s\n", sessionContext.SessionID))
			logFile.WriteString(fmt.Sprintf("Connection Type: %s\n", sessionContext.ConnType.String()))
			logFile.WriteString(fmt.Sprintf("Started: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
		}
	}

	baseRepo := repository.NewBaseRepository()
	tx := baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	service := privilege.NewPolicyEvaluator()

	// Classify Oracle policy templates (database_type_id = 3)
	classification := classifyOraclePolicyTemplates(service.GetPolicyDefaultsMap())
	grantedActions := newGrantedActionsCache()
	allowedResults := newAllowedPolicyResults()
	superPrivActors := newSuperPrivilegeActors()
	totalPolicies := 0

	// PASS 1: Super privileges - grant ALL actions on ALL objects for ALL databases
	if len(classification.superPrivileges) > 0 {
		logger.Infof("Processing Oracle Pass 1: %d super privilege templates", len(classification.superPrivileges))

		superQueries, err := processOracleSQLTemplatesForSession(
			tx, classification.superPrivileges, sessionContext.DbMgts,
			sessionContext.DbActorMgts, sessionContext.CMT, service, sessionContext.ConnType)
		if err != nil {
			logger.Errorf("Failed to build oracle super privilege queries: %v", err)
		} else {
			superPolicies := executeOracleSuperPrivilegeQueries(tx, session, superQueries, service, sessionContext.CntMgtID, sessionContext.CMT, allowedResults, superPrivActors, logFile)
			totalPolicies += superPolicies
			logger.Infof("Oracle Pass 1 completed: %d super policies created", superPolicies)
		}
	}

	// PASS 2: Action-wide privileges - grant specific action on ALL objects for ALL databases
	if len(classification.actionWidePrivs) > 0 {
		logger.Infof("Processing Oracle Pass 2: %d action-wide privilege templates", len(classification.actionWidePrivs))

		actionWideQueries, err := processOracleSQLTemplatesForSession(
			tx, classification.actionWidePrivs, sessionContext.DbMgts,
			sessionContext.DbActorMgts, sessionContext.CMT, service, sessionContext.ConnType)
		if err != nil {
			logger.Errorf("Failed to build oracle action-wide queries: %v", err)
		} else {
			actionPolicies := executeOracleActionWideQueries(tx, session, actionWideQueries, service, sessionContext.CntMgtID, sessionContext.CMT, grantedActions, allowedResults, superPrivActors, logFile)
			totalPolicies += actionPolicies
			logger.Infof("Oracle Pass 2 completed: %d action-wide policies created", actionPolicies)
		}
	}

	// PASS 3: Object-specific privileges - skip if action already granted in Pass 2
	if len(classification.objectSpecificPrivs) > 0 {
		logger.Infof("Processing Oracle Pass 3: %d object-specific privilege templates", len(classification.objectSpecificPrivs))

		// Build cache for specific object queries
		cache := newOracleQueryBuildCache(tx, sessionContext.CntMgtID, sessionContext.DbMgts, classification.objectSpecificPrivs)

		// General queries (objectId = -1)
		generalQueries, err := processOracleSQLTemplatesForSession(
			tx, classification.objectSpecificPrivs, sessionContext.DbMgts,
			sessionContext.DbActorMgts, sessionContext.CMT, service, sessionContext.ConnType)
		if err != nil {
			logger.Errorf("Failed to build oracle general object-specific queries: %v", err)
		}

		// Specific queries (objectId != -1) - uses SqlGetSpecific with object name substitution
		specificQueries, err := processOracleSpecificSQLTemplatesForSession(
			tx, classification.objectSpecificPrivs, sessionContext.DbMgts,
			sessionContext.DbActorMgts, sessionContext.CMT, service, sessionContext.ConnType, cache)
		if err != nil {
			logger.Errorf("Failed to build oracle specific object-specific queries: %v", err)
		}

		// Merge all object queries
		allObjectQueries := make(map[string]policyInput)
		for k, v := range generalQueries {
			allObjectQueries[k] = v
		}
		for k, v := range specificQueries {
			allObjectQueries[k] = v
		}

		objectPolicies := executeOracleObjectSpecificQueries(tx, session, allObjectQueries, service, sessionContext.CntMgtID, grantedActions, allowedResults, superPrivActors, logFile)
		totalPolicies += objectPolicies
		logger.Infof("Oracle Pass 3 completed: %d object-specific policies created (general=%d, specific=%d queries)",
			objectPolicies, len(generalQueries), len(specificQueries))
	}

	// Assign actors to groups based on direct query results
	// Oracle uses superPrivGroupID = 1000 (not 1 like MySQL)
	logger.Infof("Assigning Oracle actors to groups based on allowed query results from %d policies", totalPolicies)
	if err := assignOracleActorsToGroups(tx, sessionContext.CntMgtID, sessionContext.CMT, allowedResults, superPrivActors); err != nil {
		logger.Warnf("Failed to assign oracle actors to groups: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit oracle policies: %v", err)
	}
	txCommitted = true

	logger.Infof("Successfully created %d Oracle policies for job %s (super+action-wide+object-specific)",
		totalPolicies, jobID)

	// Call exportDBFPolicy to build rule files after policy insertion completed (run in background)
	logger.Infof("Starting background exportDBFPolicy to build Oracle rule files for job %s", jobID)
	go func(jID string) {
		if err := utils.ExportDBFPolicy(); err != nil {
			logger.Warnf("Failed to export DBF policy rules for Oracle job %s: %v", jID, err)
		} else {
			logger.Infof("Successfully exported DBF policy rules for Oracle job %s", jID)
		}
	}(jobID)

	return totalPolicies, nil
}

// classifyOraclePolicyTemplates categorizes Oracle policy templates into three execution tiers.
// Uses database_type_id = 3 for Oracle policies.
func classifyOraclePolicyTemplates(allPolicies map[uint]models.DBPolicyDefault) *policyClassification {
	classification := &policyClassification{
		superPrivileges:     []models.DBPolicyDefault{},
		actionWidePrivs:     []models.DBPolicyDefault{},
		objectSpecificPrivs: []models.DBPolicyDefault{},
	}

	// Oracle database_type_id = 3
	const oracleDatabaseTypeID = uint(3)

	groupListPolicyRepo := repository.NewDBGroupListPoliciesRepository()
	groupListPolicies, err := groupListPolicyRepo.GetActiveByDatabaseType(nil, oracleDatabaseTypeID)
	if err != nil {
		logger.Warnf("Failed to load Oracle DBGroupListPolicies: %v", err)
		groupListPolicies = []models.DBGroupListPolicies{}
	}

	logger.Debugf("Loaded %d active Oracle group list policies from database", len(groupListPolicies))

	groupBPolicyIDs := make(map[uint]bool)
	for _, glp := range groupListPolicies {
		if glp.DBPolicyDefaultID == nil || *glp.DBPolicyDefaultID == "" {
			continue
		}

		rawValue := *glp.DBPolicyDefaultID
		var ids []uint

		// Try parsing as JSON array first: [1,2,3]
		if err := json.Unmarshal([]byte(rawValue), &ids); err == nil {
			for _, id := range ids {
				groupBPolicyIDs[id] = true
			}
			continue
		}

		// Try parsing as single number: 123
		var singleID uint
		if err := json.Unmarshal([]byte(rawValue), &singleID); err == nil {
			groupBPolicyIDs[singleID] = true
			continue
		}

		// Try parsing as comma-separated string: "1,2,3"
		parts := strings.Split(rawValue, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			var id uint
			if _, err := fmt.Sscanf(part, "%d", &id); err == nil {
				groupBPolicyIDs[id] = true
			}
		}
	}

	logger.Infof("Total Oracle action-wide policy IDs from DBGroupListPolicies: %d", len(groupBPolicyIDs))

	// Oracle super privilege policy ID = 1001 (MySQL uses 1)
	const oracleSuperPrivilegePolicyID = uint(1001)

	// Filter policies - only include those that query Oracle privilege tables
	for id, policy := range allPolicies {
		// Check if policy has Oracle-related SQL (queries DBA_*, V_PWFILE_USERS, etc.)
		if !isOraclePolicyTemplate(policy) {
			continue
		}

		switch {
		case id == oracleSuperPrivilegePolicyID:
			classification.superPrivileges = append(classification.superPrivileges, policy)
		case groupBPolicyIDs[id]:
			classification.actionWidePrivs = append(classification.actionWidePrivs, policy)
		default:
			classification.objectSpecificPrivs = append(classification.objectSpecificPrivs, policy)
		}
	}

	logger.Debugf("Oracle policy classification: super=%d, action-wide=%d, object-specific=%d",
		len(classification.superPrivileges), len(classification.actionWidePrivs), len(classification.objectSpecificPrivs))

	return classification
}

// isOraclePolicyTemplate checks if a policy template contains Oracle-specific SQL.
func isOraclePolicyTemplate(policy models.DBPolicyDefault) bool {
	if policy.SqlGet == "" {
		return false
	}

	// Decode hex SQL
	sqlBytes, err := hex.DecodeString(policy.SqlGet)
	if err != nil {
		return false
	}

	sqlUpper := strings.ToUpper(string(sqlBytes))

	// Check for Oracle privilege table references
	oracleTables := []string{
		"DBA_SYS_PRIVS",
		"DBA_TAB_PRIVS",
		"DBA_ROLE_PRIVS",
		"V$PWFILE_USERS",
		"V_PWFILE_USERS",
		"CDB_SYS_PRIVS",
	}

	for _, table := range oracleTables {
		if strings.Contains(sqlUpper, table) {
			return true
		}
	}

	return false
}

// processOracleSQLTemplatesForSession builds SQL queries from Oracle policy templates.
// Applies variable substitution for Oracle-specific context.
// Only processes queries with objectId = -1 (wildcard).
func processOracleSQLTemplatesForSession(
	tx *gorm.DB,
	policies []models.DBPolicyDefault,
	dbMgts []models.DBMgt,
	actors []models.DBActorMgt,
	cmt *models.CntMgt,
	service privilege.PolicyEvaluator,
	connType OracleConnectionType,
) (map[string]policyInput, error) {
	queries := make(map[string]policyInput)

	for _, policy := range policies {
		if policy.SqlGet == "" {
			continue
		}

		// Decode hex SQL
		sqlBytes, err := hex.DecodeString(policy.SqlGet)
		if err != nil {
			logger.Warnf("Failed to decode SqlGet for oracle policy %d: %v", policy.ID, err)
			continue
		}

		rawSQL := string(sqlBytes)

		// Note: V$ → V_ replacement handled by ExecuteOracleTemplate → ReplaceOracleDollarViews

		// Oracle CDB/PDB scope substitution: CDB→"*", PDB→"PDB"
		scopeValue := GetObjectTypeWildcard(connType)

		// Build queries for each actor
		for _, actor := range actors {
			// Apply variable substitution
			finalSQL := rawSQL
			finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", actor.DBUser)
			finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", actor.IPAddress)
			finalSQL = strings.ReplaceAll(finalSQL, "${scope}", scopeValue)

			// Handle object type wildcard based on connection type
			objectTypeWildcard := GetObjectTypeWildcard(connType)
			finalSQL = strings.ReplaceAll(finalSQL, "${dbobject.objecttype}", objectTypeWildcard)

			// For each database/schema
			for _, dbMgt := range dbMgts {
				dbSQL := strings.ReplaceAll(finalSQL, "${dbmgt.dbname}", dbMgt.DbName)

				uniqueKey := fmt.Sprintf("oracle_policy_%d_actor_%d_db_%d", policy.ID, actor.ID, dbMgt.ID)
				queries[uniqueKey] = policyInput{
					policydf: policy,
					actorId:  actor.ID,
					objectId: -1, // Wildcard - general queries
					dbmgtId:  int(dbMgt.ID),
					finalSQL: dbSQL,
				}
			}
		}
	}

	logger.Infof("Built %d Oracle SQL queries from %d policy templates", len(queries), len(policies))
	return queries, nil
}

// processOracleSpecificSQLTemplatesForSession builds SQL queries for specific objects (objectId != -1).
// Uses SqlGetSpecific template and substitutes ${dbobjectmgt.objectname} with actual object names.
func processOracleSpecificSQLTemplatesForSession(
	tx *gorm.DB,
	policies []models.DBPolicyDefault,
	dbMgts []models.DBMgt,
	actors []models.DBActorMgt,
	cmt *models.CntMgt,
	service privilege.PolicyEvaluator,
	connType OracleConnectionType,
	cache *oracleQueryBuildCache,
) (map[string]policyInput, error) {
	queries := make(map[string]policyInput)

	for _, policy := range policies {
		if policy.SqlGetSpecific == "" {
			continue
		}

		// Decode hex SQL from SqlGetSpecific
		sqlBytes, err := hex.DecodeString(policy.SqlGetSpecific)
		if err != nil {
			logger.Warnf("Failed to decode SqlGetSpecific for oracle policy %d: %v", policy.ID, err)
			continue
		}

		rawSQL := string(sqlBytes)

		// Note: V$ → V_ replacement handled by ExecuteOracleTemplate → ReplaceOracleDollarViews

		// Oracle CDB/PDB scope substitution: CDB→"*", PDB→"PDB"
		scopeValue := GetObjectTypeWildcard(connType)

		hasDatabaseVar := strings.Contains(rawSQL, "${dbmgt.dbname}")
		hasObjectVar := strings.Contains(rawSQL, "${dbobjectmgt.objectname}")

		// Build queries for each actor
		for _, actor := range actors {
			if hasDatabaseVar {
				for _, dbMgt := range dbMgts {
					finalSQL := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbMgt.DbName)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", actor.DBUser)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", actor.IPAddress)
					finalSQL = strings.ReplaceAll(finalSQL, "${scope}", scopeValue)

					// Handle object type wildcard based on connection type
					objectTypeWildcard := GetObjectTypeWildcard(connType)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbobject.objecttype}", objectTypeWildcard)

					if hasObjectVar {
						// Lookup objects from cache by policy.ObjectId and dbMgt.ID
						key := fmt.Sprintf("%d:%d", policy.ObjectId, dbMgt.ID)
						dbObjectMgts := cache.objectsByKey[key]
						if len(dbObjectMgts) == 0 {
							continue
						}

						for _, object := range dbObjectMgts {
							finalSqlObject := strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", object.ObjectName)

							uniqueKey := fmt.Sprintf("oracle_specific_policy_%d_actor_%d_db_%d_object_%d",
								policy.ID, actor.ID, dbMgt.ID, object.ID)

							queries[uniqueKey] = policyInput{
								policydf: policy,
								actorId:  actor.ID,
								objectId: int(object.ID),
								dbmgtId:  int(dbMgt.ID),
								finalSQL: finalSqlObject,
							}
						}
					} else {
						uniqueKey := fmt.Sprintf("oracle_specific_policy_%d_actor_%d_db_%d",
							policy.ID, actor.ID, dbMgt.ID)

						queries[uniqueKey] = policyInput{
							policydf: policy,
							actorId:  actor.ID,
							objectId: -1,
							dbmgtId:  int(dbMgt.ID),
							finalSQL: finalSQL,
						}
					}
				}
			} else {
				finalSQL := rawSQL
				finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", actor.DBUser)
				finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", actor.IPAddress)
				finalSQL = strings.ReplaceAll(finalSQL, "${scope}", scopeValue)

				// Handle object type wildcard based on connection type
				objectTypeWildcard := GetObjectTypeWildcard(connType)
				finalSQL = strings.ReplaceAll(finalSQL, "${dbobject.objecttype}", objectTypeWildcard)

				if hasObjectVar {
					for _, dbMgt := range dbMgts {
						// Lookup objects from cache
						key := fmt.Sprintf("%d:%d", policy.ObjectId, dbMgt.ID)
						dbObjectMgts := cache.objectsByKey[key]
						if len(dbObjectMgts) == 0 {
							continue
						}

						for _, object := range dbObjectMgts {
							finalSqlObject := strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", object.ObjectName)

							uniqueKey := fmt.Sprintf("oracle_specific_policy_%d_actor_%d_object_%d",
								policy.ID, actor.ID, object.ID)

							queries[uniqueKey] = policyInput{
								policydf: policy,
								actorId:  actor.ID,
								objectId: int(object.ID),
								dbmgtId:  -1,
								finalSQL: finalSqlObject,
							}
						}
					}
				} else {
					uniqueKey := fmt.Sprintf("oracle_specific_policy_%d_actor_%d", policy.ID, actor.ID)

					queries[uniqueKey] = policyInput{
						policydf: policy,
						actorId:  actor.ID,
						objectId: -1,
						dbmgtId:  -1,
						finalSQL: finalSQL,
					}
				}
			}
		}
	}

	logger.Infof("Built %d Oracle specific SQL queries from %d policy templates", len(queries), len(policies))
	return queries, nil
}

// oracleQueryBuildCache caches Oracle database objects for efficient query building.
// Prevents N+1 queries when building object-specific queries.
type oracleQueryBuildCache struct {
	objectsByKey     map[string][]models.DBObjectMgt // Key: "objectId:dbMgtId"
	allActorsByCntID map[uint][]models.DBActorMgt    // Key: cnt_id
}

// newOracleQueryBuildCache creates and populates cache for Oracle query building.
func newOracleQueryBuildCache(tx *gorm.DB, cntMgtID uint, dbMgts []models.DBMgt, policies []models.DBPolicyDefault) *oracleQueryBuildCache {
	cache := &oracleQueryBuildCache{
		objectsByKey:     make(map[string][]models.DBObjectMgt),
		allActorsByCntID: make(map[uint][]models.DBActorMgt),
	}

	// Collect unique object IDs from policies
	objectIDs := make(map[uint]bool)
	for _, policy := range policies {
		if policy.ObjectId > 0 && policy.SqlGetSpecific != "" {
			objectIDs[uint(policy.ObjectId)] = true
		}
	}

	if len(objectIDs) == 0 {
		return cache
	}

	// Load all DBObjectMgt records for these object types and databases
	dbMgtIDs := make([]uint, len(dbMgts))
	for i, dbMgt := range dbMgts {
		dbMgtIDs[i] = dbMgt.ID
	}

	objectIDList := make([]uint, 0, len(objectIDs))
	for id := range objectIDs {
		objectIDList = append(objectIDList, id)
	}

	var allObjects []models.DBObjectMgt
	if err := tx.Where("dbmgt_id IN ? AND dbobject_id IN ?", dbMgtIDs, objectIDList).Find(&allObjects).Error; err != nil {
		logger.Warnf("Failed to load Oracle DBObjectMgt records for cache: %v", err)
		return cache
	}

	// Index by objectId:dbMgtId
	for _, obj := range allObjects {
		key := fmt.Sprintf("%d:%d", obj.ObjectId, obj.DBMgt)
		cache.objectsByKey[key] = append(cache.objectsByKey[key], obj)
	}

	// Load actors for actor-specific object queries (similar to MySQL ObjectId=12)
	if len(dbMgts) > 0 {
		var actors []models.DBActorMgt
		if err := tx.Where("cnt_id = ?", cntMgtID).Find(&actors).Error; err != nil {
			logger.Warnf("Failed to load Oracle actors for cache: %v", err)
		} else {
			cache.allActorsByCntID[cntMgtID] = actors
		}
	}

	logger.Debugf("Oracle query cache built: %d object keys, %d actors", len(cache.objectsByKey), len(cache.allActorsByCntID[cntMgtID]))
	return cache
}

// executeOracleSuperPrivilegeQueries executes Pass 1 super privilege queries concurrently.
// Uses goroutines + semaphore for concurrent query execution like MySQL.
// Logging is done after collecting results to avoid I/O contention in goroutines.
func executeOracleSuperPrivilegeQueries(
	tx *gorm.DB,
	session *PrivilegeSession,
	queries map[string]policyInput,
	service privilege.PolicyEvaluator,
	cntMgtID uint,
	cmt *models.CntMgt,
	allowedResults *allowedPolicyResults,
	superPrivActors *superPrivilegeActors,
	logFile *os.File,
) int {
	type queryResult struct {
		uniqueKey  string
		policyData policyInput
		finalSQL   string
		results    []map[string]interface{}
		err        error
	}

	// Execute all queries concurrently
	maxConcurrent := config.GetPrivilegeQueryConcurrency()
	logger.Debugf("Using privilege query concurrency: %d for %d Oracle super privilege queries", maxConcurrent, len(queries))
	semaphore := make(chan struct{}, maxConcurrent)
	resultsChan := make(chan queryResult, len(queries))

	queryCount := 0
	for uniqueKey, policyData := range queries {
		queryCount++
		go func(key string, input policyInput) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in executeOracleSuperPrivilegeQueries goroutine for key %s: %v", key, r)
					resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, err: fmt.Errorf("panic: %v", r)}
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			results, err := session.ExecuteOracleTemplate(input.finalSQL, nil)
			if err != nil {
				resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, err: err}
				return
			}

			resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, results: results, err: nil}
		}(uniqueKey, policyData)
	}

	// Collect all query results
	allResults := make([]queryResult, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		result := <-resultsChan
		writeOracleQueryToLogFile(logFile, "PASS-1-SUPER", result.uniqueKey, result.finalSQL)
		if result.err != nil {
			logger.Debugf("Oracle super privilege query failed for key=%s: %v", result.uniqueKey, result.err)
			continue
		}
		allResults = append(allResults, result)
	}

	// Process results serially to create policies (database operations must be serial)
	policiesCreated := 0
	for _, result := range allResults {
		if !isOraclePolicyAllowed(result.results, result.policyData.policydf) {
			continue
		}

		// Record allowed policy for actor-to-group assignment
		allowedResults.recordAllowed(result.policyData.actorId, result.policyData.policydf.ID)

		// Create policy record
		if err := createOraclePolicyRecord(tx, cntMgtID, result.policyData); err != nil {
			logger.Warnf("Failed to create oracle super policy: %v", err)
			continue
		}
		policiesCreated++

		// Mark actor as having super privileges AFTER successful policy creation
		// to ensure skip logic in PASS-2 and PASS-3 only applies to actors with actual policies
		superPrivActors.markSuperPrivilege(result.policyData.actorId)

		logger.Debugf("Created Oracle super policy: actor_id=%d marked with super privileges", result.policyData.actorId)
	}

	logger.Infof("Oracle PASS-1 completed: %d actors marked with super privileges", len(superPrivActors.actors))
	return policiesCreated
}

// executeOracleActionWideQueries executes Pass 2 action-wide queries concurrently.
// Uses goroutines + semaphore for concurrent query execution like MySQL.
// Logging is done after collecting results to avoid I/O contention in goroutines.
func executeOracleActionWideQueries(
	tx *gorm.DB,
	session *PrivilegeSession,
	queries map[string]policyInput,
	service privilege.PolicyEvaluator,
	cntMgtID uint,
	cmt *models.CntMgt,
	grantedActions *grantedActionsCache,
	allowedResults *allowedPolicyResults,
	superPrivActors *superPrivilegeActors,
	logFile *os.File,
) int {
	type queryResult struct {
		uniqueKey  string
		policyData policyInput
		finalSQL   string
		results    []map[string]interface{}
		err        error
	}

	// Pre-filter queries to skip super privilege actors
	filteredQueries := make(map[string]policyInput)
	for uniqueKey, input := range queries {
		if superPrivActors.hasSuperPrivilege(input.actorId) {
			continue
		}
		filteredQueries[uniqueKey] = input
	}

	if len(filteredQueries) == 0 {
		return 0
	}

	// Execute filtered queries concurrently
	maxConcurrent := config.GetPrivilegeQueryConcurrency()
	logger.Debugf("Using privilege query concurrency: %d for %d Oracle action-wide queries (filtered from %d)",
		maxConcurrent, len(filteredQueries), len(queries))
	semaphore := make(chan struct{}, maxConcurrent)
	resultsChan := make(chan queryResult, len(filteredQueries))

	queryCount := 0
	for uniqueKey, policyData := range filteredQueries {
		queryCount++
		go func(key string, input policyInput) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in executeOracleActionWideQueries goroutine for key %s: %v", key, r)
					resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, err: fmt.Errorf("panic: %v", r)}
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			results, err := session.ExecuteOracleTemplate(input.finalSQL, nil)
			if err != nil {
				resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, err: err}
				return
			}

			resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, results: results, err: nil}
		}(uniqueKey, policyData)
	}

	// Collect all query results
	allResults := make([]queryResult, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		result := <-resultsChan
		writeOracleQueryToLogFile(logFile, "PASS-2-ACTION", result.uniqueKey, result.finalSQL)
		if result.err != nil {
			logger.Debugf("Oracle action-wide query failed for key=%s: %v", result.uniqueKey, result.err)
			continue
		}
		allResults = append(allResults, result)
	}

	// Process results serially to create policies (database operations must be serial)
	policiesCreated := 0
	for _, result := range allResults {
		if !isOraclePolicyAllowed(result.results, result.policyData.policydf) {
			continue
		}

		// Record allowed policy for actor-to-group assignment
		allowedResults.recordAllowed(result.policyData.actorId, result.policyData.policydf.ID)
		grantedActions.markGranted(result.policyData.actorId, result.policyData.policydf.ActionId)

		// Create policy record
		if err := createOraclePolicyRecord(tx, cntMgtID, result.policyData); err != nil {
			logger.Warnf("Failed to create oracle action-wide policy: %v", err)
			continue
		}
		policiesCreated++
	}

	return policiesCreated
}

// executeOracleObjectSpecificQueries executes Pass 3 object-specific queries concurrently.
// Skips queries if actor has super privileges or action already granted in Pass 2.
// Uses goroutines + semaphore for concurrent query execution like MySQL.
// Logging is done after collecting results to avoid I/O contention in goroutines.
func executeOracleObjectSpecificQueries(
	tx *gorm.DB,
	session *PrivilegeSession,
	queries map[string]policyInput,
	service privilege.PolicyEvaluator,
	cntMgtID uint,
	grantedActions *grantedActionsCache,
	allowedResults *allowedPolicyResults,
	superPrivActors *superPrivilegeActors,
	logFile *os.File,
) int {
	type queryResult struct {
		uniqueKey  string
		policyData policyInput
		finalSQL   string
		results    []map[string]interface{}
		err        error
	}

	// Pre-filter queries to skip super privilege actors and already-granted actions
	filteredQueries := make(map[string]policyInput)
	for uniqueKey, input := range queries {
		if superPrivActors.hasSuperPrivilege(input.actorId) {
			continue
		}
		if grantedActions.isGranted(input.actorId, input.policydf.ActionId) {
			continue
		}
		filteredQueries[uniqueKey] = input
	}

	if len(filteredQueries) == 0 {
		return 0
	}

	// Execute filtered queries concurrently
	maxConcurrent := config.GetPrivilegeQueryConcurrency()
	logger.Debugf("Using privilege query concurrency: %d for %d Oracle object-specific queries (filtered from %d)",
		maxConcurrent, len(filteredQueries), len(queries))
	semaphore := make(chan struct{}, maxConcurrent)
	resultsChan := make(chan queryResult, len(filteredQueries))

	queryCount := 0
	for uniqueKey, policyData := range filteredQueries {
		queryCount++
		go func(key string, input policyInput) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in executeOracleObjectSpecificQueries goroutine for key %s: %v", key, r)
					resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, err: fmt.Errorf("panic: %v", r)}
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			results, err := session.ExecuteOracleTemplate(input.finalSQL, nil)
			if err != nil {
				resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, err: err}
				return
			}

			resultsChan <- queryResult{uniqueKey: key, policyData: input, finalSQL: input.finalSQL, results: results, err: nil}
		}(uniqueKey, policyData)
	}

	// Collect all query results
	allResults := make([]queryResult, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		result := <-resultsChan
		writeOracleQueryToLogFile(logFile, "PASS-3-OBJECT", result.uniqueKey, result.finalSQL)
		if result.err != nil {
			logger.Debugf("Oracle object-specific query failed for key=%s: %v", result.uniqueKey, result.err)
			continue
		}
		allResults = append(allResults, result)
	}

	// Process results serially to create policies (database operations must be serial)
	policiesCreated := 0
	for _, result := range allResults {
		if !isOraclePolicyAllowed(result.results, result.policyData.policydf) {
			continue
		}

		// Record allowed policy for actor-to-group assignment
		allowedResults.recordAllowed(result.policyData.actorId, result.policyData.policydf.ID)

		// Create policy record
		if err := createOraclePolicyRecord(tx, cntMgtID, result.policyData); err != nil {
			logger.Warnf("Failed to create oracle object-specific policy: %v", err)
			continue
		}
		policiesCreated++
	}

	return policiesCreated
}

// isOraclePolicyAllowed checks if query result matches allow criteria.
// SqlGetAllow/SqlGetDeny are plain strings (e.g., "Y", "N", "1", "NOT NULL"), NOT hex-encoded.
// Matches MySQL isPolicyAllowed pattern - only SqlGet/SqlGetSpecific are hex-encoded.
func isOraclePolicyAllowed(results []map[string]interface{}, policy models.DBPolicyDefault) bool {
	if len(results) == 0 {
		return false
	}

	resAllow := policy.SqlGetAllow
	resDeny := policy.SqlGetDeny

	// Get first value from result
	for _, row := range results {
		for _, val := range row {
			output := fmt.Sprintf("%v", val)

			if output == resDeny {
				return false
			}
			if output == resAllow {
				return true
			}
			if resAllow == "NOT NULL" && output != "NULL" && output != "" {
				return true
			}
		}
		break // Only check first row
	}

	return false
}

// createOraclePolicyRecord creates a DBPolicy record for Oracle.
func createOraclePolicyRecord(tx *gorm.DB, cntMgtID uint, input policyInput) error {
	// Check for existing policy
	var existing models.DBPolicy
	err := tx.Where("cnt_id = ? AND actor_id = ? AND dbpolicydefault_id = ? AND dbmgt_id = ? AND object_id = ?",
		cntMgtID, input.actorId, input.policydf.ID, input.dbmgtId, input.objectId).First(&existing).Error
	if err == nil {
		// Already exists
		return nil
	}

	policy := models.DBPolicy{
		CntMgt:          cntMgtID,
		DBPolicyDefault: input.policydf.ID,
		DBMgt:           input.dbmgtId,
		DBActorMgt:      input.actorId,
		DBObjectMgt:     input.objectId,
		Status:          "enabled",
		Description:     "Auto-collected by V2-DBF Agent",
	}

	if err := tx.Create(&policy).Error; err != nil {
		return fmt.Errorf("failed to create oracle policy: %w", err)
	}

	return nil
}

// writeOracleQueryToLogFile appends Oracle query to log file.
func writeOracleQueryToLogFile(logFile *os.File, passName, uniqueKey, query string) {
	if logFile == nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logEntry := fmt.Sprintf("[%s] [%s] [%s]\n%s\n\n", timestamp, passName, uniqueKey, query)

	if _, err := logFile.WriteString(logEntry); err != nil {
		logger.Warnf("Failed to write oracle query to log file: %v", err)
	}
}

// parseOraclePrivilegeResults converts query results into structured Oracle privilege data.
// Maps query keys to appropriate privilege structs based on Oracle system table names.
func parseOraclePrivilegeResults(results []privilege.QueryResult, connType OracleConnectionType) (*OraclePrivilegeData, error) {
	data := &OraclePrivilegeData{
		SysPrivs:    []OracleSysPriv{},
		TabPrivs:    []OracleTabPriv{},
		RolePrivs:   []OracleRolePriv{},
		PwFileUsers: []OraclePwFileUser{},
		CdbSysPrivs: []OracleCdbSysPriv{},
	}

	for _, result := range results {
		if result.Status != "success" {
			logger.Warnf("Oracle query failed for %s: status=%s", result.QueryKey, result.Status)
			continue
		}

		// Strip array index from query key (e.g., "dba_sys_privs[0]" -> "dba_sys_privs")
		queryKey := result.QueryKey
		if idx := strings.Index(queryKey, "["); idx != -1 {
			queryKey = queryKey[:idx]
		}

		switch queryKey {
		case "dba_sys_privs":
			privs, err := parseDbasSysPrivs(result.Result)
			if err != nil {
				logger.Warnf("Failed to parse dba_sys_privs: %v", err)
				continue
			}
			data.SysPrivs = append(data.SysPrivs, privs...)

		case "dba_tab_privs":
			privs, err := parseDbasTabPrivs(result.Result)
			if err != nil {
				logger.Warnf("Failed to parse dba_tab_privs: %v", err)
				continue
			}
			data.TabPrivs = append(data.TabPrivs, privs...)

		case "dba_role_privs":
			privs, err := parseDbasRolePrivs(result.Result)
			if err != nil {
				logger.Warnf("Failed to parse dba_role_privs: %v", err)
				continue
			}
			data.RolePrivs = append(data.RolePrivs, privs...)

		case "v$pwfile_users":
			users, err := parsePwFileUsers(result.Result)
			if err != nil {
				logger.Warnf("Failed to parse v$pwfile_users: %v", err)
				continue
			}
			data.PwFileUsers = append(data.PwFileUsers, users...)

		case "cdb_sys_privs":
			if connType == OracleConnectionCDB {
				privs, err := parseCdbSysPrivs(result.Result)
				if err != nil {
					logger.Warnf("Failed to parse cdb_sys_privs: %v", err)
					continue
				}
				data.CdbSysPrivs = append(data.CdbSysPrivs, privs...)
			}

		default:
			// Object queries (all_tables, all_views, etc.) - log but don't parse yet
			logger.Debugf("Oracle object query result: %s with %d rows", queryKey, len(result.Result))
		}
	}

	return data, nil
}

// parseDbasSysPrivs parses DBA_SYS_PRIVS query results into OracleSysPriv structs.
func parseDbasSysPrivs(rows [][]interface{}) ([]OracleSysPriv, error) {
	privs := make([]OracleSysPriv, 0, len(rows))

	columns, err := GetOraclePrivilegeColumnNames("dba_sys_privs")
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if len(row) < len(columns) {
			continue
		}

		priv := OracleSysPriv{
			Grantee:     safeString(row[0]),
			Privilege:   safeString(row[1]),
			AdminOption: safeString(row[2]),
			Common:      safeString(row[3]),
			Inherited:   safeString(row[4]),
		}
		privs = append(privs, priv)
	}

	return privs, nil
}

// parseDbasTabPrivs parses DBA_TAB_PRIVS query results into OracleTabPriv structs.
func parseDbasTabPrivs(rows [][]interface{}) ([]OracleTabPriv, error) {
	privs := make([]OracleTabPriv, 0, len(rows))

	columns, err := GetOraclePrivilegeColumnNames("dba_tab_privs")
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if len(row) < len(columns) {
			continue
		}

		priv := OracleTabPriv{
			Grantee:   safeString(row[0]),
			Owner:     safeString(row[1]),
			TableName: safeString(row[2]),
			Grantor:   safeString(row[3]),
			Privilege: safeString(row[4]),
			Grantable: safeString(row[5]),
			Hierarchy: safeString(row[6]),
			Common:    safeString(row[7]),
			Type:      safeString(row[8]),
			Inherited: safeString(row[9]),
		}
		privs = append(privs, priv)
	}

	return privs, nil
}

// parseDbasRolePrivs parses DBA_ROLE_PRIVS query results into OracleRolePriv structs.
func parseDbasRolePrivs(rows [][]interface{}) ([]OracleRolePriv, error) {
	privs := make([]OracleRolePriv, 0, len(rows))

	columns, err := GetOraclePrivilegeColumnNames("dba_role_privs")
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if len(row) < len(columns) {
			continue
		}

		priv := OracleRolePriv{
			Grantee:     safeString(row[0]),
			GrantedRole: safeString(row[1]),
			AdminOption: safeString(row[2]),
			DelegateOpt: safeString(row[3]),
			DefaultRole: safeString(row[4]),
			Common:      safeString(row[5]),
			Inherited:   safeString(row[6]),
		}
		privs = append(privs, priv)
	}

	return privs, nil
}

// parsePwFileUsers parses V$PWFILE_USERS query results into OraclePwFileUser structs.
func parsePwFileUsers(rows [][]interface{}) ([]OraclePwFileUser, error) {
	users := make([]OraclePwFileUser, 0, len(rows))

	columns, err := GetOraclePrivilegeColumnNames("v$pwfile_users")
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if len(row) < len(columns) {
			continue
		}

		user := OraclePwFileUser{
			Username:  safeString(row[0]),
			Sysdba:    safeString(row[1]),
			Sysoper:   safeString(row[2]),
			Sysasm:    safeString(row[3]),
			Sysbackup: safeString(row[4]),
			Sysdg:     safeString(row[5]),
			Syskm:     safeString(row[6]),
		}
		users = append(users, user)
	}

	return users, nil
}

// parseCdbSysPrivs parses CDB_SYS_PRIVS query results into OracleCdbSysPriv structs.
func parseCdbSysPrivs(rows [][]interface{}) ([]OracleCdbSysPriv, error) {
	privs := make([]OracleCdbSysPriv, 0, len(rows))

	columns, err := GetOraclePrivilegeColumnNames("cdb_sys_privs")
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if len(row) < len(columns) {
			continue
		}

		priv := OracleCdbSysPriv{
			Grantee:     safeString(row[0]),
			Privilege:   safeString(row[1]),
			AdminOption: safeString(row[2]),
			Common:      safeString(row[3]),
			Inherited:   safeString(row[4]),
			ConID:       safeInt(row[5]),
		}
		privs = append(privs, priv)
	}

	return privs, nil
}

// safeString converts interface{} to string safely.
func safeString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// safeInt converts interface{} to int safely.
func safeInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	default:
		return 0
	}
}

// collectExactMatchGroups finds group list policies where actor has all required policy defaults.
func collectExactMatchGroups(actorPolicies map[uint]bool, groupListPolicies []groupListPolicy) []uint {
	var matchedIDs []uint
	for _, glp := range groupListPolicies {
		if isExactMatch(actorPolicies, glp.policyDefaultIDs) {
			matchedIDs = append(matchedIDs, glp.listPolicyID)
		}
	}
	return matchedIDs
}

// isExactMatch checks if actor's policies are a superset of the group's required policies.
func isExactMatch(actorPolicies map[uint]bool, groupPolicies map[uint]bool) bool {
	for requiredID := range groupPolicies {
		if !actorPolicies[requiredID] {
			return false
		}
	}
	return true
}

// getPolicyIDsAsSlice converts policy ID map to sorted slice for logging.
func getPolicyIDsAsSlice(policyMap map[uint]bool) []uint {
	result := make([]uint, 0, len(policyMap))
	for id := range policyMap {
		result = append(result, id)
	}
	return result
}

// assignOracleActorsToGroups assigns Oracle actors to groups based on privilege evaluation results.
// Oracle uses superPrivGroupID = 1000 (different from MySQL's group 1).
// Uses same logic as MySQL's assignActorsToGroups but with Oracle-specific group ID.
func assignOracleActorsToGroups(tx *gorm.DB, cntMgtID uint, cmt *models.CntMgt, allowedResults *allowedPolicyResults, superPrivActors *superPrivilegeActors) error {
	logger.Infof("Assigning Oracle actors to groups for cnt_id=%d", cntMgtID)

	// Oracle database_type_id = 3
	const oracleDatabaseTypeID = uint(3)

	// Use direct query results from PASS 1, 2, 3 execution
	actorPolicies := allowedResults.actorPolicies

	logger.Debugf("Found %d Oracle actors with allowed policies from query evaluation", len(actorPolicies))

	// Load active Oracle dbgroup_listpolicies for matching
	groupListPolicyRepo := repository.NewDBGroupListPoliciesRepository()
	allGroupListPolicies, err := groupListPolicyRepo.GetActiveByDatabaseType(tx, oracleDatabaseTypeID)
	if err != nil {
		logger.Warnf("Failed to load Oracle DBGroupListPolicies for database_type_id=%d: %v", oracleDatabaseTypeID, err)
		return nil
	}

	// Parse dbgroup_listpolicies to extract policy_default_ids
	groupListPoliciesData := []groupListPolicy{}
	for _, glp := range allGroupListPolicies {
		if glp.DBPolicyDefaultID == nil || *glp.DBPolicyDefaultID == "" {
			continue
		}

		rawValue := *glp.DBPolicyDefaultID
		policyIDs := make(map[uint]bool)

		// Try JSON array: [1,2,3]
		var ids []uint
		if err := json.Unmarshal([]byte(rawValue), &ids); err == nil {
			for _, id := range ids {
				policyIDs[id] = true
			}
		} else {
			// Try single number: 123
			var singleID uint
			if err := json.Unmarshal([]byte(rawValue), &singleID); err == nil {
				policyIDs[singleID] = true
			} else {
				// Try comma-separated: "1,2,3"
				parts := strings.Split(rawValue, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					var id uint
					if _, err := fmt.Sscanf(part, "%d", &id); err == nil {
						policyIDs[id] = true
					}
				}
			}
		}

		if len(policyIDs) > 0 {
			groupListPoliciesData = append(groupListPoliciesData, groupListPolicy{
				listPolicyID:     glp.ID,
				policyDefaultIDs: policyIDs,
			})
		}
	}

	actorGroupsRepo := repository.NewDBActorGroupsRepository()

	// Build group → required listpolicy IDs map from dbpolicy_groups
	type groupRequirement struct {
		groupID              uint
		requiredListPolicies map[uint]bool
	}
	var policyGroupRows []struct {
		GroupID        uint `gorm:"column:group_id"`
		ListPoliciesID uint `gorm:"column:dbgroup_listpolicies_id"`
	}
	err = tx.Table("dbpolicy_groups pg").
		Select("pg.group_id, pg.dbgroup_listpolicies_id").
		Joins("INNER JOIN dbgroupmgt g ON pg.group_id = g.id").
		Where("pg.is_active = ? AND g.is_active = ? AND g.database_type_id = ?",
			true, true, oracleDatabaseTypeID).
		Order("pg.group_id ASC").
		Find(&policyGroupRows).Error
	if err != nil {
		logger.Warnf("Failed to load Oracle dbpolicy_groups: %v", err)
		return nil
	}

	groupRequirements := make(map[uint]*groupRequirement)
	for _, row := range policyGroupRows {
		gr, exists := groupRequirements[row.GroupID]
		if !exists {
			gr = &groupRequirement{
				groupID:              row.GroupID,
				requiredListPolicies: make(map[uint]bool),
			}
			groupRequirements[row.GroupID] = gr
		}
		gr.requiredListPolicies[row.ListPoliciesID] = true
	}

	logger.Debugf("Loaded %d Oracle groups with requirements from dbpolicy_groups", len(groupRequirements))

	// Oracle superPrivGroupID = 1000 (different from MySQL's group 1)
	const superPrivGroupID = uint(1000)

	assignedCount := 0
actorLoop:
	for actorID, actorPolicyIDs := range actorPolicies {
		if len(actorPolicyIDs) == 0 {
			logger.Debugf("Skipping Oracle actor %d - no policies", actorID)
			continue
		}

		// Super privilege actors → direct assign to group 1000 for Oracle.
		// Detect via policy_default_id=1001 in allowedResults (reliable on re-runs),
		// OR via superPrivActors (set during initial PASS-1 policy creation).
		// Oracle uses policy_default_id=1001 for super privileges (MySQL uses 1).
		const oracleSuperPrivilegePolicyID = uint(1001)
		isSuperPriv := actorPolicyIDs[oracleSuperPrivilegePolicyID] || (superPrivActors != nil && superPrivActors.hasSuperPrivilege(actorID))
		if isSuperPriv {
			existingGroups, err := actorGroupsRepo.GetActiveGroupsByActorID(tx, actorID)
			if err == nil {
				for _, ag := range existingGroups {
					if ag.GroupID == superPrivGroupID {
						logger.Infof("Oracle super privilege actor %d already assigned to group %d", actorID, superPrivGroupID)
						continue actorLoop
					}
				}
			}

			actorGroup := &models.DBActorGroups{
				ActorID:   actorID,
				GroupID:   superPrivGroupID,
				ValidFrom: time.Now(),
				IsActive:  true,
			}
			if err := actorGroupsRepo.Create(tx, actorGroup); err != nil {
				logger.Warnf("Failed to assign Oracle super privilege actor %d to group %d: %v", actorID, superPrivGroupID, err)
			} else {
				logger.Infof("Assigned Oracle super privilege actor %d to group %d (PASS-1 super privilege)", actorID, superPrivGroupID)
				assignedCount++
			}
			continue actorLoop
		}

		// Level 1: Find which dbgroup_listpolicies the actor fully satisfies
		satisfiedListPolicyIDs := collectExactMatchGroups(actorPolicyIDs, groupListPoliciesData)

		if len(satisfiedListPolicyIDs) == 0 {
			logger.Warnf("No satisfied listpolicies for Oracle actor %d (policies: %v) - skipping",
				actorID, getPolicyIDsAsSlice(actorPolicyIDs))
			continue
		}

		logger.Debugf("Oracle actor %d: satisfied %d listpolicies: %v",
			actorID, len(satisfiedListPolicyIDs), satisfiedListPolicyIDs)

		// Level 2: Find groups where actor satisfies ALL required listpolicies
		satisfiedSet := make(map[uint]bool, len(satisfiedListPolicyIDs))
		for _, lpID := range satisfiedListPolicyIDs {
			satisfiedSet[lpID] = true
		}

		var candidateGroupIDs []uint
		for gid, gr := range groupRequirements {
			allSatisfied := true
			for requiredLP := range gr.requiredListPolicies {
				if !satisfiedSet[requiredLP] {
					allSatisfied = false
					break
				}
			}
			if allSatisfied {
				candidateGroupIDs = append(candidateGroupIDs, gid)
			}
		}

		if len(candidateGroupIDs) == 0 {
			logger.Debugf("Oracle actor %d does not fully satisfy any group requirements", actorID)
			continue
		}

		// Get existing group assignments
		existingGroups, _ := actorGroupsRepo.GetActiveGroupsByActorID(tx, actorID)
		existingGroupIDs := make(map[uint]bool)
		for _, ag := range existingGroups {
			existingGroupIDs[ag.GroupID] = true
		}

		// Assign to groups not already assigned
		for _, groupID := range candidateGroupIDs {
			if existingGroupIDs[groupID] {
				logger.Debugf("Oracle actor %d already assigned to group %d", actorID, groupID)
				continue
			}

			actorGroup := &models.DBActorGroups{
				ActorID:   actorID,
				GroupID:   groupID,
				ValidFrom: time.Now(),
				IsActive:  true,
			}
			if err := actorGroupsRepo.Create(tx, actorGroup); err != nil {
				logger.Warnf("Failed to assign Oracle actor %d to group %d: %v", actorID, groupID, err)
			} else {
				logger.Infof("Assigned Oracle actor %d to group %d", actorID, groupID)
				assignedCount++
			}
		}
	}

	logger.Infof("Oracle group assignment completed: %d actor-group assignments created", assignedCount)
	return nil
}
