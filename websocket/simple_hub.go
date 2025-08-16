package websocket

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const websocketMagicString = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Hub struct {
	clients   map[*Client]bool
	broadcast chan *LogMessage
	mu        sync.RWMutex
	stats     *Statistics
}

type Statistics struct {
	TotalRequests    int64     `json:"total_requests"`
	SuccessRequests  int64     `json:"success_requests"`
	ErrorRequests    int64     `json:"error_requests"`
	StartTime        time.Time `json:"start_time"`
	LastRequestTime  time.Time `json:"last_request_time"`
	StatusCodeCounts map[int]int64 `json:"status_code_counts"`
	MethodCounts     map[string]int64 `json:"method_counts"`
	mu               sync.RWMutex
}

type Client struct {
	conn   net.Conn
	hub    *Hub
	closed bool
	mu     sync.Mutex
}

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
	ConnectDuration   string `json:"connect_duration,omitempty"`   // TCP连接时间
	DNSLookupDuration string `json:"dns_lookup_duration,omitempty"` // DNS解析时间
	TLSHandshakeDuration string `json:"tls_handshake_duration,omitempty"` // TLS握手时间
	FirstByteDuration string `json:"first_byte_duration,omitempty"` // 到第一字节的时间
	UpstreamLatency   string `json:"upstream_latency,omitempty"`    // 上游服务延迟
	TotalLatency      string `json:"total_latency,omitempty"`       // 总延迟
	ConnectionReused  bool   `json:"connection_reused,omitempty"`   // 连接是否被重用
}

func NewHub(broadcastSize int) *Hub {
	return &Hub{
		clients:   make(map[*Client]bool),
		broadcast: make(chan *LogMessage, broadcastSize),
		stats: &Statistics{
			StartTime:        time.Now(),
			StatusCodeCounts: make(map[int]int64),
			MethodCounts:     make(map[string]int64),
		},
	}
}

func (h *Hub) Run() {
	for message := range h.broadcast {
		h.mu.RLock()
		for client := range h.clients {
			go client.sendMessage(message)
		}
		h.mu.RUnlock()
	}
}

func (h *Hub) Broadcast(message *LogMessage) {
	// Update statistics
	h.updateStats(message)
	
	// Add current statistics to the message
	message.Stats = h.GetStats()
	
	select {
	case h.broadcast <- message:
	default:
		log.Println("[WARN] Broadcast channel full, dropping message")
	}
}

func (h *Hub) updateStats(message *LogMessage) {
	h.stats.mu.Lock()
	defer h.stats.mu.Unlock()
	
	h.stats.TotalRequests++
	h.stats.LastRequestTime = time.Now()
	
	// Update method counts
	h.stats.MethodCounts[message.Method]++
	
	// Update status code counts
	h.stats.StatusCodeCounts[message.StatusCode]++
	
	// Update success/error counts
	if message.StatusCode >= 200 && message.StatusCode < 400 {
		h.stats.SuccessRequests++
	} else {
		h.stats.ErrorRequests++
	}
}

func (h *Hub) GetStats() *Statistics {
	h.stats.mu.RLock()
	defer h.stats.mu.RUnlock()
	
	// Create a copy to avoid race conditions
	stats := &Statistics{
		TotalRequests:    h.stats.TotalRequests,
		SuccessRequests:  h.stats.SuccessRequests,
		ErrorRequests:    h.stats.ErrorRequests,
		StartTime:        h.stats.StartTime,
		LastRequestTime:  h.stats.LastRequestTime,
		StatusCodeCounts: make(map[int]int64),
		MethodCounts:     make(map[string]int64),
	}
	
	// Copy maps
	for k, v := range h.stats.StatusCodeCounts {
		stats.StatusCodeCounts[k] = v
	}
	for k, v := range h.stats.MethodCounts {
		stats.MethodCounts[k] = v
	}
	
	return stats
}

func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.clients[client]; exists {
		delete(h.clients, client)
		client.close()
		log.Printf("[INFO] WebSocket client removed due to write error. Total: %d", len(h.clients))
	}
}

func (c *Client) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.conn.Close()
		c.closed = true
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgradeConnection(w, r)
	if err != nil {
		http.Error(w, "Could not upgrade connection", http.StatusBadRequest)
		return
	}

	client := &Client{
		conn: conn,
		hub:  h,
	}

	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	log.Printf("[INFO] WebSocket client connected. Total: %d", len(h.clients))

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, client)
			totalClients := len(h.clients)
			h.mu.Unlock()
			client.close()
			log.Printf("[INFO] WebSocket client disconnected. Total: %d", totalClients)
		}()

		buf := make([]byte, 1024)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				break
			}
		}
	}()
}

func (h *Hub) upgradeConnection(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	if r.Header.Get("Upgrade") != "websocket" {
		return nil, fmt.Errorf("not a websocket upgrade")
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, fmt.Errorf("missing Sec-WebSocket-Key")
	}

	acceptKey := computeAcceptKey(key)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("response writer doesn't support hijacking")
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	response := fmt.Sprintf(
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		acceptKey)

	if _, err := bufrw.WriteString(response); err != nil {
		conn.Close()
		return nil, err
	}

	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func (c *Client) sendMessage(message *LogMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal WebSocket message: %v", err)
		return
	}

	frame := createTextFrame(jsonData)
	if _, err := c.conn.Write(frame); err != nil {
		log.Printf("[ERROR] Failed to write to WebSocket connection: %v", err)
		// Mark as closed and remove from hub
		c.closed = true
		go c.hub.removeClient(c)
	}
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + websocketMagicString))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func createTextFrame(payload []byte) []byte {
	payloadLen := len(payload)
	var frame []byte

	frame = append(frame, 0x81)

	if payloadLen < 126 {
		frame = append(frame, byte(payloadLen))
	} else if payloadLen < 65536 {
		frame = append(frame, 126)
		lenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBytes, uint16(payloadLen))
		frame = append(frame, lenBytes...)
	} else {
		frame = append(frame, 127)
		lenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(lenBytes, uint64(payloadLen))
		frame = append(frame, lenBytes...)
	}

	frame = append(frame, payload...)
	return frame
}
