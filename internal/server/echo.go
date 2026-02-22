package server

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/firefart/zwiebelproxy/internal/server/handlers"
	"github.com/firefart/zwiebelproxy/internal/server/templates"
	"github.com/labstack/echo/v5"
)

var cloudflareIPHeaderName = http.CanonicalHeaderKey("CF-Connecting-IP")

func (s *server) customHTTPErrorHandler(c *echo.Context, err error) {
	if resp, uErr := echo.UnwrapResponse(c.Response()); uErr == nil {
		if resp.Committed {
			return // response has been already sent to the client by handler or some middleware
		}
	}

	statusCode := http.StatusInternalServerError
	message := "Internal Server Error"

	if he, ok := errors.AsType[*echo.HTTPError](err); ok { // find error in an error chain that implements HTTPError
		statusCode = he.Code
		message = he.Message
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
