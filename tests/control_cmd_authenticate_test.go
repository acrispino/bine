package tests

import (
	"slices"
	"testing"

	"github.com/cretz/bine/tor"
)

func TestAuthenticateNull(t *testing.T) {
	ctx := NewTestContext(t, &tor.StartConf{DisableCookieAuth: true, DisableEagerAuth: true})
	defer ctx.Close()
	// Verify auth methods before auth
	info, err := ctx.Control.ProtocolInfo()
	if err != nil {
		ctx.Fatalf("ProtocolInfo: got error %v, want nil", err)
	}
	want := []string{"NULL"}
	if !sameStringElements(want, info.AuthMethods) {
		ctx.Fatalf("AuthMethods: got %q, want %q (ignoring order)", info.AuthMethods, want)
	}
	if err := ctx.Control.Authenticate(""); err != nil {
		ctx.Fatalf("Authenticate: got error %v, want nil", err)
	}
}

func TestAuthenticateSafeCookie(t *testing.T) {
	ctx := NewTestContext(t, &tor.StartConf{DisableEagerAuth: true})
	defer ctx.Close()
	// Verify auth methods before auth
	info, err := ctx.Control.ProtocolInfo()
	if err != nil {
		ctx.Fatalf("ProtocolInfo: got error %v, want nil", err)
	}
	want := []string{"COOKIE", "SAFECOOKIE"}
	if !sameStringElements(want, info.AuthMethods) {
		ctx.Fatalf("AuthMethods: got %q, want %q (ignoring order)", info.AuthMethods, want)
	}
	if err := ctx.Control.Authenticate(""); err != nil {
		ctx.Fatalf("Authenticate: got error %v, want nil", err)
	}
}

func TestAuthenticateHashedPassword(t *testing.T) {
	// "testpass" - 16:5417AE717521511A609921392778FFA8518EC089BF2162A199241AEB4A
	ctx := NewTestContext(t, &tor.StartConf{
		DisableCookieAuth: true,
		DisableEagerAuth:  true,
		ExtraArgs:         []string{"--HashedControlPassword", "16:5417AE717521511A609921392778FFA8518EC089BF2162A199241AEB4A"},
	})
	defer ctx.Close()
	// Verify auth methods before auth
	info, err := ctx.Control.ProtocolInfo()
	if err != nil {
		ctx.Fatalf("ProtocolInfo: got error %v, want nil", err)
	}
	want := []string{"HASHEDPASSWORD"}
	if !sameStringElements(want, info.AuthMethods) {
		ctx.Fatalf("AuthMethods: got %q, want %q (ignoring order)", info.AuthMethods, want)
	}
	if err := ctx.Control.Authenticate("testpass"); err != nil {
		ctx.Fatalf("Authenticate: got error %v, want nil", err)
	}
}

func sameStringElements(want, got []string) bool {
	sortedWant, sortedGot := slices.Clone(want), slices.Clone(got)
	slices.Sort(sortedWant)
	slices.Sort(sortedGot)
	return slices.Equal(sortedWant, sortedGot)
}
