// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

//go:build unittest

package client

import (
	"net/http"
	"testing"
)

// With both HTTPS_PROXY and ALL_PROXY set to a SOCKS proxy, the ALL_PROXY
// dialer must not also be installed: Transport.Proxy already tunnels through
// HTTPS_PROXY, and stacking the two dials the proxy through itself.
func TestNewClient_ProxyEnv(t *testing.T) {
	tests := []struct {
		name     string
		https    string
		all      string
		wantDial bool
	}{
		{"all_proxy only uses dialer", "", "socks5h://127.0.0.1:9999", true},
		{"https_proxy only uses transport proxy", "socks5h://127.0.0.1:9999", "", false},
		{"both set does not stack", "socks5h://127.0.0.1:9999", "socks5h://127.0.0.1:9999", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HTTPS_PROXY", tt.https)
			t.Setenv("https_proxy", "")
			t.Setenv("ALL_PROXY", tt.all)
			t.Setenv("all_proxy", "")
			c := NewClient("https://ssp.example.internal", "u", "p", true)
			tr := c.httpClient.Transport.(*http.Transport)
			if got := tr.DialContext != nil; got != tt.wantDial {
				t.Errorf("DialContext set = %v, want %v", got, tt.wantDial)
			}
		})
	}
}
