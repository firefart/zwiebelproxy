package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/firefart/zwiebelproxy/internal/dns"
	"github.com/firefart/zwiebelproxy/internal/server/handlers"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type server struct {
	logger          *slog.Logger
	dnsClient       *dns.DnsClient
	allowedHosts    []string
	allowedIPs      []string
	allowedIPRanges []netip.Prefix
}

func NewServer(ctx context.Context,
	logger *slog.Logger,
	cloudflare bool,
	revProxy bool,
	debug bool,
	domain string,
	blacklistedWords string,
	secretKeyHeaderName string,
	secretKeyHeaderValue string,
	timeout time.Duration,
	dnsCacheTimeout time.Duration,
	allowedHosts []string,
	allowedIPs []string,
	allowedIPRanges []netip.Prefix,
	transport *http.Transport,
) http.Handler {
	s := server{
		logger:          logger,
		dnsClient:       dns.NewDNSClient(timeout, dnsCacheTimeout),
		allowedHosts:    allowedHosts,
		allowedIPs:      allowedIPs,
		allowedIPRanges: allowedIPRanges,
	}

	e := echo.New()
	e.HideBanner = true
	e.Debug = debug
	e.HTTPErrorHandler = s.customHTTPErrorHandler

	if cloudflare {
		e.IPExtractor = extractIPFromCloudflareHeader()
	} else if revProxy {
		e.IPExtractor = echo.ExtractIPFromXFFHeader()
	} else {
		e.IPExtractor = echo.ExtractIPDirect()
	}

	e.Use(s.middlewareRequestLogger(ctx))
	e.Use(middleware.Secure())
	// use forwarding proxy port and schema information
	e.Use(s.xHeaderMiddleware)
	e.Use(s.ipAuthMiddleware)
	e.Use(s.middlewareRecover())

	secretKeyHeaderName = http.CanonicalHeaderKey(secretKeyHeaderName)
	e.GET("/test/panic", handlers.NewPanicHandler(s.logger, debug, secretKeyHeaderName, secretKeyHeaderValue).Handler)

	e.GET("/*", handlers.NewIndexHandler(s.logger, debug, domain, blacklistedWords, transport, timeout).Handler)
	return e
}
