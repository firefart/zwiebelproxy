package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
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

func (app *application) logError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Connection", "close")
	errorText := fmt.Sprintf("%v", err)
	app.logger.Error(errorText)
	http.Error(w, http.StatusText(status), status)
}

func (app *application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		// no port present
		host = r.Host
		port = r.URL.Port()
	}

	if !strings.HasSuffix(host, app.domain) {
		app.logError(w, fmt.Errorf("invalid domain %s", r.Host), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), app.timeout)
	defer cancel()
	r = r.WithContext(ctx)

	host = strings.TrimSuffix(host, app.domain)
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

	// needed so the ip will not be leaked
	r.Header["X-Forwarded-For"] = nil

	proxy := httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = scheme
			r.URL.Host = host
			r.Host = host

			app.logger.Debugf("port: %+v", port)
			app.logger.Debugf("r.URL: %+v", r.URL)
			app.logger.Debugf("r.RequestURI: %+v", r.RequestURI)
			app.logger.Debugf("r.Host: %+v", r.Host)
			app.logger.Debugf("r.Header: %+v", r.Header)
		},
	}

	proxy.FlushInterval = -1
	proxy.ModifyResponse = app.modifyResponse
	proxy.Transport = app.httpClient.tr.Clone()

	app.logger.Debugf("sending request %+v", r)

	proxy.ServeHTTP(w, r)
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
