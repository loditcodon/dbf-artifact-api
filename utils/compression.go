package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CompressDirectoryToTarGz compresses a directory into a .tar.gz file.
// Returns the path to the created archive file.
// The archive is created in the parent directory of sourceDir with the format: <dirname>.tar.gz
func CompressDirectoryToTarGz(sourceDir string) (string, error) {
	info, err := os.Stat(sourceDir)
	if err != nil {
		return "", fmt.Errorf("failed to stat source directory %s: %w", sourceDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source path %s is not a directory", sourceDir)
	}

	dirName := filepath.Base(sourceDir)
	parentDir := filepath.Dir(sourceDir)
	archivePath := filepath.Join(parentDir, dirName+".tar.gz")

	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to create archive file %s: %w", archivePath, err)
	}
	defer archiveFile.Close()

	gzWriter := gzip.NewWriter(archiveFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", path, err)
		}

		// Use forward slashes for cross-platform compatibility
		header.Name = filepath.ToSlash(relPath)

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", path, err)
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("failed to write file %s to archive: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("failed to walk directory %s: %w", sourceDir, err)
	}

	return archivePath, nil
}
