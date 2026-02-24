package dto

// DBQueryParam represents database query execution parameters for VeloArtifact operations.
type DBQueryParam struct {
	DBType      string `json:"db_type,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        int    `json:"port,omitempty"`
	User        string `json:"user,omitempty"`
	Password    string `json:"password,omitempty"`
	Database string `json:"database,omitempty"` // For Oracle: use service_name here
	Query    any    `json:"query,omitempty"`
	Action      string `json:"action,omitempty"`      // Optional: action like "download", "execute", "os_execute"
	Option      string `json:"option,omitempty"`      // Optional: option like "--background"
	FileName    string `json:"fileName,omitempty"`    // Optional: file name for backup/upload operations
	FilePath    string `json:"filePath,omitempty"`    // Optional: file path for upload operations
	SourceJobID string `json:"sourceJobId,omitempty"` // Optional: source job ID for upload operations
	CommandExec string `json:"commandExec,omitempty"` // Optional: OS command for os_execute action
	FileConfig  string `json:"fileconfig,omitempty"`  // Optional: path to database config file
}

// DBQueryParamBuilder provides a builder pattern for constructing DBQueryParam instances.
type DBQueryParamBuilder struct {
	dbQueryParam *DBQueryParam
}

// NewDBQueryParamBuilder creates a new DBQueryParam builder instance.
func NewDBQueryParamBuilder() *DBQueryParamBuilder {
	return &DBQueryParamBuilder{
		dbQueryParam: &DBQueryParam{},
	}
}

// SetDBType sets the database type for the query parameter.
func (r *DBQueryParamBuilder) SetDBType(dbType string) *DBQueryParamBuilder {
	r.dbQueryParam.DBType = dbType
	return r
}

// SetHost sets the database host for the query parameter.
func (r *DBQueryParamBuilder) SetHost(host string) *DBQueryParamBuilder {
	r.dbQueryParam.Host = host
	return r
}

// SetPort sets the database port for the query parameter.
func (r *DBQueryParamBuilder) SetPort(port int) *DBQueryParamBuilder {
	r.dbQueryParam.Port = port
	return r
}

// SetUser sets the database user for the query parameter.
func (r *DBQueryParamBuilder) SetUser(user string) *DBQueryParamBuilder {
	r.dbQueryParam.User = user
	return r
}

// SetPassword sets the database password for the query parameter.
func (r *DBQueryParamBuilder) SetPassword(password string) *DBQueryParamBuilder {
	r.dbQueryParam.Password = password
	return r
}

// SetDatabase sets the database name for the query parameter.
func (r *DBQueryParamBuilder) SetDatabase(database string) *DBQueryParamBuilder {
	r.dbQueryParam.Database = database
	return r
}

// SetQuery sets the SQL query or queries for the query parameter.
func (r *DBQueryParamBuilder) SetQuery(query any) *DBQueryParamBuilder {
	r.dbQueryParam.Query = query
	return r
}

// SetAction sets the VeloArtifact action type for the query parameter.
func (r *DBQueryParamBuilder) SetAction(action string) *DBQueryParamBuilder {
	r.dbQueryParam.Action = action
	return r
}

// SetOption sets the VeloArtifact command options for the query parameter.
func (r *DBQueryParamBuilder) SetOption(option string) *DBQueryParamBuilder {
	r.dbQueryParam.Option = option
	return r
}

// SetFileName sets the output file name for backup operations.
func (r *DBQueryParamBuilder) SetFileName(fileName string) *DBQueryParamBuilder {
	r.dbQueryParam.FileName = fileName
	return r
}

// SetFilePath sets the file path for upload operations.
func (r *DBQueryParamBuilder) SetFilePath(filePath string) *DBQueryParamBuilder {
	r.dbQueryParam.FilePath = filePath
	return r
}

// SetSourceJobID sets the source job ID for upload operations.
func (r *DBQueryParamBuilder) SetSourceJobID(sourceJobID string) *DBQueryParamBuilder {
	r.dbQueryParam.SourceJobID = sourceJobID
	return r
}

// SetCommandExec sets the OS command for os_execute action type.
func (r *DBQueryParamBuilder) SetCommandExec(commandExec string) *DBQueryParamBuilder {
	r.dbQueryParam.CommandExec = commandExec
	return r
}

// SetFileConfig sets the path to the database config file.
func (r *DBQueryParamBuilder) SetFileConfig(fileConfig string) *DBQueryParamBuilder {
	r.dbQueryParam.FileConfig = fileConfig
	return r
}

// Build constructs and returns the final DBQueryParam instance.
func (r *DBQueryParamBuilder) Build() *DBQueryParam {
	return r.dbQueryParam
}
