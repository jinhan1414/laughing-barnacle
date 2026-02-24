package fileutil

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultReplaceAttempts = 5
	defaultReplaceBackoff  = 20 * time.Millisecond
)

// ReplaceFileWithRetry replaces targetPath using tempPath with retry logic.
// It is intended for Windows where os.Rename over existing targets can be flaky.
func ReplaceFileWithRetry(tempPath, targetPath string) error {
	if err := os.Rename(tempPath, targetPath); err == nil {
		return nil
	}

	var lastErr error
	for i := 0; i < defaultReplaceAttempts; i++ {
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			lastErr = err
		}
		if err := os.Rename(tempPath, targetPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(i+1) * defaultReplaceBackoff)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("replace file failed")
}
