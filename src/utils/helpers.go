package utils

import (
	"os"
)

// GetEnv gets value from environment variable or fallbacks to default value
// This snippet is from https://stackoverflow.com/a/40326580/3323419
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
