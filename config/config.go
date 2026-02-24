package config

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// AppConfig holds application configuration loaded from environment variables and .env file.
type AppConfig struct {
	// Database config
	DBHost         string
	DBPort         int
	DBUser         string
	DBPass         string
	DBName         string
	VeloClientPath string
	AgentAPIPath   string

	// Logging config
	LogLevel      string
	LogFile       string
	LogMaxSize    int // MB
	LogMaxBackups int
	LogMaxAge     int // days
	LogCompress   bool

	// DBF Web config
	DBFWebTempDir       string
	VeloResultsDir      string
	NotificationFileDir string // Directory for notification-based job result files
	DownloadFileDir     string // Directory containing files available for agent download

	// VeloArtifact timeout and retry config
	VeloExecutionTimeout time.Duration // Timeout for SQL execution commands
	VeloDownloadTimeout  time.Duration // Timeout for file download commands
	VeloMaxRetries       int           // Maximum retry attempts
	VeloRetryBaseDelay   time.Duration // Base delay between retries
	VeloDownloadRetries  int           // Maximum retry attempts for downloads

	// Agent API timeout and retry config
	AgentExecutionTimeout time.Duration // Timeout for agent command execution
	AgentMaxRetries       int           // Maximum retry attempts for agent commands
	AgentRetryBaseDelay   time.Duration // Base delay between agent command retries

	// Database and User Exclusion Lists - configurable system objects to skip during sync
	SystemDatabases []string // System databases that should not be managed
	SystemUsers     []string // System database users that should not be managed

	// Policy Compliance Tool Path
	DBFCheckPolicyCompliancePath string // Path to dbfcheckpolicycompliance executable

	// Concurrency config for privilege session processing
	PrivilegeLoadConcurrency  int // Max concurrent privilege table loads (0 = auto based on CPU)
	PrivilegeQueryConcurrency int // Max concurrent policy queries (0 = auto based on CPU)

	// Performance-critical: MySQL privilege query logging generates large files during bulk analysis
	// Enable only in development for debugging MySQL privilege template execution issues
	EnableMySQLPrivilegeQueryLogging bool
}

// Cfg is the global application configuration instance.
var Cfg AppConfig

// LoadConfig loads and validates application configuration from .env file and environment variables.
func LoadConfig() error {
	// Nếu có file .env thì load
	err := godotenv.Load()
	if err != nil {
		// Use standard log here since logger is not initialized yet
		log.Printf("[WARN] .env file not found or cannot be loaded: %v", err)
	} else {
		log.Printf("[INFO] .env file loaded successfully")
	}

	Cfg.DBHost = getEnv("DB_HOST", "127.0.0.1")
	Cfg.DBUser = getEnv("DB_USER", "root")
	Cfg.DBPass = getEnv("DB_PASS", "")
	Cfg.DBName = getEnv("DB_NAME", "test_db")

	portStr := getEnv("DB_PORT", "3306")
	portInt, _ := strconv.Atoi(portStr)
	Cfg.DBPort = portInt

	Cfg.VeloClientPath = "/usr/local/bin/veloapiclient"
	Cfg.AgentAPIPath = getEnv("AGENT_API_PATH", "/usr/local/bin/dbfAgentAPI")

	// Load logging config
	Cfg.LogLevel = getEnv("LOG_LEVEL", "DEBUG")
	Cfg.LogFile = getEnv("LOG_FILE", "/var/log/dbf/dbfartifactapi.log")

	Cfg.LogMaxSize = getEnvInt("LOG_MAX_SIZE", 10)
	Cfg.LogMaxBackups = getEnvInt("LOG_MAX_BACKUPS", 3)
	Cfg.LogMaxAge = getEnvInt("LOG_MAX_AGE", 28)
	Cfg.LogCompress = getEnvBool("LOG_COMPRESS", true)

	// Load DBF Web config
	Cfg.DBFWebTempDir = getEnv("DBFWEB_TEMP_DIR", "/etc/saids/idsconfig/tmp/dbfweb")
	Cfg.VeloResultsDir = getEnv("VELO_RESULTS_DIR", "/var/log/edr/evident")
	Cfg.NotificationFileDir = getEnv("NOTIFICATION_FILE_DIR", "/etc/saids/idsconfig/tmp/dbfresults")
	Cfg.DownloadFileDir = getEnv("DOWNLOAD_FILE_DIR", "/etc/saids/idsconfig/tmp/downloads")

	// Load VeloArtifact timeout and retry config
	Cfg.VeloExecutionTimeout = time.Duration(getEnvInt("VELO_EXECUTION_TIMEOUT", 120)) * time.Second // Default: 120 seconds
	Cfg.VeloDownloadTimeout = time.Duration(getEnvInt("VELO_DOWNLOAD_TIMEOUT", 60)) * time.Second    // Default: 60 seconds
	Cfg.VeloMaxRetries = getEnvInt("VELO_MAX_RETRIES", 5)                                            // Default: 5 retries
	Cfg.VeloRetryBaseDelay = time.Duration(getEnvInt("VELO_RETRY_BASE_delay", 2)) * time.Second      // Default: 2 seconds
	Cfg.VeloDownloadRetries = getEnvInt("VELO_DOWNLOAD_RETRIES", 5)                                  // Default: 5 retries

	// Load Agent API timeout and retry config
	Cfg.AgentExecutionTimeout = time.Duration(getEnvInt("AGENT_EXECUTION_TIMEOUT", 120)) * time.Second // Default: 120 seconds
	Cfg.AgentMaxRetries = getEnvInt("AGENT_MAX_RETRIES", 5)                                            // Default: 5 retries
	Cfg.AgentRetryBaseDelay = time.Duration(getEnvInt("AGENT_RETRY_BASE_DELAY", 2)) * time.Second      // Default: 2 seconds

	// Load system exclusion lists with defaults
	Cfg.SystemDatabases = getEnvStringSlice("SYSTEM_DATABASES", []string{
		"information_schema",
		"mysql",
		"performance_schema",
		"sys",
	})
	Cfg.SystemUsers = getEnvStringSlice("SYSTEM_USERS", []string{
		"mysql.infoschema",
		"mysql.session",
		"mysql.sys",
	})

	// Load policy compliance tool path
	Cfg.DBFCheckPolicyCompliancePath = getEnv("DBF_CHECK_POLICY_COMPLIANCE_PATH", "/etc/v2/dbf/dbfcheckpolicycompliance/dbfcheckpolicycompliance")

	// Load concurrency config (0 = auto-detect based on CPU cores)
	Cfg.PrivilegeLoadConcurrency = getEnvInt("PRIVILEGE_LOAD_CONCURRENCY", 0)
	Cfg.PrivilegeQueryConcurrency = getEnvInt("PRIVILEGE_QUERY_CONCURRENCY", 0)

	// Load MySQL privilege query logging config (default: false for production)
	Cfg.EnableMySQLPrivilegeQueryLogging = getEnvBool("ENABLE_MYSQL_PRIVILEGE_QUERY_LOGGING", false)

	log.Printf("[INFO] Config loaded - DB: %s@%s:%d/%s, LogLevel: %s",
		Cfg.DBUser, Cfg.DBHost, Cfg.DBPort, Cfg.DBName, Cfg.LogLevel)
	log.Printf("[INFO] VeloArtifact config - ExecTimeout: %v, DownloadTimeout: %v, MaxRetries: %d, BaseDelay: %v",
		Cfg.VeloExecutionTimeout, Cfg.VeloDownloadTimeout, Cfg.VeloMaxRetries, Cfg.VeloRetryBaseDelay)
	log.Printf("[INFO] Agent API config - Path: %s, ExecTimeout: %v, MaxRetries: %d, BaseDelay: %v",
		Cfg.AgentAPIPath, Cfg.AgentExecutionTimeout, Cfg.AgentMaxRetries, Cfg.AgentRetryBaseDelay)
	log.Printf("[INFO] System exclusion lists - Databases: %v, Users: %v",
		Cfg.SystemDatabases, Cfg.SystemUsers)

	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

// getEnvStringSlice parses comma-separated environment variable into string slice
// Format: "item1,item2,item3" -> []string{"item1", "item2", "item3"}
func getEnvStringSlice(key string, defaultVal []string) []string {
	if val := os.Getenv(key); val != "" {
		// Split by comma and trim whitespace
		items := strings.Split(val, ",")
		result := make([]string, 0, len(items))
		for _, item := range items {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}
	return defaultVal
}

// IsSystemDatabase checks if a database name is in the system exclusion list
func IsSystemDatabase(dbName string) bool {
	for _, sysDB := range Cfg.SystemDatabases {
		if dbName == sysDB {
			return true
		}
	}
	return false
}

// IsSystemUser checks if a database user is in the system exclusion list
func IsSystemUser(dbUser string) bool {
	for _, sysUser := range Cfg.SystemUsers {
		if dbUser == sysUser {
			return true
		}
	}
	return false
}

// GetPrivilegeLoadConcurrency returns optimal concurrency for privilege data loading.
// Auto-detects based on CPU cores if config value is 0, ensuring minimum of 2 and maximum of 20.
// Conservative limit prevents memory exhaustion when loading large privilege datasets.
func GetPrivilegeLoadConcurrency() int {
	if Cfg.PrivilegeLoadConcurrency > 0 {
		return Cfg.PrivilegeLoadConcurrency
	}

	// Auto-detect: use half of CPU cores to leave resources for other operations
	numCPU := runtime.NumCPU()
	// maxConcurrent := numCPU / 2
	maxConcurrent := numCPU
	// if maxConcurrent < 2 {
	// 	maxConcurrent = 2
	// }
	// if maxConcurrent > 20 {
	// 	maxConcurrent = 20
	// }
	return maxConcurrent
}

// GetPrivilegeQueryConcurrency returns optimal concurrency for policy query execution.
// Auto-detects based on CPU cores if config value is 0, ensuring minimum of 4 and maximum of 50.
// Higher than load concurrency because queries are lighter weight than data loading.
func GetPrivilegeQueryConcurrency() int {
	if Cfg.PrivilegeQueryConcurrency > 0 {
		return Cfg.PrivilegeQueryConcurrency
	}

	// Auto-detect: use full CPU cores since queries are CPU-bound operations
	numCPU := runtime.NumCPU()
	maxConcurrent := numCPU
	// if maxConcurrent < 4 {
	// 	maxConcurrent = 4
	// }
	// if maxConcurrent > 50 {
	// 	maxConcurrent = 50
	// }
	return maxConcurrent
}
