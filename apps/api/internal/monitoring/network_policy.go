package monitoring

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/safidy/go-maso/apps/api/internal/validation"
)

type resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type NetworkPolicy struct {
	resolver resolver
}

func NewNetworkPolicy() *NetworkPolicy {
	return &NetworkPolicy{resolver: net.DefaultResolver}
}

func (p *NetworkPolicy) ValidateURL(ctx context.Context, value string) error {
	if err := validation.HTTPURL(value, true); err != nil {
		return err
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return errors.New("URL must be valid")
	}

	_, err = p.ResolveHost(ctx, parsed.Hostname())
	return err
}

func (p *NetworkPolicy) ResolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return nil, errors.New("target host is required")
	}

	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		if prohibited(address) {
			return nil, fmt.Errorf("target address %s is not allowed", address)
		}
		return []netip.Addr{address}, nil
	}

	addresses, err := p.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve target host: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("target host resolved to no addresses")
	}

	result := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if prohibited(address) {
			return nil, fmt.Errorf("target address %s is not allowed", address)
		}
		result = append(result, address)
	}

	return result, nil
}

var prohibitedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func prohibited(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return true
	}

	for _, network := range prohibitedNetworks {
		if network.Contains(address) {
			return true
		}
	}

	return false
}
