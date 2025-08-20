package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ccproxy/types"
)

// HistoryStorage 历史记录存储结构
type HistoryStorage struct {
	filePath string
	mu       sync.RWMutex
	maxFiles int
	maxLines int
}

// NewHistoryStorage 创建新的历史记录存储
func NewHistoryStorage(dataDir string, maxFiles, maxLines int) (*HistoryStorage, error) {
	// 确保数据目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// 使用当前日期作为文件名
	filename := fmt.Sprintf("history_%s.jsonl", time.Now().Format("2006-01-02"))
	filePath := filepath.Join(dataDir, filename)

	return &HistoryStorage{
		filePath: filePath,
		maxFiles: maxFiles,
		maxLines: maxLines,
	}, nil
}

// AppendMessage 追加消息到历史记录
func (h *HistoryStorage) AppendMessage(msg *types.LogMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查是否需要轮转文件
	if err := h.rotateFileIfNeeded(); err != nil {
		return fmt.Errorf("failed to rotate file: %w", err)
	}

	// 打开文件进行追加
	file, err := os.OpenFile(h.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	// 将消息序列化为JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 写入一行JSON数据
	if _, err := fmt.Fprintf(file, "%s\n", data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// GetRecentMessages 获取最近的消息
func (h *HistoryStorage) GetRecentMessages(limit int) ([]*types.LogMessage, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var messages []*types.LogMessage

	// 首先尝试从当前文件读取
	currentMessages, err := h.readMessagesFromFile(h.filePath, limit)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read current file: %w", err)
	}
	messages = append(messages, currentMessages...)

	// 如果当前文件的消息不够，从历史文件中读取
	if len(messages) < limit {
		remaining := limit - len(messages)
		historyMessages, err := h.readFromHistoryFiles(remaining)
		if err != nil {
			return nil, fmt.Errorf("failed to read history files: %w", err)
		}
		messages = append(messages, historyMessages...)
	}

	// 如果消息数量超过限制，截取最近的
	if len(messages) > limit {
		messages = messages[:limit]
	}

	return messages, nil
}

// readMessagesFromFile 从单个文件读取消息
func (h *HistoryStorage) readMessagesFromFile(filePath string, limit int) ([]*types.LogMessage, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []*types.LogMessage
	scanner := bufio.NewScanner(file)
	
	// 增加缓冲区大小以处理大的JSON行
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 设置最大token大小为1MB

	// 读取最后1000行（限制内存使用）
	var lines []string
	lineCount := 0
	tempLines := make([]string, 1000) // 循环缓冲区
	
	for scanner.Scan() {
		tempLines[lineCount%1000] = scanner.Text()
		lineCount++
	}
	
	// 如果总行数小于等于1000，直接使用
	if lineCount <= 1000 {
		lines = tempLines[:lineCount]
	} else {
		// 重新排列获取最后1000行
		start := lineCount % 1000
		lines = make([]string, 1000)
		for i := 0; i < 1000; i++ {
			lines[i] = tempLines[(start+i)%1000]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan file: %w", err)
	}

	// 从最后开始读取（最新的消息）
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}

	for i := len(lines) - 1; i >= start; i-- {
		var msg types.LogMessage
		if err := json.Unmarshal([]byte(lines[i]), &msg); err != nil {
			// 跳过无法解析的行
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

// readFromHistoryFiles 从历史文件中读取消息
func (h *HistoryStorage) readFromHistoryFiles(limit int) ([]*types.LogMessage, error) {
	dataDir := filepath.Dir(h.filePath)
	
	// 获取所有历史文件
	files, err := filepath.Glob(filepath.Join(dataDir, "history_*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob history files: %w", err)
	}

	var messages []*types.LogMessage

	// 按文件名倒序处理（最新的文件优先）
	for i := len(files) - 1; i >= 0 && len(messages) < limit; i-- {
		if files[i] == h.filePath {
			continue // 跳过当前文件
		}

		remaining := limit - len(messages)
		fileMessages, err := h.readMessagesFromFile(files[i], remaining)
		if err != nil {
			continue // 跳过有问题的文件
		}
		messages = append(messages, fileMessages...)
	}

	return messages, nil
}

// rotateFileIfNeeded 检查是否需要轮转文件
func (h *HistoryStorage) rotateFileIfNeeded() error {
	// 检查当前文件是否存在以及行数
	lineCount, err := h.countLines(h.filePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// 如果文件不存在或行数未超过限制，不需要轮转
	if os.IsNotExist(err) || lineCount < h.maxLines {
		return nil
	}

	// 需要轮转到新文件
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	dataDir := filepath.Dir(h.filePath)
	newFilePath := filepath.Join(dataDir, fmt.Sprintf("history_%s.jsonl", timestamp))
	h.filePath = newFilePath

	// 清理旧文件
	return h.cleanupOldFiles()
}

// countLines 计算文件行数
func (h *HistoryStorage) countLines(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	
	// 增加缓冲区大小以处理大的JSON行
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 设置最大token大小为1MB
	
	for scanner.Scan() {
		count++
	}

	return count, scanner.Err()
}

// cleanupOldFiles 清理过多的历史文件
func (h *HistoryStorage) cleanupOldFiles() error {
	dataDir := filepath.Dir(h.filePath)
	
	files, err := filepath.Glob(filepath.Join(dataDir, "history_*.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to glob history files: %w", err)
	}

	// 如果文件数量超过限制，删除最老的文件
	if len(files) > h.maxFiles {
		// 按文件名排序（时间顺序）
		for i := 0; i < len(files)-(h.maxFiles); i++ {
			if err := os.Remove(files[i]); err != nil {
				// 记录错误但继续处理
				fmt.Printf("Warning: failed to remove old history file %s: %v\n", files[i], err)
			}
		}
	}

	return nil
}

// ClearHistory 清空所有历史记录
func (h *HistoryStorage) ClearHistory() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	dataDir := filepath.Dir(h.filePath)
	
	// 获取所有历史文件
	files, err := filepath.Glob(filepath.Join(dataDir, "history_*.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to glob history files: %w", err)
	}

	// 删除所有历史文件
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			// 如果文件不存在，跳过错误
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove history file %s: %w", file, err)
			}
		}
	}

	// 更新当前文件路径，使用新的时间戳
	filename := fmt.Sprintf("history_%s.jsonl", time.Now().Format("2006-01-02"))
	h.filePath = filepath.Join(dataDir, filename)

	return nil
}