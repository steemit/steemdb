package main

import "testing"

// TestIsLoopbackListenAddr covers the guard that decides whether the
// unauthenticated ingest server warns about a non-loopback bind.
func TestIsLoopbackListenAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"localhost:8080", true},
		{"127.0.0.1:0", true},
		{":8080", false},        // all interfaces
		{"0.0.0.0:8080", false}, // all interfaces
		{"192.168.1.10:8080", false},
		{"10.0.0.5:8080", false},
		{"", false}, // http.DefaultAddr (all interfaces)
	}

	for _, tt := range tests {
		if got := isLoopbackListenAddr(tt.addr); got != tt.want {
			t.Errorf("isLoopbackListenAddr(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}
