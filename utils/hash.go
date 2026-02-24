package utils

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
)

// CalculateFileMD5 computes the MD5 hash of a file.
// Returns the hex-encoded MD5 hash string.
func CalculateFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate MD5 for file %s: %w", filePath, err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
