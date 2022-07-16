package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func lookupEnvOrString(log *logrus.Logger, key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func lookupEnvOrBool(log *logrus.Logger, key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		v, err := strconv.ParseBool(val)
		if err != nil {
			log.Fatalf("lookupEnvOrBool[%s]: %v", key, err)
		}
		return v
	}
	return defaultVal
}

func lookupEnvOrDuration(log *logrus.Logger, key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		v, err := time.ParseDuration(val)
		if err != nil {
			log.Fatalf("lookupEnvOrDuration[%s]: %v", key, err)
		}
		return v
	}
	return defaultVal
}

func sliceContains[T comparable](slice []T, value T) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

func sanitizeString(in string) string {
	escaped := strings.Replace(in, "\n", "", -1)
	escaped = strings.Replace(escaped, "\r", "", -1)
	return escaped
}
