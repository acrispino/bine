package tests

import (
	"context"
	"testing"
	"time"

	"github.com/cretz/bine/control"
)

const hsFetchOnion = "2gzyxa5ihm7nsggfxnu52rck2vv4rvmdlkiu3zzui5du4xyclen53wid"

func TestHSFetch(t *testing.T) {
	ctx := GlobalEnabledNetworkContext(t)
	// Add listener
	eventCh := make(chan control.Event)
	defer close(eventCh)
	err := ctx.Control.AddEventListener(eventCh, control.EventCodeHSDescContent)
	if err != nil {
		ctx.Fatalf("AddEventListener: got error %v, want nil", err)
	}
	defer ctx.Control.RemoveEventListener(eventCh, control.EventCodeHSDescContent)
	// Lookup HS
	err = ctx.Control.GetHiddenServiceDescriptorAsync(hsFetchOnion, "")
	if err != nil {
		ctx.Fatalf("GetHiddenServiceDescriptorAsync: got error %v, want nil", err)
	}
	// Grab events
	eventCtx, eventCancel := context.WithTimeout(ctx, 45*time.Second)
	defer eventCancel()
	errCh := make(chan error, 1)
	go func() { errCh <- ctx.Control.HandleEvents(eventCtx) }()
	select {
	case <-eventCtx.Done():
		if err := eventCtx.Err(); err != nil {
			ctx.Fatalf("event context: got error %v, want nil", err)
		}
	case err := <-errCh:
		if err != nil {
			ctx.Fatalf("HandleEvents: got error %v, want nil", err)
		}
	case event := <-eventCh:
		hsEvent := event.(*control.HSDescContentEvent)
		if hsEvent.Address != hsFetchOnion {
			ctx.Fatalf("HSDescContent address: got %q, want %q", hsEvent.Address, hsFetchOnion)
		}
	}
}
