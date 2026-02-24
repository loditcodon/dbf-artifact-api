#!/bin/bash
# Auto-add golint comments to exported items
# Usage: bash add_golint_comments.sh

cd "$(dirname "$0")/.."

echo "Adding golint comments to fix 138 'should have comment' warnings..."
echo "This will NOT rename fields (ObjectId, SqlParam, etc.)"
echo ""

# ============================================================================
# MODELS - TableName methods
# ============================================================================
find models -name "*.go" -type f | while read file; do
    # Add TableName comment if missing
    if grep -q "func.*TableName.*string" "$file" && ! grep -B1 "func.*TableName" "$file" | grep -q "// TableName"; then
        sed -i '/func.*TableName.*string/i\// TableName returns the database table name for GORM model.' "$file"
        echo "✓ Added TableName comment to $file"
    fi
done

echo ""
echo "✓ Models done!"
echo ""

# ============================================================================
# CONTROLLERS - Set/Register functions
# ============================================================================
echo "Adding comments to controllers..."

# SetXxxService functions
find controllers -name "*_controller.go" -type f | while read file; do
    if grep -q "func Set.*Service" "$file" && ! grep -B1 "func Set" "$file" | grep -q "// Set"; then
        service_name=$(grep "func Set" "$file" | sed 's/func Set\(.*\)Service.*/\1/')
        sed -i "/func Set${service_name}Service/i\// Set${service_name}Service initializes the ${service_name,,} service instance." "$file"
        echo "✓ Added Set comment to $file"
    fi

    # RegisterXxxRoutes functions
    if grep -q "func Register.*Routes" "$file" && ! grep -B1 "func Register" "$file" | grep -q "// Register"; then
        controller_name=$(grep "func Register" "$file" | sed 's/func Register\(.*\)Routes.*/\1/')
        sed -i "/func Register${controller_name}Routes/i\// Register${controller_name}Routes registers HTTP endpoints for ${controller_name,,} operations." "$file"
        echo "✓ Added Register comment to $file"
    fi
done

echo "✓ Controllers done!"
echo ""

# ============================================================================
# REPOSITORY - Interfaces and constructors
# ============================================================================
echo "Adding comments to repositories..."

find repository -name "*_repository.go" -type f | while read file; do
    # Add interface comments
    if grep -q "^type.*Repository interface" "$file" && ! grep -B1 "^type.*Repository interface" "$file" | grep -q "//.*Repository"; then
        repo_name=$(grep "^type.*Repository interface" "$file" | sed 's/type \(.*\)Repository interface.*/\1/')
        sed -i "/^type ${repo_name}Repository interface/i\// ${repo_name}Repository defines data access operations for ${repo_name,,} entities." "$file"
        echo "✓ Added interface comment to $file"
    fi

    # Add New constructor comments
    if grep -q "^func New.*Repository" "$file" && ! grep -B1 "^func New.*Repository" "$file" | grep -q "// New"; then
        repo_name=$(grep "^func New" "$file" | sed 's/func New\(.*\)Repository.*/\1/')
        sed -i "/^func New${repo_name}Repository/i\// New${repo_name}Repository creates a new ${repo_name}Repository instance." "$file"
        echo "✓ Added constructor comment to $file"
    fi
done

echo "✓ Repositories done!"
echo ""

# ============================================================================
# PKG/LOGGER - All exported functions
# ============================================================================
echo "Adding comments to pkg/logger..."

# Logger type
sed -i '/^type Logger struct/i\// Logger provides structured logging with configurable output and levels.' pkg/logger/logger.go 2>/dev/null

# LogLevel type
sed -i '/^type LogLevel int/i\// LogLevel represents the severity level for log messages.' pkg/logger/logger.go 2>/dev/null

# Constants
sed -i '/^const (/a\    // DEBUG level for detailed diagnostic information' pkg/logger/logger.go 2>/dev/null

# Functions - add generic comments
for func in Init InitWithConfig NewLogger NewLoggerWithConfig ParseLogLevel SetLevel GetLevel \
            Debug Debugf Info Infof Warn Warnf Error Errorf Fatal Fatalf; do
    if ! grep -B1 "^func.*${func}" pkg/logger/logger.go | grep -q "// ${func}"; then
        sed -i "/^func.*${func}/i\// ${func} handles ${func,,} operations for logger." pkg/logger/logger.go 2>/dev/null
    fi
done

echo "✓ pkg/logger done!"
echo ""

# ============================================================================
# CONFIG & BOOTSTRAP
# ============================================================================
echo "Adding comments to config and bootstrap..."

# AppConfig
sed -i '/^type AppConfig struct/i\// AppConfig holds application configuration loaded from environment variables.' config/config.go 2>/dev/null

# Cfg variable
sed -i '/^var Cfg/i\// Cfg is the global application configuration instance.' config/config.go 2>/dev/null

# LoadConfig
sed -i 's|^// Loads.*|// LoadConfig loads and validates application configuration from environment.|' config/config.go 2>/dev/null

# ConnectDB
sed -i '/^func ConnectDB/i\// ConnectDB establishes database connection using GORM with configured credentials.' config/database.go 2>/dev/null

# LoadData
sed -i '/^func LoadData/i\// LoadData initializes bootstrap data including policy defaults and actor mappings.' bootstrap/load_data.go 2>/dev/null

# DBActorAll
sed -i '/^var DBActorAll/i\// DBActorAll stores the global mapping of actors for quick lookup.' bootstrap/load_data.go 2>/dev/null

echo "✓ config & bootstrap done!"
echo ""

# ============================================================================
# UTILS
# ============================================================================
echo "Adding comments to utils..."

# Exported functions in utils
for func in ValidateStruct CreateArtifactJSON CreateNoHexArtifactJSON InitLogger NewCustomLogger \
            GetPolicyLogger LoggerMiddleware JSONResponse ErrorResponse LogInfo LogDebug LogWarn LogError LogFatal; do
    find utils -name "*.go" -type f -exec grep -l "func ${func}" {} \; | while read file; do
        if ! grep -B1 "^func ${func}" "$file" | grep -q "// ${func}"; then
            sed -i "/^func ${func}/i\// ${func} provides ${func,,} functionality." "$file" 2>/dev/null
            echo "✓ Added comment for ${func} in $file"
        fi
    done
done

echo "✓ utils done!"
echo ""

echo "================================================================"
echo "✓ DONE! Comments added for exported items."
echo "================================================================"
echo ""
echo "Verification:"
golint ./... 2>&1 | grep "should have comment" | wc -l
echo "remaining 'should have comment' warnings"
echo ""
echo "Note: Field naming warnings (ObjectId→ObjectID) are NOT fixed"
echo "      to avoid breaking API changes."
