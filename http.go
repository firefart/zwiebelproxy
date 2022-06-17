package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

type httpClient struct {
	tr *http.Transport
}

func newHTTPClient(timeout time.Duration, proxyURL string) (*httpClient, error) {
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url %s: %w", proxyURL, err)
	}

	// used to clone the defaul transport
	tr := http.DefaultTransport.(*http.Transport)
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	tr.Proxy = http.ProxyURL(proxy)
	tr.TLSHandshakeTimeout = timeout
	tr.ExpectContinueTimeout = timeout
	tr.ResponseHeaderTimeout = timeout
	tr.DialContext = (&net.Dialer{
		Timeout:   timeout,
		KeepAlive: timeout,
	}).DialContext

	return &httpClient{
		tr: tr,
	}, nil
}
