// SSRF 防护：拒绝私网/回环地址，拨号前校验目标 IP
package httpx

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"
)

var privateCIDRs = []string{
	// IPv4
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	// IPv6
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
	"2001:db8::/32",
}

var privateNets []*net.IPNet

func init() {
	for _, c := range privateCIDRs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			privateNets = append(privateNets, n)
		}
	}
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// resolveAndCheck 解析 host，并确保其所有 IP 都不是内网/回环地址
func resolveAndCheck(host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("blocked: %s is a private address", host)
		}
		return []net.IP{ip}, nil
	}
	ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP for %s", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("blocked: %s resolves to private address %s", host, ip)
		}
	}
	return ips, nil
}

// BlockPrivate 拒绝私网/回环地址（导出供 test 与其它包复用）
func BlockPrivate(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil
	}
	_, err = resolveAndCheck(u.Hostname())
	return err
}

func blockPrivate(rawURL string) error { return BlockPrivate(rawURL) }

// safeDialContext 拨号前校验目标 IP，并直接用已校验的 IP 连接，消除 DNS rebinding 的 TOCTOU
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 15 * time.Second}
	if proxyURL != nil {
		// 走代理时拨号目标是代理本身（常为 127.0.0.1），跳过私网校验
		// 目标地址的 SSRF 校验由 Request 里的 BlockPrivate 负责
		return d.DialContext(ctx, network, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := resolveAndCheck(host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no address for %s", host)
	}
	return nil, lastErr
}
