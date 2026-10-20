package monitoring

import (
	"context"
	"net/netip"
	"testing"
)

type fakeResolver map[string][]netip.Addr

func (r fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return r[host], nil
}

func TestNetworkPolicyRejectsNonPublicTargets(t *testing.T) {
	t.Parallel()

	policy := &NetworkPolicy{resolver: fakeResolver{
		"loopback.test": {netip.MustParseAddr("127.0.0.1")},
		"private.test":  {netip.MustParseAddr("10.0.0.1")},
		"metadata.test": {netip.MustParseAddr("169.254.169.254")},
		"mixed.test": {
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("192.168.1.10"),
		},
	}}

	for _, target := range []string{
		"http://127.0.0.1/health",
		"http://loopback.test/health",
		"https://private.test/health",
		"http://metadata.test/latest/meta-data",
		"https://mixed.test/health",
	} {
		if err := policy.ValidateURL(context.Background(), target); err == nil {
			t.Fatalf("ValidateURL(%q) succeeded, want rejection", target)
		}
	}
}

func TestNetworkPolicyAcceptsPublicTarget(t *testing.T) {
	t.Parallel()

	policy := &NetworkPolicy{resolver: fakeResolver{
		"example.test": {netip.MustParseAddr("93.184.216.34")},
	}}

	if err := policy.ValidateURL(context.Background(), "https://example.test/health"); err != nil {
		t.Fatalf("ValidateURL() error = %v", err)
	}
}
