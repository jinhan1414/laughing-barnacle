package localapi

import (
	"net"
	"net/url"
	"strings"
)

const DefaultBaseURL = "http://127.0.0.1:8080"

func ResolveBaseURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return DefaultBaseURL
	}

	if strings.Contains(addr, "://") {
		parsed, err := url.Parse(addr)
		if err == nil && strings.TrimSpace(parsed.Host) != "" {
			scheme := strings.TrimSpace(parsed.Scheme)
			if scheme == "" {
				scheme = "http"
			}
			host := normalizeHostPort(parsed.Host)
			if host != "" {
				return scheme + "://" + host
			}
		}
	}

	host := normalizeHostPort(addr)
	if host == "" {
		return DefaultBaseURL
	}
	return "http://" + host
}

func normalizeHostPort(raw string) string {
	hostPort := strings.TrimSpace(raw)
	if hostPort == "" {
		return ""
	}
	if strings.HasPrefix(hostPort, ":") {
		hostPort = "127.0.0.1" + hostPort
	}
	hostPort = strings.Split(hostPort, "/")[0]
	if hostPort == "" {
		return ""
	}

	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		host = normalizeHost(host)
		if host == "" || strings.TrimSpace(port) == "" {
			return ""
		}
		return net.JoinHostPort(host, port)
	}

	return normalizeHost(hostPort)
}

func normalizeHost(raw string) string {
	host := strings.Trim(strings.TrimSpace(raw), "[]")
	switch host {
	case "", "0.0.0.0", "::":
		return "127.0.0.1"
	default:
		return host
	}
}
