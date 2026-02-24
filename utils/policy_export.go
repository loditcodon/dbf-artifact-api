package utils

import (
	"fmt"
	"os/exec"

	"dbfartifactapi/pkg/logger"
)

// ExportDBFPolicy calls /usr/local/bin/exportDBFPolicy to build rule files after policy insertion.
// Returns error if command execution fails, allowing caller to log or handle appropriately.
func ExportDBFPolicy() error {
	exportCmd := "/usr/local/bin/exportDBFPolicy"

	logger.Infof("Executing policy export command: %s", exportCmd)

	cmd := exec.Command(exportCmd)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("failed to execute %s: %w, output: %s", exportCmd, err, string(output))
	}

	logger.Infof("Policy export completed successfully, output: %s", string(output))
	return nil
}
