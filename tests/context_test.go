package tests

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cretz/bine/control"
	"github.com/cretz/bine/tor"
)

var torEnabled bool
var torExePath string
var torVerbose bool
var torIncludeNetworkTests bool
var globalEnabledNetworkContext *TestContext

func TestMain(m *testing.M) {
	flag.BoolVar(&torEnabled, "tor", false, "Whether any of the integration tests are enabled")
	flag.StringVar(&torExePath, "tor.path", "tor", "The Tor exe path")
	flag.BoolVar(&torVerbose, "tor.verbose", false, "Show verbose test info")
	flag.BoolVar(&torIncludeNetworkTests, "tor.network", false, "Include network tests")
	flag.Parse()
	exitCode := m.Run()
	if globalEnabledNetworkContext != nil {
		globalEnabledNetworkContext.CloseTorOnClose = true
		globalEnabledNetworkContext.Close()
	}
	os.Exit(exitCode)
}

func GlobalEnabledNetworkContext(t *testing.T) *TestContext {
	t.Helper()
	if !torEnabled || !torIncludeNetworkTests {
		t.Skip("Only runs if -tor and -tor.network are set")
	}
	if globalEnabledNetworkContext == nil {
		ctx := NewTestContext(t, nil)
		ctx.CloseTorOnClose = false
		// 45 second wait for enable network
		enableCtx, enableCancel := context.WithTimeout(ctx, 45*time.Second)
		defer enableCancel()
		if err := ctx.EnableNetwork(enableCtx, true); err != nil {
			ctx.Fatalf("EnableNetwork: got error %v, want nil", err)
		}
		globalEnabledNetworkContext = ctx
	} else {
		globalEnabledNetworkContext.T = t
	}
	return globalEnabledNetworkContext
}

type TestContext struct {
	context.Context
	*testing.T
	*tor.Tor
	CloseTorOnClose bool
}

func NewTestContext(t *testing.T, conf *tor.StartConf) *TestContext {
	t.Helper()
	if !torEnabled {
		t.Skip("Only runs if -tor is set")
	}
	// Build start conf
	if conf == nil {
		conf = &tor.StartConf{}
	}
	conf.ExePath = torExePath
	if torVerbose {
		conf.DebugWriter = os.Stdout
		conf.NoHush = true
	} else {
		conf.ExtraArgs = append(conf.ExtraArgs, "--quiet")
	}
	ret := &TestContext{Context: context.Background(), T: t, CloseTorOnClose: true}
	// Start tor
	var err error
	if ret.Tor, err = tor.Start(ret.Context, conf); err != nil {
		defer ret.Close()
		t.Fatal(err)
	}
	return ret
}

// Deadline to disambiguate context and test deadline and use test one.
func (t *TestContext) Deadline() (time.Time, bool) { return t.T.Deadline() }

func (t *TestContext) Close() {
	if t.CloseTorOnClose {
		if err := t.Tor.Close(); err != nil {
			if t.Failed() {
				t.Logf("Failure on close: %v", err)
			} else {
				t.Errorf("Failure on close: %v", err)
			}
		}
	}
}

func formatKeyVals(entries []*control.KeyVal) string {
	formatted := make([]string, len(entries))
	for i, entry := range entries {
		if entry == nil {
			formatted[i] = "nil"
		} else {
			formatted[i] = fmt.Sprintf("{Key:%q Val:%q ValSetAndEmpty:%t}", entry.Key, entry.Val, entry.ValSetAndEmpty)
		}
	}
	return "[" + strings.Join(formatted, " ") + "]"
}
