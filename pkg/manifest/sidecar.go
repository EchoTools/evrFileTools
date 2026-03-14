package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Sidecar holds the original symbol IDs for a named extracted file.
// Written as a JSON file alongside each extracted file to enable
// deterministic repackaging from friendly-named working copies.
type Sidecar struct {
	TypeSymbol string `json:"typeSymbol"` // hex, e.g. "beac1969cb7b8861"
	FileSymbol string `json:"fileSymbol"` // hex, e.g. "74d228d09dc5dd8f"
}

// SidecarPath returns the sidecar path for a given file path.
func SidecarPath(filePath string) string {
	return filePath + ".evrmeta"
}

// WriteSidecar writes a sidecar file for the given file path.
func WriteSidecar(filePath string, typeSymbol, fileSymbol int64) error {
	sc := Sidecar{
		TypeSymbol: fmt.Sprintf("%016x", uint64(typeSymbol)),
		FileSymbol: fmt.Sprintf("%016x", uint64(fileSymbol)),
	}
	data, err := json.Marshal(sc)
	if err != nil {
		return err
	}
	return os.WriteFile(SidecarPath(filePath), data, 0644)
}

// ReadSidecar reads a sidecar file and returns the symbol IDs.
// Returns (0, 0, nil) if the sidecar does not exist (not an error).
func ReadSidecar(filePath string) (typeSymbol, fileSymbol int64, err error) {
	data, err := os.ReadFile(SidecarPath(filePath))
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}

	var sc Sidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return 0, 0, fmt.Errorf("parse sidecar: %w", err)
	}

	ts, err := strconv.ParseUint(sc.TypeSymbol, 16, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse typeSymbol %q: %w", sc.TypeSymbol, err)
	}
	fs, err := strconv.ParseUint(sc.FileSymbol, 16, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse fileSymbol %q: %w", sc.FileSymbol, err)
	}

	return int64(ts), int64(fs), nil
}
