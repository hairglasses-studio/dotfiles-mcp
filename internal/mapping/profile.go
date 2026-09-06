package mapping

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// LoadMappingProfile reads and parses a mapping profile from a TOML file.
func LoadMappingProfile(path string) (*MappingProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	return ParseMappingProfile(string(data), path)
}

// ParseMappingProfile parses TOML content into a MappingProfile.
// It auto-detects legacy vs unified format.
func ParseMappingProfile(content, sourcePath string) (*MappingProfile, error) {
	var raw map[string]any
	if _, err := toml.Decode(content, &raw); err != nil {
		return nil, fmt.Errorf("invalid TOML: %w", err)
	}

	p := &MappingProfile{
		SourcePath: sourcePath,
		SourceName: strings.TrimSuffix(filepath.Base(sourcePath), ".toml"),
	}

	// Check for unified format marker.
	if _, hasProfile := raw["profile"]; hasProfile {
		if _, err := toml.Decode(content, p); err != nil {
			return nil, fmt.Errorf("decode unified profile: %w", err)
		}
		// Propagate profile-level layer to rules that don't set their own.
		if p.Profile != nil && p.Profile.Layer != 0 {
			for i := range p.Mappings {
				if p.Mappings[i].Layer == 0 {
					p.Mappings[i].Layer = p.Profile.Layer
				}
			}
		}
		return p, nil
	}

	// Legacy format: parse into the legacy fields.
	type legacyProfile struct {
		Remap     map[string][]string `toml:"remap"`
		Commands  map[string][]string `toml:"commands"`
		Movements map[string]string   `toml:"movements"`
		Settings  map[string]string   `toml:"settings"`
	}
	var legacy legacyProfile
	if _, err := toml.Decode(content, &legacy); err != nil {
		return nil, fmt.Errorf("decode legacy profile: %w", err)
	}

	p.Remap = legacy.Remap
	p.Commands = legacy.Commands
	p.Movements = legacy.Movements
	p.LegacySettings = legacy.Settings

	return p, nil
}
