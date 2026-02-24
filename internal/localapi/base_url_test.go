package localapi

import "testing"

func TestResolveBaseURL(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{name: "empty uses default", addr: "", want: DefaultBaseURL},
		{name: "port only", addr: ":9080", want: "http://127.0.0.1:9080"},
		{name: "wildcard host", addr: "0.0.0.0:9080", want: "http://127.0.0.1:9080"},
		{name: "explicit scheme", addr: "https://localhost:9443", want: "https://localhost:9443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBaseURL(tt.addr)
			if got != tt.want {
				t.Fatalf("ResolveBaseURL(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}
