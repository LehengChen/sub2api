package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func NewProxyExitInfoProber(cfg *config.Config) service.ProxyExitInfoProber {
	insecure := false
	allowPrivate := false
	validateResolvedIP := true
	maxResponseBytes := defaultProxyProbeResponseMaxBytes
	if cfg != nil {
		insecure = cfg.Security.ProxyProbe.InsecureSkipVerify
		allowPrivate = cfg.Security.URLAllowlist.AllowPrivateHosts
		validateResolvedIP = cfg.Security.URLAllowlist.Enabled
		if cfg.Gateway.ProxyProbeResponseReadMaxBytes > 0 {
			maxResponseBytes = cfg.Gateway.ProxyProbeResponseReadMaxBytes
		}
	}
	if insecure {
		log.Printf("[ProxyProbe] Warning: insecure_skip_verify is not allowed and will cause probe failure.")
	}
	return &proxyProbeService{
		insecureSkipVerify: insecure,
		allowPrivateHosts:  allowPrivate,
		validateResolvedIP: validateResolvedIP,
		maxResponseBytes:   maxResponseBytes,
	}
}

const (
	defaultProxyProbeTimeout          = 10 * time.Second
	defaultProxyProbeResponseMaxBytes = int64(1024 * 1024)
)

type proxyProbeTarget struct {
	url    string
	parser string
}

// defaultProxyProbeTargets are HTTPS-only so testing a proxy never requires
// opening plaintext port 80 on a fixed egress node.
var defaultProxyProbeTargets = []proxyProbeTarget{
	{"https://ipwho.is/?fields=success,message,ip,city,region,country,country_code", "ipwhois"},
	{"https://httpbin.org/ip", "httpbin"},
}

type proxyProbeService struct {
	insecureSkipVerify bool
	allowPrivateHosts  bool
	validateResolvedIP bool
	maxResponseBytes   int64
	probeTargets       []proxyProbeTarget
}

func (s *proxyProbeService) ProbeProxy(ctx context.Context, proxyURL string) (*service.ProxyExitInfo, int64, error) {
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:           proxyURL,
		Timeout:            defaultProxyProbeTimeout,
		InsecureSkipVerify: s.insecureSkipVerify,
		ValidateResolvedIP: s.validateResolvedIP,
		AllowPrivateHosts:  s.allowPrivateHosts,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create proxy client: %w", err)
	}

	probes := s.probeTargets
	if len(probes) == 0 {
		probes = defaultProxyProbeTargets
	}

	var lastErr error
	for _, probe := range probes {
		exitInfo, latencyMs, err := s.probeWithURL(ctx, client, probe.url, probe.parser)
		if err == nil {
			return exitInfo, latencyMs, nil
		}
		lastErr = err
	}

	return nil, 0, fmt.Errorf("all probe URLs failed, last error: %w", lastErr)
}

func (s *proxyProbeService) probeWithURL(ctx context.Context, client *http.Client, url string, parser string) (*service.ProxyExitInfo, int64, error) {
	startTime := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("proxy connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	latencyMs := time.Since(startTime).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		return nil, latencyMs, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	maxResponseBytes := s.maxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultProxyProbeResponseMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, latencyMs, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, latencyMs, fmt.Errorf("proxy probe response exceeds limit: %d", maxResponseBytes)
	}

	switch parser {
	case "ipwhois":
		return s.parseIPWhoIs(body, latencyMs)
	case "httpbin":
		return s.parseHTTPBin(body, latencyMs)
	default:
		return nil, latencyMs, fmt.Errorf("unknown parser: %s", parser)
	}
}

func (s *proxyProbeService) parseIPWhoIs(body []byte, latencyMs int64) (*service.ProxyExitInfo, int64, error) {
	var ipInfo struct {
		Success     bool   `json:"success"`
		Message     string `json:"message"`
		IP          string `json:"ip"`
		City        string `json:"city"`
		Region      string `json:"region"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
	}

	if err := json.Unmarshal(body, &ipInfo); err != nil {
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, latencyMs, fmt.Errorf("failed to parse response: %w (body: %s)", err, preview)
	}
	if !ipInfo.Success {
		if ipInfo.Message == "" {
			ipInfo.Message = "ipwho.is request failed"
		}
		return nil, latencyMs, fmt.Errorf("ipwho.is request failed: %s", ipInfo.Message)
	}
	if strings.TrimSpace(ipInfo.IP) == "" {
		return nil, latencyMs, fmt.Errorf("ipwho.is: no IP found in response")
	}

	return &service.ProxyExitInfo{
		IP:          ipInfo.IP,
		City:        ipInfo.City,
		Region:      ipInfo.Region,
		Country:     ipInfo.Country,
		CountryCode: ipInfo.CountryCode,
	}, latencyMs, nil
}

func (s *proxyProbeService) parseHTTPBin(body []byte, latencyMs int64) (*service.ProxyExitInfo, int64, error) {
	// httpbin.org/ip 返回格式: {"origin": "1.2.3.4"}
	var result struct {
		Origin string `json:"origin"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, latencyMs, fmt.Errorf("failed to parse httpbin response: %w", err)
	}
	if result.Origin == "" {
		return nil, latencyMs, fmt.Errorf("httpbin: no IP found in response")
	}
	return &service.ProxyExitInfo{
		IP: result.Origin,
	}, latencyMs, nil
}
