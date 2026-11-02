package tests

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/cretz/bine/tor"
	"github.com/cretz/bine/torutil"
)

func TestListenSimpleHTTPV3(t *testing.T) {
	ctx := GlobalEnabledNetworkContext(t)
	// Create an onion service to listen on random port but show as 80
	conf := &tor.ListenConf{RemotePorts: []int{80}}
	// _, conf.Key, _ = ed25519.GenerateKey(nil)
	client, server, onion := startHTTPServer(ctx, conf, "/test", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("Test Content"))
		if err != nil {
			ctx.Errorf("Write: got error %v, want nil", err)
		}
	})
	defer server.Shutdown(ctx)
	// Check the service ID
	if want := torutil.OnionServiceIDFromPrivateKey(onion.Key); onion.ID != want {
		ctx.Fatalf("onion ID: got %q, want %q", onion.ID, want)
	}
	// Call /test
	byts := httpGet(ctx, client, "http://"+onion.ID+".onion/test")
	if string(byts) != "Test Content" {
		ctx.Fatalf("response: got %q, want %q", string(byts), "Test Content")
	}
}

// Only have to shutdown the HTTP server
func startHTTPServer(
	ctx *TestContext,
	listenConf *tor.ListenConf,
	handlePattern string,
	handler func(http.ResponseWriter, *http.Request),
) (*http.Client, *http.Server, *tor.OnionService) {
	ctx.Helper()
	httpClient := httpClient(ctx, nil)
	// Wait at most a few minutes for the entire test
	listenCtx, listenCancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer listenCancel()
	// Create an onion service to listen on random port but show as 80
	onion, err := ctx.Listen(listenCtx, listenConf)
	if err != nil {
		ctx.Fatalf("Listen: got error %v, want nil", err)
	}
	// Make HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc(handlePattern, handler)
	httpServer := &http.Server{Handler: mux}
	go func() {
		if err := httpServer.Serve(onion); !errors.Is(err, http.ErrServerClosed) {
			ctx.Errorf("Serve: got %v, want %v", err, http.ErrServerClosed)
		}
	}()
	return httpClient, httpServer, onion
}
