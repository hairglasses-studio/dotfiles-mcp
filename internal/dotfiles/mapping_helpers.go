// mapping_helpers.go — Local wrappers around the shared mapping module.
package dotfiles

import "github.com/hairglasses-studio/dotfiles-mcp/internal/mapping"

// listMappingProfiles wraps mapping.ListMappingProfiles with the local makima directory.
func listMappingProfiles() ([]mapping.MappingProfileSummary, error) {
	return mapping.ListMappingProfiles(makimaDir())
}
