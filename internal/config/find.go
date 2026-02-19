package config

import "os"

// FindConfigPath resolves the configuration path.
//
// Precedence:
//  1. explicit argument
//  2. SNIP_CONFIG env var
//  3. default .snip.yaml in the current working directory
func FindConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := os.Getenv("SNIP_CONFIG"); v != "" {
		return v
	}
	return ".snip.yaml"
}
