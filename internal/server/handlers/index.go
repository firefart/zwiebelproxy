package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/firefart/zwiebelproxy/internal/server/templates"
	"github.com/firefart/zwiebelproxy/internal/tor"
	"github.com/labstack/echo/v5"
)

type IndexHandler struct {
	domain           string
	debug            bool
	blacklistedWords string
	logger           *slog.Logger
	transport        *http.Transport
	timeout          time.Duration
}

func NewIndexHandler(logger *slog.Logger, debug bool, domain string, blacklistedWords string, transport *http.Transport, timeout time.Duration) *IndexHandler {
	return &IndexHandler{
		logger:           logger,
		debug:            debug,
		domain:           domain,
		blacklistedWords: blacklistedWords,
		transport:        transport,
		timeout:          timeout,
	}
}

func (h *IndexHandler) Handler(c *echo.Context) error {
	r := c.Request()
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		// no port present
		host = r.Host
	}

	// show info page when top domain is called
	if host == strings.TrimLeft(h.domain, ".") {
		return Render(c, http.StatusOK, templates.Index(""))
	}

	if !strings.HasSuffix(host, h.domain) {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid domain %s called. The domain needs to end in %s", host, h.domain))
	}

	tor, err := tor.New(h.logger, h.domain, h.blacklistedWords)
	if err != nil {
		return fmt.Errorf("could not create tor object: %w", err)
	}

	proxy := httputil.ReverseProxy{
		Rewrite:        tor.Rewrite,
		FlushInterval:  -1,
		ModifyResponse: tor.ModifyResponse,
		Transport:      h.transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			h.logger.Error("error on reverse proxy", slog.String("url", r.RequestURI), slog.String("err", err.Error()))
			w.WriteHeader(http.StatusBadGateway)
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Connection", "close")
			if err := templates.Index(err.Error()).Render(r.Context(), w); err != nil {
				panic(err.Error())
			}
		},
	}

	h.logger.Debug("original request", slog.String("request", fmt.Sprintf("%+v", r)))

	// set a custom timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	r = r.WithContext(ctx)

	res, err := echo.UnwrapResponse(c.Response())
	if err != nil {
		return fmt.Errorf("could not unwrap response: %w", err)
	}

	proxy.ServeHTTP(res, r)
	return nil
}
