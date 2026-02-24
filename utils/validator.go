package utils

import (
	"net"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
}

// ValidateStruct: hàm validate struct
func ValidateStruct(obj interface{}) error {
	return validate.Struct(obj)
}

// IsValidMySQLHost validates MySQL host format including %, localhost, IPv4, and IPv6
func IsValidMySQLHost(host string) bool {
	if host == "" {
		return false
	}

	// MySQL wildcard - allows any host
	if host == "%" {
		return true
	}

	// Localhost variations
	if strings.ToLower(host) == "localhost" {
		return true
	}

	// IPv4 or IPv6 address validation
	if net.ParseIP(host) != nil {
		return true
	}

	// Domain name validation - basic check for valid characters
	// MySQL allows hostnames with letters, numbers, dots, and hyphens
	if len(host) > 0 && len(host) <= 253 {
		for _, char := range host {
			if !((char >= 'a' && char <= 'z') ||
				(char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') ||
				char == '.' || char == '-' || char == '_') {
				return false
			}
		}
		// Basic domain format check - shouldn't start or end with dot/hyphen
		if !strings.HasPrefix(host, ".") && !strings.HasSuffix(host, ".") &&
			!strings.HasPrefix(host, "-") && !strings.HasSuffix(host, "-") {
			return true
		}
	}

	return false
}
