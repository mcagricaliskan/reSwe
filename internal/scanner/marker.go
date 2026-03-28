package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const markerDir = ".reswe"
const markerFile = "folder.json"

// FolderMarker is the content stored in .reswe/folder.json inside each tracked folder
type FolderMarker struct {
	UUID      string    `json:"uuid"`
	ProjectID int64     `json:"project_id"`
	Name      string    `json:"name"`
	AddedAt   time.Time `json:"added_at"`
}

// WriteMarker creates .reswe/folder.json inside a folder
func WriteMarker(folderPath string, marker FolderMarker) error {
	dir := filepath.Join(folderPath, markerDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, markerFile), data, 0644)
}

// ReadMarker reads .reswe/folder.json from a folder. Returns nil if not found.
func ReadMarker(folderPath string) *FolderMarker {
	path := filepath.Join(folderPath, markerDir, markerFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var m FolderMarker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}

// HasMarker checks if a folder has a .reswe marker
func HasMarker(folderPath string) bool {
	_, err := os.Stat(filepath.Join(folderPath, markerDir, markerFile))
	return err == nil
}

// FindMarkerInParent walks up from a path looking for a .reswe marker.
// Useful when a folder was moved — check if it landed somewhere nearby.
func FindMarkerInParent(startPath string, maxLevels int) (string, *FolderMarker) {
	current := startPath
	for i := 0; i < maxLevels; i++ {
		m := ReadMarker(current)
		if m != nil {
			return current, m
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", nil
}

// ScanForMarkers scans a directory tree for .reswe markers.
// Returns a map of UUID → absolute path for all found markers.
func ScanForMarkers(root string, maxDepth int) map[string]string {
	results := make(map[string]string)
	if maxDepth <= 0 {
		maxDepth = 4
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return results
	}

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		if rel != "." {
			depth := len(filepath.SplitList(rel))
			if depth > maxDepth {
				return filepath.SkipDir
			}
		}

		// Skip junk
		name := info.Name()
		if name == "node_modules" || name == "__pycache__" || name == ".venv" {
			return filepath.SkipDir
		}

		m := ReadMarker(path)
		if m != nil {
			results[m.UUID] = path
		}

		return nil
	})

	return results
}
