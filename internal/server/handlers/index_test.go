package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/firefart/zwiebelproxy/internal/server"
	"github.com/firefart/zwiebelproxy/internal/server/handlers"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// This test is currently non functional as we would need to mock all tor proxy stuff and I'm currently to lazy to implement it
func TestIndex(t *testing.T) {
	t.Skip("this test is currently disabled until all mockings are implemented")

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	file, err := os.CreateTemp("", "*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	tr := http.DefaultTransport.(*http.Transport)
	e := server.NewServer(ctx, logger, false, false, false, "localhost.onion", "", "TEST", "TEST", 1*time.Minute, 1*time.Minute, nil, nil, nil, tr)
	x, ok := e.(*echo.Echo)
	require.True(t, ok)
	req := httptest.NewRequest(http.MethodGet, "https://test.localhost.onion", nil)
	rec := httptest.NewRecorder()
	cont := x.NewContext(req, rec)
	require.Nil(t, handlers.NewIndexHandler(logger, false, "localhost.onion", "", tr, 1*time.Minute).Handler(cont))
	require.Equal(t, http.StatusOK, rec.Code) //
	require.Greater(t, len(rec.Body.String()), 10)
}
