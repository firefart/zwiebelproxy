package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func (s *server) middlewareRecover() echo.MiddlewareFunc {
	return middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			// send the error to the default error handler
			return fmt.Errorf("PANIC! %v - %s", err, string(stack))
		},
	})
}

func (s *server) middlewareRequestLogger(ctx context.Context) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:        true,
		LogURI:           true,
		LogUserAgent:     true,
		LogLatency:       true,
		LogRemoteIP:      true,
		LogMethod:        true,
		LogContentLength: true,
		LogResponseSize:  true,
		LogError:         true,
		HandleError:      true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			logLevel := slog.LevelInfo
			errString := ""
			// only set error on real errors
			if v.Error != nil && v.Status > 499 {
				errString = v.Error.Error()
				logLevel = slog.LevelError
			}
			s.logger.LogAttrs(ctx, logLevel, "REQUEST",
				slog.String("ip", v.RemoteIP),
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.String("user-agent", v.UserAgent),
				slog.Duration("request-duration", v.Latency),
				slog.String("request-length", v.ContentLength), // request content length
				slog.Int64("response-size", v.ResponseSize),
				slog.String("err", errString))

			return nil
		},
	})
}

func (s *server) xHeaderMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		r := c.Request()
		for headerName, headerValue := range r.Header {
			switch strings.ToLower(headerName) {
			case "x-forwarded-port":
				port := headerValue[0]
				host, _, err := net.SplitHostPort(r.URL.Host)
				if err != nil {
					// err occurs if no port present so append one
					if port != "" && port != "80" && port != "443" {
						r.URL.Host = net.JoinHostPort(r.URL.Host, port)
					}
				} else {
					if port != "" && port != "80" && port != "443" {
						r.URL.Host = net.JoinHostPort(host, port)
					} else {
						r.URL.Host = host
					}
				}
				host, _, err = net.SplitHostPort(r.Host)
				if err != nil {
					// err occurs if no port present so append one
					if port != "" && port != "80" && port != "443" {
						r.Host = net.JoinHostPort(r.Host, port)
					}
				} else {
					if port != "" && port != "80" && port != "443" {
						r.Host = net.JoinHostPort(host, port)
					} else {
						r.Host = host
					}
				}
				delete(r.Header, headerName)
			case "x-forwarded-proto":
				r.URL.Scheme = headerValue[0]
				delete(r.Header, headerName)
			}
		}
		return next(c)
	}
}

func (s *server) ipAuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if len(s.allowedHosts) == 0 && len(s.allowedIPs) == 0 && len(s.allowedIPRanges) == 0 {
			// configured as a public server, no ip checks
			return next(c)
		}

		r := c.Request()

		remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteIP = r.RemoteAddr
		}
		remoteIP = strings.TrimSpace(remoteIP)

		if remoteIP == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "could not determine remote ip")
		}

		ipParsed, err := netip.ParseAddr(remoteIP)
		if err != nil {
			s.logger.Error("could not parse remote ip", slog.String("err", err.Error()))
			return echo.NewHTTPError(http.StatusBadGateway, "could not parse remote ip")
		}

		for _, ip := range s.allowedIPs {
			if ip == remoteIP {
				s.logger.Info("allowing whitelisted ip", slog.String("ip", ip))
				return next(c)
			}
		}

		for _, prefix := range s.allowedIPRanges {
			if prefix.Contains(ipParsed) {
				s.logger.Info("allowing whitelisted ip range", slog.String("ip", remoteIP), slog.String("matched-prefix", prefix.String()))
				return next(c)
			}
		}

		for _, d := range s.allowedHosts {
			dynamicIP, err := s.dnsClient.IPLookup(r.Context(), d)
			if err != nil {
				s.logger.Error("invalid domain in config", slog.String("domain", d), slog.String("err", err.Error()))
				return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
			}

			s.logger.Debug("dns resolved", slog.String("host", d), slog.String("ips", strings.Join(dynamicIP, ", ")))
			for _, i := range dynamicIP {
				if i == remoteIP {
					s.logger.Info("allowing client", slog.String("ip", remoteIP), slog.String("hostname", d))
					return next(c)
				}
			}
		}

		s.logger.Error("access denied", slog.String("remote-ip", remoteIP))
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}
}
