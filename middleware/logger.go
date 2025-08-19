package middleware

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ccproxy/config"
	"ccproxy/websocket"
)

type LoggerMiddleware struct {
	handler http.Handler
	hub     *websocket.Hub
	config  *config.Config
}

func NewLoggerMiddleware(handler http.Handler, hub *websocket.Hub, config *config.Config) *LoggerMiddleware {
	return &LoggerMiddleware{
		handler: handler,
		hub:     hub,
		config:  config,
	}
}

func (l *LoggerMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	

	var requestBody []byte
	if r.Body != nil {
		requestBody, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(requestBody))
	}

	wrapped := &responseWriterCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           &bytes.Buffer{},
	}

	l.handler.ServeHTTP(wrapped, r)


	duration := time.Since(start)

	// Use actual headers sent to target if available, otherwise use original headers
	requestHeaders := make(map[string]string)
	if actualHeaders := r.Context().Value("actual_request_headers"); actualHeaders != nil {
		if headerMap, ok := actualHeaders.(map[string]string); ok {
			requestHeaders = headerMap
		}
	} else {
		// Fallback to original headers if actual headers not available
		for key, values := range r.Header {
			if len(values) > 0 {
				requestHeaders[key] = values[0]
			}
		}
	}

	responseHeaders := make(map[string]string)
	for key, values := range wrapped.Header() {
		if len(values) > 0 {
			responseHeaders[key] = values[0]
		}
	}

	// Get target URL from wrapped response writer instead of context
	targetURL := wrapped.targetURL

	// Process response body for both streaming and regular responses
	responseBody := l.processResponseBody(wrapped.body.Bytes(), responseHeaders)
	
	// Add streaming indicator to help identify the response type
	if wrapped.isStreaming && responseBody != "" {
		responseBody = fmt.Sprintf("[STREAMING RESPONSE - %d bytes]\n%s", 
			len(wrapped.body.Bytes()), responseBody)
	}

	logMessage := &websocket.LogMessage{
		Timestamp:       start.Format("2006-01-02 15:04:05.000"),
		Method:          r.Method,
		Path:            r.URL.Path,
		Query:           r.URL.RawQuery,
		RequestHeaders:  requestHeaders,
		ResponseHeaders: responseHeaders,
		RemoteAddr:      r.RemoteAddr,
		StatusCode:      wrapped.statusCode,
		Duration:        duration.String(),
		TargetURL:       targetURL,
		RequestBody:     string(requestBody),
		ResponseBody:    responseBody,
	}

	// Extract and set connection metrics if available
	l.setConnectionMetrics(logMessage, r, duration)

	if l.hub != nil {
		l.hub.Broadcast(logMessage)
	}
}

func (l *LoggerMiddleware) processResponseBody(body []byte, headers map[string]string) string {
	if len(body) == 0 {
		return ""
	}

	// Check if the response is compressed
	contentEncoding := headers["Content-Encoding"]
	if contentEncoding == "" {
		contentEncoding = headers["content-encoding"]
	}

	// Handle different compression formats
	contentEncodingLower := strings.ToLower(contentEncoding)

	if strings.Contains(contentEncodingLower, "gzip") {
		if decompressed, err := l.decompressGzip(body); err == nil {
			return string(decompressed)
		}
		return "[GZIP COMPRESSED DATA - Failed to decompress]\n" + string(body)
	}

	if strings.Contains(contentEncodingLower, "deflate") {
		if decompressed, err := l.decompressDeflate(body); err == nil {
			return string(decompressed)
		}
		return "[DEFLATE COMPRESSED DATA - Failed to decompress]\n" + string(body)
	}

	// For non-compressed data, return as-is
	return string(body)
}

func (l *LoggerMiddleware) decompressGzip(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}

	return decompressed, nil
}

func (l *LoggerMiddleware) decompressDeflate(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		return nil, err
	}
	defer zlibReader.Close()

	decompressed, err := io.ReadAll(zlibReader)
	if err != nil {
		return nil, err
	}

	return decompressed, nil
}

// ConnectionMetrics represents connection timing information
// This mirrors the struct in proxy/forward.go to avoid import cycles
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

func (l *LoggerMiddleware) setConnectionMetrics(logMessage *websocket.LogMessage, r *http.Request, totalDuration time.Duration) {
	// Always set total latency from the middleware's perspective
	logMessage.TotalLatency = totalDuration.String()
	logMessage.UpstreamLatency = totalDuration.String()
	
	// Try to extract connection metrics from request context
	if metricsVal := r.Context().Value("connection_metrics"); metricsVal != nil {
		// Use type assertion to check if it's our expected type
		// Since we can't import proxy package, we'll use reflection-like approach
		// For now, let's use a map-based approach for flexibility
		if metricsMap, ok := metricsVal.(map[string]interface{}); ok {
			l.extractMetricsFromMap(logMessage, metricsMap)
		}
	}
}

func (l *LoggerMiddleware) extractDetailedMetrics(logMessage *websocket.LogMessage, metrics *ConnectionMetrics) {
	// Calculate individual timing components
	if !metrics.DNSLookupStart.IsZero() && !metrics.DNSLookupEnd.IsZero() {
		dnsLookup := metrics.DNSLookupEnd.Sub(metrics.DNSLookupStart)
		logMessage.DNSLookupDuration = dnsLookup.String()
	}
	
	if !metrics.ConnectStart.IsZero() && !metrics.ConnectEnd.IsZero() {
		connectTime := metrics.ConnectEnd.Sub(metrics.ConnectStart)
		logMessage.ConnectDuration = connectTime.String()
	}
	
	if !metrics.TLSHandshakeStart.IsZero() && !metrics.TLSHandshakeEnd.IsZero() {
		tlsTime := metrics.TLSHandshakeEnd.Sub(metrics.TLSHandshakeStart)
		logMessage.TLSHandshakeDuration = tlsTime.String()
	}
	
	if !metrics.FirstByteTime.IsZero() && !metrics.RequestStart.IsZero() {
		firstByteTime := metrics.FirstByteTime.Sub(metrics.RequestStart)
		logMessage.FirstByteDuration = firstByteTime.String()
	}
	
	if !metrics.RequestStart.IsZero() && !metrics.RequestEnd.IsZero() {
		upstreamLatency := metrics.RequestEnd.Sub(metrics.RequestStart)
		logMessage.UpstreamLatency = upstreamLatency.String()
		logMessage.TotalLatency = upstreamLatency.String()
	}
	
	logMessage.ConnectionReused = metrics.ConnectionReused
}

func (l *LoggerMiddleware) extractMetricsFromMap(logMessage *websocket.LogMessage, metricsMap map[string]interface{}) {
	// Extract timing durations from the metrics map
	if dnsStart, ok := metricsMap["dns_lookup_start"].(time.Time); ok {
		if dnsEnd, ok := metricsMap["dns_lookup_end"].(time.Time); ok && !dnsStart.IsZero() && !dnsEnd.IsZero() {
			logMessage.DNSLookupDuration = dnsEnd.Sub(dnsStart).String()
		}
	}
	
	if connectStart, ok := metricsMap["connect_start"].(time.Time); ok {
		if connectEnd, ok := metricsMap["connect_end"].(time.Time); ok && !connectStart.IsZero() && !connectEnd.IsZero() {
			logMessage.ConnectDuration = connectEnd.Sub(connectStart).String()
		}
	}
	
	if tlsStart, ok := metricsMap["tls_handshake_start"].(time.Time); ok {
		if tlsEnd, ok := metricsMap["tls_handshake_end"].(time.Time); ok && !tlsStart.IsZero() && !tlsEnd.IsZero() {
			logMessage.TLSHandshakeDuration = tlsEnd.Sub(tlsStart).String()
		}
	}
	
	if requestStart, ok := metricsMap["request_start"].(time.Time); ok {
		if firstByte, ok := metricsMap["first_byte_time"].(time.Time); ok && !requestStart.IsZero() && !firstByte.IsZero() {
			logMessage.FirstByteDuration = firstByte.Sub(requestStart).String()
		}
		
		if requestEnd, ok := metricsMap["request_end"].(time.Time); ok && !requestStart.IsZero() && !requestEnd.IsZero() {
			logMessage.UpstreamLatency = requestEnd.Sub(requestStart).String()
		}
	}
	
	if reused, ok := metricsMap["connection_reused"].(bool); ok {
		logMessage.ConnectionReused = reused
	}
}

type responseWriterCapture struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
	isStreaming bool
	targetURL  string
}

func (rw *responseWriterCapture) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterCapture) Write(b []byte) (int, error) {
	// For streaming responses, capture the body for logging but mark as streaming
	contentType := rw.Header().Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") ||
		strings.Contains(contentType, "application/x-ndjson") ||
		rw.Header().Get("Transfer-Encoding") == "chunked" {
		rw.isStreaming = true
		// Still capture the body for complete logging
		rw.body.Write(b)
		return rw.ResponseWriter.Write(b)
	}
	
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

// Implement http.Flusher interface
func (rw *responseWriterCapture) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// SetTargetURL allows setting the target URL for logging
func (rw *responseWriterCapture) SetTargetURL(url string) {
	rw.targetURL = url
}
