package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// modify the request
func (app *application) director(r *http.Request) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		// no port present
		host = r.Host
		port = r.URL.Port()
	}

	domain := app.domain
	if !strings.HasPrefix(domain, ".") {
		domain = fmt.Sprintf(".%s", domain)
	}

	host = strings.TrimSuffix(host, domain)
	host = strings.TrimSuffix(host, ".")
	host = fmt.Sprintf("%s.onion", host)
	if port != "" && port != "80" && port != "443" {
		host = net.JoinHostPort(host, port)
	}

	scheme := r.URL.Scheme
	if scheme == "" {
		switch port {
		case "":
			scheme = "http"
		case "80":
			scheme = "http"
		case "443":
			scheme = "https"
		default:
			scheme = "http"
		}
	}

	app.logger.Debugf("r.port: %#v", sanitizeString(fmt.Sprintf("%#v", port)))
	app.logger.Debugf("r.URL: %#v", sanitizeString(fmt.Sprintf("%#v", r.URL)))
	app.logger.Debugf("r.RequestURI: %#v", sanitizeString(fmt.Sprintf("%#v", r.RequestURI)))
	app.logger.Debugf("r.Host: %#v", sanitizeString(fmt.Sprintf("%#v", r.Host)))
	app.logger.Debugf("r.Header: %#v", sanitizeString(fmt.Sprintf("%#v", r.Header)))

	// needed so the ip will not be leaked
	r.Header["X-Forwarded-For"] = nil

	r.URL.Scheme = scheme
	r.URL.Host = host
	r.Host = host

	app.logger.Debugf("r.port: %#v", sanitizeString(fmt.Sprintf("%#v", port)))
	app.logger.Debugf("r.URL: %#v", sanitizeString(fmt.Sprintf("%#v", r.URL)))
	app.logger.Debugf("r.RequestURI: %#v", sanitizeString(fmt.Sprintf("%#v", r.RequestURI)))
	app.logger.Debugf("r.Host: %#v", sanitizeString(fmt.Sprintf("%#v", r.Host)))
	app.logger.Debugf("r.Header: %#v", sanitizeString(fmt.Sprintf("%#v", r.Header)))
}

// modify the response
func (app *application) modifyResponse(resp *http.Response) error {
	app.logger.Debugf("entered modifyResponse for %s with status %d", sanitizeString(resp.Request.URL.String()), resp.StatusCode)

	domain := app.domain
	if !strings.HasPrefix(domain, ".") {
		domain = fmt.Sprintf(".%s", domain)
	}

	app.logger.Debugf("Header: %#v", resp.Header)
	for k, v := range resp.Header {
		k = strings.ReplaceAll(k, ".onion", domain)
		resp.Header[k] = []string{}
		for _, v2 := range v {
			v2 = strings.ReplaceAll(v2, ".onion", domain)
			resp.Header[k] = append(resp.Header[k], v2)
		}
	}

	// no body modification on file downloads
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Disposition
	contentDisp, ok := resp.Header["Content-Disposition"]
	if ok && len(contentDisp) > 0 && strings.HasPrefix(contentDisp[0], "attachment") {
		app.logger.Debugf("%s - detected file download, not attempting to modify body", sanitizeString(resp.Request.URL.String()))
		return nil
	}

	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types/Common_types
	contentTypesForReplace := []string{
		"text/plain",
		"text/html",
		"text/css",
		"text/javascript",
		"text/xml",
		"application/javascript",
		"application/json",
		"application/ld+json",
		"application/xml",
		"application/rss+xml",
		"application/atom+xml",
		"application/rdf+xml",
	}

	contentType, ok := resp.Header["Content-Type"]
	if !ok {
		app.logger.Debugf("%s - no content type skipping replace", sanitizeString(resp.Request.URL.String()))
		return nil
	}

	if ok && len(contentType) > 0 {
		// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Type
		cleanedUpContentType := strings.Split(contentType[0], ";")[0]
		if !sliceContains(contentTypesForReplace, cleanedUpContentType) {
			app.logger.Debugf("%s - content type is %s, not replacing", sanitizeString(resp.Request.URL.String()), cleanedUpContentType)
			return nil
		}
	}

	app.logger.Debugf("%s - found content type %s, replacing strings", sanitizeString(resp.Request.URL.String()), contentType[0])

	// for all other content replace .onion urls with our custom domain
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error on reading body: %w", err)
	}
	app.logger.Debugf("%s: Got a %d body len", sanitizeString(resp.Request.URL.String()), len(body))
	// replace stuff for domain replacement
	body = bytes.ReplaceAll(body, []byte(".onion/"), []byte(fmt.Sprintf("%s/", domain)))
	body = bytes.ReplaceAll(body, []byte(`.onion"`), []byte(fmt.Sprintf(`%s"`, domain)))
	body = bytes.ReplaceAll(body, []byte(".onion<"), []byte(fmt.Sprintf("%s<", domain)))

	// body can be read only once so recreate a new reader
	resp.Body = io.NopCloser(bytes.NewBuffer(body))
	// update the content-length to our new body
	resp.Header["Content-Length"] = []string{fmt.Sprint(len(body))}
	return nil
}
