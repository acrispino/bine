package tests

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/cretz/bine/tor"
	"golang.org/x/net/context/ctxhttp"
)

func TestDialerSimpleHTTP(t *testing.T) {
	ctx := GlobalEnabledNetworkContext(t)
	httpClient := httpClient(ctx, nil)
	// IsTor check
	byts := httpGet(ctx, httpClient, "https://check.torproject.org/api/ip")
	jsn := map[string]interface{}{}
	if err := json.Unmarshal(byts, &jsn); err != nil {
		ctx.Fatalf("Unmarshal: got error %v, want nil", err)
	}
	if !jsn["IsTor"].(bool) {
		ctx.Fatalf("IsTor: got %v, want true", jsn["IsTor"])
	}
}

func httpClient(ctx *TestContext, conf *tor.DialConf) *http.Client {
	ctx.Helper()
	// 15 seconds max to dial
	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dialCancel()
	// Make connection
	dialer, err := ctx.Dialer(dialCtx, conf)
	if err != nil {
		ctx.Fatalf("Dialer: got error %v, want nil", err)
	}
	return &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}
}

func httpGet(ctx *TestContext, client *http.Client, url string) []byte {
	ctx.Helper()
	// The first request to a newly published onion service has taken over 30
	// seconds on the live Tor network, because the client must fetch the
	// descriptor and build introduction and rendezvous circuits.
	callCtx, callCancel := context.WithTimeout(ctx, time.Minute)
	defer callCancel()
	resp, err := ctxhttp.Get(callCtx, client, url)
	if err != nil {
		ctx.Fatalf("GET %q: got error %v, want nil", url, err)
	}
	defer resp.Body.Close()
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		ctx.Fatalf("ReadAll(%q): got error %v, want nil", url, err)
	}
	return respBytes
}
