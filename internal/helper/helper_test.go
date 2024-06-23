package helper

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSliceContains(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		slice    []string
		value    string
		expected bool
	}{
		{"negative", []string{"1", "2", "3"}, "5", false},
		{"positive", []string{"1", "2", "3"}, "1", true},
		{"empty string negative", []string{"1", "2", "3"}, "", false},
		{"empty string positive", []string{"", "1", "2", "3"}, "", true},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other

			res := SliceContains(tt.slice, tt.value)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestLookupEnvOrString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		setEnv       bool
		value        string
		defaultValue string
		expected     string
	}{
		{setEnv: true, value: "asdf", defaultValue: "qwr", expected: "asdf"},
		{setEnv: false, value: "asdf", defaultValue: "qwr", expected: "qwr"},
		{setEnv: true, value: "", defaultValue: "qwr", expected: ""},
		{setEnv: false, value: "", defaultValue: "", expected: ""},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run("", func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other

			envName := RandString(10)

			if tt.setEnv {
				os.Setenv(envName, tt.value)
				defer os.Unsetenv(envName)
			}
			res := LookupEnvOrString(envName, tt.defaultValue)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestLookupEnvOrBool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		setEnv       bool
		value        string
		defaultValue bool
		expected     bool
	}{
		{setEnv: true, value: "invalid", defaultValue: false, expected: false},
		{setEnv: true, value: "false", defaultValue: true, expected: false},
		{setEnv: true, value: "true", defaultValue: false, expected: true},
		{setEnv: true, value: "FALSE", defaultValue: true, expected: false},
		{setEnv: true, value: "TRUE", defaultValue: false, expected: true},
		{setEnv: true, value: "False", defaultValue: true, expected: false},
		{setEnv: true, value: "True", defaultValue: false, expected: true},
		{setEnv: true, value: "0", defaultValue: true, expected: false},
		{setEnv: true, value: "1", defaultValue: false, expected: true},
		{setEnv: false, value: "", defaultValue: false, expected: false},
		{setEnv: false, value: "", defaultValue: true, expected: true},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run("", func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other

			envName := RandString(10)

			if tt.setEnv {
				os.Setenv(envName, tt.value)
				defer os.Unsetenv(envName)
			}
			res := LookupEnvOrBool(envName, tt.defaultValue)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestLookupEnvOrDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		setEnv       bool
		value        string
		defaultValue time.Duration
		expected     time.Duration
	}{
		{setEnv: true, value: "invalid", defaultValue: 10 * time.Minute, expected: 10 * time.Minute},
		{setEnv: true, value: "10m", defaultValue: 1 * time.Minute, expected: 10 * time.Minute},
		{setEnv: true, value: "10h", defaultValue: 1 * time.Minute, expected: 10 * time.Hour},
		{setEnv: false, value: "", defaultValue: 10 * time.Minute, expected: 10 * time.Minute},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run("", func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other

			envName := RandString(10)

			if tt.setEnv {
				os.Setenv(envName, tt.value)
				defer os.Unsetenv(envName)
			}
			res := LookupEnvOrDuration(envName, tt.defaultValue)
			assert.Equal(t, tt.expected, res)
		})
	}
}
