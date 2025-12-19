package tool

import (
	"fmt"
	"path/filepath"
)

func packageFilePath(baseDir string, packageName string, packageNum int) string {
	return filepath.Join(baseDir, "packages", fmt.Sprintf("%s_%d", packageName, packageNum))
}
