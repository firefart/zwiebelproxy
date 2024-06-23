package tor

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strings"

	"github.com/firefart/zwiebelproxy/internal/helper"

	"github.com/andybalholm/brotli"
)

type Tor struct {
	logger           *slog.Logger
	domain           string
	blacklistedwords map[string]*regexp.Regexp
}

func New(logger *slog.Logger, domain string, blacklistedWords string) (*Tor, error) {
	t := Tor{
		logger:           logger,
		domain:           domain,
		blacklistedwords: make(map[string]*regexp.Regexp),
	}

	for _, word := range strings.Split(blacklistedWords, ",") {
		if word == "" {
			continue
		}
		fullRegex := fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(word))
		re, err := regexp.Compile(fullRegex)
		if err != nil {
			return nil, err
		}
		t.blacklistedwords[word] = re
	}

	return &t, nil
}

func (t *Tor) Rewrite(r *httputil.ProxyRequest) {
	domain := t.domain
	if !strings.HasPrefix(domain, ".") {
		domain = fmt.Sprintf(".%s", domain)
	}

	host, port, err := net.SplitHostPort(r.In.Host)
	if err != nil {
		// no port present
		host = r.In.Host
		port = r.In.URL.Port()
	}

	host = strings.TrimSuffix(host, domain)
	host = strings.TrimSuffix(host, ".")
	host = fmt.Sprintf("%s.onion", host)
	if port != "" && port != "80" && port != "443" {
		host = net.JoinHostPort(host, port)
	}

	scheme := r.In.URL.Scheme
	if scheme == "" {
		h := r.In.Header.Get("X-Forwarded-Proto")
		if h != "" {
			scheme = h
		} else {
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
	}
	if r.In.TLS != nil {
		scheme = "https"
	}

	r.Out.Host = host
	r.Out.URL.Scheme = scheme
	r.Out.URL.Host = host

	t.logger.Debug("modified request", slog.String("request", fmt.Sprintf("%+v", r.Out)))
}

// modify the response
func (t *Tor) ModifyResponse(resp *http.Response) error {
	t.logger.Debug("entered modifyResponse",
		slog.String("url", helper.SanitizeString(resp.Request.URL.String())),
		slog.Int("status-code", resp.StatusCode),
		slog.String("headers", fmt.Sprintf("%#v", resp.Header)),
	)

	domain := t.domain
	if !strings.HasPrefix(domain, ".") {
		domain = fmt.Sprintf(".%s", domain)
	}

	for k, v := range resp.Header {
		k = strings.ReplaceAll(k, ".onion", domain)
		resp.Header[k] = []string{}
		for _, v2 := range v {
			v2 = strings.ReplaceAll(v2, ".onion", domain)
			resp.Header[k] = append(resp.Header[k], v2)
		}
	}

	// remove headers like HSTS
	headersToRemove := []string{"Strict-Transport-Security", "Public-Key-Pins", "Public-Key-Pins-Report-Only"}
	for _, h := range headersToRemove {
		resp.Header.Del(h)
	}

	// no body modification on file downloads
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Disposition
	contentDisp, ok := resp.Header["Content-Disposition"]
	if ok && len(contentDisp) > 0 && strings.HasPrefix(contentDisp[0], "attachment") {
		t.logger.Debug("detected file download, not attempting to modify body", slog.String("url", helper.SanitizeString(resp.Request.URL.String())))
		return nil
	}

	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types/Common_types
	contentTypesForReplace := []string{
		"text/plain",
		"text/html",
		"text/css",
		"text/javascript",
		"text/xml",
		"application/x-javascript",
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
		t.logger.Debug("no content type skipping replace", slog.String("url", helper.SanitizeString(resp.Request.URL.String())))
		return nil
	}

	if ok && len(contentType) > 0 {
		// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Type
		cleanedUpContentType := strings.Split(contentType[0], ";")[0]
		if !helper.SliceContains(contentTypesForReplace, cleanedUpContentType) {
			t.logger.Debug("did not replace because of content type", slog.String("url", helper.SanitizeString(resp.Request.URL.String())), slog.String("content-type", cleanedUpContentType))
			return nil
		}
	}

	var reader io.Reader
	usedGzip := false
	usedZlib := false
	usedBrotli := false
	contentEncoding := resp.Header.Get("Content-Encoding")
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Encoding
	switch {
	case strings.EqualFold(contentEncoding, "gzip"):
		t.logger.Debug("detected gzipped body", slog.String("url", helper.SanitizeString(resp.Request.URL.String())))
		var err error
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("could not create gzip reader: %w", err)
		}
		// resp.Header.Del("Content-Encoding")
		usedGzip = true
	case strings.EqualFold(contentEncoding, "deflate"):
		t.logger.Debug("detected zlib body", slog.String("url", helper.SanitizeString(resp.Request.URL.String())))
		var err error
		reader, err = zlib.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("could not create zlib reader: %w", err)
		}
		usedZlib = true
	case strings.EqualFold(contentEncoding, "br"):
		t.logger.Debug("detected brotli body", slog.String("url", helper.SanitizeString(resp.Request.URL.String())))
		reader = brotli.NewReader(resp.Body)
		usedBrotli = true
	default:
		reader = resp.Body
	}

	// for all other content replace .onion urls with our custom domain
	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("error on reading body: %w", err)
	}

	// replace stuff for domain replacement
	body = bytes.ReplaceAll(body, []byte(".onion/"), []byte(fmt.Sprintf("%s/", domain)))
	body = bytes.ReplaceAll(body, []byte(`.onion"`), []byte(fmt.Sprintf(`%s"`, domain)))
	body = bytes.ReplaceAll(body, []byte(".onion<"), []byte(fmt.Sprintf("%s<", domain)))

	for word, re := range t.blacklistedwords {
		if re.Match(body) {
			return fmt.Errorf("access to the site is forbidden because it contains the blacklisted word %q", word)
		}
	}

	// if we unpacked before, respect the client and repack the modified body (the header is still set)
	if usedGzip {
		t.logger.Debug("re gzipping body", slog.String("url", helper.SanitizeString(resp.Request.URL.String())))
		gzipped, err := helper.GzipInput(body)
		if err != nil {
			return fmt.Errorf("could not gzip body: %w", err)
		}
		body = gzipped
	} else if usedZlib {
		t.logger.Debug("re zlibbing body", slog.String("url", helper.SanitizeString(resp.Request.URL.String())))
		zlibed, err := helper.ZlibInput(body)
		if err != nil {
			return fmt.Errorf("could not zlib body: %w", err)
		}
		body = zlibed
	} else if usedBrotli {
		t.logger.Debug("re brotliing body", slog.String("url", helper.SanitizeString(resp.Request.URL.String())))
		b, err := helper.BrotliInput(body)
		if err != nil {
			return fmt.Errorf("could not brotli body: %w", err)
		}
		body = b
	}

	// body can be read only once so recreate a new reader
	resp.Body = io.NopCloser(bytes.NewBuffer(body))

	// update the content-length to our new body
	resp.Header["Content-Length"] = []string{fmt.Sprint(len(body))}
	return nil
}
