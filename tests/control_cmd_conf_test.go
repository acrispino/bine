package tests

import (
	"io/ioutil"
	"slices"
	"strings"
	"testing"

	"github.com/cretz/bine/control"
)

func TestGetSetAndResetConf(t *testing.T) {
	ctx := NewTestContext(t, nil)
	defer ctx.Close()
	// Simple get conf
	assertConfVals := func(val string) {
		ctx.Helper()
		entries, err := ctx.Control.GetConf("LogMessageDomains", "ProtocolWarnings")
		if err != nil {
			ctx.Fatalf("GetConf: got error %v, want nil", err)
		}
		if len(entries) != 2 {
			ctx.Fatalf("GetConf entries: got length %d, want 2; entries: %s", len(entries), formatKeyVals(entries))
		}
		for _, key := range []string{"LogMessageDomains", "ProtocolWarnings"} {
			want := control.NewKeyVal(key, val)
			// Compare all KeyVal fields because the pointers are separately allocated.
			if !slices.ContainsFunc(entries, func(entry *control.KeyVal) bool {
				return entry != nil && *entry == *want
			}) {
				ctx.Fatalf("GetConf: got %s, want an entry equal to %s",
					formatKeyVals(entries), formatKeyVals([]*control.KeyVal{want}))
			}
		}
	}
	assertConfVals("0")
	// Change em both to 1
	one := "1"
	err := ctx.Control.SetConf(control.KeyVals("LogMessageDomains", "1", "ProtocolWarnings", "1")...)
	if err != nil {
		ctx.Fatalf("SetConf: got error %v, want nil", err)
	}
	// Check again
	assertConfVals(one)
	// Reset em both
	err = ctx.Control.ResetConf(control.KeyVals("LogMessageDomains", "", "ProtocolWarnings", "")...)
	if err != nil {
		ctx.Fatalf("ResetConf: got error %v, want nil", err)
	}
	// Make sure both back to zero
	assertConfVals("0")
}

func TestLoadConf(t *testing.T) {
	ctx := NewTestContext(t, nil)
	defer ctx.Close()
	// Get entire conf text
	vals, err := ctx.Control.GetInfo("config-text")
	if err != nil {
		ctx.Fatalf("GetInfo(config-text): got error %v, want nil", err)
	}
	if len(vals) != 1 {
		ctx.Fatalf("GetInfo(config-text): got length %d, want 1; vals: %s", len(vals), formatKeyVals(vals))
	}
	if vals[0].Key != "config-text" {
		ctx.Fatalf("GetInfo key: got %q, want %q", vals[0].Key, "config-text")
	}
	confText := vals[0].Val
	// Append new conf val and load
	if strings.Contains(confText, "LogMessageDomains") {
		ctx.Fatalf("config-text: got %q, want no occurrence of %q", confText, "LogMessageDomains")
	}
	confText += "\r\nLogMessageDomains 1"
	if err := ctx.Control.LoadConf(confText); err != nil {
		ctx.Fatalf("LoadConf: got error %v, want nil", err)
	}
	// Check the new val
	vals, err = ctx.Control.GetInfo("config-text")
	if err != nil {
		ctx.Fatalf("GetInfo(config-text): got error %v, want nil", err)
	}
	if len(vals) != 1 {
		ctx.Fatalf("GetInfo(config-text): got length %d, want 1; vals: %s", len(vals), formatKeyVals(vals))
	}
	if vals[0].Key != "config-text" {
		ctx.Fatalf("GetInfo key: got %q, want %q", vals[0].Key, "config-text")
	}
	if !strings.Contains(vals[0].Val, "LogMessageDomains 1") {
		ctx.Fatalf("config-text: got %q, want a string containing %q", vals[0].Val, "LogMessageDomains 1")
	}
}

func TestSaveConf(t *testing.T) {
	ctx := NewTestContext(t, nil)
	defer ctx.Close()
	// Get conf filename
	vals, err := ctx.Control.GetInfo("config-file")
	if err != nil {
		ctx.Fatalf("GetInfo(config-file): got error %v, want nil", err)
	}
	if len(vals) != 1 {
		ctx.Fatalf("GetInfo(config-file): got length %d, want 1; vals: %s", len(vals), formatKeyVals(vals))
	}
	if vals[0].Key != "config-file" {
		ctx.Fatalf("GetInfo key: got %q, want %q", vals[0].Key, "config-file")
	}
	confFile := vals[0].Val
	// Save it
	if err := ctx.Control.SaveConf(false); err != nil {
		ctx.Fatalf("SaveConf: got error %v, want nil", err)
	}
	// Read and make sure, say, the DataDirectory is accurate
	confText, err := ioutil.ReadFile(confFile)
	if err != nil {
		ctx.Fatalf("ReadFile(%q): got error %v, want nil", confFile, err)
	}
	if want := "DataDirectory " + ctx.DataDir; !strings.Contains(string(confText), want) {
		ctx.Fatalf("config file: got %q, want a string containing %q", string(confText), want)
	}
}
