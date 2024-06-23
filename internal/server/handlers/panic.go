package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

type PanicHandler struct {
	debug                bool
	logger               *slog.Logger
	secretKeyHeaderName  string
	secretKeyHeaderValue string
}

func NewPanicHandler(logger *slog.Logger, debug bool, secretKeyHeaderName, secretKeyHeaderValue string) *PanicHandler {
	return &PanicHandler{
		debug:                debug,
		logger:               logger,
		secretKeyHeaderName:  secretKeyHeaderName,
		secretKeyHeaderValue: secretKeyHeaderValue,
	}
}

func (h *PanicHandler) Handler(c echo.Context) error {
	// no checks in debug mode
	if h.debug {
		panic("test")
	}

	headerValue := c.Request().Header.Get(h.secretKeyHeaderName)
	if headerValue == "" {
		h.logger.Error("test_panic called without secret header")
	} else if headerValue == h.secretKeyHeaderValue {
		panic("test")
	} else {
		h.logger.Error("test_panic called without valid header")
	}
	return c.NoContent(http.StatusOK)
}
