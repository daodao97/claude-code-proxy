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

	"ccproxy/storage"
	"ccproxy/types"
)

const websocketMagicString = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// 类型别名，保持 API 兼容性
type LogMessage = types.LogMessage
type Statistics = types.Statistics

type Hub struct {
	clients       map[*Client]bool
	broadcast     chan *LogMessage
	mu            sync.RWMutex
	stats         *Statistics
	history       []*LogMessage  // 保留少量内存缓存用于快速访问
	historyMu     sync.RWMutex
	maxHistory    int
	historyStorage *storage.HistoryStorage // 持久化存储
	statsMu        sync.RWMutex          // 统计信息的锁
}

type Client struct {
	conn   net.Conn
	hub    *Hub
	closed bool
	mu     sync.Mutex
}


func NewHub(broadcastSize int, dataDir string) (*Hub, error) {
	// 创建持久化存储
	historyStorage, err := storage.NewHistoryStorage(dataDir, 10, 10000) // 最多10个文件，每个文件10000行
	if err != nil {
		return nil, fmt.Errorf("failed to create history storage: %w", err)
	}

	return &Hub{
		clients:        make(map[*Client]bool),
		broadcast:      make(chan *LogMessage, broadcastSize),
		history:        make([]*LogMessage, 0),
		maxHistory:     20, // 内存中只保留最近20条用于快速访问
		historyStorage: historyStorage,
		stats: &types.Statistics{
			StartTime:        time.Now(),
			StatusCodeCounts: make(map[int]int64),
			MethodCounts:     make(map[string]int64),
		},
	}, nil
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
	
	// Store message in history
	h.addToHistory(message)
	
	select {
	case h.broadcast <- message:
	default:
		log.Println("[WARN] Broadcast channel full, dropping message")
	}
}

func (h *Hub) updateStats(message *LogMessage) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	
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
	h.statsMu.RLock()
	defer h.statsMu.RUnlock()
	
	// Create a copy to avoid race conditions
	stats := &types.Statistics{
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

func (h *Hub) addToHistory(message *LogMessage) {
	// 深拷贝消息以避免后续修改影响历史记录
	messageCopy := &LogMessage{
		Timestamp:            message.Timestamp,
		Method:               message.Method,
		Path:                 message.Path,
		Query:                message.Query,
		RequestHeaders:       make(map[string]string),
		ResponseHeaders:      make(map[string]string),
		RemoteAddr:           message.RemoteAddr,
		StatusCode:           message.StatusCode,
		Duration:             message.Duration,
		TargetURL:            message.TargetURL,
		RequestBody:          message.RequestBody,
		ResponseBody:         message.ResponseBody,
		Error:                message.Error,
		ConnectDuration:      message.ConnectDuration,
		DNSLookupDuration:    message.DNSLookupDuration,
		TLSHandshakeDuration: message.TLSHandshakeDuration,
		FirstByteDuration:    message.FirstByteDuration,
		UpstreamLatency:      message.UpstreamLatency,
		TotalLatency:         message.TotalLatency,
		ConnectionReused:     message.ConnectionReused,
		Stats:                message.Stats,
	}
	
	// 拷贝headers
	for k, v := range message.RequestHeaders {
		messageCopy.RequestHeaders[k] = v
	}
	for k, v := range message.ResponseHeaders {
		messageCopy.ResponseHeaders[k] = v
	}
	
	// 首先保存到持久化存储
	if h.historyStorage != nil {
		if err := h.historyStorage.AppendMessage(messageCopy); err != nil {
			log.Printf("[ERROR] Failed to persist message to disk: %v", err)
		}
	}
	
	// 然后添加到内存缓存
	h.historyMu.Lock()
	defer h.historyMu.Unlock()
	
	h.history = append(h.history, messageCopy)
	
	// 内存中只保留最近的几条记录
	if len(h.history) > h.maxHistory {
		h.history = h.history[1:]
	}
}

func (h *Hub) GetHistory(limit int) ([]*LogMessage, error) {
	// 如果没有持久化存储，返回内存中的记录
	if h.historyStorage == nil {
		h.historyMu.RLock()
		defer h.historyMu.RUnlock()
		
		// 返回历史记录的拷贝，倒序排列（最新的在前）
		history := make([]*LogMessage, len(h.history))
		for i, msg := range h.history {
			history[len(h.history)-1-i] = msg
		}
		return history, nil
	}
	
	// 从持久化存储读取历史记录
	return h.historyStorage.GetRecentMessages(limit)
}

// ClearHistory 清空所有历史记录
func (h *Hub) ClearHistory() error {
	// 清空内存中的历史记录
	h.historyMu.Lock()
	h.history = make([]*LogMessage, 0)
	h.historyMu.Unlock()

	// 如果有持久化存储，也清空磁盘文件
	if h.historyStorage != nil {
		return h.historyStorage.ClearHistory()
	}
	
	return nil
}

func (h *Hub) sendHistoryToClient(client *Client) {
	history, err := h.GetHistory(50) // 向新客户端发送最近50条记录
	if err != nil {
		log.Printf("[ERROR] Failed to get history for new client: %v", err)
		return
	}
	
	for _, message := range history {
		// 给历史消息添加一个标识
		historicalMessage := *message
		// 可以在这里添加一个字段来标识这是历史消息，但为了兼容性，我们先不改变LogMessage结构
		client.sendMessage(&historicalMessage)
	}
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
	clientCount := len(h.clients)
	h.mu.Unlock()

	log.Printf("[INFO] WebSocket client connected. Total: %d", clientCount)

	// 不再自动发送历史消息，由前端通过API获取
	// go h.sendHistoryToClient(client)

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
