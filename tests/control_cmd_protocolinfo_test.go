package tests

import (
	"slices"
	"strings"
	"testing"
)

func TestProtocolInfo(t *testing.T) {
	ctx := NewTestContext(t, nil)
	defer ctx.Close()
	info, err := ctx.Control.ProtocolInfo()
	if err != nil {
		ctx.Fatalf("ProtocolInfo: got error %v, want nil", err)
	}
	if !slices.Contains(info.AuthMethods, "SAFECOOKIE") {
		ctx.Fatalf("AuthMethods: got %q, want an element equal to %q", info.AuthMethods, "SAFECOOKIE")
	}
	if !strings.HasPrefix(info.TorVersion, "0.4") {
		ctx.Fatalf("TorVersion: got %q, want prefix %q", info.TorVersion, "0.4")
	}
}
