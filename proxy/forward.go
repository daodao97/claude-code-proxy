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

	// Get effective proxy URL and create client
	proxyURL := p.getEffectiveProxy(target)
	client, err := p.createHTTPClientWithProxy(proxyURL)
	if err != nil {
		return fmt.Errorf("failed to create HTTP client with proxy: %w", err)
	}

	// Log proxy usage for debugging
	if proxyURL != "" {
		fmt.Printf("[INFO] Using HTTP proxy: %s for target: %s\n", proxyURL, targetURL)
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

	resp, err := client.Do(req)
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

	// Store metrics and target_url in request context for logger middleware
	ctx := r.Context()
	ctx = context.WithValue(ctx, "connection_metrics", metricsMap)
	
	// Preserve target_url from the original context
	if targetURLVal := r.Context().Value("target_url"); targetURLVal != nil {
		ctx = context.WithValue(ctx, "target_url", targetURLVal)
	}
	
	*r = *r.WithContext(ctx)

	p.copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)

	// Check if this is a streaming response (SSE or similar)
	contentType := resp.Header.Get("Content-Type")
	transferEncoding := resp.Header.Get("Transfer-Encoding")

	isStreaming := strings.Contains(contentType, "text/event-stream") ||
		strings.Contains(contentType, "application/x-ndjson") ||
		strings.Contains(transferEncoding, "chunked") ||
		resp.ContentLength == -1 || // Unknown content length indicates streaming
		strings.Contains(contentType, "text/plain")

	if isStreaming {
		err = p.streamResponse(w, resp)
	} else {
		_, err = io.Copy(w, resp.Body)
	}

	if err != nil {
		return fmt.Errorf("failed to copy response body: %w", err)
	}

	return nil
}

func (p *ProxyHandler) streamResponse(w http.ResponseWriter, resp *http.Response) error {
	// Get the Flusher interface to enable streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Fallback to regular copy if Flusher is not available
		_, err := io.Copy(w, resp.Body)
		return err
	}

	// Check if this is an SSE response for byte-based streaming
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		return p.streamSSEResponse(w, resp.Body, flusher)
	}

	// For other streaming content, use byte-based streaming
	return p.streamBytesResponse(w, resp.Body, flusher)
}

func (p *ProxyHandler) streamSSEResponse(w http.ResponseWriter, body io.Reader, flusher http.Flusher) error {
	// Use small buffer to minimize latency but avoid breaking SSE chunks
	buffer := make([]byte, 1024)
	
	for {
		n, err := body.Read(buffer)
		if n > 0 {
			// Write the exact bytes received to preserve SSE format
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return fmt.Errorf("failed to write SSE chunk: %w", writeErr)
			}
			// Flush immediately to ensure real-time streaming
			flusher.Flush()
		}
		
		if err != nil {
			if err == io.EOF {
				// End of stream - this is expected
				break
			}
			return fmt.Errorf("failed to read SSE response: %w", err)
		}
	}
	
	return nil
}

func (p *ProxyHandler) streamBytesResponse(w http.ResponseWriter, body io.Reader, flusher http.Flusher) error {
	// Use small buffer for non-SSE streaming content
	buffer := make([]byte, 64)

	for {
		n, err := body.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return fmt.Errorf("failed to write chunk: %w", writeErr)
			}
			flusher.Flush()
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read from response body: %w", err)
		}
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

	// Ensure proper SSE headers for streaming responses
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		// Ensure SSE headers are properly set
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
	}
}
