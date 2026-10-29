/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package endpointaudit

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

var allowPrivateNetworks atomic.Bool

// AllowPrivateNetworksForTest lets tests call a local HTTPS server.
// Production keeps the default, which rejects loopback and private addresses.
func AllowPrivateNetworksForTest(allow bool) func() {
	previous := allowPrivateNetworks.Load()
	allowPrivateNetworks.Store(allow)
	return func() { allowPrivateNetworks.Store(previous) }
}

func NormalizeMessagesURL(ctx context.Context, raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return nil, errors.New("only http and https endpoints can be audited")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, errors.New("only http and https endpoints can be audited")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("only http and https endpoints can be audited")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("endpoint address is not allowed")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if blockedHostname(host) {
		return nil, errors.New("endpoint address is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if blockedIP(ip) {
			return nil, errors.New("endpoint address is not allowed")
		}
	} else if err := resolveAllowed(ctx, host); err != nil {
		return nil, err
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/v1/messages"
	case strings.HasSuffix(path, "/v1"):
		parsed.Path = path + "/messages"
	case strings.HasSuffix(path, "/messages"):
		parsed.Path = path
	default:
		parsed.Path = path + "/v1/messages"
	}
	parsed.RawPath = ""
	return parsed, nil
}

func blockedHostname(host string) bool {
	if allowPrivateNetworks.Load() {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	switch host {
	case "metadata.google.internal", "metadata.internal":
		return true
	}
	return false
}

func blockedIP(ip net.IP) bool {
	if allowPrivateNetworks.Load() {
		return false
	}
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
		return true
	}
	return false
}

func resolveAllowed(ctx context.Context, host string) error {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return errors.New("endpoint address could not be resolved")
	}
	for _, ip := range ips {
		if blockedIP(ip.IP) {
			return errors.New("endpoint address is not allowed")
		}
	}
	return nil
}

func NewSafeClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if ip := net.ParseIP(host); ip != nil {
				if blockedIP(ip) {
					return nil, errors.New("endpoint address is not allowed")
				}
				return dialer.DialContext(ctx, network, addr)
			}
			if err := resolveAllowed(ctx, host); err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("endpoint address could not be resolved")
			}
			var last error
			for _, ip := range ips {
				if blockedIP(ip.IP) {
					return nil, errors.New("endpoint address is not allowed")
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				last = dialErr
			}
			if last == nil {
				last = errors.New("endpoint address is not allowed")
			}
			return nil, last
		},
		ForceAttemptHTTP2:   false,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return errors.New("endpoint address is not allowed")
			}
			_, err := NormalizeMessagesURL(req.Context(), req.URL.String())
			return err
		},
	}
}

var clientFactory = NewSafeClient

func Client() *http.Client {
	return clientFactory()
}

func SetClientForTest(client *http.Client) func() {
	previous := clientFactory
	clientFactory = func() *http.Client { return client }
	return func() { clientFactory = previous }
}
