// mapping_helpers.go — Local wrappers around the shared mapping module.
package main

import "github.com/hairglasses-studio/mapping"

// listMappingProfiles wraps mapping.ListMappingProfiles with the local makima directory.
func listMappingProfiles() ([]mapping.MappingProfileSummary, error) {
	return mapping.ListMappingProfiles(makimaDir())
}
