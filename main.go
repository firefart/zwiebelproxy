package main

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

type application struct {
	xForwardedFor    bool
	allowedHosts     []string
	allowedIPs       []string
	allowedIPRanges  []netip.Prefix
	blacklistedwords map[string]*regexp.Regexp
	transport        *http.Transport
	domain           string
	timeout          time.Duration
	logger           Logger
	templates        *template.Template
	dnsClient        dnsClient
}

var (
	//go:embed templates
	templateFS embed.FS
)

func main() {
	log := logrus.New()
	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.InfoLevel)

	err := godotenv.Load()
	if err != nil {
		log.Warnf("could not load .env file: %v. continuing without", err)
	}

	if err := run(log); err != nil {
		log.Error(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func run(log *logrus.Logger) error {
	host := flag.String("host", lookupEnvOrString(log, "ZWIEBEL_HOST", ""), "IP to bind to. You can also use the ZWIEBEL_HOST environment variable or an entry in the .env file to set this parameter.")
	httpPort := flag.String("http-port", lookupEnvOrString(log, "ZWIEBEL_HTTP_PORT", "80"), "HTTP port to use")
	httpsPort := flag.String("https-port", lookupEnvOrString(log, "ZWIEBEL_HTTPS_PORT", "443"), "HTTPS port to use")
	debug := flag.Bool("debug", lookupEnvOrBool(log, "ZWIEBEL_DEBUG", false), "Enable DEBUG mode. You can also use the ZWIEBEL_DEBUG environment variable or an entry in the .env file to set this parameter.")
	domain := flag.String("domain", lookupEnvOrString(log, "ZWIEBEL_DOMAIN", ""), "domain to use. You can also use the ZWIEBEL_DOMAIN environment variable or an entry in the .env file to set this parameter.")
	tor := flag.String("tor", lookupEnvOrString(log, "ZWIEBEL_TOR", "socks5://127.0.0.1:9050"), "TOR Proxy server. You can also use the ZWIEBEL_TOR environment variable or an entry in the .env file to set this parameter.")
	wait := flag.Duration("graceful-timeout", lookupEnvOrDuration(log, "ZWIEBEL_GRACEFUL_TIMEOUT", 5*time.Second), "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m. You can also use the ZWIEBEL_GRACEFUL_TIMEOUT environment variable or an entry in the .env file to set this parameter.")
	timeout := flag.Duration("timeout", lookupEnvOrDuration(log, "ZWIEBEL_TIMEOUT", 5*time.Minute), "http timeout. You can also use the ZWIEBEL_TIMEOUT environment variable or an entry in the .env file to set this parameter.")
	dnsCacheTimeout := flag.Duration("dns-timeout", lookupEnvOrDuration(log, "ZWIEBEL_DNS_TIMEOUT", 10*time.Minute), "timeout for the DNS cache. DNS entries are cached for this duration")
	xForwardedFor := flag.Bool("x-forwarded-for", lookupEnvOrBool(log, "ZWIEBEL_X_FORWARDED_FOR", false), "Use X-Forwarded-For Header to get real client ip. Only set it behind a reverse proxy, otherwise the IP Access check can easily be bypassed.")
	allowedIPs := flag.String("allowed-ips", lookupEnvOrString(log, "ZWIEBEL_ALLOWED_IPS", ""), "if set, only the specified IPs are allowed. Split multiple IPs by comma. If empty, all IPs are allowed.")
	allowedIPRangesRaw := flag.String("allowed-ip-ranges", lookupEnvOrString(log, "ZWIEBEL_ALLOWED_IPRANGES", ""), "if set, only the specified IP ranges are allowed. Split multiple IP ranges by comma. If empty, all IPs are allowed. Please supply in CIDR notation (eg. 10.0.0.0/8)")
	allowedHosts := flag.String("allowed-hosts", lookupEnvOrString(log, "ZWIEBEL_ALLOWED_HOSTS", ""), "if set, only the specified hosts are allowed. A reverse lookup for the host is done to compare the request ip with the dns value. This way you can allow DynDNS domains for dynamic IPs. Supply multiple values seperated by comma. If empty, all IPs are allowed.")
	publicKeyFile := flag.String("public-key", lookupEnvOrString(log, "ZWIEBEL_PUBLIC_KEY", ""), "TLS public key to use")
	privateKeyFile := flag.String("private-key", lookupEnvOrString(log, "ZWIEBEL_PRIVATE_KEY", ""), "TLS private key to use")
	blacklistedWords := flag.String("blacklisted-words", lookupEnvOrString(log, "ZWIEBEL_BLACKLISTED_WORDS", ""), "Comma separated list of blacklisted words. This word is matched with a boundary regex (\bword\b) and if it matches the response body the request is aborted")
	flag.Parse()

	if *debug {
		log.SetLevel(logrus.DebugLevel)
		log.Debug("DEBUG mode enabled")
	}

	if len(*domain) == 0 {
		return fmt.Errorf("please provide a domain")
	}

	if !strings.HasPrefix(*domain, ".") {
		var a = fmt.Sprintf(".%s", *domain)
		domain = &a
	}

	torProxyURL, err := url.Parse(*tor)
	if err != nil {
		return fmt.Errorf("invalid proxy url %s: %v", *tor, err)
	}

	// used to clone the default transport
	tr := http.DefaultTransport.(*http.Transport)
	tr.Proxy = http.ProxyURL(torProxyURL)
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	tr.TLSHandshakeTimeout = *timeout
	tr.ExpectContinueTimeout = *timeout
	tr.ResponseHeaderTimeout = *timeout

	tr.DialContext = (&net.Dialer{
		Timeout:   *timeout,
		KeepAlive: *timeout,
	}).DialContext

	var allowedIPRanges []netip.Prefix
	allowedIPRangesSplit := DeleteEmptyItems(strings.Split(*allowedIPRangesRaw, ","))
	for _, x := range allowedIPRangesSplit {
		prefix, err := netip.ParsePrefix(x)
		if err != nil {
			return fmt.Errorf("invalid range %s: %w", x, err)
		}
		allowedIPRanges = append(allowedIPRanges, prefix)
	}

	app := &application{
		transport:        tr,
		domain:           *domain,
		timeout:          *timeout,
		logger:           log,
		templates:        template.Must(template.ParseFS(templateFS, "templates/*.tmpl")),
		dnsClient:        *newDNSClient(*timeout, *dnsCacheTimeout),
		xForwardedFor:    *xForwardedFor,
		allowedIPs:       DeleteEmptyItems(strings.Split(*allowedIPs, ",")),
		allowedHosts:     DeleteEmptyItems(strings.Split(*allowedHosts, ",")),
		allowedIPRanges:  allowedIPRanges,
		blacklistedwords: make(map[string]*regexp.Regexp),
	}

	for _, word := range strings.Split(*blacklistedWords, ",") {
		fullRegex := fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(word))
		re, err := regexp.Compile(fullRegex)
		if err != nil {
			return err
		}
		app.blacklistedwords[word] = re
	}

	httpSrv := &http.Server{
		Addr:    net.JoinHostPort(*host, *httpPort),
		Handler: app.routes(),
	}
	httpsSrv := &http.Server{
		Addr:    net.JoinHostPort(*host, *httpsPort),
		Handler: app.routes(),
	}
	log.Infof("Starting server on %s and %s", httpSrv.Addr, httpsSrv.Addr)

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil {
			// not interested in server closed messages
			if !errors.Is(err, http.ErrServerClosed) {
				app.logger.Errorf("httpSrv Error: %v", err)
				app.logger.Debugf("%#v", err)
			}
		}
	}()

	go func() {
		if err := httpsSrv.ListenAndServeTLS(*publicKeyFile, *privateKeyFile); err != nil {
			// not interested in server closed messages
			if !errors.Is(err, http.ErrServerClosed) {
				app.logger.Errorf("httpsSrv Error: %v", err)
				app.logger.Debugf("%#v", err)
			}
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	<-c
	ctx, cancel := context.WithTimeout(context.Background(), *wait)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		return err
	}
	if err := httpsSrv.Shutdown(ctx); err != nil {
		return err
	}
	log.Info("shutting down")
	return nil
}

func (app *application) routes() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	if app.xForwardedFor {
		r.Use(middleware.RealIP)
		r.Use(app.xHeaderMiddleware)
	}
	r.Use(middleware.Logger)
	r.Use(app.ipAuthModdleware)
	r.Use(middleware.Recoverer)

	ph := http.HandlerFunc(app.proxyHandler)
	r.Handle("/*", ph)
	return r
}

func (app *application) logError(w http.ResponseWriter, err error, statusCode int) {
	w.WriteHeader(statusCode)
	w.Header().Set("Connection", "close")
	errorText := fmt.Sprintf("%v", err)
	app.logger.Error(errorText)

	data := struct {
		Error string
	}{
		Error: errorText,
	}
	if err2 := app.templates.ExecuteTemplate(w, "default.tmpl", data); err2 != nil {
		app.logger.Error(err2)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (app *application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		// no port present
		host = r.Host
	}

	// show info page when top domain is called
	if host == strings.TrimLeft(app.domain, ".") {
		if err := app.templates.ExecuteTemplate(w, "default.tmpl", nil); err != nil {
			panic(fmt.Sprintf("error on executing template: %v", err))
		}
		return
	}

	if !strings.HasSuffix(host, app.domain) {
		app.logError(w, fmt.Errorf("invalid domain %s called. The domain needs to end in %s", host, app.domain), http.StatusBadRequest)
		return
	}

	proxy := httputil.ReverseProxy{
		Rewrite:        app.rewrite,
		FlushInterval:  -1,
		ModifyResponse: app.modifyResponse,
		Transport:      app.transport,
		ErrorHandler:   app.proxyErrorHandler,
	}

	app.logger.Debugf("original request: %+v", r)

	// set a custom timeout
	ctx, cancel := context.WithTimeout(r.Context(), app.timeout)
	defer cancel()
	r = r.WithContext(ctx)
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

func (app *application) ipAuthModdleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if len(app.allowedHosts) == 0 && len(app.allowedIPs) == 0 && len(app.allowedIPRanges) == 0 {
			// configured as a public server, no ip checks
			next.ServeHTTP(rw, r)
			return
		}

		remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteIP = r.RemoteAddr
		}
		remoteIP = strings.TrimSpace(remoteIP)

		if remoteIP == "" {
			app.logError(rw, fmt.Errorf("could not determine remote ip"), http.StatusBadRequest)
			return
		}

		ipParsed, err := netip.ParseAddr(remoteIP)
		if err != nil {
			app.logError(rw, fmt.Errorf("could not parse remote ip: %w", err), http.StatusBadRequest)
			return
		}

		for _, ip := range app.allowedIPs {
			if ip == remoteIP {
				app.logger.Infof("allowing whitelisted ip %s", ip)
				next.ServeHTTP(rw, r)
				return
			}
		}

		for _, prefix := range app.allowedIPRanges {
			if prefix.Contains(ipParsed) {
				app.logger.Infof("allowing whitelisted ip %s because it is in whitelisted prefix %s", remoteIP, prefix.String())
				next.ServeHTTP(rw, r)
				return
			}
		}

		for _, d := range app.allowedHosts {
			dynamicIP, err := app.dnsClient.ipLookup(r.Context(), d)
			if err != nil {
				app.logError(rw, fmt.Errorf("invalid domain %q in config: %w", d, err), http.StatusInternalServerError)
				return
			}
			app.logger.Debugf("resolved %s to %s", d, strings.Join(dynamicIP, ", "))
			for _, i := range dynamicIP {
				if i == remoteIP {
					app.logger.Infof("allowing client %s with hostnames %s", remoteIP, d)
					next.ServeHTTP(rw, r)
					return
				}
			}
		}

		app.logError(rw, fmt.Errorf("access denied for %s", remoteIP), http.StatusForbidden)
	})
}
