package main

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func lookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func lookupEnvOrBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		v, err := strconv.ParseBool(val)
		if err != nil {
			return defaultVal
		}
		return v
	}
	return defaultVal
}

func lookupEnvOrDuration(key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		v, err := time.ParseDuration(val)
		if err != nil {
			return defaultVal
		}
		return v
	}
	return defaultVal
}

func sliceContains(slice []string, value string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, value) {
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

func gzipInput(data []byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)

	_, err := gz.Write(data)
	if err != nil {
		return nil, err
	}

	if err = gz.Flush(); err != nil {
		return nil, err
	}

	if err = gz.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func zlibInput(data []byte) ([]byte, error) {
	var b bytes.Buffer
	z := zlib.NewWriter(&b)

	_, err := z.Write(data)
	if err != nil {
		return nil, err
	}

	if err = z.Flush(); err != nil {
		return nil, err
	}

	if err = z.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func brotliInput(data []byte) ([]byte, error) {
	var b bytes.Buffer
	z := brotli.NewWriter(&b)

	_, err := z.Write(data)
	if err != nil {
		return nil, err
	}

	if err = z.Flush(); err != nil {
		return nil, err
	}

	if err = z.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func DeleteEmptyItems(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}
