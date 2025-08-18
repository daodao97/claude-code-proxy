package types

import "time"

// LogMessage 日志消息结构体
type LogMessage struct {
	Timestamp       string            `json:"timestamp"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	Query           string            `json:"query"`
	RequestHeaders  map[string]string `json:"request_headers"`
	ResponseHeaders map[string]string `json:"response_headers"`
	RemoteAddr      string            `json:"remote_addr"`
	StatusCode      int               `json:"status_code"`
	Duration        string            `json:"duration"`
	TargetURL       string            `json:"target_url"`
	RequestBody     string            `json:"request_body,omitempty"`
	ResponseBody    string            `json:"response_body,omitempty"`
	Error           string            `json:"error,omitempty"`
	Stats           *Statistics       `json:"stats,omitempty"`
	// Connection metrics
	ConnectDuration   string `json:"connect_duration,omitempty"`
	DNSLookupDuration string `json:"dns_lookup_duration,omitempty"`
	TLSHandshakeDuration string `json:"tls_handshake_duration,omitempty"`
	FirstByteDuration string `json:"first_byte_duration,omitempty"`
	UpstreamLatency   string `json:"upstream_latency,omitempty"`
	TotalLatency      string `json:"total_latency,omitempty"`
	ConnectionReused  bool   `json:"connection_reused,omitempty"`
}

// Statistics 统计信息结构体
type Statistics struct {
	TotalRequests    int64     `json:"total_requests"`
	SuccessRequests  int64     `json:"success_requests"`
	ErrorRequests    int64     `json:"error_requests"`
	StartTime        time.Time `json:"start_time"`
	LastRequestTime  time.Time `json:"last_request_time"`
	StatusCodeCounts map[int]int64 `json:"status_code_counts"`
	MethodCounts     map[string]int64 `json:"method_counts"`
}