package utils

import (
	"reflect"
	"testing"
	"time"
)

func TestReplaceAccount(t *testing.T) {
	tests := []struct {
		name     string
		account  string
		endpoint string
		prefixes []string
		expected string
	}{
		{
			name:     "endpoint with account and existing prefix",
			account:  "john-doe",
			endpoint: "https://swift.openstack.domain.com:443/swift/v1/AUTH_randomstring1234",
			prefixes: []string{"AUTH_", "SERVICE_"},
			expected: "https://swift.openstack.domain.com:443/swift/v1/AUTH_john-doe",
		},
		{
			name:     "endpoint with account and non-existing prefix",
			account:  "john-doe",
			endpoint: "https://swift.openstack.domain.com:443/swift/v1/AUTH_randomstring1234",
			prefixes: []string{"IMAGE_"},
			expected: "https://swift.openstack.domain.com:443/swift/v1/AUTH_randomstring1234",
		},
		{
			name:     "endpoint without scheme, with account and existing prefix",
			account:  "john-doe",
			endpoint: "swift.domain.com:443/swift/v1/SERVICE_randomstring1234",
			prefixes: []string{"SERVICE_"},
			expected: "swift.domain.com:443/swift/v1/SERVICE_john-doe",
		},
	}
	for _, tt := range tests {
		path := ReplaceAccount(tt.account, tt.endpoint, tt.prefixes)
		if path != tt.expected {
			t.Errorf("[%s] failed - output %s doesn't match expected '%s'", tt.name, path, tt.expected)
		}
	}
}

func TestRand(t *testing.T) {
	Rand.Seed(time.Now().UTC().UnixNano())
	a := Rand.Uint64()
	Rand.Seed(time.Now().UTC().UnixNano())
	b := Rand.Uint64()

	if a == b {
		t.Errorf("failed to verify random seed generator %v == %v", a, b)
	}

	Rand.Seed(int64(time.Now().UTC().Second()))
	a = Rand.Uint64()
	Rand.Seed(int64(time.Now().UTC().Second()))
	b = Rand.Uint64()

	if a != b {
		t.Errorf("failed to verify random seed generator %v != %v", a, b)
	}
}

func TestCompareMicroversions(t *testing.T) {
	type vals struct {
		want string
		op   string
		have string
		res  bool
	}
	tests := []vals{
		{
			"2.7", "lte", "2.50", true,
		},
		{
			"1.7", "lte", "2.50", true,
		},
		{
			"3.7", "lte", "2.50", false,
		},
		{
			"2.50", "lte", "2.50", true,
		},
		{
			"2.7", "gte", "2.50", false,
		},
		{
			"1.50", "gte", "2.50", false,
		},
		{
			"2.50", "gte", "2.50", true,
		},
	}

	for i, test := range tests {
		if v, err := CompareMicroversions(test.op, test.want, test.have); err != nil {
			t.Errorf("[%d] test failed: %v", i, err)
		} else if test.res != v {
			t.Errorf("[%d] test failed: expected %t, got %t", i, test.res, v)
		}
	}
}

func TestMerge(t *testing.T) {
	tests := map[*map[string]string][]map[string]string{
		&(map[string]string{
			"a": "c",
			"b": "c",
			"c": "d",
		}): {
			{
				"a": "b",
				"c": "d",
			},
			{
				"a": "c",
			},
			{
				"b": "c",
			},
		},
	}

	for d, s := range tests {
		if m := Merge(s...); !reflect.DeepEqual(*d, m) {
			t.Errorf("test failed: expected %q, got %q", *d, m)
		}
	}
}

func TestDurationToSeconds(t *testing.T) {
	tests := map[string]int{
		"5m":  300,
		"30m": 1800,
		"1h":  3600,
		"8h":  28800,
	}

	for d, s := range tests {
		if v, err := DurationToSeconds(d); err != nil {
			t.Errorf("[%s] test failed: %v", d, err)
		} else if v != s {
			t.Errorf("[%s] test failed: expected %d, got %d", d, s, v)
		}
	}
}
