package main

import (
	"context"
	"net"
	"time"

	"github.com/patrickmn/go-cache"
)

type dnsClient struct {
	cache    *cache.Cache
	resolver *net.Resolver
	timeout  time.Duration
}

func newDNSClient(timeout, dnsCacheTimeout time.Duration) *dnsClient {
	var r *net.Resolver

	return &dnsClient{
		cache:    cache.New(dnsCacheTimeout, 1*time.Hour),
		resolver: r,
		timeout:  timeout,
	}
}

func (d *dnsClient) ipLookup(ctx context.Context, domain string) ([]string, error) {
	val, found := d.cache.Get(domain)
	if found {
		return val.([]string), nil
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
