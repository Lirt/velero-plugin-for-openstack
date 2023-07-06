package utils

import (
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Rand is used for a random generator exclusively for this go module
	Rand = rand.New(rand.NewSource(time.Now().UTC().UnixNano()))
	// regexp to parse OpenStack service microversion
	mvRe = regexp.MustCompile(`^(\d+).(\d+)$`)
)

// GetEnv gets value from environment variable or fallbacks to default value
// This snippet is from https://stackoverflow.com/a/40326580/3323419
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// GetConf gets a value from a config map or fallbacks to default value
func GetConf(config map[string]string, key, fallback string) string {
	if value, ok := config[key]; ok && value != "" {
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

// CompareMicroversions compares two microversions using operators:
// lte: less than or equal
// gte: greater than or equal
func CompareMicroversions(operator, want, have string) (bool, error) {
	if operator != "lte" && operator != "gte" {
		return false, fmt.Errorf("invalid microversions comparison %q operator, must be lte or gte", operator)
	}

	w, err := microversionToInt(want)
	if err != nil {
		return false, err
	}

	h, err := microversionToInt(have)
	if err != nil {
		return false, err
	}

	// lte
	if operator == "lte" {
		if w[0] < h[0] {
			return true, nil
		}

		return w[0] <= h[0] && w[1] <= h[1], nil
	}

	// gte
	if w[0] > h[0] {
		return true, nil
	}

	return w[0] >= h[0] && w[1] >= h[1], nil
}

func microversionToInt(mv string) ([]int, error) {
	res := mvRe.FindAllStringSubmatch(mv, -1)
	if len(res) == 1 && len(res[0]) == 3 {
		ver := res[0][1:]
		major, _ := strconv.Atoi(ver[0])
		minor, _ := strconv.Atoi(ver[1])
		return []int{
			major,
			minor,
		}, nil
	}
	return nil, fmt.Errorf("invalid microversion string: %v", mv)
}

// SliceContains checks whether a slice of comparable type contains an element
func SliceContains[T comparable](elems []T, e T) bool {
	for _, v := range elems {
		if v == e {
			return true
		}
	}
	return false
}

// Merge merges maps. If more than one given map with the same key, then the
// one that is later in the argument sequence takes precedence
func Merge(args ...map[string]string) map[string]string {
	m := make(map[string]string)
	for _, arg := range args {
		for k, v := range arg {
			m[k] = v
		}
	}
	return m
}
