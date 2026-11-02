package tests

import "testing"

func TestSignal(t *testing.T) {
	ctx := NewTestContext(t, nil)
	defer ctx.Close()
	if err := ctx.Control.Signal("HEARTBEAT"); err != nil {
		ctx.Fatalf("Signal: got error %v, want nil", err)
	}
}
