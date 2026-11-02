package tests

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cretz/bine/tor"
	"golang.org/x/net/proxy"
)

// TestIsolateSocksAuth checks that different SOCKS credentials share a circuit
// only when SOCKS authentication isolation is disabled.
// Conflux is disabled because it can put streams on different circuits even
// without isolation. That would make the shared-circuit check fail and let the
// isolation check pass even if the credentials never reached Tor.
func TestIsolateSocksAuth(t *testing.T) {
	if !torIncludeNetworkTests {
		t.Skip("Only runs if -tor and -tor.network are set")
	}
	t.Run("NoIsolateSOCKSAuth", func(t *testing.T) {
		ctx := NewTestContext(t, &tor.StartConf{
			NoAutoSocksPort: true,
			ExtraArgs:       []string{"--SocksPort", "auto NoIsolateSOCKSAuth", "--ConfluxEnabled", "0"},
		})
		t.Cleanup(ctx.Close)
		circuitIDs := separateAuthStreamCircuitIDs(ctx)
		if circuitIDs[1] != circuitIDs[0] {
			ctx.Fatalf("different SOCKS credentials must share a circuit with NoIsolateSOCKSAuth: got %q, want %q", circuitIDs[1], circuitIDs[0])
		}
	})
	t.Run("IsolateSOCKSAuth", func(t *testing.T) {
		ctx := NewTestContext(t, &tor.StartConf{ExtraArgs: []string{"--ConfluxEnabled", "0"}})
		t.Cleanup(ctx.Close)
		circuitIDs := separateAuthStreamCircuitIDs(ctx)
		if circuitIDs[1] == circuitIDs[0] {
			ctx.Fatalf("different SOCKS credentials must use different circuits with IsolateSOCKSAuth: got %q, want a value different from %q", circuitIDs[1], circuitIDs[0])
		}
	})
}

// separateAuthStreamCircuitIDs dials check.torproject.org:443 twice through Tor
// with different SOCKS credentials and returns the two streams' circuit IDs.
// It fails the test unless stream-status contains exactly two SUCCEEDED streams
// to port 443, not counting Tor's own .exit directory streams.
// Both connections stay open until the subtest ends because stream-status omits
// closed streams.
func separateAuthStreamCircuitIDs(ctx *TestContext) []string {
	ctx.Helper()
	enableCtx, enableCancel := context.WithTimeout(ctx, 100*time.Second)
	defer enableCancel()
	if err := ctx.EnableNetwork(enableCtx, true); err != nil {
		ctx.Fatalf("EnableNetwork: got error %v, want nil", err)
	}

	// The first connection must still be open when stream-status is read.
	// check.torproject.org closes connections that send no data, so the deadline
	// covers both dials and should stay short.
	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()
	firstDialer, err := ctx.Dialer(dialCtx, &tor.DialConf{ProxyAuth: &proxy.Auth{User: "foo", Password: "bar"}})
	if err != nil {
		ctx.Fatalf("first Dialer: got error %v, want nil", err)
	}
	secondDialer, err := ctx.Dialer(dialCtx, &tor.DialConf{ProxyAuth: &proxy.Auth{User: "baz", Password: "qux"}})
	if err != nil {
		ctx.Fatalf("second Dialer: got error %v, want nil", err)
	}
	for _, dialer := range []*tor.Dialer{firstDialer, secondDialer} {
		conn, err := dialer.DialContext(dialCtx, "tcp", "check.torproject.org:443")
		if err != nil {
			ctx.Fatalf("DialContext: got error %v, want nil", err)
		}
		ctx.Cleanup(func() { conn.Close() })
	}

	vals, err := ctx.Control.GetInfo("stream-status")
	if err != nil {
		ctx.Fatalf("GetInfo(stream-status): got error %v, want nil", err)
	}
	if len(vals) != 1 {
		ctx.Fatalf("GetInfo(stream-status): got length %d, want 1; vals: %s", len(vals), formatKeyVals(vals))
	}
	if vals[0].Key != "stream-status" {
		ctx.Fatalf("GetInfo key: got %q, want %q", vals[0].Key, "stream-status")
	}
	raw := vals[0].Val
	ctx.Logf("stream-status:\n%s", raw)
	var circuitIDs []string
	// Tor reports a SOCKS stream's target as the resolved IP once it succeeds,
	// so the filter matches SUCCEEDED streams on port 443 instead of the hostname.
	// Tor's directory streams can also use port 443, so hosts ending in .exit are
	// excluded.
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[1] != "SUCCEEDED" {
			continue
		}
		host, port, err := net.SplitHostPort(fields[3])
		if err != nil || port != "443" || strings.HasSuffix(host, ".exit") {
			continue
		}
		circuitIDs = append(circuitIDs, fields[2])
	}
	if len(circuitIDs) != 2 {
		ctx.Fatalf("expected exactly two SOCKS streams; stream-status: %q; got length %d, want 2", raw, len(circuitIDs))
	}
	return circuitIDs
}
