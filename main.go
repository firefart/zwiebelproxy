package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

type application struct {
	httpClient *httpClient
	domain     string
	timeout    time.Duration
	logger     *logrus.Logger
}

func main() {
	log := logrus.New()
	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.InfoLevel)

	err := godotenv.Load()
	if err != nil {
		log.Warnf("could not load .env file: %v. continuing without", err)
	}

	host := flag.String("host", lookupEnvOrString(log, "ZWIEBEL_HOST", "127.0.0.1:8080"), "IP and Port to bind to. You can also use the ZWIEBEL_HOST environment variable or an entry in the .env file to set this parameter.")
	debug := flag.Bool("debug", lookupEnvOrBool(log, "ZWIEBEL_DEBUG", false), "Enable DEBUG mode. You can also use the ZWIEBEL_DEBUG environment variable or an entry in the .env file to set this parameter.")
	domain := flag.String("domain", lookupEnvOrString(log, "ZWIEBEL_DOMAIN", ""), "domain to use. You can also use the ZWIEBEL_DOMAIN environment variable or an entry in the .env file to set this parameter.")
	tor := flag.String("tor", lookupEnvOrString(log, "ZWIEBEL_TOR", "socks5://127.0.0.1:9050"), "TOR Proxy server. You can also use the ZWIEBEL_TOR environment variable or an entry in the .env file to set this parameter.")
	wait := flag.Duration("graceful-timeout", lookupEnvOrDuration(log, "ZWIEBEL_GRACEFUL_TIMEOUT", 5*time.Second), "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m. You can also use the ZWIEBEL_GRACEFUL_TIMEOUT environment variable or an entry in the .env file to set this parameter.")
	timeout := flag.Duration("timeout", lookupEnvOrDuration(log, "ZWIEBEL_TIMEOUT", 5*time.Minute), "http timeout. You can also use the ZWIEBEL_TIMEOUT environment variable or an entry in the .env file to set this parameter.")
	flag.Parse()

	if *debug {
		log.SetLevel(logrus.DebugLevel)
		log.Debug("DEBUG mode enabled")
	}

	if len(*domain) == 0 {
		log.Errorf("please provide a domain")
		os.Exit(1)
	}

	if !strings.HasPrefix(*domain, ".") {
		var a = fmt.Sprintf(".%s", *domain)
		domain = &a
	}

	httpClient, err := newHTTPClient(*timeout, *tor)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	app := &application{
		httpClient: httpClient,
		domain:     *domain,
		timeout:    *timeout,
		logger:     log,
	}

	srv := &http.Server{
		Addr:    *host,
		Handler: app.routes(),
	}
	log.Infof("Starting server on %s", *host)

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error(err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	<-c
	ctx, cancel := context.WithTimeout(context.Background(), *wait)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error(err)
	}
	log.Info("shutting down")
	os.Exit(0)
}

func (app *application) routes() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(app.xHeaderMiddleware)
	r.Use(middleware.Recoverer)

	r.Use(middleware.Timeout(app.timeout))

	ph := http.HandlerFunc(app.proxyHandler)
	r.Handle("/*", ph)
	return r
}

func (app *application) logError(w http.ResponseWriter, err error, withTrace bool, status int) {
	w.Header().Set("Connection", "close")
	errorText := fmt.Sprintf("%v", err)
	app.logger.Error(errorText)
	if withTrace {
		app.logger.Errorf("%s", debug.Stack())
	}
	http.Error(w, http.StatusText(status), status)
}

func (app *application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.Host, app.domain) {
		app.logError(w, fmt.Errorf("invalid domain %s", r.Host), false, http.StatusBadRequest)
		return
	}
	req := r.Clone(r.Context())
	req.RequestURI = ""
	// strip own hostname
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		// no port present
		host = r.Host
		port = r.URL.Port()
	}
	host = strings.TrimSuffix(host, app.domain)
	host = fmt.Sprintf("%s.onion", host)
	if port != "" && port != "80" && port != "443" {
		host = net.JoinHostPort(host, port)
	}
	req.URL.Host = host
	req.Host = host

	if req.URL.Scheme == "" {
		switch port {
		case "":
			req.URL.Scheme = "http"
		case "80":
			req.URL.Scheme = "http"
		case "443":
			req.URL.Scheme = "https"
		default:
			req.URL.Scheme = "http"
		}
	}

	app.logger.Debugf("sending request %+v", req)

	resp, err := app.httpClient.client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Timeout() {
			app.logError(w, fmt.Errorf("timeout on %s: %w", req.URL.String(), err), false, http.StatusGatewayTimeout)
			return
		}
		app.logError(w, fmt.Errorf("error on calling %s: %w", req.URL.String(), err), false, http.StatusGatewayTimeout)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		app.logError(w, fmt.Errorf("error on reading body: %w", err), false, http.StatusGatewayTimeout)
		return
	}

	app.logger.Debugf("%s: Got a %d body len with a status of %d", req.URL.String(), len(body), resp.StatusCode)

	// replace stuff for domain replacement
	body = bytes.ReplaceAll(body, []byte(`.onion/`), []byte(fmt.Sprintf(`%s/`, app.domain)))
	body = bytes.ReplaceAll(body, []byte(`.onion"`), []byte(fmt.Sprintf(`%s"`, app.domain)))

	for k, v := range resp.Header {
		k = strings.ReplaceAll(k, ".onion", app.domain)
		for _, v2 := range v {
			v2 = strings.ReplaceAll(v2, ".onion", app.domain)
			app.logger.Debugf("%s: Adding header %s: %s", req.URL.String(), k, v2)
			w.Header().Add(k, v2)
		}
	}

	// write will set the content-length. All headers need to be finalized before calling WriteHeader below!
	w.Header().Del("Content-Length")

	w.WriteHeader(resp.StatusCode)

	_, err = w.Write(body)
	if err != nil {
		app.logError(w, fmt.Errorf("error on writing body to ResponseWriter: %w", err), false, http.StatusInternalServerError)
		return
	}
}

func (app *application) xHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		for headerName, headerValue := range r.Header {
			switch strings.ToLower(headerName) {
			case "x-real-ip":
				// this is already handled by the RealIP middleware
				delete(r.Header, headerName)
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
		next.ServeHTTP(rw, r)
	})
}
