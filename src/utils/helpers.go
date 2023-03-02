package utils

import (
	"os"
	"strings"
)

// GetEnv gets value from environment variable or fallbacks to default value
// This snippet is from https://stackoverflow.com/a/40326580/3323419
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// ReplaceAccount replaces an endpoint account part with a new account value
func ReplaceAccount(account, path string, prefixes []string) string {
	parts := strings.Split(path, "/")
	for _, prefix := range prefixes {
		for i, part := range parts {
			if strings.HasPrefix(part, prefix) {
				parts[i] = prefix + account
				break
			}
		}
	}
	return strings.Join(parts, "/")
}
