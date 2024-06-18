package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
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
	"time"

	"github.com/charmbracelet/log"
	"github.com/firefart/zwiebelproxy/templates"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/mattn/go-isatty"

	_ "go.uber.org/automaxprocs"
)

type application struct {
	ipHeader         bool
	allowedHosts     []string
	allowedIPs       []string
	allowedIPRanges  []netip.Prefix
	blacklistedwords map[string]*regexp.Regexp
	transport        *http.Transport
	domain           string
	timeout          time.Duration
	logger           *slog.Logger
	dnsClient        dnsClient
}

func newLogger(debugMode, jsonOutput bool) *slog.Logger {
	w := os.Stdout
	var level = new(slog.LevelVar)
	level.Set(slog.LevelInfo)

	var replaceFunc func(groups []string, a slog.Attr) slog.Attr
	if debugMode {
		level.Set(slog.LevelDebug)
		// add source file information
		wd, err := os.Getwd()
		if err != nil {
			panic("unable to determine working directory")
		}
		replaceFunc = func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				source := a.Value.Any().(*slog.Source)
				// remove current working directory and only leave the relative path to the program
				if file, ok := strings.CutPrefix(source.File, wd); ok {
					source.File = file
				}
			}
			return a
		}
	}

	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level:       level,
			AddSource:   debugMode,
			ReplaceAttr: replaceFunc,
		})
	} else if !isatty.IsTerminal(w.Fd()) {
		// running as a service
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{
			Level:       level,
			AddSource:   debugMode,
			ReplaceAttr: replaceFunc,
		})
	} else {
		// pretty output
		l := log.InfoLevel
		if debugMode {
			l = log.DebugLevel
		}
		handler = log.NewWithOptions(w, log.Options{
			ReportCaller: true,
			Level:        l,
		})
	}
	return slog.New(handler)
}

type cliOptions struct {
	host               *string
	httpPort           *string
	httpsPort          *string
	publicKeyFile      *string
	privateKeyFile     *string
	debug              *bool
	jsonOutput         *bool
	domain             *string
	tor                *string
	wait               *time.Duration
	timeout            *time.Duration
	dnsCacheTimeout    *time.Duration
	ipheader           *bool
	allowedIPs         *string
	allowedIPRangesRaw *string
	allowedHosts       *string
	blacklistedWords   *string
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("could not load .env file: %v. continuing without\n", err)
	}

	var opts cliOptions

	opts.host = flag.String("host", lookupEnvOrString("ZWIEBEL_HOST", ""), "IP to bind to. You can also use the ZWIEBEL_HOST environment variable or an entry in the .env file to set this parameter.")
	opts.httpPort = flag.String("http-port", lookupEnvOrString("ZWIEBEL_HTTP_PORT", "80"), "HTTP port to use")
	opts.httpsPort = flag.String("https-port", lookupEnvOrString("ZWIEBEL_HTTPS_PORT", "443"), "HTTPS port to use")
	opts.publicKeyFile = flag.String("public-key", lookupEnvOrString("ZWIEBEL_PUBLIC_KEY", ""), "TLS public key to use")
	opts.privateKeyFile = flag.String("private-key", lookupEnvOrString("ZWIEBEL_PRIVATE_KEY", ""), "TLS private key to use")
	opts.debug = flag.Bool("debug", lookupEnvOrBool("ZWIEBEL_DEBUG", false), "Enable DEBUG mode. You can also use the ZWIEBEL_DEBUG environment variable or an entry in the .env file to set this parameter.")
	opts.jsonOutput = flag.Bool("json-out", lookupEnvOrBool("ZWIEBEL_JSON_OUTPUT", false), "Log as JSON. You can also use the ZWIEBEL_JSON_OUTPUT environment variable or an entry in the .env file to set this parameter.")
	opts.domain = flag.String("domain", lookupEnvOrString("ZWIEBEL_DOMAIN", ""), "domain to use. You can also use the ZWIEBEL_DOMAIN environment variable or an entry in the .env file to set this parameter.")
	opts.tor = flag.String("tor", lookupEnvOrString("ZWIEBEL_TOR", "socks5://127.0.0.1:9050"), "TOR Proxy server. You can also use the ZWIEBEL_TOR environment variable or an entry in the .env file to set this parameter.")
	opts.wait = flag.Duration("graceful-timeout", lookupEnvOrDuration("ZWIEBEL_GRACEFUL_TIMEOUT", 5*time.Second), "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m. You can also use the ZWIEBEL_GRACEFUL_TIMEOUT environment variable or an entry in the .env file to set this parameter.")
	opts.timeout = flag.Duration("timeout", lookupEnvOrDuration("ZWIEBEL_TIMEOUT", 5*time.Minute), "http timeout. You can also use the ZWIEBEL_TIMEOUT environment variable or an entry in the .env file to set this parameter.")
	opts.dnsCacheTimeout = flag.Duration("dns-timeout", lookupEnvOrDuration("ZWIEBEL_DNS_TIMEOUT", 10*time.Minute), "timeout for the DNS cache. DNS entries are cached for this duration")
	opts.ipheader = flag.Bool("ip-header", lookupEnvOrBool("ZWIEBEL_IP_HEADER", false), "Use Header like X-Forwarded-For or CF-Connecting-IPto get real client ip. Only set it behind a reverse proxy, otherwise the IP Access check can easily be bypassed.")
	opts.allowedIPs = flag.String("allowed-ips", lookupEnvOrString("ZWIEBEL_ALLOWED_IPS", ""), "if set, only the specified IPs are allowed. Split multiple IPs by comma. If empty, all IPs are allowed.")
	opts.allowedIPRangesRaw = flag.String("allowed-ip-ranges", lookupEnvOrString("ZWIEBEL_ALLOWED_IPRANGES", ""), "if set, only the specified IP ranges are allowed. Split multiple IP ranges by comma. If empty, all IPs are allowed. Please supply in CIDR notation (eg. 10.0.0.0/8)")
	opts.allowedHosts = flag.String("allowed-hosts", lookupEnvOrString("ZWIEBEL_ALLOWED_HOSTS", ""), "if set, only the specified hosts are allowed. A reverse lookup for the host is done to compare the request ip with the dns value. This way you can allow DynDNS domains for dynamic IPs. Supply multiple values seperated by comma. If empty, all IPs are allowed.")
	opts.blacklistedWords = flag.String("blacklisted-words", lookupEnvOrString("ZWIEBEL_BLACKLISTED_WORDS", ""), "Comma separated list of blacklisted words. This word is matched with a boundary regex (\bword\b) and if it matches the response body the request is aborted")
	flag.Parse()

	log := newLogger(*opts.debug, *opts.jsonOutput)

	if err := run(log, opts); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

func run(log *slog.Logger, opts cliOptions) error {
	if len(*opts.domain) == 0 {
		return fmt.Errorf("please provide a domain")
	}

	if !strings.HasPrefix(*opts.domain, ".") {
		var a = fmt.Sprintf(".%s", *opts.domain)
		opts.domain = &a
	}

	torProxyURL, err := url.Parse(*opts.tor)
	if err != nil {
		return fmt.Errorf("invalid proxy url %s: %v", *opts.tor, err)
	}

	// used to clone the default transport
	tr := http.DefaultTransport.(*http.Transport)
	tr.Proxy = http.ProxyURL(torProxyURL)
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	tr.TLSHandshakeTimeout = *opts.timeout
	tr.ExpectContinueTimeout = *opts.timeout
	tr.ResponseHeaderTimeout = *opts.timeout

	tr.DialContext = (&net.Dialer{
		Timeout:   *opts.timeout,
		KeepAlive: *opts.timeout,
	}).DialContext

	var allowedIPRanges []netip.Prefix
	allowedIPRangesSplit := DeleteEmptyItems(strings.Split(*opts.allowedIPRangesRaw, ","))
	for _, x := range allowedIPRangesSplit {
		prefix, err := netip.ParsePrefix(x)
		if err != nil {
			return fmt.Errorf("invalid range %s: %w", x, err)
		}
		allowedIPRanges = append(allowedIPRanges, prefix)
	}

	app := &application{
		transport:        tr,
		domain:           *opts.domain,
		timeout:          *opts.timeout,
		logger:           log,
		dnsClient:        *newDNSClient(*opts.timeout, *opts.dnsCacheTimeout),
		ipHeader:         *opts.ipheader,
		allowedIPs:       DeleteEmptyItems(strings.Split(*opts.allowedIPs, ",")),
		allowedHosts:     DeleteEmptyItems(strings.Split(*opts.allowedHosts, ",")),
		allowedIPRanges:  allowedIPRanges,
		blacklistedwords: make(map[string]*regexp.Regexp),
	}

	for _, word := range strings.Split(*opts.blacklistedWords, ",") {
		if word == "" {
			continue
		}
		fullRegex := fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(word))
		re, err := regexp.Compile(fullRegex)
		if err != nil {
			return err
		}
		app.blacklistedwords[word] = re
	}

	httpSrv := &http.Server{
		Addr:    net.JoinHostPort(*opts.host, *opts.httpPort),
		Handler: app.routes(),
	}
	httpsSrv := &http.Server{
		Addr:    net.JoinHostPort(*opts.host, *opts.httpsPort),
		Handler: app.routes(),
	}
	log.Info("starting server", slog.String("http", httpSrv.Addr), slog.String("https", httpsSrv.Addr))

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil {
			// not interested in server closed messages
			if !errors.Is(err, http.ErrServerClosed) {
				app.logger.Error("httpSrv Error", slog.String("error", err.Error()))
			}
		}
	}()

	// only start https server if we provide certificates
	if *opts.publicKeyFile != "" && *opts.privateKeyFile != "" {
		go func() {
			if err := httpsSrv.ListenAndServeTLS(*opts.publicKeyFile, *opts.privateKeyFile); err != nil {
				// not interested in server closed messages
				if !errors.Is(err, http.ErrServerClosed) {
					app.logger.Error("httpsSrv Error", slog.String("error", err.Error()))
				}
			}
		}()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	<-c
	ctx, cancel := context.WithTimeout(context.Background(), *opts.wait)
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
	if app.ipHeader {
		r.Use(middleware.RealIP)
		r.Use(app.xHeaderMiddleware)
	}
	r.Use(middleware.Logger)
	r.Use(app.ipAuthMiddleware)
	r.Use(middleware.Recoverer)

	ph := http.HandlerFunc(app.proxyHandler)
	r.Handle("/*", ph)
	return r
}

func (app *application) logError(ctx context.Context, w http.ResponseWriter, err error, statusCode int) {
	w.WriteHeader(statusCode)
	w.Header().Set("Connection", "close")
	errorText := fmt.Sprintf("%v", err)
	app.logger.Error(errorText)

	if err2 := templates.Index(errorText).Render(ctx, w); err2 != nil {
		app.logger.Error(err2.Error())
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
		if err := templates.Index("").Render(r.Context(), w); err != nil {
			panic(fmt.Sprintf("error on executing template: %v", err))
		}
		return
	}

	if !strings.HasSuffix(host, app.domain) {
		app.logError(r.Context(), w, fmt.Errorf("invalid domain %s called. The domain needs to end in %s", host, app.domain), http.StatusBadRequest)
		return
	}

	proxy := httputil.ReverseProxy{
		Rewrite:        app.rewrite,
		FlushInterval:  -1,
		ModifyResponse: app.modifyResponse,
		Transport:      app.transport,
		ErrorHandler:   app.proxyErrorHandler,
	}

	app.logger.Debug("original request", slog.String("request", fmt.Sprintf("%+v", r)))

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

func (app *application) ipAuthMiddleware(next http.Handler) http.Handler {
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
			app.logError(r.Context(), rw, fmt.Errorf("could not determine remote ip"), http.StatusBadRequest)
			return
		}

		ipParsed, err := netip.ParseAddr(remoteIP)
		if err != nil {
			app.logError(r.Context(), rw, fmt.Errorf("could not parse remote ip: %w", err), http.StatusBadRequest)
			return
		}

		for _, ip := range app.allowedIPs {
			if ip == remoteIP {
				app.logger.Info("allowing whitelisted ip", slog.String("ip", ip))
				next.ServeHTTP(rw, r)
				return
			}
		}

		for _, prefix := range app.allowedIPRanges {
			if prefix.Contains(ipParsed) {
				app.logger.Info("allowing whitelisted ip range", slog.String("ip", remoteIP), slog.String("matched-prefix", prefix.String()))
				next.ServeHTTP(rw, r)
				return
			}
		}

		for _, d := range app.allowedHosts {
			dynamicIP, err := app.dnsClient.ipLookup(r.Context(), d)
			if err != nil {
				app.logError(r.Context(), rw, fmt.Errorf("invalid domain %q in config: %w", d, err), http.StatusInternalServerError)
				return
			}

			app.logger.Debug("dns resolved", slog.String("host", d), slog.String("ips", strings.Join(dynamicIP, ", ")))
			for _, i := range dynamicIP {
				if i == remoteIP {
					app.logger.Info("allowing client", slog.String("ip", remoteIP), slog.String("hostname", d))
					next.ServeHTTP(rw, r)
					return
				}
			}
		}

		app.logError(r.Context(), rw, fmt.Errorf("access denied for %s", remoteIP), http.StatusForbidden)
	})
}
