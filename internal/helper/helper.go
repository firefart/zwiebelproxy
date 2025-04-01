package helper

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

func LookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func LookupEnvOrBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		v, err := strconv.ParseBool(val)
		if err != nil {
			return defaultVal
		}
		return v
	}
	return defaultVal
}

func LookupEnvOrDuration(key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		v, err := time.ParseDuration(val)
		if err != nil {
			return defaultVal
		}
		return v
	}
	return defaultVal
}

func SliceContains(slice []string, value string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func SanitizeString(in string) string {
	escaped := strings.ReplaceAll(in, "\n", "")
	escaped = strings.ReplaceAll(escaped, "\r", "")
	return escaped
}

func GzipInput(data []byte) ([]byte, error) {
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

func ZlibInput(data []byte) ([]byte, error) {
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

func BrotliInput(data []byte) ([]byte, error) {
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
