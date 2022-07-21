package main

import (
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func lookupEnvOrString(log Logger, key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func lookupEnvOrBool(log Logger, key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		v, err := strconv.ParseBool(val)
		if err != nil {
			log.Errorf("lookupEnvOrBool[%s]: %v", key, err)
			return defaultVal
		}
		return v
	}
	return defaultVal
}

func lookupEnvOrDuration(log Logger, key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		v, err := time.ParseDuration(val)
		if err != nil {
			log.Errorf("lookupEnvOrDuration[%s]: %v", key, err)
			return defaultVal
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
