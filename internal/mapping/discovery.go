package mapping

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MappingProfileSummary is a lightweight view of a profile for listing.
type MappingProfileSummary struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Format       string   `json:"format"` // "unified", "legacy", "error"
	DeviceName   string   `json:"device_name,omitempty"`
	AppClass     string   `json:"app_class,omitempty"`
	MappingCount int      `json:"mapping_count"`
	Tags         []string `json:"tags,omitempty"`
	Description  string   `json:"description,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// ListMappingProfiles scans a directory for all TOML profiles.
func ListMappingProfiles(dir string) ([]MappingProfileSummary, error) {
	var summaries []MappingProfileSummary

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read profiles dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		p, err := LoadMappingProfile(path)
		if err != nil {
			summaries = append(summaries, MappingProfileSummary{
				Name:   strings.TrimSuffix(e.Name(), ".toml"),
				Path:   path,
				Format: "error",
				Error:  err.Error(),
			})
			continue
		}
		s := ProfileToSummary(p)
		summaries = append(summaries, s)
	}

	return summaries, nil
}

// ProfileToSummary creates a summary from a loaded profile.
func ProfileToSummary(p *MappingProfile) MappingProfileSummary {
	s := MappingProfileSummary{
		Name:         p.SourceName,
		Path:         p.SourcePath,
		MappingCount: p.MappingCount(),
		DeviceName:   p.DeviceName(),
	}
	if p.IsUnifiedFormat() {
		s.Format = "unified"
		if p.Profile != nil {
			s.Tags = p.Profile.Tags
			s.Description = p.Profile.Description
			s.AppClass = p.Profile.AppClass
		}
	} else {
		s.Format = "legacy"
		parts := strings.SplitN(p.SourceName, "::", 2)
		if len(parts) > 1 {
			if _, err := strconv.Atoi(parts[1]); err != nil {
				s.AppClass = parts[1]
			}
		}
	}
	return s
}
