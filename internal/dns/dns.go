package dns

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/patrickmn/go-cache"
)

type Client struct {
	cache    *cache.Cache
	resolver *net.Resolver
	timeout  time.Duration
}

func NewDNSClient(timeout, dnsCacheTimeout time.Duration) *Client {
	var r *net.Resolver

	return &Client{
		cache:    cache.New(dnsCacheTimeout, 1*time.Hour),
		resolver: r,
		timeout:  timeout,
	}
}

func (d *Client) IPLookup(ctx context.Context, domain string) ([]string, error) {
	val, found := d.cache.Get(domain)
	if found {
		x, ok := val.([]string)
		if !ok {
			return nil, errors.New("cache value is not a string slice")
		}
		return x, nil
	}

	ctx2, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	addr, err := d.resolver.LookupHost(ctx2, domain)
	if err != nil {
		return nil, err
	}

	d.cache.Set(domain, addr, cache.DefaultExpiration)

	return addr, nil
}
