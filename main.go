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
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/firefart/zwiebelproxy/internal/helper"
	"github.com/firefart/zwiebelproxy/internal/server"
	"github.com/joho/godotenv"
	"github.com/mattn/go-isatty"

	"go.uber.org/automaxprocs/maxprocs"
)

func init() {
	// added in init to prevent the forced logline
	if _, err := maxprocs.Set(); err != nil {
		panic(fmt.Sprintf("Error on gomaxprocs: %v\n", err))
	}
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
	host                 *string
	httpPort             *string
	httpsPort            *string
	publicKeyFile        *string
	privateKeyFile       *string
	debug                *bool
	jsonOutput           *bool
	domain               *string
	tor                  *string
	wait                 *time.Duration
	timeout              *time.Duration
	dnsCacheTimeout      *time.Duration
	cloudflare           *bool
	revProxy             *bool
	allowedIPs           *string
	allowedIPRangesRaw   *string
	allowedHosts         *string
	blacklistedWords     *string
	secretKeyHeaderName  *string
	secretKeyHeaderValue *string
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("could not load .env file: %v. continuing without\n", err)
	}

	var opts cliOptions

	opts.host = flag.String("host", helper.LookupEnvOrString("ZWIEBEL_HOST", ""), "IP to bind to. You can also use the ZWIEBEL_HOST environment variable or an entry in the .env file to set this parameter.")
	opts.httpPort = flag.String("http-port", helper.LookupEnvOrString("ZWIEBEL_HTTP_PORT", "80"), "HTTP port to use")
	opts.httpsPort = flag.String("https-port", helper.LookupEnvOrString("ZWIEBEL_HTTPS_PORT", "443"), "HTTPS port to use")
	opts.publicKeyFile = flag.String("public-key", helper.LookupEnvOrString("ZWIEBEL_PUBLIC_KEY", ""), "TLS public key to use")
	opts.privateKeyFile = flag.String("private-key", helper.LookupEnvOrString("ZWIEBEL_PRIVATE_KEY", ""), "TLS private key to use")
	opts.debug = flag.Bool("debug", helper.LookupEnvOrBool("ZWIEBEL_DEBUG", false), "Enable DEBUG mode. You can also use the ZWIEBEL_DEBUG environment variable or an entry in the .env file to set this parameter.")
	opts.jsonOutput = flag.Bool("json-out", helper.LookupEnvOrBool("ZWIEBEL_JSON_OUTPUT", false), "Log as JSON. You can also use the ZWIEBEL_JSON_OUTPUT environment variable or an entry in the .env file to set this parameter.")
	opts.domain = flag.String("domain", helper.LookupEnvOrString("ZWIEBEL_DOMAIN", ""), "domain to use. You can also use the ZWIEBEL_DOMAIN environment variable or an entry in the .env file to set this parameter.")
	opts.tor = flag.String("tor", helper.LookupEnvOrString("ZWIEBEL_TOR", "socks5://127.0.0.1:9050"), "TOR Proxy server. You can also use the ZWIEBEL_TOR environment variable or an entry in the .env file to set this parameter.")
	opts.wait = flag.Duration("graceful-timeout", helper.LookupEnvOrDuration("ZWIEBEL_GRACEFUL_TIMEOUT", 5*time.Second), "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m. You can also use the ZWIEBEL_GRACEFUL_TIMEOUT environment variable or an entry in the .env file to set this parameter.")
	opts.timeout = flag.Duration("timeout", helper.LookupEnvOrDuration("ZWIEBEL_TIMEOUT", 5*time.Minute), "http timeout. You can also use the ZWIEBEL_TIMEOUT environment variable or an entry in the .env file to set this parameter.")
	opts.dnsCacheTimeout = flag.Duration("dns-timeout", helper.LookupEnvOrDuration("ZWIEBEL_DNS_TIMEOUT", 10*time.Minute), "timeout for the DNS cache. DNS entries are cached for this duration")
	opts.cloudflare = flag.Bool("cloudflare", helper.LookupEnvOrBool("ZWIEBEL_CLOUDFLARE", false), "Set this if you are running behind cloudflare. This way the cloudflare ip headers are used")
	opts.revProxy = flag.Bool("revproxy", helper.LookupEnvOrBool("ZWIEBEL_REV_PROXY", false), "Set this to extract the ip from various X headers. Only set if running behind a reverse proxy!")
	opts.allowedIPs = flag.String("allowed-ips", helper.LookupEnvOrString("ZWIEBEL_ALLOWED_IPS", ""), "if set, only the specified IPs are allowed. Split multiple IPs by comma. If empty, all IPs are allowed.")
	opts.allowedIPRangesRaw = flag.String("allowed-ip-ranges", helper.LookupEnvOrString("ZWIEBEL_ALLOWED_IPRANGES", ""), "if set, only the specified IP ranges are allowed. Split multiple IP ranges by comma. If empty, all IPs are allowed. Please supply in CIDR notation (eg. 10.0.0.0/8)")
	opts.allowedHosts = flag.String("allowed-hosts", helper.LookupEnvOrString("ZWIEBEL_ALLOWED_HOSTS", ""), "if set, only the specified hosts are allowed. A reverse lookup for the host is done to compare the request ip with the dns value. This way you can allow DynDNS domains for dynamic IPs. Supply multiple values seperated by comma. If empty, all IPs are allowed.")
	opts.blacklistedWords = flag.String("blacklisted-words", helper.LookupEnvOrString("ZWIEBEL_BLACKLISTED_WORDS", ""), "Comma separated list of blacklisted words. This word is matched with a boundary regex (\bword\b) and if it matches the response body the request is aborted")
	opts.secretKeyHeaderName = flag.String("secret-key-header-name", helper.LookupEnvOrString("ZWIEBEL_SECRET_KEY_HEADER_NAME", "X-Secret-Key-Header"), "Header name to test error handler")
	opts.secretKeyHeaderValue = flag.String("secret-key-header-value", helper.LookupEnvOrString("ZWIEBEL_SECRET_KEY_HEADER_VALUE", ""), "Header value to test error handler")
	flag.Parse()

	log := newLogger(*opts.debug, *opts.jsonOutput)

	ctx := context.Background()
	if err := run(ctx, log, opts); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

func run(ctx context.Context, log *slog.Logger, opts cliOptions) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

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
	allowedIPRangesSplit := helper.DeleteEmptyItems(strings.Split(*opts.allowedIPRangesRaw, ","))
	for _, x := range allowedIPRangesSplit {
		prefix, err := netip.ParsePrefix(x)
		if err != nil {
			return fmt.Errorf("invalid range %s: %w", x, err)
		}
		allowedIPRanges = append(allowedIPRanges, prefix)
	}
	allowedIPs := helper.DeleteEmptyItems(strings.Split(*opts.allowedIPs, ","))
	allowedHosts := helper.DeleteEmptyItems(strings.Split(*opts.allowedHosts, ","))

	s := server.NewServer(ctx, log, *opts.cloudflare, *opts.revProxy, *opts.debug, *opts.domain, *opts.blacklistedWords, *opts.secretKeyHeaderName, *opts.secretKeyHeaderValue, *opts.timeout, *opts.dnsCacheTimeout, allowedHosts, allowedIPs, allowedIPRanges, tr)

	httpSrv := &http.Server{
		Addr:    net.JoinHostPort(*opts.host, *opts.httpPort),
		Handler: s,
	}
	httpsSrv := &http.Server{
		Addr:    net.JoinHostPort(*opts.host, *opts.httpsPort),
		Handler: s,
	}
	log.Info("starting server", slog.String("http", httpSrv.Addr), slog.String("https", httpsSrv.Addr))

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil {
			// not interested in server closed messages
			if !errors.Is(err, http.ErrServerClosed) {
				log.Error("httpSrv Error", slog.String("error", err.Error()))
			}
		}
	}()

	// only start https server if we provide certificates
	if *opts.publicKeyFile != "" && *opts.privateKeyFile != "" {
		go func() {
			if err := httpsSrv.ListenAndServeTLS(*opts.publicKeyFile, *opts.privateKeyFile); err != nil {
				// not interested in server closed messages
				if !errors.Is(err, http.ErrServerClosed) {
					log.Error("httpsSrv Error", slog.String("error", err.Error()))
				}
			}
		}()
	}

	// listen for interrupt signal
	<-ctx.Done()

	ctx, cancel2 := context.WithTimeout(context.Background(), *opts.wait)
	defer cancel2()
	if err := httpSrv.Shutdown(ctx); err != http.ErrServerClosed {
		return err
	}
	if err := httpsSrv.Shutdown(ctx); err != http.ErrServerClosed {
		return err
	}
	log.Info("shutting down")
	return nil
}
