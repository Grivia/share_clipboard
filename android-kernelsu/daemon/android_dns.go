package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type dnsCacheEntry struct {
	addresses []net.IP
	expiresAt time.Time
}

type androidResolver struct {
	dialer  *net.Dialer
	jarPath string
	mu      sync.Mutex
	cache   map[string]dnsCacheEntry
}

func newAndroidResolver(dialer *net.Dialer, jarPath string) *androidResolver {
	return &androidResolver{
		dialer: dialer, jarPath: jarPath, cache: make(map[string]dnsCacheEntry),
	}
}

func (r *androidResolver) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if net.ParseIP(host) != nil {
		return r.dialer.DialContext(ctx, network, address)
	}
	addresses, err := r.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range addresses {
		if network == "tcp4" && ip.To4() == nil {
			continue
		}
		if network == "tcp6" && ip.To4() != nil {
			continue
		}
		connection, err := r.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable address")
	}
	return nil, fmt.Errorf("dial %s: %w", host, lastErr)
}

func (r *androidResolver) resolve(ctx context.Context, host string) ([]net.IP, error) {
	r.mu.Lock()
	entry, found := r.cache[host]
	if found && time.Now().Before(entry.expiresAt) {
		addresses := append([]net.IP(nil), entry.addresses...)
		r.mu.Unlock()
		return addresses, nil
	}
	r.mu.Unlock()

	resolveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var output []byte
	var err error
	var failures []string
	for _, launcher := range dnsAppProcessLaunchers() {
		attemptCtx, attemptCancel := context.WithTimeout(resolveCtx, 3*time.Second)
		output, err = r.runResolver(attemptCtx, host, launcher)
		attemptCancel()
		if err == nil {
			break
		}
		failures = append(failures, launcher.name+": "+err.Error())
	}
	if err != nil {
		return nil, fmt.Errorf(
			"resolve %s with Android launchers: %s",
			host,
			strings.Join(failures, "; "),
		)
	}
	addresses := parseResolvedIPs(string(output))
	if len(addresses) == 0 {
		return nil, fmt.Errorf("Android resolver returned no address for %s", host)
	}
	r.mu.Lock()
	r.cache[host] = dnsCacheEntry{addresses: addresses, expiresAt: time.Now().Add(5 * time.Minute)}
	r.mu.Unlock()
	return append([]net.IP(nil), addresses...), nil
}

func (r *androidResolver) runResolver(
	ctx context.Context,
	host string,
	launcher appProcessLauncher,
) ([]byte, error) {
	if r.jarPath == "" {
		return nil, fmt.Errorf("bridge JAR path is empty")
	}
	command, err := launcher.command(
		ctx,
		r.jarPath,
		"hair.zhy.fastcopy.DnsResolver",
		host,
	)
	if err != nil {
		return nil, err
	}
	return command.Output()
}

func parseResolvedIPs(output string) []net.IP {
	var addresses []net.IP
	for _, line := range strings.Fields(output) {
		if ip := net.ParseIP(strings.TrimSpace(line)); ip != nil {
			addresses = append(addresses, ip)
		}
	}
	return addresses
}
