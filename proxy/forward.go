package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"

	"ccproxy/config"
)

// ConnectionMetrics tracks connection-related timing information
type ConnectionMetrics struct {
	DNSLookupStart    time.Time
	DNSLookupEnd      time.Time
	ConnectStart      time.Time
	ConnectEnd        time.Time
	TLSHandshakeStart time.Time
	TLSHandshakeEnd   time.Time
	FirstByteTime     time.Time
	RequestStart      time.Time
	RequestEnd        time.Time
	ConnectionReused  bool
}

func (p *ProxyHandler) forwardRequest(w http.ResponseWriter, r *http.Request, target *config.ProxyTarget) error {
	targetURL, err := p.buildTargetURL(r.URL, target)
	if err != nil {
		return fmt.Errorf("build target URL error: %w", err)
	}

	// Cache request body for potential retries
	bodyBytes, err := p.readAndCacheBody(r)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Initialize connection metrics
	metrics := &ConnectionMetrics{
		RequestStart: time.Now(),
	}

	// Create HTTP trace to collect connection metrics
	trace := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			metrics.DNSLookupStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			metrics.DNSLookupEnd = time.Now()
		},
		ConnectStart: func(network, addr string) {
			metrics.ConnectStart = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			metrics.ConnectEnd = time.Now()
		},
		TLSHandshakeStart: func() {
			metrics.TLSHandshakeStart = time.Now()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			metrics.TLSHandshakeEnd = time.Now()
		},
		GotFirstResponseByte: func() {
			metrics.FirstByteTime = time.Now()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			metrics.ConnectionReused = info.Reused
		},
	}

	req, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add trace to request context
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	p.copyHeaders(req, r, target)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP client error: %w", err)
	}
	defer resp.Body.Close()

	metrics.RequestEnd = time.Now()

	// Convert metrics to map to avoid import cycle issues
	metricsMap := map[string]interface{}{
		"dns_lookup_start":    metrics.DNSLookupStart,
		"dns_lookup_end":      metrics.DNSLookupEnd,
		"connect_start":       metrics.ConnectStart,
		"connect_end":         metrics.ConnectEnd,
		"tls_handshake_start": metrics.TLSHandshakeStart,
		"tls_handshake_end":   metrics.TLSHandshakeEnd,
		"first_byte_time":     metrics.FirstByteTime,
		"request_start":       metrics.RequestStart,
		"request_end":         metrics.RequestEnd,
		"connection_reused":   metrics.ConnectionReused,
	}

	// Store metrics in request context for logger middleware
	ctx := context.WithValue(r.Context(), "connection_metrics", metricsMap)
	*r = *r.WithContext(ctx)

	p.copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy response body: %w", err)
	}

	return nil
}

func (p *ProxyHandler) readAndCacheBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body.Close()

	// Restore the body for the original request
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return bodyBytes, nil
}

func (p *ProxyHandler) buildTargetURL(requestURL *url.URL, target *config.ProxyTarget) (string, error) {
	targetURL, err := url.Parse(target.TargetURL)
	if err != nil {
		return "", err
	}

	path := requestURL.Path
	if strings.HasSuffix(target.Path, "*") {
		// For wildcard paths like /v1/*, preserve the full original path
		// and append it to the target URL's path
		targetBasePath := strings.TrimSuffix(targetURL.Path, "/")
		if targetBasePath == "" {
			path = requestURL.Path
		} else {
			path = targetBasePath + requestURL.Path
		}
	} else {
		// For exact path matches, use the target URL's path
		path = targetURL.Path
	}

	finalURL := &url.URL{
		Scheme:   targetURL.Scheme,
		Host:     targetURL.Host,
		Path:     path,
		RawQuery: requestURL.RawQuery,
	}

	return finalURL.String(), nil
}

func (p *ProxyHandler) copyHeaders(req *http.Request, original *http.Request, target *config.ProxyTarget) {
	for key, values := range original.Header {
		if key == "Host" {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	for key, value := range target.Headers {
		req.Header.Set(key, value)
	}
}

func (p *ProxyHandler) copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}
