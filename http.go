package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type httpClient struct {
	client *http.Client
}

func newHTTPClient(timeout time.Duration, proxyURL string) (*httpClient, error) {
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url %s: %w", proxyURL, err)
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		Proxy:           http.ProxyURL(proxy),
	}
	client := http.Client{
		Timeout:   timeout,
		Transport: tr,
	}
	return &httpClient{
		client: &client,
	}, nil
}
