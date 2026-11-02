package tests

import (
	"testing"

	"github.com/cretz/bine/tor"
	"github.com/cretz/bine/torutil/geoipembed"
)

func TestEmbeddedGeoIPFile(t *testing.T) {
	ctx := NewTestContext(t, &tor.StartConf{GeoIPFileReader: geoipembed.GeoIPReader})
	defer ctx.Close()
	// Check available and grab a couple of known IPs and check the country
	// (taken from https://my.pingdom.com/probes/feed)
	usIpv4, usIpv6 := "209.58.139.193", "2605:fe80:2100:a00f:4::4045"
	kv, err := ctx.Control.GetInfo(
		"ip-to-country/ipv4-available",
		"ip-to-country/ipv6-available",
		"ip-to-country/"+usIpv4,
		"ip-to-country/"+usIpv6,
	)
	if err != nil {
		ctx.Fatalf("GetInfo: got error %v, want nil", err)
	}
	vals := map[string]string{}
	for _, kv := range kv {
		vals[kv.Key] = kv.Val
	}
	if len(vals) != 4 {
		ctx.Fatalf("GetInfo: got length %d, want 4; vals: %v", len(vals), vals)
	}
	if vals["ip-to-country/ipv4-available"] != "1" {
		ctx.Fatalf("IPv4 availability: got %q, want %q", vals["ip-to-country/ipv4-available"], "1")
	}
	if vals["ip-to-country/ipv6-available"] != "1" {
		ctx.Fatalf("IPv6 availability: got %q, want %q", vals["ip-to-country/ipv6-available"], "1")
	}
	if vals["ip-to-country/"+usIpv4] != "us" {
		ctx.Fatalf("country for %q: got %q, want %q", usIpv4, vals["ip-to-country/"+usIpv4], "us")
	}
	if vals["ip-to-country/"+usIpv6] != "us" {
		ctx.Fatalf("country for %q: got %q, want %q", usIpv6, vals["ip-to-country/"+usIpv6], "us")
	}
}
