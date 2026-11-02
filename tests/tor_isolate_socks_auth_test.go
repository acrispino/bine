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
		ctx.Require.Equal(circuitIDs[0], circuitIDs[1], "different SOCKS credentials must share a circuit with NoIsolateSOCKSAuth")
	})
	t.Run("IsolateSOCKSAuth", func(t *testing.T) {
		ctx := NewTestContext(t, &tor.StartConf{ExtraArgs: []string{"--ConfluxEnabled", "0"}})
		t.Cleanup(ctx.Close)
		circuitIDs := separateAuthStreamCircuitIDs(ctx)
		ctx.Require.NotEqual(circuitIDs[0], circuitIDs[1], "different SOCKS credentials must use different circuits with IsolateSOCKSAuth")
	})
}

// separateAuthStreamCircuitIDs dials check.torproject.org:443 twice through Tor
// with different SOCKS credentials and returns the two streams' circuit IDs.
// It fails the test unless stream-status contains exactly two SUCCEEDED streams
// to port 443, not counting Tor's own .exit directory streams.
// Both connections stay open until the subtest ends because stream-status omits
// closed streams.
func separateAuthStreamCircuitIDs(ctx *TestContext) []string {
	enableCtx, enableCancel := context.WithTimeout(ctx, 100*time.Second)
	defer enableCancel()
	ctx.Require.NoError(ctx.EnableNetwork(enableCtx, true))

	// The first connection must still be open when stream-status is read.
	// check.torproject.org closes connections that send no data, so the deadline
	// covers both dials and should stay short.
	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()
	firstDialer, err := ctx.Dialer(dialCtx, &tor.DialConf{ProxyAuth: &proxy.Auth{User: "foo", Password: "bar"}})
	ctx.Require.NoError(err)
	secondDialer, err := ctx.Dialer(dialCtx, &tor.DialConf{ProxyAuth: &proxy.Auth{User: "baz", Password: "qux"}})
	ctx.Require.NoError(err)
	for _, dialer := range []*tor.Dialer{firstDialer, secondDialer} {
		conn, err := dialer.DialContext(dialCtx, "tcp", "check.torproject.org:443")
		ctx.Require.NoError(err)
		ctx.Cleanup(func() { conn.Close() })
	}

	vals, err := ctx.Control.GetInfo("stream-status")
	ctx.Require.NoError(err)
	ctx.Require.Len(vals, 1)
	ctx.Require.Equal("stream-status", vals[0].Key)
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
	ctx.Require.Len(circuitIDs, 2, "expected exactly two SOCKS streams; stream-status: %q", raw)
	return circuitIDs
}
