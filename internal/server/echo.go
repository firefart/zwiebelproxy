package server

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/firefart/zwiebelproxy/internal/server/handlers"
	"github.com/firefart/zwiebelproxy/internal/server/templates"
	"github.com/labstack/echo/v4"
)

var cloudflareIPHeaderName = http.CanonicalHeaderKey("CF-Connecting-IP")

func (s *server) customHTTPErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	statusCode := http.StatusInternalServerError
	message := "An internal error occured."
	var echoError *echo.HTTPError
	if errors.As(err, &echoError) {
		statusCode = echoError.Code
		message = echoError.Message.(string)
	}

	// ignore 404 and stuff
	if err != nil && statusCode > 499 {
		s.logger.Error("error on request", slog.String("err", err.Error()))
	}

	if err2 := handlers.Render(c, statusCode, templates.Index(message)); err2 != nil {
		s.logger.Error(err2.Error())
	}
}

func extractIPFromCloudflareHeader() echo.IPExtractor {
	return func(req *http.Request) string {
		if realIP := req.Header.Get(cloudflareIPHeaderName); realIP != "" {
			return realIP
		}
		// fall back to normal ip extraction
		return echo.ExtractIPDirect()(req)
	}
}
