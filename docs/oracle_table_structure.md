# Danh sách bảng dùng để lấy các object trong dbobject
all_clusters
all_objects 
dba_context 
all_directories
all_procedures
all_indexes
all_db_links
all_mviews
all_dimensions
dba_flashback_archive
dba_profiles
all_sequences
all_synonyms
all_tables
dba_roles
all_triggers
all_types
dba_rollback_segs
dba_tablespaces
all_views
dba_users
DBA_PDBS
all_hierarchies
all_analytic_views
all_attribute_dimensions
dba_lockdown_profiles
all_zonemaps
v$database
v$instance
v$session
# Danh sách bảng dùng để kiểm tra quyền trong dbpolicydefault
cdb_sys_privs  - Kiểm tra quyền trên phạm vi tất cả PDB
dba_sys_privs  - Kiểm tra quyền trên phạm vi 1 PDB
dba_tab_privs  - Kiểm tra quyền trên phạm vi 1 object
v$pwfile_users - Kiểm tra 1 số quyền hệ thống đặc biệt

## ALL_CLUSTERS
| Name               | Null?    | Type          | Ý nghĩa                                                                                  | Phiên bản xuất hiện     |
|--------------------|----------|---------------|------------------------------------------------------------------------------------------|-------------------------|
| OWNER              | NOT NULL | VARCHAR2(128) | Schema sở hữu cluster                                                                    | Tất cả                  |
| CLUSTER_NAME       | NOT NULL | VARCHAR2(128) | Tên của cluster                                                                          | Tất cả                  |
| TABLESPACE_NAME    | NOT NULL | VARCHAR2(30)  | Tablespace chứa cluster                                                                  | Tất cả                  |
| PCT_FREE           |          | NUMBER        | Phần trăm không gian dành cho future updates trong mỗi block (0-99)                      | Tất cả                  |
| PCT_USED           |          | NUMBER        | Phần trăm tối thiểu để block quay lại freelist (DEPRECATED từ 10g, không còn sử dụng)    | Tất cả (deprecated 10g+)|
| KEY_SIZE           |          | NUMBER        | Kích thước ước tính của cluster key (bytes)                                              | Tất cả                  |
| INI_TRANS          | NOT NULL | NUMBER        | Số transaction slots ban đầu được allocated trong mỗi block                              | Tất cả                  |
| MAX_TRANS          | NOT NULL | NUMBER        | Số transaction slots tối đa trong mỗi block (DEPRECATED, luôn = 255)                     | Tất cả (deprecated 10g+)|
| INITIAL_EXTENT     |          | NUMBER        | Kích thước của extent đầu tiên được allocated (bytes)                                    | Tất cả                  |
| NEXT_EXTENT        |          | NUMBER        | Kích thước của extent tiếp theo sẽ được allocated (bytes)                                | Tất cả                  |
| MIN_EXTENTS        | NOT NULL | NUMBER        | Số extent tối thiểu được allocated khi tạo cluster                                       | Tất cả                  |
| MAX_EXTENTS        | NOT NULL | NUMBER        | Số extent tối đa có thể allocated cho cluster                                            | Tất cả                  |
| PCT_INCREASE       |          | NUMBER        | Phần trăm tăng kích thước của mỗi extent sau NEXT_EXTENT                                 | Tất cả                  |
| FREELISTS          |          | NUMBER        | Số freelists được allocated cho segment này                                              | Tất cả                  |
| FREELIST_GROUPS    |          | NUMBER        | Số freelist groups được allocated cho segment                                            | Tất cả                  |
| AVG_BLOCKS_PER_KEY |          | NUMBER        | Trung bình số blocks chứa rows với cùng cluster key value                                | Tất cả                  |
| CLUSTER_TYPE       |          | VARCHAR2(5)   | Loại cluster: INDEX (index cluster) hoặc HASH (hash cluster)                             | Tất cả                  |
| FUNCTION           |          | VARCHAR2(15)  | Hash function nếu là hash cluster                                                        | Tất cả                  |
| HASHKEYS           |          | NUMBER        | Số hash keys (buckets) cho hash cluster                                                  | Tất cả                  |
| DEGREE             |          | VARCHAR2(10)  | Số threads per instance cho parallel scan                                                | Tất cả                  |
| INSTANCES          |          | VARCHAR2(10)  | Số instances để scan cluster (RAC environments)                                          | Tất cả                  |
| CACHE              |          | VARCHAR2(5)   | Y/N - buffer cache preference cho full table scan                                        | Tất cả                  |
| BUFFER_POOL        |          | VARCHAR2(7)   | Default buffer pool cho cluster: DEFAULT, KEEP, hoặc RECYCLE                             | Tất cả                  |
| FLASH_CACHE        |          | VARCHAR2(7)   | Database Smart Flash Cache hint: DEFAULT, KEEP, hoặc NONE (Exadata)                      | 11g+                    |
| CELL_FLASH_CACHE   |          | VARCHAR2(7)   | Cell flash cache hint: DEFAULT, KEEP, hoặc NONE (Exadata Storage Server)                 | 11g+                    |
| SINGLE_TABLE       |          | VARCHAR2(5)   | Y nếu là single-table cluster, N nếu multi-table cluster                                 | Tất cả                  |
| DEPENDENCIES       |          | VARCHAR2(8)   | Row-level dependency tracking: ENABLED hoặc DISABLED                                     | 12c+                    |

## ALL_OBJECTS
| Name              | Null?    | Type           | Ý nghĩa                                                                                 | Phiên bản xuất hiện |
|-------------------|----------|----------------|---------------------------------------------------------------------------------------- |---------------------|
| OWNER             | NOT NULL | VARCHAR2(128)  | Schema sở hữu object                                                                    | Tất cả              |
| OBJECT_NAME       | NOT NULL | VARCHAR2(128)  | Tên của database object                                                                 | Tất cả              |
| SUBOBJECT_NAME    |          | VARCHAR2(128)  | Tên của subobject (ví dụ: partition name, subpartition name)                            | Tất cả              |
| OBJECT_ID         | NOT NULL | NUMBER         | Dictionary object identifier (duy nhất trong database)                                  | Tất cả              |
| DATA_OBJECT_ID    |          | NUMBER         | Segment/data object identifier (thay đổi khi rebuild, reorganize)                       | Tất cả              |
| OBJECT_TYPE       |          | VARCHAR2(23)   | Loại object: TABLE, INDEX, PACKAGE, PROCEDURE, FUNCTION, SEQUENCE, SYNONYM, etc.        | Tất cả              |
| CREATED           | NOT NULL | DATE           | Timestamp khi object được tạo                                                           | Tất cả              |
| LAST_DDL_TIME     | NOT NULL | DATE           | Timestamp của lần DDL cuối cùng (bao gồm GRANT, REVOKE)                                 | Tất cả              |
| TIMESTAMP         |          | VARCHAR2(19)   | Timestamp specification của object (dùng để track dependencies)                         | Tất cả              |
| STATUS            |          | VARCHAR2(7)    | Trạng thái object: VALID, INVALID, hoặc N/A                                             | Tất cả              |
| TEMPORARY         |          | VARCHAR2(1)    | Y nếu là temporary object (session-specific), N nếu permanent                           | Tất cả              |
| GENERATED         |          | VARCHAR2(1)    | Y nếu object name được system-generated, N nếu user-defined                             | Tất cả              |
| SECONDARY         |          | VARCHAR2(1)    | Y nếu là secondary object được tạo bởi ODCIIndexCreate của domain index                 | Tất cả              |
| NAMESPACE         | NOT NULL | NUMBER         | Namespace number: 1=TABLE/PROCEDURE, 2=BODY, 3=TRIGGER, 4=INDEX, etc.                   | 11g+                |
| EDITION_NAME      |          | VARCHAR2(128)  | Edition name nếu object thuộc về một edition cụ thể (Edition-Based Redefinition)        | 11gR2+              |
| SHARING           |          | VARCHAR2(18)   | Sharing mode trong CDB: METADATA LINK, DATA LINK, EXTENDED DATA LINK, OBJECT LINK, NONE | 12c+                |
| EDITIONABLE       |          | VARCHAR2(1)    | Y = editionable, N = noneditionable, INHERITED = kế thừa từ object type default         | 12c+                |
| ORACLE_MAINTAINED |          | VARCHAR2(1)    | Y nếu object được Oracle maintain (như SYS, system objects), N nếu user-created         | 12c+                |
| APPLICATION       |          | VARCHAR2(1)    | Y nếu là application common object (Application Containers), N nếu không                | 12c+                |
| DEFAULT_COLLATION |          | VARCHAR2(100)  | Default collation cho object (hỗ trợ multiple linguistic sorts)                         | 12.2+               |
| DUPLICATED        |          | VARCHAR2(1)    | Y nếu object được duplicate trên tất cả shards, N nếu không (Sharding)                  | 12.2+               |
| SHARDED           |          | VARCHAR2(1)    | Y nếu là sharded object (Oracle Sharding), N nếu không                                  | 12.2+               |
| IMPORTED_OBJECT   |          | VARCHAR2(1)    | Y nếu object được import qua DBMS_CLOUD (Autonomous Database), N nếu không              | 18c+                |
| CREATED_APPID     |          | NUMBER         | Application ID tạo object (Application Continuity tracking)                             | 18c+                |
| CREATED_VSNID     |          | NUMBER         | Application version ID tạo object                                                       | 18c+                |
| MODIFIED_APPID    |          | NUMBER         | Application ID thực hiện modification cuối cùng                                         | 18c+                |
| MODIFIED_VSNID    |          | NUMBER         | Application version ID thực hiện modification cuối cùng                                 | 18c+                |
 
## DBA_CONTEXT
| Name          | Null?    | Type          | Ý nghĩa                                                                                            | Phiên bản xuất hiện |
|---------------|----------|---------------|----------------------------------------------------------------------------------------------------|---------------------|
| NAMESPACE     | NOT NULL | VARCHAR2(128) | Tên namespace của application context (được tạo bởi CREATE CONTEXT)                                | Tất cả              |
| SCHEMA        | NOT NULL | VARCHAR2(128) | Schema owner của package chứa context procedures                                                   | Tất cả              |
| PACKAGE       | NOT NULL | VARCHAR2(128) | Package name chứa các procedures để set/clear context attributes                                   | Tất cả              |
| TYPE          |          | VARCHAR2(22)  | Loại context: ACCESSED GLOBALLY (global application context) hoặc ACCESSED LOCALLY (local context) | 10g+                |
| ORIGIN_CON_ID |          | NUMBER        | Container ID (CDB$ROOT=1) nơi context namespace được tạo (Multitenant)                             | 12c+                |
| TRACKING      |          | VARCHAR2(3)   | YES nếu session tracking enabled cho context, NO nếu không (dùng cho Fine-Grained Auditing)        | 12.2+               |
 
## ALL_DIRECTORIES
| Name           | Null?    | Type           | Ý nghĩa                                                                   | Phiên bản xuất hiện                           |
|----------------|----------|----------------|---------------------------------------------------------------------------|-----------------------------------------------|
| OWNER          | NOT NULL | VARCHAR2(128)  | Owner của directory object (thường là SYS hoặc PUBLIC)                    | Tất cả                                        |
| DIRECTORY_NAME | NOT NULL | VARCHAR2(128)  | Tên logical của directory object (được tạo bởi CREATE DIRECTORY)          | Tất cả                                        |
| DIRECTORY_PATH |          | VARCHAR2(4000) | Full physical path trên server operating system file system               | Tất cả (VARCHAR2 size tăng dần: 512→1000→4000)|
| ORIGIN_CON_ID  |          | NUMBER         | Container ID nơi directory được tạo (CDB$ROOT=1, PDB=3+)                  | 12c+                                          |
 
## ALL_PROCEDURES
| Name                       | Null? | Type           | Ý nghĩa                                                                                              | Phiên bản xuất hiện |
|----------------------------|-------|----------------|------------------------------------------------------------------------------------------------------|---------------------|
| OWNER                      |       | VARCHAR2(128)  | Schema owner của procedure/function                                                                  | Tất cả              |
| OBJECT_NAME                |       | VARCHAR2(128)  | Object name (package name, type name, hoặc standalone procedure/function name)                       | Tất cả              |
| PROCEDURE_NAME             |       | VARCHAR2(128)  | Procedure/function name trong package/type (NULL cho standalone)                                     | Tất cả              |
| OBJECT_ID                  |       | NUMBER         | Object identifier của object chứa procedure/function                                                 | Tất cả              |
| SUBPROGRAM_ID              |       | NUMBER         | Subprogram identifier (unique trong object)                                                          | Tất cả              |
| OVERLOAD                   |       | VARCHAR2(40)   | Overload unique identifier cho overloaded procedures/functions (số thứ tự)                           | Tất cả              |
| OBJECT_TYPE                |       | VARCHAR2(13)   | Loại object: PROCEDURE, FUNCTION, PACKAGE, TYPE, etc.                                                | Tất cả              |
| AGGREGATE                  |       | VARCHAR2(3)    | YES nếu là aggregate function (dùng trong GROUP BY), NO nếu không                                    | 10g+                |
| PIPELINED                  |       | VARCHAR2(3)    | YES nếu là pipelined table function (trả về rows incrementally), NO nếu không                        | 10g+                |
| IMPLTYPEOWNER              |       | VARCHAR2(128)  | Owner của implementation type (cho object type methods)                                              | 10g+                |
| IMPLTYPENAME               |       | VARCHAR2(128)  | Name của implementation type (cho object type methods)                                               | 10g+                |
| PARALLEL                   |       | VARCHAR2(3)    | YES nếu parallel enabled (có thể chạy parallel trong SQL), NO nếu không                              | 11g+                |
| INTERFACE                  |       | VARCHAR2(3)    | YES nếu chỉ là interface (abstract method), NO nếu có implementation                                 | 11g+                |
| DETERMINISTIC              |       | VARCHAR2(3)    | YES nếu function là deterministic (cùng input → cùng output), NO nếu không                           | 11g+                |
| AUTHID                     |       | VARCHAR2(12)   | Execution authority: DEFINER (owner's privileges) hoặc CURRENT_USER (invoker's privileges)           | 11g+                |
| RESULT_CACHE               |       | VARCHAR2(3)    | YES nếu result cache enabled (Oracle cache kết quả function), NO nếu không                           | 11gR2+              |
| ORIGIN_CON_ID              |       | NUMBER         | Container ID nơi procedure được tạo (Multitenant architecture)                                       | 12c+                |
| POLYMORPHIC                |       | VARCHAR2(5)    | YES nếu là polymorphic table function (runtime return type), NO nếu không                            | 18c+                |
| SQL_MACRO                  |       | VARCHAR2(6)    | Loại SQL Macro: SCALAR (scalar expression) hoặc TABLE (table expression), NULL nếu không phải        | 19c+                |
| BLOCKCHAIN                 |       | VARCHAR2(3)    | YES nếu là blockchain table procedure, NO nếu không (Blockchain Tables feature)                      | 21c                 |
| BLOCKCHAIN_MANDATORY_VOTES |       | VARCHAR2(4000) | Configuration cho mandatory votes trong blockchain table operations                                  | 21c                 |

## ALL_INDEXES
| Name                        | Null?    | Type           | Ý nghĩa                                                                                        | Phiên bản xuất hiện |
|-----------------------------|----------|----------------|------------------------------------------------------------------------------------------------|---------------------|
| OWNER                       | NOT NULL | VARCHAR2(128)  | Schema sở hữu index                                                                            | Tất cả              |
| INDEX_NAME                  | NOT NULL | VARCHAR2(128)  | Tên của index                                                                                  | Tất cả              |
| INDEX_TYPE                  |          | VARCHAR2(27)   | Loại index: NORMAL, BITMAP, FUNCTION-BASED NORMAL, DOMAIN, IOT - TOP, etc.                     | Tất cả              |
| TABLE_OWNER                 | NOT NULL | VARCHAR2(128)  | Owner của table/materialized view được index                                                   | Tất cả              |
| TABLE_NAME                  | NOT NULL | VARCHAR2(128)  | Tên của table/materialized view được index                                                     | Tất cả              |
| TABLE_TYPE                  |          | CHAR(5)        | Loại object: TABLE hoặc MAT_VIEW (Materialized View)                                           | Tất cả              |
| UNIQUENESS                  |          | VARCHAR2(9)    | UNIQUE hoặc NONUNIQUE - index có enforce uniqueness không                                      | Tất cả              |
| COMPRESSION                 |          | VARCHAR2(13)   | ENABLED (compressed), DISABLED, hoặc loại compression (ADVANCED LOW, ADVANCED HIGH)            | Tất cả              |
| PREFIX_LENGTH               |          | NUMBER         | Số columns trong compression key prefix                                                        | Tất cả              |
| TABLESPACE_NAME             |          | VARCHAR2(30)   | Tablespace chứa index                                                                          | Tất cả              |
| INI_TRANS                   |          | NUMBER         | Số transaction slots ban đầu trong mỗi block                                                   | Tất cả              |
| MAX_TRANS                   |          | NUMBER         | Số transaction slots tối đa (deprecated, luôn = 255)                                           | Tất cả (deprecated 10g+)|
| INITIAL_EXTENT              |          | NUMBER         | Kích thước extent đầu tiên (bytes)                                                             | Tất cả              |
| NEXT_EXTENT                 |          | NUMBER         | Kích thước extent tiếp theo (bytes)                                                            | Tất cả              |
| MIN_EXTENTS                 |          | NUMBER         | Số extent tối thiểu                                                                            | Tất cả              |
| MAX_EXTENTS                 |          | NUMBER         | Số extent tối đa                                                                               | Tất cả              |
| PCT_INCREASE                |          | NUMBER         | Phần trăm tăng kích thước extent                                                               | Tất cả              |
| PCT_THRESHOLD               |          | NUMBER         | Threshold percentage của block space cho mỗi index entry (IOT)                                 | Tất cả              |
| INCLUDE_COLUMN              |          | NUMBER         | Column ID của last column trong index-organized table primary key                              | Tất cả              |
| FREELISTS                   |          | NUMBER         | Số freelists được allocated                                                                    | Tất cả              |
| FREELIST_GROUPS             |          | NUMBER         | Số freelist groups được allocated                                                              | Tất cả              |
| PCT_FREE                    |          | NUMBER         | Phần trăm không gian dành cho updates trong mỗi block                                          | Tất cả              |
| LOGGING                     |          | VARCHAR2(3)    | YES/NO - logging information được record trong redo log                                        | Tất cả              |
| BLEVEL                      |          | NUMBER         | B-Tree level: depth của index từ root block đến leaf blocks                                    | Tất cả              |
| LEAF_BLOCKS                 |          | NUMBER         | Số leaf blocks trong index                                                                     | Tất cả              |
| DISTINCT_KEYS               |          | NUMBER         | Số distinct indexed values                                                                     | Tất cả              |
| AVG_LEAF_BLOCKS_PER_KEY     |          | NUMBER         | Trung bình số leaf blocks mỗi distinct key value                                               | Tất cả              |
| AVG_DATA_BLOCKS_PER_KEY     |          | NUMBER         | Trung bình số data blocks mỗi distinct key value                                               | Tất cả              |
| CLUSTERING_FACTOR           |          | NUMBER         | Mức độ ordering của rows theo index (gần NUM_ROWS=well ordered, gần blocks=random)             | Tất cả              |
| STATUS                      |          | VARCHAR2(8)    | VALID, UNUSABLE, hoặc N/A - trạng thái của index                                               | Tất cả              |
| NUM_ROWS                    |          | NUMBER         | Số rows trong index (từ statistics)                                                            | Tất cả              |
| SAMPLE_SIZE                 |          | NUMBER         | Sample size dùng khi analyze index                                                             | Tất cả              |
| LAST_ANALYZED               |          | DATE           | Timestamp của lần analyze gần nhất                                                             | Tất cả              |
| DEGREE                      |          | VARCHAR2(40)   | Số threads per instance cho parallel scan                                                      | Tất cả              |
| INSTANCES                   |          | VARCHAR2(40)   | Số instances để scan index (RAC)                                                               | Tất cả              |
| PARTITIONED                 |          | VARCHAR2(3)    | YES nếu index được partition, NO nếu không                                                     | Tất cả              |
| TEMPORARY                   |          | VARCHAR2(1)    | Y nếu index trên temporary table, N nếu permanent                                              | Tất cả              |
| GENERATED                   |          | VARCHAR2(1)    | Y nếu name là system-generated, N nếu user-defined                                             | Tất cả              |
| SECONDARY                   |          | VARCHAR2(1)    | Y nếu là secondary object của ODCIIndexCreate (domain index)                                   | Tất cả              |
| BUFFER_POOL                 |          | VARCHAR2(7)    | Default buffer pool: DEFAULT, KEEP, hoặc RECYCLE                                               | Tất cả              |
| FLASH_CACHE                 |          | VARCHAR2(7)    | Database Smart Flash Cache hint (Exadata)                                                      | 11g+                |
| CELL_FLASH_CACHE            |          | VARCHAR2(7)    | Cell flash cache hint (Exadata Storage Server)                                                 | 11g+                |
| USER_STATS                  |          | VARCHAR2(3)    | YES nếu statistics được set trực tiếp bởi user, NO nếu do Oracle gather                        | Tất cả              |
| DURATION                    |          | VARCHAR2(15)   | Duration của temporary index: SYS$SESSION hoặc SYS$TRANSACTION                                 | Tất cả              |
| PCT_DIRECT_ACCESS           |          | NUMBER         | Phần trăm rows có VALID guess trong IOT secondary index                                        | Tất cả              |
| ITYP_OWNER                  |          | VARCHAR2(128)  | Owner của index type (cho domain indexes)                                                      | Tất cả              |
| ITYP_NAME                   |          | VARCHAR2(128)  | Name của index type (cho domain indexes)                                                       | Tất cả              |
| PARAMETERS                  |          | VARCHAR2(1000) | Parameter string cho domain index                                                              | Tất cả              |
| GLOBAL_STATS                |          | VARCHAR2(3)    | YES nếu statistics được gather cho toàn index, NO nếu từ underlying partitions                 | Tất cả              |
| DOMIDX_STATUS               |          | VARCHAR2(12)   | Status của domain index: VALID, IDXTYP_INVLD, etc.                                             | Tất cả              |
| DOMIDX_OPSTATUS             |          | VARCHAR2(6)    | Operation status của domain index: VALID, FAILED                                               | Tất cả              |
| FUNCIDX_STATUS              |          | VARCHAR2(8)    | Status của function-based index: ENABLED hoặc DISABLED                                         | Tất cả              |
| JOIN_INDEX                  |          | VARCHAR2(3)    | YES nếu là join index, NO nếu không                                                            | Tất cả              |
| IOT_REDUNDANT_PKEY_ELIM     |          | VARCHAR2(3)    | YES nếu elimination of redundant primary key columns enabled cho IOT secondary index           | Tất cả              |
| DROPPED                     |          | VARCHAR2(3)    | YES nếu index đã bị drop và nằm trong recycle bin, NO nếu không                                | 10g+                |
| VISIBILITY                  |          | VARCHAR2(9)    | VISIBLE hoặc INVISIBLE - optimizer có thể sử dụng index không                                  | 11g+                |
| DOMIDX_MANAGEMENT           |          | VARCHAR2(14)   | MANUAL_MAINTENANCE hoặc SYSTEM_MANAGED (domain index management)                               | 11g+                |
| SEGMENT_CREATED             |          | VARCHAR2(3)    | YES nếu segment đã được tạo, NO nếu deferred segment creation                                  | 11gR2+              |
| ORPHANED_ENTRIES            |          | VARCHAR2(3)    | YES nếu có orphaned entries trong global index sau partition maintenance, NO nếu không         | 11gR2+              |
| INDEXING                    |          | VARCHAR2(7)    | ON/OFF/PARTIAL - indexing property cho partial indexes on partitioned tables                   | 12c+                |
| AUTO                        |          | VARCHAR2(3)    | YES nếu là auto index (Automatic Indexing), NO nếu không                                       | 19c+                |
| CONSTRAINT_INDEX            |          | VARCHAR2(3)    | YES nếu index được tạo bởi constraint, NO nếu không                                            | Tất cả              |

## ALL_DB_LINKS
| Name             | Null?    | Type           | Ý nghĩa                                                                                  | Phiên bản xuất hiện |
|------------------|----------|----------------|------------------------------------------------------------------------------------------|---------------------|
| OWNER            | NOT NULL | VARCHAR2(128)  | Schema owner của database link                                                           | Tất cả              |
| DB_LINK          | NOT NULL | VARCHAR2(128)  | Tên của database link                                                                    | Tất cả              |
| USERNAME         |          | VARCHAR2(128)  | Username để login vào remote database (NULL nếu current user)                            | Tất cả              |
| CREDENTIAL_NAME  |          | VARCHAR2(128)  | Tên credential object được sử dụng cho authentication                                    | 18c+                |
| CREDENTIAL_OWNER |          | VARCHAR2(128)  | Owner của credential object                                                              | 18c+                |
| HOST             |          | VARCHAR2(2000) | SQL*Net string hoặc service name để connect remote database                              | Tất cả              |
| CREATED          | NOT NULL | DATE           | Timestamp khi database link được tạo                                                     | Tất cả              |
| HIDDEN           |          | VARCHAR2(3)    | YES nếu là hidden link (internal use), NO nếu visible                                    | 12c+                |
| SHARD_INTERNAL   |          | VARCHAR2(3)    | YES nếu là internal shard link (Oracle Sharding), NO nếu không                           | 12.2+               |
| VALID            |          | VARCHAR2(3)    | YES nếu link valid và có thể kết nối, NO nếu có vấn đề                                   | 12c+                |
| INTRA_CDB        |          | VARCHAR2(3)    | YES nếu link giữa containers trong cùng CDB, NO nếu external                             | 12c+                |

## ALL_MVIEWS
| Name                      | Null?    | Type          | Ý nghĩa                                                                                     | Phiên bản xuất hiện |
|---------------------------|----------|---------------|---------------------------------------------------------------------------------------------|---------------------|
| OWNER                     | NOT NULL | VARCHAR2(128) | Schema owner của materialized view                                                          | Tất cả              |
| MVIEW_NAME                | NOT NULL | VARCHAR2(128) | Tên của materialized view                                                                   | Tất cả              |
| CONTAINER_NAME            | NOT NULL | VARCHAR2(128) | Tên của container table (chứa data của materialized view)                                   | Tất cả              |
| QUERY                     |          | LONG          | Query text định nghĩa materialized view                                                     | Tất cả              |
| QUERY_LEN                 |          | NUMBER(38)    | Độ dài của query text (bytes)                                                               | Tất cả              |
| UPDATABLE                 |          | VARCHAR2(1)   | Y nếu materialized view updatable, N nếu read-only                                          | Tất cả              |
| UPDATE_LOG                |          | VARCHAR2(128) | Tên của materialized view log nếu có                                                        | Tất cả              |
| MASTER_ROLLBACK_SEG       |          | VARCHAR2(128) | Rollback segment được dùng cho refresh (deprecated)                                         | Tất cả (deprecated) |
| MASTER_LINK               |          | VARCHAR2(128) | Database link đến master table site                                                         | Tất cả              |
| REWRITE_ENABLED           |          | VARCHAR2(1)   | Y nếu query rewrite enabled, N nếu disabled                                                 | Tất cả              |
| REWRITE_CAPABILITY        |          | VARCHAR2(9)   | GENERAL, NONE - khả năng query rewrite của materialized view                                | Tất cả              |
| REFRESH_MODE              |          | VARCHAR2(9)   | DEMAND (manual), COMMIT (on commit), NEVER - refresh mode                                   | Tất cả              |
| REFRESH_METHOD            |          | VARCHAR2(8)   | COMPLETE, FAST, FORCE, NEVER - refresh method                                               | Tất cả              |
| BUILD_MODE                |          | VARCHAR2(9)   | IMMEDIATE (populate ngay), DEFERRED (populate sau) - build mode                             | Tất cả              |
| FAST_REFRESHABLE          |          | VARCHAR2(18)  | DIRLOAD_DML, DML, DIRLOAD_LIMITEDDML - loại DML hỗ trợ fast refresh                         | Tất cả              |
| LAST_REFRESH_TYPE         |          | VARCHAR2(8)   | COMPLETE, FAST, NA - loại refresh được thực hiện lần cuối                                   | Tất cả              |
| LAST_REFRESH_DATE         |          | DATE          | Timestamp bắt đầu refresh cuối cùng                                                         | Tất cả              |
| LAST_REFRESH_END_TIME     |          | DATE          | Timestamp kết thúc refresh cuối cùng                                                        | 10g+                |
| STALENESS                 |          | VARCHAR2(19)  | FRESH, STALE, UNUSABLE, UNKNOWN, NEEDS_COMPILE - trạng thái staleness                       | Tất cả              |
| AFTER_FAST_REFRESH        |          | VARCHAR2(19)  | FRESH, STALE, UNUSABLE - staleness sau khi fast refresh                                     | Tất cả              |
| UNKNOWN_PREBUILT          |          | VARCHAR2(1)   | Y nếu materialized view prebuilt nhưng không biết staleness, N nếu không                    | Tất cả              |
| UNKNOWN_PLSQL_FUNC        |          | VARCHAR2(1)   | Y nếu có PL/SQL function không deterministic trong query, N nếu không                       | Tất cả              |
| UNKNOWN_EXTERNAL_TABLE    |          | VARCHAR2(1)   | Y nếu có external table trong query, N nếu không                                            | Tất cả              |
| UNKNOWN_CONSIDER_FRESH    |          | VARCHAR2(1)   | Y nếu CONSIDER FRESH được dùng, N nếu không                                                 | Tất cả              |
| UNKNOWN_IMPORT            |          | VARCHAR2(1)   | Y nếu được import và staleness không xác định, N nếu không                                  | Tất cả              |
| UNKNOWN_TRUSTED_FD        |          | VARCHAR2(1)   | Y nếu có trusted constraint không enforced, N nếu không                                     | Tất cả              |
| COMPILE_STATE             |          | VARCHAR2(19)  | VALID, NEEDS_COMPILE, ERROR, COMPILATION_ERROR - trạng thái compile                         | Tất cả              |
| USE_NO_INDEX              |          | VARCHAR2(1)   | Y nếu NO_INDEX hint được dùng, N nếu không                                                  | Tất cả              |
| STALE_SINCE               |          | DATE          | Timestamp khi materialized view trở nên stale                                               | 10g+                |
| NUM_PCT_TABLES            |          | NUMBER        | Số PCT (Partition Change Tracking) tables trong materialized view                           | Tất cả              |
| NUM_FRESH_PCT_REGIONS     |          | NUMBER        | Số PCT regions đang fresh (up-to-date)                                                      | Tất cả              |
| NUM_STALE_PCT_REGIONS     |          | NUMBER        | Số PCT regions đang stale (out-of-date)                                                     | Tất cả              |
| SEGMENT_CREATED           |          | VARCHAR2(3)   | YES nếu segment đã tạo, NO nếu deferred segment creation                                    | 11gR2+              |
| EVALUATION_EDITION        |          | VARCHAR2(128) | Edition name nơi materialized view được evaluate                                            | 11gR2+              |
| UNUSABLE_BEFORE           |          | VARCHAR2(128) | Edition trước đó mà materialized view unusable                                              | 11gR2+              |
| UNUSABLE_BEGINNING        |          | VARCHAR2(128) | Edition đầu tiên mà materialized view bắt đầu unusable                                      | 11gR2+              |
| DEFAULT_COLLATION         |          | VARCHAR2(100) | Default collation cho materialized view                                                     | 12.2+               |
| ON_QUERY_COMPUTATION      |          | VARCHAR2(1)   | Y nếu refresh ON QUERY COMPUTATION, N nếu không                                             | 18c+                |
| AUTO                      |          | VARCHAR2(3)   | YES nếu là auto materialized view (Automatic Materialized Views), NO nếu không              | 19c+                |

## ALL_DIMENSIONS
| Name           | Null?    | Type          | Ý nghĩa                                                                          | Phiên bản xuất hiện |
|----------------|----------|---------------|----------------------------------------------------------------------------------|---------------------|
| OWNER          | NOT NULL | VARCHAR2(128) | Schema owner của dimension                                                       | Tất cả (9i+)        |
| DIMENSION_NAME | NOT NULL | VARCHAR2(128) | Tên của dimension                                                                | Tất cả (9i+)        |
| INVALID        |          | VARCHAR2(1)   | Y nếu dimension invalid (cần revalidate), N nếu valid                            | Tất cả (9i+)        |
| COMPILE_STATE  |          | VARCHAR2(13)  | VALID hoặc NEEDS_COMPILE - trạng thái compile của dimension                      | 10g+                |
| REVISION       |          | NUMBER        | Revision number của dimension (tăng mỗi khi thay đổi)                            | 10g+                |

## DBA_FLASHBACK_ARCHIVE
| Name                   | Null?    | Type           | Ý nghĩa                                                                     | Phiên bản xuất hiện |
|------------------------|----------|----------------|-----------------------------------------------------------------------------|---------------------|
| OWNER_NAME             |          | VARCHAR2(255)  | Owner của flashback archive                                                 | 11g+                |
| FLASHBACK_ARCHIVE_NAME | NOT NULL | VARCHAR2(255)  | Tên của flashback archive                                                   | 11g+                |
| FLASHBACK_ARCHIVE#     | NOT NULL | NUMBER         | Unique identifier của flashback archive                                     | 11g+                |
| RETENTION_IN_DAYS      | NOT NULL | NUMBER         | Số ngày retention cho historical data                                       | 11g+                |
| CREATE_TIME            |          | TIMESTAMP(9)   | Timestamp khi flashback archive được tạo                                    | 11g+                |
| LAST_PURGE_TIME        |          | TIMESTAMP(9)   | Timestamp của lần purge gần nhất                                            | 11g+                |
| STATUS                 |          | VARCHAR2(7)    | DEFAULT hoặc empty - có phải default flashback archive không                | 11g+                |

## DBA_PROFILES
| Name              | Null?    | Type         | Ý nghĩa                                                                            | Phiên bản xuất hiện |
|-------------------|----------|--------------|------------------------------------------------------------------------------------|---------------------|
| PROFILE           | NOT NULL | VARCHAR2(128)| Tên của profile                                                                    | Tất cả              |
| RESOURCE_NAME     | NOT NULL | VARCHAR2(32) | Tên của resource: SESSIONS_PER_USER, CPU_PER_SESSION, PASSWORD_LIFE_TIME, etc.     | Tất cả              |
| RESOURCE_TYPE     |          | VARCHAR2(8)  | KERNEL (resource limits) hoặc PASSWORD (password parameters)                       | Tất cả              |
| LIMIT             |          | VARCHAR2(257)| Giá trị limit: number, UNLIMITED, DEFAULT, hoặc expression                         | Tất cả              |
| COMMON            |          | VARCHAR2(3)  | YES nếu là common object (CDB), NO nếu local                                       | 12c+                |
| INHERITED         |          | VARCHAR2(3)  | YES nếu limit được kế thừa từ parent container, NO nếu không                       | 12c+                |
| IMPLICIT          |          | VARCHAR2(3)  | YES nếu profile được tạo implicitly, NO nếu explicit                               | 12c+                |
| ORACLE_MAINTAINED |          | VARCHAR2(3)  | YES nếu được Oracle maintain (như DEFAULT profile), NO nếu user-created            | 12c+                |
| MANDATORY         |          | VARCHAR2(3)  | YES nếu là mandatory profile (PDB lockdown), NO nếu không                          | 18c+                |

## ALL_SEQUENCES
| Name           | Null?    | Type          | Ý nghĩa                                                                               | Phiên bản xuất hiện |
|----------------|----------|---------------|---------------------------------------------------------------------------------------|---------------------|
| SEQUENCE_OWNER | NOT NULL | VARCHAR2(128) | Schema owner của sequence                                                             | Tất cả              |
| SEQUENCE_NAME  | NOT NULL | VARCHAR2(128) | Tên của sequence                                                                      | Tất cả              |
| MIN_VALUE      |          | NUMBER        | Giá trị minimum của sequence                                                          | Tất cả              |
| MAX_VALUE      |          | NUMBER        | Giá trị maximum của sequence                                                          | Tất cả              |
| INCREMENT_BY   | NOT NULL | NUMBER        | Giá trị increment (có thể âm cho descending sequences)                                | Tất cả              |
| CYCLE_FLAG     |          | VARCHAR2(1)   | Y nếu sequence cycle khi đạt limit, N nếu không                                       | Tất cả              |
| ORDER_FLAG     |          | VARCHAR2(1)   | Y nếu numbers được generate theo order (RAC), N nếu không                             | Tất cả              |
| CACHE_SIZE     | NOT NULL | NUMBER        | Số sequence numbers được cache trong memory (0 = NOCACHE)                             | Tất cả              |
| LAST_NUMBER    | NOT NULL | NUMBER        | Last sequence number written to disk (next available number)                          | Tất cả              |
| SCALE_FLAG     |          | VARCHAR2(1)   | Y nếu sequence scalable (extend/noextend), N nếu không                                | 12c+                |
| EXTEND_FLAG    |          | VARCHAR2(1)   | Y nếu sequence extended (beyond original max), N nếu không                            | 12c+                |
| SHARDED_FLAG   |          | VARCHAR2(1)   | Y nếu sequence sharded (Oracle Sharding), N nếu không                                 | 18c+                |
| SESSION_FLAG   |          | VARCHAR2(1)   | Y nếu session sequence (session-specific values), N nếu global                        | 18c+                |
| KEEP_VALUE     |          | VARCHAR2(1)   | Y nếu KEEP attribute enabled (preserve last value), N nếu không                       | 18c+                |

## ALL_SYNONYMS
| Name          | Null? | Type          | Ý nghĩa                                                                      | Phiên bản xuất hiện |
|---------------|-------|---------------|------------------------------------------------------------------------------|---------------------|
| OWNER         |       | VARCHAR2(128) | Schema owner của synonym (PUBLIC cho public synonyms)                        | Tất cả              |
| SYNONYM_NAME  |       | VARCHAR2(128) | Tên của synonym                                                              | Tất cả              |
| TABLE_OWNER   |       | VARCHAR2(128) | Owner của object được refer bởi synonym                                      | Tất cả              |
| TABLE_NAME    |       | VARCHAR2(128) | Tên của object được refer bởi synonym                                        | Tất cả              |
| DB_LINK       |       | VARCHAR2(128) | Database link name nếu synonym trỏ đến remote object, NULL nếu local         | Tất cả              |
| ORIGIN_CON_ID |       | NUMBER        | Container ID nơi synonym được tạo (Multitenant)                              | 12c+                |

## ALL_TABLES
| Name                     | Null?    | Type           | Ý nghĩa                                                                                        | Phiên bản xuất hiện |
|--------------------------|----------|----------------|------------------------------------------------------------------------------------------------|---------------------|
| OWNER                    | NOT NULL | VARCHAR2(128)  | Schema owner của table                                                                         | Tất cả              |
| TABLE_NAME               | NOT NULL | VARCHAR2(128)  | Tên của table                                                                                  | Tất cả              |
| TABLESPACE_NAME          |          | VARCHAR2(30)   | Tablespace chứa table (NULL cho partitioned, IOT, external tables)                             | Tất cả              |
| CLUSTER_NAME             |          | VARCHAR2(128)  | Tên của cluster nếu table thuộc cluster                                                        | Tất cả              |
| IOT_NAME                 |          | VARCHAR2(128)  | Tên của IOT (Index-Organized Table) nếu là overflow/mapping table                              | Tất cả              |
| STATUS                   |          | VARCHAR2(8)    | VALID, UNUSABLE, hoặc N/A - trạng thái của table                                               | Tất cả              |
| PCT_FREE                 |          | NUMBER         | Phần trăm không gian dành cho future updates trong mỗi block                                   | Tất cả              |
| PCT_USED                 |          | NUMBER         | Phần trăm tối thiểu để block quay lại freelist (deprecated từ 10g)                             | Tất cả (deprecated 10g+)|
| INI_TRANS                |          | NUMBER         | Số transaction slots ban đầu trong mỗi block                                                   | Tất cả              |
| MAX_TRANS                |          | NUMBER         | Số transaction slots tối đa (deprecated, luôn = 255)                                           | Tất cả (deprecated 10g+)|
| INITIAL_EXTENT           |          | NUMBER         | Kích thước extent đầu tiên (bytes)                                                             | Tất cả              |
| NEXT_EXTENT              |          | NUMBER         | Kích thước extent tiếp theo (bytes)                                                            | Tất cả              |
| MIN_EXTENTS              |          | NUMBER         | Số extent tối thiểu                                                                            | Tất cả              |
| MAX_EXTENTS              |          | NUMBER         | Số extent tối đa                                                                               | Tất cả              |
| PCT_INCREASE             |          | NUMBER         | Phần trăm tăng kích thước extent                                                               | Tất cả              |
| FREELISTS                |          | NUMBER         | Số freelists được allocated                                                                    | Tất cả              |
| FREELIST_GROUPS          |          | NUMBER         | Số freelist groups được allocated                                                              | Tất cả              |
| LOGGING                  |          | VARCHAR2(3)    | YES/NO - logging information được record trong redo log                                        | Tất cả              |
| BACKED_UP                |          | VARCHAR2(1)    | Y nếu table đã được backup từ lần change cuối, N nếu chưa                                      | Tất cả              |
| NUM_ROWS                 |          | NUMBER         | Số rows trong table (từ statistics)                                                            | Tất cả              |
| BLOCKS                   |          | NUMBER         | Số data blocks được allocated cho table                                                        | Tất cả              |
| EMPTY_BLOCKS             |          | NUMBER         | Số empty blocks trong table (deprecated)                                                       | Tất cả (deprecated) |
| AVG_SPACE                |          | NUMBER         | Average available free space trong mỗi block (bytes)                                           | Tất cả              |
| CHAIN_CNT                |          | NUMBER         | Số rows bị chained/migrated sang blocks khác                                                   | Tất cả              |
| AVG_ROW_LEN              |          | NUMBER         | Average length của row (bytes)                                                                 | Tất cả              |
| AVG_SPACE_FREELIST_BLOCKS|          | NUMBER         | Average freelist space (deprecated)                                                            | Tất cả (deprecated) |
| NUM_FREELIST_BLOCKS      |          | NUMBER         | Số blocks trong freelist (deprecated)                                                          | Tất cả (deprecated) |
| DEGREE                   |          | VARCHAR2(10)   | Số threads per instance cho parallel scan                                                      | Tất cả              |
| INSTANCES                |          | VARCHAR2(10)   | Số instances để scan table (RAC)                                                               | Tất cả              |
| CACHE                    |          | VARCHAR2(5)    | Y/N - buffer cache preference cho full table scan                                              | Tất cả              |
| TABLE_LOCK               |          | VARCHAR2(8)    | ENABLED/DISABLED - table lock có được phép không                                               | Tất cả              |
| SAMPLE_SIZE              |          | NUMBER         | Sample size dùng khi analyze table                                                             | Tất cả              |
| LAST_ANALYZED            |          | DATE           | Timestamp của lần analyze gần nhất                                                             | Tất cả              |
| PARTITIONED              |          | VARCHAR2(3)    | YES nếu table được partition, NO nếu không                                                     | Tất cả              |
| IOT_TYPE                 |          | VARCHAR2(12)   | IOT, IOT_OVERFLOW, IOT_MAPPING - loại Index-Organized Table                                    | Tất cả              |
| TEMPORARY                |          | VARCHAR2(1)    | Y nếu temporary table, N nếu permanent                                                         | Tất cả              |
| SECONDARY                |          | VARCHAR2(1)    | Y nếu là secondary object (domain index), N nếu không                                          | Tất cả              |
| NESTED                   |          | VARCHAR2(3)    | YES nếu là nested table, NO nếu không                                                          | Tất cả              |
| BUFFER_POOL              |          | VARCHAR2(7)    | Default buffer pool: DEFAULT, KEEP, hoặc RECYCLE                                               | Tất cả              |
| FLASH_CACHE              |          | VARCHAR2(7)    | Database Smart Flash Cache hint (Exadata)                                                      | 11g+                |
| CELL_FLASH_CACHE         |          | VARCHAR2(7)    | Cell flash cache hint (Exadata Storage Server)                                                 | 11g+                |
| ROW_MOVEMENT             |          | VARCHAR2(8)    | ENABLED/DISABLED - row movement cho partitioned tables                                         | Tất cả              |
| GLOBAL_STATS             |          | VARCHAR2(3)    | YES nếu statistics được gather cho toàn table, NO nếu từ partitions                            | Tất cả              |
| USER_STATS               |          | VARCHAR2(3)    | YES nếu statistics được set bởi user, NO nếu Oracle gather                                     | Tất cả              |
| DURATION                 |          | VARCHAR2(15)   | Duration của temporary table: SYS$SESSION hoặc SYS$TRANSACTION                                 | Tất cả              |
| SKIP_CORRUPT             |          | VARCHAR2(8)    | ENABLED/DISABLED - skip corrupted blocks khi scan                                              | Tất cả              |
| MONITORING               |          | VARCHAR2(3)    | YES/NO - monitoring modification cho table (deprecated từ 12c)                                 | Tất cả (deprecated 12c+)|
| CLUSTER_OWNER            |          | VARCHAR2(128)  | Owner của cluster nếu table thuộc cluster                                                      | Tất cả              |
| DEPENDENCIES             |          | VARCHAR2(8)    | ENABLED/DISABLED - row-level dependency tracking                                               | 12c+                |
| COMPRESSION              |          | VARCHAR2(8)    | ENABLED/DISABLED - table compression                                                           | 11g+                |
| COMPRESS_FOR             |          | VARCHAR2(30)   | BASIC, OLTP, QUERY LOW/HIGH, ARCHIVE LOW/HIGH - compression type                               | 11g+                |
| DROPPED                  |          | VARCHAR2(3)    | YES nếu table trong recycle bin, NO nếu không                                                  | 10g+                |
| READ_ONLY                |          | VARCHAR2(3)    | YES nếu table read-only, NO nếu read-write                                                     | 11g+                |
| SEGMENT_CREATED          |          | VARCHAR2(3)    | YES nếu segment đã tạo, NO nếu deferred segment creation                                       | 11gR2+              |
| RESULT_CACHE             |          | VARCHAR2(7)    | DEFAULT, FORCE, MANUAL - result cache mode                                                     | 11gR2+              |
| CLUSTERING               |          | VARCHAR2(3)    | YES nếu attribute clustering enabled, NO nếu không                                             | 12c+                |
| ACTIVITY_TRACKING        |          | VARCHAR2(23)   | Loại activity tracking: ON, OFF, etc.                                                          | 12c+                |
| DML_TIMESTAMP            |          | VARCHAR2(25)   | SCN hoặc timestamp của DML cuối cùng (dùng cho heat maps)                                      | 12c+                |
| HAS_IDENTITY             |          | VARCHAR2(3)    | YES nếu table có identity column, NO nếu không                                                 | 12c+                |
| CONTAINER_DATA           |          | VARCHAR2(3)    | YES nếu container data object, NO nếu không                                                    | 12c+                |
| INMEMORY                 |          | VARCHAR2(8)    | ENABLED/DISABLED - In-Memory Column Store                                                      | 12c+                |
| INMEMORY_PRIORITY        |          | VARCHAR2(8)    | NONE, LOW, MEDIUM, HIGH, CRITICAL - In-Memory priority                                         | 12c+                |
| INMEMORY_DISTRIBUTE      |          | VARCHAR2(15)   | AUTO, BY ROWID RANGE, etc. - In-Memory distribution (RAC)                                      | 12c+                |
| INMEMORY_COMPRESSION     |          | VARCHAR2(17)   | NO MEMCOMPRESS, FOR DML, FOR QUERY LOW/HIGH, FOR CAPACITY LOW/HIGH                             | 12c+                |
| INMEMORY_DUPLICATE       |          | VARCHAR2(13)   | NO DUPLICATE, DUPLICATE, DUPLICATE ALL - In-Memory duplicate (RAC)                             | 12c+                |
| DEFAULT_COLLATION        |          | VARCHAR2(100)  | Default collation cho table                                                                    | 12.2+               |
| DUPLICATED               |          | VARCHAR2(1)    | Y nếu table duplicated trên all shards, N nếu không                                            | 12.2+               |
| SHARDED                  |          | VARCHAR2(1)    | Y nếu sharded table, N nếu không                                                               | 12.2+               |
| EXTERNALLY_SHARDED       |          | VARCHAR2(1)    | Y nếu externally sharded, N nếu không                                                          | 12.2+               |
| EXTERNALLY_DUPLICATED    |          | VARCHAR2(1)    | Y nếu externally duplicated, N nếu không                                                       | 12.2+               |
| EXTERNAL                 |          | VARCHAR2(3)    | YES nếu external table, NO nếu không                                                           | Tất cả              |
| HYBRID                   |          | VARCHAR2(3)    | YES nếu hybrid partitioned table, NO nếu không                                                 | 18c+                |
| CELLMEMORY               |          | VARCHAR2(24)   | Exadata cell memory setting                                                                    | 12c+                |
| CONTAINERS_DEFAULT       |          | VARCHAR2(3)    | YES nếu containers default, NO nếu không                                                       | 12c+                |
| CONTAINER_MAP            |          | VARCHAR2(3)    | YES nếu container map object, NO nếu không                                                     | 12c+                |
| EXTENDED_DATA_LINK       |          | VARCHAR2(3)    | YES nếu extended data link, NO nếu không                                                       | 12c+                |
| EXTENDED_DATA_LINK_MAP   |          | VARCHAR2(3)    | YES nếu extended data link map, NO nếu không                                                   | 12c+                |
| INMEMORY_SERVICE         |          | VARCHAR2(12)   | DEFAULT, NONE, ALL, USER_DEFINED - In-Memory service                                           | 18c+                |
| INMEMORY_SERVICE_NAME    |          | VARCHAR2(1000) | Service name cho In-Memory population                                                          | 18c+                |
| CONTAINER_MAP_OBJECT     |          | VARCHAR2(3)    | YES nếu container map object, NO nếu không                                                     | 12c+                |
| MEMOPTIMIZE_READ         |          | VARCHAR2(8)    | ENABLED/DISABLED - memoptimize for read (Fast Ingest)                                          | 18c+                |
| MEMOPTIMIZE_WRITE        |          | VARCHAR2(8)    | ENABLED/DISABLED - memoptimize for write (Fast Ingest)                                         | 18c+                |
| HAS_SENSITIVE_COLUMN     |          | VARCHAR2(3)    | YES nếu có sensitive column (Data Redaction), NO nếu không                                     | 19c+                |
| ADMIT_NULL               |          | VARCHAR2(3)    | YES nếu admit null enabled (Blockchain Tables), NO nếu không                                   | 21c                 |
| DATA_LINK_DML_ENABLED    |          | VARCHAR2(3)    | YES nếu data link DML enabled, NO nếu không                                                    | 19c+                |
| LOGICAL_REPLICATION      |          | VARCHAR2(8)    | ENABLED/DISABLED - logical replication                                                         | 21c                 |

## DBA_ROLES
| Name                | Null?    | Type           | Ý nghĩa                                                                           | Phiên bản xuất hiện |
|---------------------|----------|----------------|-----------------------------------------------------------------------------------|---------------------|
| ROLE                | NOT NULL | VARCHAR2(128)  | Tên của role                                                                      | Tất cả              |
| ROLE_ID             | NOT NULL | NUMBER         | Unique identifier của role                                                        | Tất cả              |
| PASSWORD_REQUIRED   |          | VARCHAR2(8)    | YES nếu role yêu cầu password, NO nếu không                                       | Tất cả              |
| AUTHENTICATION_TYPE |          | VARCHAR2(11)   | NONE, PASSWORD, EXTERNAL, GLOBAL, APPLICATION - loại authentication               | 10g+                |
| COMMON              |          | VARCHAR2(3)    | YES nếu là common role (CDB), NO nếu local                                        | 12c+                |
| ORACLE_MAINTAINED   |          | VARCHAR2(1)    | Y nếu được Oracle maintain (system roles), N nếu user-created                     | 12c+                |
| INHERITED           |          | VARCHAR2(3)    | YES nếu role được kế thừa từ parent container, NO nếu không                       | 12c+                |
| IMPLICIT            |          | VARCHAR2(3)    | YES nếu role được tạo implicitly, NO nếu explicit                                 | 12c+                |
| EXTERNAL_NAME       |          | VARCHAR2(4000) | External name cho globally authorized role (LDAP, Kerberos, etc.)                 | Tất cả              |

## ALL_TRIGGERS
| Name             | Null? | Type          | Ý nghĩa                                                                                      | Phiên bản xuất hiện |
|------------------|-------|---------------|----------------------------------------------------------------------------------------------|---------------------|
| OWNER            |       | VARCHAR2(128) | Schema owner của trigger                                                                     | Tất cả              |
| TRIGGER_NAME     |       | VARCHAR2(128) | Tên của trigger                                                                              | Tất cả              |
| TRIGGER_TYPE     |       | VARCHAR2(16)  | BEFORE STATEMENT, AFTER EACH ROW, INSTEAD OF, COMPOUND, etc. - loại và timing của trigger    | Tất cả              |
| TRIGGERING_EVENT |       | VARCHAR2(246) | DML statement (INSERT, UPDATE, DELETE) hoặc system event firing trigger                      | Tất cả              |
| TABLE_OWNER      |       | VARCHAR2(128) | Owner của table/view trigger được define                                                     | Tất cả              |
| BASE_OBJECT_TYPE |       | VARCHAR2(18)  | TABLE, VIEW, SCHEMA, DATABASE - loại base object                                             | Tất cả              |
| TABLE_NAME       |       | VARCHAR2(128) | Tên của table/view trigger được define (NULL cho database/schema triggers)                   | Tất cả              |
| COLUMN_NAME      |       | VARCHAR2(4000)| Column name nếu trigger có UPDATE OF clause                                                  | Tất cả              |
| REFERENCING_NAMES|       | VARCHAR2(422) | REFERENCING clause (OLD AS, NEW AS names)                                                    | Tất cả              |
| WHEN_CLAUSE      |       | VARCHAR2(4000)| WHEN condition nếu có                                                                        | Tất cả              |
| STATUS           |       | VARCHAR2(8)   | ENABLED hoặc DISABLED - trạng thái của trigger                                               | Tất cả              |
| DESCRIPTION      |       | VARCHAR2(4000)| Full description của trigger (deprecated, dùng TRIGGER_TYPE + TRIGGERING_EVENT)              | Tất cả (deprecated) |
| ACTION_TYPE      |       | VARCHAR2(11)  | CALL hoặc PL/SQL - loại action                                                               | Tất cả              |
| TRIGGER_BODY     |       | LONG          | PL/SQL code của trigger body                                                                 | Tất cả              |
| CROSSEDITION     |       | VARCHAR2(7)   | FORWARD, REVERSE, NULL - cross-edition trigger direction (Edition-Based Redefinition)        | 11gR2+              |
| BEFORE_STATEMENT |       | VARCHAR2(3)   | YES nếu có BEFORE STATEMENT timing point, NO nếu không (compound triggers)                   | 11g+                |
| BEFORE_ROW       |       | VARCHAR2(3)   | YES nếu có BEFORE EACH ROW timing point, NO nếu không (compound triggers)                    | 11g+                |
| AFTER_ROW        |       | VARCHAR2(3)   | YES nếu có AFTER EACH ROW timing point, NO nếu không (compound triggers)                     | 11g+                |
| AFTER_STATEMENT  |       | VARCHAR2(3)   | YES nếu có AFTER STATEMENT timing point, NO nếu không (compound triggers)                    | 11g+                |
| INSTEAD_OF_ROW   |       | VARCHAR2(3)   | YES nếu là INSTEAD OF trigger, NO nếu không                                                  | 11g+                |
| FIRE_ONCE        |       | VARCHAR2(3)   | YES nếu trigger fires once per transaction (deprecated), NO nếu không                        | Tất cả (deprecated) |
| APPLY_SERVER_ONLY|       | VARCHAR2(3)   | YES nếu trigger chỉ fire trên apply server (Streams/GoldenGate), NO nếu không                | 11g+                |

## ALL_TYPES
| Name             | Null? | Type          | Ý nghĩa                                                                          | Phiên bản xuất hiện |
|------------------|-------|---------------|----------------------------------------------------------------------------------|---------------------|
| OWNER            |       | VARCHAR2(128) | Schema owner của type                                                            | Tất cả (8i+)        |
| TYPE_NAME        |       | VARCHAR2(128) | Tên của type                                                                     | Tất cả (8i+)        |
| TYPE_OID         |       | RAW(16)       | Object identifier của type                                                       | Tất cả (8i+)        |
| TYPECODE         |       | VARCHAR2(128) | Typecode của type: OBJECT, COLLECTION, VARRAY, TABLE, etc.                       | Tất cả (8i+)        |
| ATTRIBUTES       |       | NUMBER        | Số attributes trong type (cho object types)                                      | Tất cả (8i+)        |
| METHODS          |       | NUMBER        | Số methods trong type (cho object types)                                         | Tất cả (8i+)        |
| PREDEFINED       |       | VARCHAR2(3)   | YES nếu là predefined type (built-in), NO nếu user-defined                       | Tất cả (8i+)        |
| INCOMPLETE       |       | VARCHAR2(3)   | YES nếu type incomplete (forward declaration), NO nếu complete                   | Tất cả (8i+)        |
| FINAL            |       | VARCHAR2(3)   | YES nếu type là final (không thể subtype), NO nếu có thể subtype                 | Tất cả (8i+)        |
| INSTANTIABLE     |       | VARCHAR2(3)   | YES nếu type có thể instantiate, NO nếu abstract                                 | Tất cả (8i+)        |
| PERSISTABLE      |       | VARCHAR2(3)   | YES nếu type có thể persist (store trong table), NO nếu transient                | 10g+                |
| SUPERTYPE_OWNER  |       | VARCHAR2(128) | Owner của supertype (cho type inheritance)                                       | Tất cả (8i+)        |
| SUPERTYPE_NAME   |       | VARCHAR2(128) | Name của supertype (cho type inheritance)                                        | Tất cả (8i+)        |
| LOCAL_ATTRIBUTES |       | NUMBER        | Số attributes được define locally (không kế thừa)                                | Tất cả (8i+)        |
| LOCAL_METHODS    |       | NUMBER        | Số methods được define locally (không kế thừa)                                   | Tất cả (8i+)        |
| TYPEID           |       | RAW(16)       | Type identifier (unique across types in type hierarchy)                          | Tất cả (8i+)        |

## DBA_ROLLBACK_SEGS
| Name           | Null?    | Type         | Ý nghĩa                                                                          | Phiên bản xuất hiện |
|----------------|----------|--------------|----------------------------------------------------------------------------------|---------------------|
| SEGMENT_NAME   | NOT NULL | VARCHAR2(30) | Tên của rollback/undo segment                                                    | Tất cả              |
| OWNER          |          | VARCHAR2(6)  | Owner của rollback segment (PUBLIC hoặc SYS)                                     | Tất cả              |
| TABLESPACE_NAME| NOT NULL | VARCHAR2(30) | Tablespace chứa rollback/undo segment                                            | Tất cả              |
| SEGMENT_ID     | NOT NULL | NUMBER       | Segment ID number                                                                | Tất cả              |
| FILE_ID        | NOT NULL | NUMBER       | File identifier của file chứa segment header                                     | Tất cả              |
| BLOCK_ID       | NOT NULL | NUMBER       | Block number của segment header                                                  | Tất cả              |
| INITIAL_EXTENT |          | NUMBER       | Kích thước extent đầu tiên (bytes)                                               | Tất cả              |
| NEXT_EXTENT    |          | NUMBER       | Kích thước extent tiếp theo (bytes)                                              | Tất cả              |
| MIN_EXTENTS    | NOT NULL | NUMBER       | Số extent tối thiểu                                                              | Tất cả              |
| MAX_EXTENTS    | NOT NULL | NUMBER       | Số extent tối đa                                                                 | Tất cả              |
| PCT_INCREASE   |          | NUMBER       | Phần trăm tăng kích thước extent (deprecated cho undo tablespaces)               | Tất cả              |
| STATUS         |          | VARCHAR2(16) | ONLINE, OFFLINE, NEEDS RECOVERY, PARTLY AVAILABLE, INVALID - trạng thái segment  | Tất cả              |
| INSTANCE_NUM   |          | VARCHAR2(40) | Instance number sở hữu segment (RAC), NULL cho public segments                   | Tất cả              |
| RELATIVE_FNO   | NOT NULL | NUMBER       | Relative file number trong tablespace                                            | Tất cả              |

## DBA_TABLESPACES
| Name                      | Null?    | Type           | Ý nghĩa                                                                              | Phiên bản xuất hiện |
|---------------------------|----------|----------------|--------------------------------------------------------------------------------------|---------------------|
| TABLESPACE_NAME           | NOT NULL | VARCHAR2(30)   | Tên của tablespace                                                                   | Tất cả              |
| BLOCK_SIZE                | NOT NULL | NUMBER         | Block size của tablespace (bytes)                                                    | Tất cả              |
| INITIAL_EXTENT            |          | NUMBER         | Default initial extent size (bytes) cho objects                                      | Tất cả              |
| NEXT_EXTENT               |          | NUMBER         | Default next extent size (bytes)                                                     | Tất cả              |
| MIN_EXTENTS               | NOT NULL | NUMBER         | Default minimum số extents                                                           | Tất cả              |
| MAX_EXTENTS               |          | NUMBER         | Default maximum số extents (NULL = unlimited trong LMT)                              | Tất cả              |
| MAX_SIZE                  |          | NUMBER         | Maximum size cho autoextend datafiles                                                | Tất cả              |
| PCT_INCREASE              |          | NUMBER         | Default percent increase trong extent size (deprecated cho LMT)                      | Tất cả              |
| MIN_EXTLEN                |          | NUMBER         | Minimum extent length                                                                | Tất cả              |
| STATUS                    |          | VARCHAR2(9)    | ONLINE, OFFLINE, READ ONLY - trạng thái tablespace                                   | Tất cả              |
| CONTENTS                  |          | VARCHAR2(21)   | PERMANENT, TEMPORARY, UNDO - loại content                                            | Tất cả              |
| LOGGING                   |          | VARCHAR2(9)    | LOGGING hoặc NOLOGGING - default logging attribute                                   | Tất cả              |
| FORCE_LOGGING             |          | VARCHAR2(3)    | YES nếu force logging mode, NO nếu không                                             | 9i+                 |
| EXTENT_MANAGEMENT         |          | VARCHAR2(10)   | DICTIONARY hoặc LOCAL - extent management method                                     | 9i+                 |
| ALLOCATION_TYPE           |          | VARCHAR2(9)    | SYSTEM (system-managed), UNIFORM (uniform extents), USER (dictionary-managed)        | 9i+                 |
| PLUGGED_IN                |          | VARCHAR2(3)    | YES nếu tablespace được plug in từ backup, NO nếu không                              | 10g+                |
| SEGMENT_SPACE_MANAGEMENT  |          | VARCHAR2(6)    | MANUAL (freelists) hoặc AUTO (bitmap) - segment space management                     | 9i+                 |
| DEF_TAB_COMPRESSION       |          | VARCHAR2(8)    | ENABLED/DISABLED - default table compression                                         | 11g+                |
| RETENTION                 |          | VARCHAR2(11)   | GUARANTEE, NOGUARANTEE, NOT APPLY - undo retention guarantee                         | 9i+                 |
| BIGFILE                   |          | VARCHAR2(3)    | YES nếu bigfile tablespace, NO nếu smallfile                                         | 10g+                |
| PREDICATE_EVALUATION      |          | VARCHAR2(7)    | HOST (database evaluates), STORAGE (storage server evaluates) - Exadata              | 11g+                |
| ENCRYPTED                 |          | VARCHAR2(3)    | YES nếu tablespace encrypted, NO nếu không (TDE)                                     | 11g+                |
| COMPRESS_FOR              |          | VARCHAR2(30)   | BASIC, OLTP, QUERY LOW/HIGH, ARCHIVE LOW/HIGH - default compression type             | 11g+                |
| DEF_INMEMORY              |          | VARCHAR2(8)    | ENABLED/DISABLED - default In-Memory Column Store                                    | 12c+                |
| DEF_INMEMORY_PRIORITY     |          | VARCHAR2(8)    | NONE, LOW, MEDIUM, HIGH, CRITICAL - default In-Memory priority                       | 12c+                |
| DEF_INMEMORY_DISTRIBUTE   |          | VARCHAR2(15)   | AUTO, BY ROWID RANGE, etc. - default In-Memory distribution                          | 12c+                |
| DEF_INMEMORY_COMPRESSION  |          | VARCHAR2(17)   | NO MEMCOMPRESS, FOR DML, FOR QUERY, FOR CAPACITY - default In-Memory compression     | 12c+                |
| DEF_INMEMORY_DUPLICATE    |          | VARCHAR2(13)   | NO DUPLICATE, DUPLICATE, DUPLICATE ALL - default In-Memory duplicate                 | 12c+                |
| SHARED                    |          | VARCHAR2(13)   | SHARED, LOCAL ON ALL, LOCAL ON LEAF - tablespace sharing mode (Sharding)             | 12.2+               |
| DEF_INDEX_COMPRESSION     |          | VARCHAR2(8)    | ENABLED/DISABLED - default index compression                                         | 11g+                |
| INDEX_COMPRESS_FOR        |          | VARCHAR2(13)   | ADVANCED LOW/HIGH - default index compression type                                   | 11g+                |
| DEF_CELLMEMORY            |          | VARCHAR2(14)   | Exadata cell memory setting                                                          | 12c+                |
| DEF_INMEMORY_SERVICE      |          | VARCHAR2(12)   | DEFAULT, NONE, ALL, USER_DEFINED - default In-Memory service                         | 18c+                |
| DEF_INMEMORY_SERVICE_NAME |          | VARCHAR2(1000) | Service name cho In-Memory population                                                | 18c+                |
| LOST_WRITE_PROTECT        |          | VARCHAR2(7)    | ENABLED, DISABLED, SUSPEND, TYPICAL, REMOVE - lost write protection                  | 12c+                |
| CHUNK_TABLESPACE          |          | VARCHAR2(1)    | Y nếu chunk tablespace (SecureFiles), N nếu không                                    | 11gR2+              |

## ALL_VIEWS
| Name                     | Null?    | Type          | Ý nghĩa                                                                                        | Phiên bản xuất hiện |
|--------------------------|----------|---------------|------------------------------------------------------------------------------------------------|---------------------|
| OWNER                    | NOT NULL | VARCHAR2(128) | Schema owner của view                                                                          | Tất cả              |
| VIEW_NAME                | NOT NULL | VARCHAR2(128) | Tên của view                                                                                   | Tất cả              |
| TEXT_LENGTH              |          | NUMBER        | Length của view text trong cột TEXT (bytes)                                                    | Tất cả              |
| TEXT                     |          | LONG          | View query text (LONG datatype, deprecated)                                                    | Tất cả              |
| TEXT_VC                  |          | VARCHAR2(4000)| View query text trong VARCHAR2 (truncated nếu > 4000 chars)                                    | 10g+                |
| TYPE_TEXT_LENGTH         |          | NUMBER        | Length của type clause text                                                                    | Tất cả              |
| TYPE_TEXT                |          | VARCHAR2(4000)| Text của type clause cho object views                                                          | Tất cả              |
| OID_TEXT_LENGTH          |          | NUMBER        | Length của WITH OID clause                                                                     | Tất cả              |
| OID_TEXT                 |          | VARCHAR2(4000)| Text của WITH OID clause cho object views                                                      | Tất cả              |
| VIEW_TYPE_OWNER          |          | VARCHAR2(128) | Owner của type nếu là typed view (object view)                                                 | Tất cả              |
| VIEW_TYPE                |          | VARCHAR2(128) | Type name nếu là typed view (object view)                                                      | Tất cả              |
| SUPERVIEW_NAME           |          | VARCHAR2(128) | Name của superview (parent view trong view hierarchy)                                          | Tất cả              |
| EDITIONING_VIEW          |          | VARCHAR2(1)   | Y nếu là editioning view (Edition-Based Redefinition), N nếu không                             | 11gR2+              |
| READ_ONLY                |          | VARCHAR2(1)   | Y nếu view read-only (WITH READ ONLY), N nếu updatable                                         | 11g+                |
| CONTAINER_DATA           |          | VARCHAR2(1)   | Y nếu container data object (queries across PDBs), N nếu không                                 | 12c+                |
| BEQUEATH                 |          | VARCHAR2(12)  | CURRENT_USER hoặc DEFINER - bequeath clause cho view definer's rights                          | 12c+                |
| ORIGIN_CON_ID            |          | NUMBER        | Container ID nơi view được tạo (Multitenant)                                                   | 12c+                |
| DEFAULT_COLLATION        |          | VARCHAR2(100) | Default collation cho view                                                                     | 12.2+               |
| CONTAINERS_DEFAULT       |          | VARCHAR2(3)   | YES nếu containers clause default, NO nếu không                                                | 12c+                |
| CONTAINER_MAP            |          | VARCHAR2(3)   | YES nếu container map view, NO nếu không                                                       | 12c+                |
| EXTENDED_DATA_LINK       |          | VARCHAR2(3)   | YES nếu extended data link, NO nếu không                                                       | 12c+                |
| EXTENDED_DATA_LINK_MAP   |          | VARCHAR2(3)   | YES nếu extended data link map, NO nếu không                                                   | 12c+                |
| HAS_SENSITIVE_COLUMN     |          | VARCHAR2(3)   | YES nếu view có sensitive column (Data Redaction), NO nếu không                                | 19c+                |
| ADMIT_NULL               |          | VARCHAR2(3)   | YES nếu view admits null predicate (optimization hint), NO nếu không                           | 19c+                |
| PDB_LOCAL_ONLY           |          | VARCHAR2(3)   | YES nếu view chỉ truy cập local PDB data, NO nếu cross-container                               | 21c                 |

## DBA_USERS
| Name                         | Null?    | Type                        | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|------------------------------|----------|-----------------------------|----------------------------------------------------------------------------------------|---------------------|
| USERNAME                     | NOT NULL | VARCHAR2(128)               | Username của database user                                                             | Tất cả              |
| USER_ID                      | NOT NULL | NUMBER                      | User ID number (unique identifier)                                                     | Tất cả              |
| PASSWORD                     |          | VARCHAR2(4000)              | Encrypted password (deprecated, NULL trong modern versions vì security)                | Tất cả              |
| ACCOUNT_STATUS               | NOT NULL | VARCHAR2(32)                | OPEN, LOCKED, EXPIRED, EXPIRED & LOCKED, EXPIRED(GRACE), LOCKED(TIMED), etc.           | Tất cả              |
| LOCK_DATE                    |          | DATE                        | Date khi account bị lock                                                               | Tất cả              |
| EXPIRY_DATE                  |          | DATE                        | Date khi password hết hạn                                                              | Tất cả              |
| DEFAULT_TABLESPACE           | NOT NULL | VARCHAR2(30)                | Default tablespace cho user's objects                                                  | Tất cả              |
| TEMPORARY_TABLESPACE         | NOT NULL | VARCHAR2(30)                | Tablespace cho temporary segments                                                      | Tất cả              |
| LOCAL_TEMP_TABLESPACE        |          | VARCHAR2(30)                | Local temporary tablespace (container-specific)                                        | 12c+                |
| CREATED                      | NOT NULL | DATE                        | Date khi user được tạo                                                                 | Tất cả              |
| PROFILE                      | NOT NULL | VARCHAR2(128)               | Resource profile name được assigned cho user                                           | Tất cả              |
| INITIAL_RSRC_CONSUMER_GROUP  |          | VARCHAR2(128)               | Initial resource consumer group cho user (Resource Manager)                            | 10g+                |
| EXTERNAL_NAME                |          | VARCHAR2(4000)              | External name của user (cho enterprise users, LDAP)                                    | Tất cả              |
| PASSWORD_VERSIONS            |          | VARCHAR2(17)                | List các password versions: 10G, 11G, 12C (để track old password hashes)               | 11g+                |
| EDITIONS_ENABLED             |          | VARCHAR2(1)                 | Y nếu editions enabled cho schema, N nếu không (Edition-Based Redefinition)            | 11gR2+              |
| AUTHENTICATION_TYPE          |          | VARCHAR2(8)                 | PASSWORD, EXTERNAL, GLOBAL, NONE - authentication method                               | 11g+                |
| PROXY_ONLY_CONNECT           |          | VARCHAR2(1)                 | Y nếu user chỉ có thể connect qua proxy, N nếu direct connect allowed                  | 11g+                |
| COMMON                       |          | VARCHAR2(3)                 | YES nếu common user (CDB-level), NO nếu local user (PDB-level)                         | 12c+                |
| LAST_LOGIN                   |          | TIMESTAMP(9) WITH TIME ZONE | Timestamp của lần login gần nhất                                                       | 12c+                |
| ORACLE_MAINTAINED            |          | VARCHAR2(1)                 | Y nếu Oracle-maintained user (như SYS, SYSTEM), N nếu user-created                     | 12c+                |
| INHERITED                    |          | VARCHAR2(3)                 | YES nếu user được kế thừa từ CDB, NO nếu không                                         | 12c+                |
| DEFAULT_COLLATION            |          | VARCHAR2(100)               | Default collation cho user's schema                                                    | 12.2+               |
| IMPLICIT                     |          | VARCHAR2(3)                 | YES nếu user được tạo implicitly, NO nếu explicit                                      | 12c+                |
| ALL_SHARD                    |          | VARCHAR2(3)                 | YES nếu user accessible trên all shards, NO nếu không                                  | 18c+                |
| EXTERNAL_SHARD               |          | VARCHAR2(3)                 | YES nếu external shard user, NO nếu không                                              | 18c+                |
| PASSWORD_CHANGE_DATE         |          | DATE                        | Date khi password được thay đổi lần cuối                                               | 18c+                |
| MANDATORY_PROFILE_VIOLATION  |          | VARCHAR2(3)                 | YES nếu vi phạm mandatory profile, NO nếu compliant                                    | 19c+                |

## DBA_PDBS
| Name                      | Null?    | Type                        | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|---------------------------|----------|-----------------------------|----------------------------------------------------------------------------------------|---------------------|
| PDB_ID                    | NOT NULL | NUMBER                      | Unique PDB identifier (3 trở lên, 1=CDB$ROOT, 2=PDB$SEED)                              | 12c+                |
| PDB_NAME                  | NOT NULL | VARCHAR2(128)               | Tên của PDB                                                                            | 12c+                |
| DBID                      | NOT NULL | NUMBER                      | Database ID của PDB (unique trong CDB)                                                 | 12c+                |
| CON_UID                   | NOT NULL | NUMBER                      | Container unique ID (không thay đổi qua unplug/plug operations)                        | 12c+                |
| GUID                      |          | RAW(16)                     | Globally Unique Identifier cho PDB                                                     | 12c+                |
| STATUS                    |          | VARCHAR2(10)                | NORMAL, NEW, UNPLUGGED - trạng thái của PDB                                            | 12c+                |
| CREATION_SCN              |          | NUMBER                      | SCN khi PDB được tạo                                                                   | 12c+                |
| VSN                       |          | NUMBER                      | Version number của PDB                                                                 | 12c+                |
| LOGGING                   |          | VARCHAR2(9)                 | LOGGING hoặc NOLOGGING - logging mode                                                  | 12c+                |
| FORCE_LOGGING             |          | VARCHAR2(39)                | YES, NO, hoặc STANDBY - force logging mode                                             | 12c+                |
| FORCE_NOLOGGING           |          | VARCHAR2(3)                 | YES nếu force nologging enabled, NO nếu không                                          | 18c+                |
| APPLICATION_ROOT          |          | VARCHAR2(3)                 | YES nếu là application root, NO nếu không (Application Containers)                     | 12c+                |
| APPLICATION_PDB           |          | VARCHAR2(3)                 | YES nếu là application PDB, NO nếu không                                               | 12c+                |
| APPLICATION_SEED          |          | VARCHAR2(3)                 | YES nếu là application seed, NO nếu không                                              | 12c+                |
| APPLICATION_ROOT_CON_ID   |          | NUMBER                      | Container ID của application root (nếu là app PDB)                                     | 12c+                |
| IS_PROXY_PDB              |          | VARCHAR2(3)                 | YES nếu là proxy PDB (remote reference), NO nếu local                                  | 12.2+               |
| CON_ID                    | NOT NULL | NUMBER                      | Container ID (giống PDB_ID)                                                            | 12c+                |
| UPGRADE_PRIORITY          |          | NUMBER                      | Priority order cho upgrades (1=highest)                                                | 12c+                |
| APPLICATION_CLONE         |          | VARCHAR2(3)                 | YES nếu PDB là application clone, NO nếu không                                         | 12.2+               |
| FOREIGN_CDB_DBID          |          | NUMBER                      | DBID của foreign CDB (cho proxy PDBs)                                                  | 12.2+               |
| UNPLUG_SCN                |          | NUMBER                      | SCN khi PDB được unplug                                                                | 12c+                |
| FOREIGN_PDB_ID            |          | NUMBER                      | PDB ID trong foreign CDB (cho proxy PDBs)                                              | 12.2+               |
| CREATION_TIME             | NOT NULL | DATE                        | Timestamp khi PDB được tạo                                                             | 12c+                |
| REFRESH_MODE              |          | VARCHAR2(6)                 | MANUAL, EVERY (auto refresh), NONE - refresh mode cho refreshable clone PDB            | 18c+                |
| REFRESH_INTERVAL          |          | NUMBER                      | Refresh interval (minutes) cho auto refresh                                            | 18c+                |
| TEMPLATE                  |          | VARCHAR2(3)                 | YES nếu là PDB template (SNAPSHOT COPY), NO nếu không                                  | 18c+                |
| LAST_REFRESH_SCN          |          | NUMBER                      | SCN của lần refresh cuối cho refreshable clone PDB                                     | 18c+                |
| TENANT_ID                 |          | VARCHAR2(32767)             | Tenant identifier (multi-tenant SaaS scenarios)                                        | 18c+                |
| SNAPSHOT_MODE             |          | VARCHAR2(6)                 | MANUAL, EVERY - snapshot mode cho snapshot copy PDBs                                   | 18c+                |
| SNAPSHOT_INTERVAL         |          | NUMBER                      | Snapshot interval (minutes) cho auto snapshot                                          | 18c+                |
| CREDENTIAL_NAME           |          | VARCHAR2(262)               | Credential name cho remote refresh operations                                          | 19c+                |
| LAST_REFRESH_TIME         |          | DATE                        | Timestamp của lần refresh cuối                                                         | 18c+                |
| CLOUD_IDENTITY            |          | VARCHAR2(4000)              | Cloud identity information (Oracle Cloud Infrastructure)                               | 19c+                |
| SOURCE_PDB_NAME           |          | VARCHAR2(128)               | Tên của source PDB cho clones/refreshable clones                                       | 18c+                |
| SOURCE_DB_LINK            |          | VARCHAR2(128)               | Database link đến source PDB cho remote clones                                         | 18c+                |

## ALL_HIERARCHIES
| Name             | Null?    | Type          | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|------------------|----------|---------------|----------------------------------------------------------------------------------------|---------------------|
| OWNER            | NOT NULL | VARCHAR2(128) | Schema owner của hierarchy                                                             | 12.2+               |
| HIER_NAME        | NOT NULL | VARCHAR2(128) | Tên của hierarchy                                                                      | 12.2+               |
| DIMENSION_OWNER  | NOT NULL | VARCHAR2(128) | Owner của attribute dimension mà hierarchy thuộc về                                    | 12.2+               |
| DIMENSION_NAME   | NOT NULL | VARCHAR2(128) | Tên của attribute dimension                                                            | 12.2+               |
| PARENT_ATTR      |          | VARCHAR2      | Parent attribute definition (định nghĩa mối quan hệ parent-child trong hierarchy)      | 12.2+               |
| COMPILE_STATE    |          | VARCHAR2(7)   | VALID hoặc INVALID - trạng thái compile của hierarchy                                  | 12.2+               |
| ORIGIN_CON_ID    |          | NUMBER        | Container ID nơi hierarchy được tạo (Multitenant)                                      | 12.2+               |

## ALL_ANALYTIC_VIEWS
| Name                         | Null?    | Type          | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|------------------------------|----------|---------------|----------------------------------------------------------------------------------------|---------------------|
| OWNER                        | NOT NULL | VARCHAR2(128) | Schema owner của analytic view                                                         | 12.2+               |
| ANALYTIC_VIEW_NAME           | NOT NULL | VARCHAR2(128) | Tên của analytic view                                                                  | 12.2+               |
| TABLE_OWNER                  | NOT NULL | VARCHAR2(128) | Owner của fact table                                                                   | 12.2+               |
| TABLE_NAME                   | NOT NULL | VARCHAR2(128) | Tên của fact table                                                                     | 12.2+               |
| TABLE_ALIAS                  |          | VARCHAR2(128) | Alias của fact table trong analytic view                                               | 12.2+               |
| IS_REMOTE                    |          | VARCHAR2(1)   | Y nếu fact table là remote (via db link), N nếu local                                  | 12.2+               |
| DEFAULT_AGGR                 |          | VARCHAR2(128) | Default aggregate function cho measures (SUM, AVG, MAX, MIN, COUNT, etc.)              | 12.2+               |
| DEFAULT_AGGR_GROUP_NAME      | NOT NULL | VARCHAR2(128) | Tên của default aggregate group                                                        | 12.2+               |
| DEFAULT_MEASURE              |          | VARCHAR2(128) | Tên của default measure (metric)                                                       | 12.2+               |
| COMPILE_STATE                |          | VARCHAR2(7)   | VALID hoặc INVALID - trạng thái compile của analytic view                              | 12.2+               |
| DYN_ALL_CACHE                |          | VARCHAR2(1)   | Y nếu dynamic ALL cache enabled, N nếu không                                           | 12.2+               |
| QUERY_TRANSFORM_ENABLED      |          | VARCHAR2(1)   | Y nếu query transformation enabled, N nếu không (optimizer rewrite)                    | 12.2+               |
| QUERY_TRANSFORM_RELY         |          | VARCHAR2(1)   | Y nếu rely on query transform constraints, N nếu không                                 | 12.2+               |
| ORIGIN_CON_ID                |          | NUMBER        | Container ID nơi analytic view được tạo (Multitenant)                                  | 12.2+               |

## ALL_ATTRIBUTE_DIMENSIONS
| Name                   | Null?    | Type          | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|------------------------|----------|---------------|----------------------------------------------------------------------------------------|---------------------|
| OWNER                  | NOT NULL | VARCHAR2(128) | Schema owner của attribute dimension                                                   | 12.2+               |
| DIMENSION_NAME         | NOT NULL | VARCHAR2(128) | Tên của attribute dimension                                                            | 12.2+               |
| DIMENSION_TYPE         |          | VARCHAR2(8)   | STANDARD hoặc TIME - loại dimension                                                    | 12.2+               |
| CACHE_STAR             | NOT NULL | VARCHAR2(12)  | CACHE STAR hoặc NO CACHE - cache strategy cho dimension members                        | 12.2+               |
| MAT_TABLE_OWNER        |          | VARCHAR2(128) | Owner của materialization table nếu có                                                 | 12.2+               |
| MAT_TABLE_NAME         |          | VARCHAR2(128) | Tên của materialization table (chứa pre-computed dimension data)                       | 12.2+               |
| ALL_MEMBER_NAME        |          | CLOB          | Name của ALL member trong dimension (aggregation level cao nhất)                       | 12.2+               |
| ALL_MEMBER_CAPTION     |          | CLOB          | Caption/display text cho ALL member                                                    | 12.2+               |
| ALL_MEMBER_DESCRIPTION |          | CLOB          | Description của ALL member                                                             | 12.2+               |
| COMPILE_STATE          |          | VARCHAR2(7)   | VALID hoặc INVALID - trạng thái compile của dimension                                  | 12.2+               |
| ORIGIN_CON_ID          |          | NUMBER        | Container ID nơi dimension được tạo (Multitenant)                                      | 12.2+               |

## DBA_LOCKDOWN_PROFILES
| Name          | Null?    | Type           | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|---------------|----------|----------------|----------------------------------------------------------------------------------------|---------------------|
| PROFILE_NAME  | NOT NULL | VARCHAR2(128)  | Tên của lockdown profile                                                               | 12c+                |
| RULE_TYPE     |          | VARCHAR2(128)  | STATEMENT, OPTION, COMMON_USER, NETWORK_ACCESS - loại rule                             | 12c+                |
| RULE          |          | VARCHAR2(128)  | Rule name: SQL statement hoặc feature (ALTER SYSTEM, CREATE PROCEDURE, etc.)           | 12c+                |
| CLAUSE        |          | VARCHAR2(128)  | Clause của statement (SET CONTAINER, EXTERNAL, TABLESPACE, etc.)                       | 12c+                |
| CLAUSE_OPTION |          | VARCHAR2(128)  | Option của clause (nếu có)                                                             | 12c+                |
| OPTION_VALUE  |          | VARCHAR2(4000) | Value cho option (nếu có)                                                              | 12c+                |
| MIN_VALUE     |          | VARCHAR2(4000) | Minimum value allowed (cho numeric restrictions)                                       | 12c+                |
| MAX_VALUE     |          | VARCHAR2(4000) | Maximum value allowed (cho numeric restrictions)                                       | 12c+                |
| LIST          |          | VARCHAR2(4000) | List of allowed values (comma-separated)                                               | 12c+                |
| STATUS        |          | VARCHAR2(7)    | ENABLE hoặc DISABLE - trạng thái của rule                                              | 12c+                |
| USERS         |          | VARCHAR2(6)    | ALL, COMMON, LOCAL - scope của users affected by rule                                  | 12c+                |
| EXCEPT_USERS  |          | CLOB           | List of users excluded từ rule (comma-separated)                                       | 12c+                |

## ALL_ZONEMAPS
| Name                | Null? | Type          | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|---------------------|-------|---------------|----------------------------------------------------------------------------------------|---------------------|
| OWNER               |       | VARCHAR2(128) | Schema owner của zone map                                                              | 12c+                |
| ZONEMAP_NAME        |       | VARCHAR2(128) | Tên của zone map                                                                       | 12c+                |
| FACT_OWNER          |       | VARCHAR2(128) | Owner của fact table                                                                   | 12c+                |
| FACT_TABLE          |       | VARCHAR2(128) | Tên của fact table được zone map                                                       | 12c+                |
| SCALE               |       | NUMBER        | Scale factor cho zone map (số zones per partition)                                     | 12c+                |
| HIERARCHICAL        |       | VARCHAR2(12)  | HIERARCHICAL hoặc FLAT - structure của zone map                                        | 12c+                |
| WITH_CLUSTERING     |       | VARCHAR2(15)  | YES nếu zone map được tạo với attribute clustering, NO nếu không                       | 12c+                |
| AUTOMATIC           |       | VARCHAR2(9)   | AUTOMATIC hoặc MANUAL - maintenance mode                                               | 12c+                |
| QUERY               |       | LONG          | Query text định nghĩa zone map (LONG datatype)                                         | 12c+                |
| QUERY_LEN           |       | NUMBER(38)    | Length của query text (bytes)                                                          | 12c+                |
| PRUNING             |       | VARCHAR2(8)   | ENABLED hoặc DISABLED - pruning capability của zone map                                | 12c+                |
| REFRESH_MODE        |       | VARCHAR2(17)  | ON DEMAND, ON COMMIT, ON STATEMENT - refresh mode                                      | 12c+                |
| REFRESH_METHOD      |       | VARCHAR2(14)  | COMPLETE, FAST - refresh method                                                        | 12c+                |
| LAST_REFRESH_METHOD |       | VARCHAR2(19)  | Method được dùng cho lần refresh gần nhất                                              | 12c+                |
| LAST_REFRESH_TIME   |       | TIMESTAMP(9)  | Timestamp của lần refresh cuối                                                         | 12c+                |
| INVALID             |       | VARCHAR2(7)   | INVALID nếu zone map invalid, VALID nếu không                                          | 12c+                |
| STALE               |       | VARCHAR2(7)   | STALE nếu data đã thay đổi, FRESH nếu up-to-date                                       | 12c+                |
| PARTLY_STALE        |       | VARCHAR2(12)  | PARTLY STALE nếu một số zones stale, NO nếu toàn bộ fresh/stale                        | 12c+                |
| INCOMPLETE          |       | VARCHAR2(12)  | INCOMPLETE nếu zone map chưa fully created, COMPLETE nếu không                         | 12c+                |
| UNUSABLE            |       | VARCHAR2(8)   | UNUSABLE nếu không thể dùng, USABLE nếu có thể dùng                                    | 12c+                |
| COMPILE_STATE       |       | VARCHAR2(19)  | VALID, NEEDS_COMPILE, ERROR - trạng thái compile                                       | 12c+                |

## V$DATABASE
| Name                            | Null? | Type          | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|---------------------------------|-------|---------------|----------------------------------------------------------------------------------------|---------------------|
| DBID                            |       | NUMBER        | Database identifier (unique trong Oracle environment)                                  | Tất cả              |
| NAME                            |       | VARCHAR2(9)   | Database name (DB_NAME parameter)                                                      | Tất cả              |
| CREATED                         |       | DATE          | Date khi database được tạo                                                             | Tất cả              |
| RESETLOGS_CHANGE#               |       | NUMBER        | SCN của RESETLOGS operation gần nhất                                                   | Tất cả              |
| RESETLOGS_TIME                  |       | DATE          | Timestamp của RESETLOGS operation gần nhất                                             | Tất cả              |
| PRIOR_RESETLOGS_CHANGE#         |       | NUMBER        | SCN của RESETLOGS operation trước đó                                                   | Tất cả              |
| PRIOR_RESETLOGS_TIME            |       | DATE          | Timestamp của RESETLOGS operation trước đó                                             | Tất cả              |
| LOG_MODE                        |       | VARCHAR2(12)  | ARCHIVELOG hoặc NOARCHIVELOG - redo log archiving mode                                 | Tất cả              |
| CHECKPOINT_CHANGE#              |       | NUMBER        | SCN của checkpoint gần nhất                                                            | Tất cả              |
| ARCHIVE_CHANGE#                 |       | NUMBER        | SCN để bắt đầu archiving từ đó                                                         | Tất cả              |
| CONTROLFILE_TYPE                |       | VARCHAR2(7)   | CURRENT hoặc STANDBY - loại controlfile                                                | Tất cả              |
| CONTROLFILE_CREATED             |       | DATE          | Date khi controlfile được tạo                                                          | Tất cả              |
| CONTROLFILE_SEQUENCE#           |       | NUMBER        | Controlfile sequence number (tăng khi update controlfile)                              | Tất cả              |
| CONTROLFILE_CHANGE#             |       | NUMBER        | SCN trong controlfile                                                                  | Tất cả              |
| CONTROLFILE_TIME                |       | DATE          | Timestamp của controlfile modification cuối                                            | Tất cả              |
| OPEN_RESETLOGS                  |       | VARCHAR2(11)  | ALLOWED, NOT ALLOWED, REQUIRED - resetlogs required để open?                           | Tất cả              |
| VERSION_TIME                    |       | DATE          | Timestamp của database version                                                         | Tất cả              |
| OPEN_MODE                       |       | VARCHAR2(20)  | READ WRITE, READ ONLY, MOUNTED - open mode của database                                | Tất cả              |
| PROTECTION_MODE                 |       | VARCHAR2(20)  | MAXIMUM PROTECTION/AVAILABILITY/PERFORMANCE - Data Guard protection mode               | 9i+                 |
| PROTECTION_LEVEL                |       | VARCHAR2(20)  | Actual protection level đang active (có thể < protection_mode)                         | 9i+                 |
| REMOTE_ARCHIVE                  |       | VARCHAR2(8)   | ENABLED/DISABLED - remote archiving (Data Guard)                                       | Tất cả              |
| ACTIVATION#                     |       | NUMBER        | Activation identifier (Data Guard failover tracking)                                   | Tất cả              |
| SWITCHOVER#                     |       | NUMBER        | Switchover sequence number                                                             | Tất cả              |
| DATABASE_ROLE                   |       | VARCHAR2(16)  | PRIMARY, PHYSICAL STANDBY, LOGICAL STANDBY, SNAPSHOT STANDBY - role trong Data Guard   | 9i+                 |
| ARCHIVELOG_CHANGE#              |       | NUMBER        | SCN của archivelog cuối đã được archived                                               | Tất cả              |
| ARCHIVELOG_COMPRESSION          |       | VARCHAR2(8)   | ENABLED/DISABLED - archivelog compression                                              | 11g+                |
| SWITCHOVER_STATUS               |       | VARCHAR2(20)  | NOT ALLOWED, SESSIONS ACTIVE, TO PRIMARY, TO STANDBY - switchover readiness            | 9i+                 |
| DATAGUARD_BROKER                |       | VARCHAR2(8)   | ENABLED/DISABLED - Data Guard Broker status                                            | 10g+                |
| GUARD_STATUS                    |       | VARCHAR2(7)   | ALL, STANDBY, NONE - guard status (protects against user errors)                       | 10g+                |
| SUPPLEMENTAL_LOG_DATA_MIN       |       | VARCHAR2(8)   | YES/NO - minimum supplemental logging enabled (cho LogMiner, GoldenGate)               | 9i+                 |
| SUPPLEMENTAL_LOG_DATA_PK        |       | VARCHAR2(3)   | YES/NO - primary key supplemental logging                                              | 9i+                 |
| SUPPLEMENTAL_LOG_DATA_UI        |       | VARCHAR2(3)   | YES/NO - unique index supplemental logging                                             | 9i+                 |
| FORCE_LOGGING                   |       | VARCHAR2(39)  | YES, NO, STANDBY - force logging mode (tất cả operations được log)                     | 9i+                 |
| PLATFORM_ID                     |       | NUMBER        | Platform identifier (cross-platform transportable)                                     | 10g+                |
| PLATFORM_NAME                   |       | VARCHAR2(101) | Platform name (Linux x86-64, Windows, Solaris, AIX, etc.)                              | 10g+                |
| RECOVERY_TARGET_INCARNATION#    |       | NUMBER        | Target incarnation number cho recovery                                                 | 10g+                |
| LAST_OPEN_INCARNATION#          |       | NUMBER        | Incarnation number khi database được open lần cuối                                     | 10g+                |
| CURRENT_SCN                     |       | NUMBER        | Current System Change Number của database                                              | 10g+                |
| FLASHBACK_ON                    |       | VARCHAR2(18)  | YES, NO, RESTORE POINT ONLY - Flashback Database enabled?                              | 10g+                |
| SUPPLEMENTAL_LOG_DATA_FK        |       | VARCHAR2(3)   | YES/NO - foreign key supplemental logging                                              | 10g+                |
| SUPPLEMENTAL_LOG_DATA_ALL       |       | VARCHAR2(3)   | YES/NO - all column supplemental logging                                               | 10g+                |
| DB_UNIQUE_NAME                  |       | VARCHAR2(30)  | Unique database name (DB_UNIQUE_NAME parameter, unique trong Data Guard config)        | 10g+                |
| STANDBY_BECAME_PRIMARY_SCN      |       | NUMBER        | SCN khi standby database became primary                                                | 11g+                |
| FS_FAILOVER_MODE                |       | VARCHAR2(19)  | DISABLED, OBSERVER, TARGET - Fast-Start Failover mode                                  | 11g+                |
| FS_FAILOVER_STATUS              |       | VARCHAR2(22)  | DISABLED, TARGET UNDER LAG LIMIT, etc. - FSFO status                                   | 11g+                |
| FS_FAILOVER_CURRENT_TARGET      |       | VARCHAR2(30)  | Current Fast-Start Failover target database                                            | 11g+                |
| FS_FAILOVER_THRESHOLD           |       | NUMBER        | Lag threshold (seconds) cho automatic failover                                         | 11g+                |
| FS_FAILOVER_OBSERVER_PRESENT    |       | VARCHAR2(7)   | YES/NO - Data Guard Observer connected?                                                | 11g+                |
| FS_FAILOVER_OBSERVER_HOST       |       | VARCHAR2(512) | Hostname của Fast-Start Failover observer                                              | 11g+                |
| CONTROLFILE_CONVERTED           |       | VARCHAR2(3)   | YES nếu controlfile converted từ backup controlfile, NO nếu không                      | 11g+                |
| PRIMARY_DB_UNIQUE_NAME          |       | VARCHAR2(30)  | DB_UNIQUE_NAME của primary database (trong standby)                                    | 11g+                |
| SUPPLEMENTAL_LOG_DATA_PL        |       | VARCHAR2(3)   | YES/NO - procedural supplemental logging                                               | 11g+                |
| MIN_REQUIRED_CAPTURE_CHANGE#    |       | NUMBER        | Minimum SCN required cho LogMiner/GoldenGate capture                                   | 11g+                |
| CDB                             |       | VARCHAR2(3)   | YES nếu là Container Database (CDB), NO nếu non-CDB                                    | 12c+                |
| CON_ID                          |       | NUMBER        | Container ID (0=CDB$ROOT, 1=root container, 3+=PDB)                                    | 12c+                |
| PENDING_ROLE_CHANGE_TASKS       |       | VARCHAR2(512) | Tasks cần complete trước khi role change (Data Guard)                                  | 12c+                |
| CON_DBID                        |       | NUMBER        | Container DBID (unique cho mỗi PDB)                                                    | 12c+                |
| FORCE_FULL_DB_CACHING           |       | VARCHAR2(3)   | YES/NO - force full database caching trong buffer cache                                | 12c+                |
| SUPPLEMENTAL_LOG_DATA_SR        |       | VARCHAR2(3)   | YES/NO - Sync/Async Replication supplemental logging                                   | 12.2+               |
| GOLDENGATE_BLOCKING_MODE        |       | VARCHAR2(8)   | ENABLED/DISABLED - GoldenGate integrated capture blocking mode                         | 12.2+               |

## V$INSTANCE
| Name               | Null? | Type         | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|--------------------|-------|--------------|----------------------------------------------------------------------------------------|---------------------|
| INSTANCE_NUMBER    |       | NUMBER       | Instance number (RAC environment, 1 cho single instance)                               | Tất cả              |
| INSTANCE_NAME      |       | VARCHAR2(16) | Instance name (INSTANCE_NAME parameter)                                                | Tất cả              |
| HOST_NAME          |       | VARCHAR2(64) | Hostname của server chạy instance                                                      | Tất cả              |
| VERSION            |       | VARCHAR2(17) | Oracle Database version (short format)                                                 | Tất cả              |
| VERSION_LEGACY     |       | VARCHAR2(17) | Legacy version format (compatibility)                                                  | 18c+                |
| VERSION_FULL       |       | VARCHAR2(17) | Full version string với patch level                                                    | 18c+                |
| STARTUP_TIME       |       | DATE         | Timestamp khi instance được start                                                      | Tất cả              |
| STATUS             |       | VARCHAR2(12) | STARTED, MOUNTED, OPEN, OPEN MIGRATE - instance status                                 | Tất cả              |
| PARALLEL           |       | VARCHAR2(3)  | YES nếu RAC instance, NO nếu single instance                                           | Tất cả              |
| THREAD#            |       | NUMBER       | Redo thread number (RAC - mỗi instance có thread riêng)                                | Tất cả              |
| ARCHIVER           |       | VARCHAR2(7)  | STARTED, STOPPED, FAILED - archiver process status                                     | Tất cả              |
| LOG_SWITCH_WAIT    |       | VARCHAR2(15) | ARCHIVE LOG, CLEAR LOG, CHECKPOINT - operation đang chờ trước log switch               | Tất cả              |
| LOGINS             |       | VARCHAR2(10) | ALLOWED, RESTRICTED - login restriction status                                         | Tất cả              |
| SHUTDOWN_PENDING   |       | VARCHAR2(3)  | YES nếu shutdown đang process, NO nếu không                                            | Tất cả              |
| DATABASE_STATUS    |       | VARCHAR2(17) | ACTIVE, SUSPENDED, INSTANCE RECOVERY - database operation status                       | 10g+                |
| INSTANCE_ROLE      |       | VARCHAR2(18) | PRIMARY_INSTANCE, SECONDARY_INSTANCE - instance role                                   | 10g+                |
| ACTIVE_STATE       |       | VARCHAR2(9)  | NORMAL, QUIESCING, QUIESCED - quiesce state của instance                               | 10g+                |
| BLOCKED            |       | VARCHAR2(3)  | YES nếu instance blocked, NO nếu không (quiesce operation)                             | 10g+                |
| CON_ID             |       | NUMBER       | Container ID (0 cho non-CDB view, 1+ cho CDB)                                          | 12c+                |
| INSTANCE_MODE      |       | VARCHAR2(11) | REGULAR, READ ONLY, READ-MOSTLY - instance operating mode                              | 12c+                |
| EDITION            |       | VARCHAR2(7)  | Edition của Oracle: EE (Enterprise), SE (Standard), XE (Express)                       | 11g+                |
| FAMILY             |       | VARCHAR2(80) | Oracle product family information                                                      | 18c+                |
| DATABASE_TYPE      |       | VARCHAR2(15) | RAC, RACONENODE, SINGLE - database type configuration                                  | 11gR2+              |

## V$SESSION
| Name                           | Null? | Type         | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|--------------------------------|-------|--------------|----------------------------------------------------------------------------------------|---------------------|
| SADDR                          |       | RAW(8)       | Session address (internal memory address)                                              | Tất cả              |
| SID                            |       | NUMBER       | Session identifier (unique trong instance)                                             | Tất cả              |
| SERIAL#                        |       | NUMBER       | Session serial number (tăng mỗi khi SID được reuse)                                    | Tất cả              |
| AUDSID                         |       | NUMBER       | Auditing session ID                                                                    | Tất cả              |
| PADDR                          |       | RAW(8)       | Process address (V$PROCESS.ADDR)                                                       | Tất cả              |
| USER#                          |       | NUMBER       | Oracle user identifier                                                                 | Tất cả              |
| USERNAME                       |       | VARCHAR2(128)| Oracle username                                                                        | Tất cả              |
| COMMAND                        |       | NUMBER       | Command number đang execute (1=CREATE TABLE, 3=SELECT, 6=UPDATE, 7=DELETE, etc.)       | Tất cả              |
| OWNERID                        |       | NUMBER       | Owner identifier của migrated session                                                  | Tất cả              |
| TADDR                          |       | VARCHAR2(16) | Transaction address (V$TRANSACTION.ADDR)                                               | Tất cả              |
| LOCKWAIT                       |       | VARCHAR2(16) | Address của lock đang chờ (NULL nếu không wait)                                        | Tất cả              |
| STATUS                         |       | VARCHAR2(8)  | ACTIVE, INACTIVE, KILLED, CACHED, SNIPED - session status                              | Tất cả              |
| SERVER                         |       | VARCHAR2(9)  | DEDICATED, SHARED, PSEUDO, POOLED, NONE - server type                                  | Tất cả              |
| SCHEMA#                        |       | NUMBER       | Schema identifier                                                                      | Tất cả              |
| SCHEMANAME                     |       | VARCHAR2(128)| Current schema name                                                                    | Tất cả              |
| OSUSER                         |       | VARCHAR2(128)| Operating system username                                                              | Tất cả              |
| PROCESS                        |       | VARCHAR2(24) | Operating system process ID                                                            | Tất cả              |
| MACHINE                        |       | VARCHAR2(64) | Client machine name                                                                    | Tất cả              |
| PORT                           |       | NUMBER       | Client port number                                                                     | Tất cả              |
| TERMINAL                       |       | VARCHAR2(30) | Client terminal name                                                                   | Tất cả              |
| PROGRAM                        |       | VARCHAR2(84) | Client program name (sqlplus, JDBC, etc.)                                              | Tất cả              |
| TYPE                           |       | VARCHAR2(10) | USER, BACKGROUND - session type                                                        | Tất cả              |
| SQL_ADDRESS                    |       | RAW(8)       | SQL statement address (V$SQL.ADDRESS)                                                  | Tất cả              |
| SQL_HASH_VALUE                 |       | NUMBER       | Hash value của SQL statement                                                           | Tất cả              |
| SQL_ID                         |       | VARCHAR2(13) | SQL identifier (unique cho mỗi unique SQL text)                                        | 10g+                |
| SQL_CHILD_NUMBER               |       | NUMBER       | Child cursor number (V$SQL.CHILD_NUMBER)                                               | 10g+                |
| SQL_EXEC_START                 |       | DATE         | Timestamp khi SQL execution started                                                    | 11g+                |
| SQL_EXEC_ID                    |       | NUMBER       | SQL execution identifier                                                               | 11g+                |
| PREV_SQL_ADDR                  |       | RAW(8)       | Previous SQL statement address                                                         | Tất cả              |
| PREV_HASH_VALUE                |       | NUMBER       | Hash value của previous SQL statement                                                  | Tất cả              |
| PREV_SQL_ID                    |       | VARCHAR2(13) | Previous SQL identifier                                                                | 10g+                |
| PREV_CHILD_NUMBER              |       | NUMBER       | Previous child cursor number                                                           | 10g+                |
| PREV_EXEC_START                |       | DATE         | Previous SQL execution start time                                                      | 11g+                |
| PREV_EXEC_ID                   |       | NUMBER       | Previous SQL execution ID                                                              | 11g+                |
| PLSQL_ENTRY_OBJECT_ID          |       | NUMBER       | Entry point PL/SQL object ID                                                           | 10g+                |
| PLSQL_ENTRY_SUBPROGRAM_ID      |       | NUMBER       | Entry point subprogram ID trong PL/SQL object                                          | 10g+                |
| PLSQL_OBJECT_ID                |       | NUMBER       | Current PL/SQL object ID đang execute                                                  | 10g+                |
| PLSQL_SUBPROGRAM_ID            |       | NUMBER       | Current subprogram ID trong PL/SQL object                                              | 10g+                |
| MODULE                         |       | VARCHAR2(64) | Application module name (set via DBMS_APPLICATION_INFO)                                | Tất cả              |
| MODULE_HASH                    |       | NUMBER       | Hash value của module name                                                             | 10g+                |
| ACTION                         |       | VARCHAR2(64) | Application action name (set via DBMS_APPLICATION_INFO)                                | Tất cả              |
| ACTION_HASH                    |       | NUMBER       | Hash value của action name                                                             | 10g+                |
| CLIENT_INFO                    |       | VARCHAR2(64) | Client information (set via DBMS_APPLICATION_INFO)                                     | Tất cả              |
| FIXED_TABLE_SEQUENCE           |       | NUMBER       | Sequence number cho fixed table consistency                                            | Tất cả              |
| ROW_WAIT_OBJ#                  |       | NUMBER       | Object ID của table chứa row đang wait                                                 | Tất cả              |
| ROW_WAIT_FILE#                 |       | NUMBER       | File ID chứa row đang wait                                                             | Tất cả              |
| ROW_WAIT_BLOCK#                |       | NUMBER       | Block ID chứa row đang wait                                                            | Tất cả              |
| ROW_WAIT_ROW#                  |       | NUMBER       | Row number đang wait                                                                   | Tất cả              |
| TOP_LEVEL_CALL#                |       | NUMBER       | Top-level call depth                                                                   | Tất cả              |
| LOGON_TIME                     |       | DATE         | Timestamp khi session logged on                                                        | Tất cả              |
| LAST_CALL_ET                   |       | NUMBER       | Elapsed time (seconds) từ lần call cuối (0 nếu ACTIVE, >0 nếu INACTIVE)                | Tất cả              |
| PDML_ENABLED                   |       | VARCHAR2(3)  | YES/NO - Parallel DML enabled cho session?                                             | Tất cả              |
| FAILOVER_TYPE                  |       | VARCHAR2(13) | SESSION, SELECT, TRANSACTION, NONE - TAF failover type                                 | Tất cả              |
| FAILOVER_METHOD                |       | VARCHAR2(10) | BASIC, PRECONNECT - TAF failover method                                                | Tất cả              |
| FAILED_OVER                    |       | VARCHAR2(3)  | YES nếu session failed over, NO nếu không                                              | Tất cả              |
| RESOURCE_CONSUMER_GROUP        |       | VARCHAR2(32) | Resource Manager consumer group name                                                   | 10g+                |
| PDML_STATUS                    |       | VARCHAR2(8)  | ENABLED, DISABLED, FORCED - Parallel DML status                                        | Tất cả              |
| PDDL_STATUS                    |       | VARCHAR2(8)  | ENABLED, DISABLED, FORCED - Parallel DDL status                                        | Tất cả              |
| PQ_STATUS                      |       | VARCHAR2(8)  | ENABLED, DISABLED, FORCED - Parallel Query status                                      | Tất cả              |
| CURRENT_QUEUE_DURATION         |       | NUMBER       | Queue time (seconds) trong Resource Manager                                            | 10g+                |
| CLIENT_IDENTIFIER              |       | VARCHAR2(64) | Client identifier (set via DBMS_SESSION.SET_IDENTIFIER)                                | 10g+                |
| BLOCKING_SESSION_STATUS        |       | VARCHAR2(11) | VALID, NO HOLDER, UNKNOWN, NOT IN WAIT - blocking session status                       | 10g+                |
| BLOCKING_INSTANCE              |       | NUMBER       | Instance number của blocking session (RAC)                                             | 10g+                |
| BLOCKING_SESSION               |       | NUMBER       | SID của session đang block session này                                                 | 10g+                |
| FINAL_BLOCKING_SESSION_STATUS  |       | VARCHAR2(11) | Status của root blocking session trong blocking chain                                  | 11g+                |
| FINAL_BLOCKING_INSTANCE        |       | NUMBER       | Instance number của final blocking session                                             | 11g+                |
| FINAL_BLOCKING_SESSION         |       | NUMBER       | SID của final blocking session (root cause)                                            | 11g+                |
| SEQ#                           |       | NUMBER       | Sequence number để uniquely identify wait (tăng với mỗi wait)                          | Tất cả              |
| EVENT#                         |       | NUMBER       | Event number đang wait (reference V$EVENT_NAME)                                        | Tất cả              |
| EVENT                          |       | VARCHAR2(64) | Event name đang wait (db file sequential read, log file sync, etc.)                    | Tất cả              |
| P1TEXT                         |       | VARCHAR2(64) | Description của wait parameter 1                                                       | Tất cả              |
| P1                             |       | NUMBER       | Wait parameter 1 value                                                                 | Tất cả              |
| P1RAW                          |       | RAW(8)       | Wait parameter 1 raw value                                                             | Tất cả              |
| P2TEXT                         |       | VARCHAR2(64) | Description của wait parameter 2                                                       | Tất cả              |
| P2                             |       | NUMBER       | Wait parameter 2 value                                                                 | Tất cả              |
| P2RAW                          |       | RAW(8)       | Wait parameter 2 raw value                                                             | Tất cả              |
| P3TEXT                         |       | VARCHAR2(64) | Description của wait parameter 3                                                       | Tất cả              |
| P3                             |       | NUMBER       | Wait parameter 3 value                                                                 | Tất cả              |
| P3RAW                          |       | RAW(8)       | Wait parameter 3 raw value                                                             | Tất cả              |
| WAIT_CLASS_ID                  |       | NUMBER       | Wait class identifier                                                                  | 10g+                |
| WAIT_CLASS#                    |       | NUMBER       | Wait class number                                                                      | 10g+                |
| WAIT_CLASS                     |       | VARCHAR2(64) | Wait class: User I/O, System I/O, Concurrency, Application, Network, etc.              | 10g+                |
| WAIT_TIME                      |       | NUMBER       | Wait time (centiseconds) nếu not waiting, 0 nếu đang wait                              | Tất cả              |
| SECONDS_IN_WAIT                |       | NUMBER       | Seconds đã wait cho current event (nếu đang wait)                                      | Tất cả              |
| STATE                          |       | VARCHAR2(19) | WAITING, WAITED UNKNOWN TIME, WAITED SHORT TIME, WAITED KNOWN TIME                     | Tất cả              |
| WAIT_TIME_MICRO                |       | NUMBER       | Wait time (microseconds)                                                               | 10g+                |
| TIME_REMAINING_MICRO           |       | NUMBER       | Time remaining (microseconds) cho timed wait                                           | 10g+                |
| TOTAL_TIME_WAITED_MICRO        |       | NUMBER       | Total time waited (microseconds) cho event này                                         | 10g+                |
| HEUR_TIME_WAITED_MICRO         |       | NUMBER       | Heuristic time waited (microseconds) - estimated cho current wait                      | 10g+                |
| TIME_SINCE_LAST_WAIT_MICRO     |       | NUMBER       | Time (microseconds) từ last wait                                                       | 10g+                |
| SERVICE_NAME                   |       | VARCHAR2(64) | Service name của session                                                               | 10g+                |
| SQL_TRACE                      |       | VARCHAR2(8)  | ENABLED/DISABLED - SQL trace status                                                    | Tất cả              |
| SQL_TRACE_WAITS                |       | VARCHAR2(5)  | TRUE/FALSE - trace waits                                                               | Tất cả              |
| SQL_TRACE_BINDS                |       | VARCHAR2(5)  | TRUE/FALSE - trace bind variables                                                      | Tất cả              |
| SQL_TRACE_PLAN_STATS           |       | VARCHAR2(10) | NEVER, FIRST, ALL - trace plan statistics                                              | 11g+                |
| SESSION_EDITION_ID             |       | NUMBER       | Edition ID cho session (Edition-Based Redefinition)                                    | 11gR2+              |
| CREATOR_ADDR                   |       | RAW(8)       | Address của creator session (cho proxy sessions)                                       | 10g+                |
| CREATOR_SERIAL#                |       | NUMBER       | Serial# của creator session                                                            | 10g+                |
| ECID                           |       | VARCHAR2(64) | Execution Context ID (end-to-end tracing)                                              | 11g+                |
| SQL_TRANSLATION_PROFILE_ID     |       | NUMBER       | SQL Translation profile ID (migration tool)                                            | 12c+                |
| PGA_TUNABLE_MEM                |       | NUMBER       | Current tunable PGA memory (bytes) allocated cho session                               | 11g+                |
| SHARD_DDL_STATUS               |       | VARCHAR2(8)  | ENABLED/DISABLED - shard DDL status (Oracle Sharding)                                  | 12.2+               |
| CON_ID                         |       | NUMBER       | Container ID (0=non-CDB, 1=CDB$ROOT, 3+=PDB)                                           | 12c+                |
| EXTERNAL_NAME                  |       | VARCHAR2(1024)| External user name (Enterprise User Security, LDAP)                                   | 12c+                |
| PLSQL_DEBUGGER_CONNECTED       |       | VARCHAR2(5)  | TRUE/FALSE - PL/SQL debugger connected?                                                | 18c+                |

## CDB_SYS_PRIVS
| Name       | Null? | Type         | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|------------|-------|--------------|----------------------------------------------------------------------------------------|---------------------|
| GRANTEE    |       | VARCHAR2(128)| Username hoặc role được grant privilege                                                | 12c+                |
| PRIVILEGE  |       | VARCHAR2(40) | System privilege name: CREATE SESSION, SELECT ANY TABLE, DBA, etc.                     | 12c+                |
| ADMIN_OPTION|      | VARCHAR2(3)  | YES nếu grantee có thể grant privilege cho users khác, NO nếu không                    | 12c+                |
| COMMON     |       | VARCHAR2(3)  | YES nếu grant common (CDB-level), NO nếu local (PDB-level)                             | 12c+                |
| INHERITED  |       | VARCHAR2(3)  | YES nếu privilege được inherit từ CDB, NO nếu granted locally                          | 12c+                |
| CON_ID     |       | NUMBER       | Container ID (1=CDB$ROOT, 3+=PDB)                                                      | 12c+                |

## DBA_SYS_PRIVS
| Name        | Null? | Type         | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|-------------|-------|--------------|----------------------------------------------------------------------------------------|---------------------|
| GRANTEE     |       | VARCHAR2(128)| Username hoặc role được grant privilege                                                | Tất cả              |
| PRIVILEGE   |       | VARCHAR2(40) | System privilege name: CREATE SESSION, SELECT ANY TABLE, CREATE ANY TABLE, etc.        | Tất cả              |
| ADMIN_OPTION|       | VARCHAR2(3)  | YES nếu grantee có thể grant privilege cho users khác (WITH ADMIN OPTION), NO nếu không| Tất cả              |
| COMMON      |       | VARCHAR2(3)  | YES nếu grant common (CDB-level), NO nếu local (PDB-level)                             | 12c+                |
| INHERITED   |       | VARCHAR2(3)  | YES nếu privilege được inherit từ parent container, NO nếu granted locally             | 12c+                |

## DBA_TAB_PRIVS
| Name       | Null? | Type         | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|------------|-------|--------------|----------------------------------------------------------------------------------------|---------------------|
| GRANTEE    |       | VARCHAR2(128)| Username hoặc role nhận privilege                                                      | Tất cả              |
| OWNER      |       | VARCHAR2(128)| Owner của object                                                                       | Tất cả              |
| TABLE_NAME |       | VARCHAR2(128)| Tên của table/view/sequence                                                            | Tất cả              |
| GRANTOR    |       | VARCHAR2(128)| Username của user đã grant privilege                                                   | Tất cả              |
| PRIVILEGE  |       | VARCHAR2(40) | Object privilege: SELECT, INSERT, UPDATE, DELETE, EXECUTE, ALTER, INDEX, REFERENCES    | Tất cả              |
| GRANTABLE  |       | VARCHAR2(3)  | YES nếu grantee có thể grant privilege cho users khác (WITH GRANT OPTION), NO nếu không| Tất cả              |
| HIERARCHY  |       | VARCHAR2(3)  | YES nếu privilege WITH HIERARCHY OPTION (object types), NO nếu không                   | Tất cả              |
| COMMON     |       | VARCHAR2(3)  | YES nếu grant common (CDB-level), NO nếu local (PDB-level)                             | 12c+                |
| TYPE       |       | VARCHAR2(24) | Object type: TABLE, VIEW, SEQUENCE, PROCEDURE, FUNCTION, PACKAGE, etc.                 | 11g+                |
| INHERITED  |       | VARCHAR2(3)  | YES nếu privilege được inherit từ parent container, NO nếu granted locally             | 12c+                |

## V$PWFILE_USERS
| Name                 | Null? | Type                        | Ý nghĩa                                                                                | Phiên bản xuất hiện |
|----------------------|-------|-----------------------------|----------------------------------------------------------------------------------------|---------------------|
| USERNAME             |       | VARCHAR2(128)               | Username trong password file                                                           | Tất cả              |
| SYSDBA               |       | VARCHAR2(5)                 | TRUE nếu có SYSDBA privilege, FALSE nếu không                                          | Tất cả              |
| SYSOPER              |       | VARCHAR2(5)                 | TRUE nếu có SYSOPER privilege, FALSE nếu không                                         | Tất cả              |
| SYSASM               |       | VARCHAR2(5)                 | TRUE nếu có SYSASM privilege (ASM administration), FALSE nếu không                     | 11g+                |
| SYSBACKUP            |       | VARCHAR2(5)                 | TRUE nếu có SYSBACKUP privilege (backup operations), FALSE nếu không                   | 12c+                |
| SYSDG                |       | VARCHAR2(5)                 | TRUE nếu có SYSDG privilege (Data Guard operations), FALSE nếu không                   | 12c+                |
| SYSKM                |       | VARCHAR2(5)                 | TRUE nếu có SYSKM privilege (Key Management - TDE), FALSE nếu không                    | 12c+                |
| ACCOUNT_STATUS       |       | VARCHAR2(30)                | OPEN, LOCKED, EXPIRED - account status                                                 | 12c+                |
| PASSWORD_PROFILE     |       | VARCHAR2(128)               | Password profile name applied cho user                                                 | 18c+                |
| LAST_LOGIN           |       | TIMESTAMP(9) WITH TIME ZONE | Timestamp của lần login gần nhất với administrative privilege                          | 18c+                |
| LOCK_DATE            |       | DATE                        | Date khi account bị lock                                                               | 18c+                |
| EXPIRY_DATE          |       | DATE                        | Date khi password hết hạn                                                              | 18c+                |
| EXTERNAL_NAME        |       | VARCHAR2(1024)              | External name cho user (Enterprise User Security)                                      | 12c+                |
| AUTHENTICATION_TYPE  |       | VARCHAR2(8)                 | PASSWORD, EXTERNAL, GLOBAL - authentication method                                     | 12c+                |
| COMMON               |       | VARCHAR2(3)                 | YES nếu common user (CDB-level), NO nếu local                                          | 12c+                |
| PASSWORD_VERSIONS    |       | VARCHAR2(12)                | List password versions trong password file: 10G, 11G, 12C                              | 12c+                |
| CON_ID               |       | NUMBER                      | Container ID (0 cho password file entry)                                               | 12c+                |