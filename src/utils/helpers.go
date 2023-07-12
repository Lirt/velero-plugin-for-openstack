package utils

import (
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
)

var (
	// Rand is used for a random generator exclusively for this go module
	Rand = rand.New(rand.NewSource(time.Now().UTC().UnixNano()))
	// regexp to parse OpenStack service microversion
	mvRe = regexp.MustCompile(`^(\d+).(\d+)$`)
)

// ErrStatus is used to indicate that a resource has unexpected Status
type ErrStatus struct {
	Status string
}

// Error satisfies golang error interface
func (e ErrStatus) Error() string {
	return fmt.Sprintf("unexpected %s status", e.Status)
}

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

// DurationToSeconds parses the string into a time.Duration format and returns
// seconds in int format
func DurationToSeconds(str string) (int, error) {
	t, err := time.ParseDuration(str)
	if err != nil {
		return 0, err
	}

	return int(t.Round(time.Second).Seconds()), nil
}

// WaitForStatus wait until the resource status satisfies the expected statuses
func WaitForStatus(statuses []string, timeout int, checkFunc func() (string, error)) error {
	return gophercloud.WaitFor(timeout, func() (bool, error) {
		status, err := checkFunc()
		if err != nil {
			if _, ok := err.(gophercloud.ErrDefault404); ok && SliceContains(statuses, "deleted") {
				return true, nil
			}
			return false, err
		}

		if SliceContains(statuses, status) {
			return true, nil
		}

		if strings.Contains(status, "error") {
			return false, ErrStatus{Status: status}
		}

		return false, nil
	})
}

// EnsureDeleted ensures that the resource is deleted, resets the resource
// status if it's in error state and tries again until the timeout
func EnsureDeleted(deleteFunc, checkFunc, resetFunc func() error, timeout int, delay int) error {
	retryDelay := time.Duration(delay) * time.Second
	duration := time.Duration(timeout) * time.Second
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		err := deleteFunc()
		if err != nil {
			switch err.(type) {
			case gophercloud.ErrDefault404:
				return nil
			case gophercloud.ErrDefault409:
				time.Sleep(retryDelay)
				continue
			}
			return err
		}

		errChan := make(chan error, 1)
		go func() {
			errChan <- checkFunc()
		}()

		select {
		case <-timer.C:
			go func() {
				// drain errChan if timer triggered before errChan
				<-errChan
			}()
			return fmt.Errorf("wait time exceeded: %s", duration)
		case err := <-errChan:
			// resource is deleted
			if err == nil {
				return nil
			}

			// if the status not expected, reset the status and try again
			if _, ok := err.(ErrStatus); !ok {
				return err
			}
		}

		// reset status and try to delete it again
		err = resetFunc()
		if err != nil {
			if _, ok := err.(gophercloud.ErrDefault404); ok {
				return nil
			}
			return err
		}

		time.Sleep(retryDelay)
	}
}
