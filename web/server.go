package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"ccproxy/config"
	"ccproxy/websocket"
	
	"gopkg.in/yaml.v2"
)

//go:embed static/*
var staticFiles embed.FS

type WebServer struct {
	hub    *websocket.Hub
	config *config.Config
}

func NewWebServer(hub *websocket.Hub, cfg *config.Config) *WebServer {
	return &WebServer{
		hub:    hub,
		config: cfg,
	}
}

// getConfigFilePath returns the correct config file path based on user home directory
func (w *WebServer) getConfigFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	
	confDir := filepath.Join(home, ".ccproxy")
	confFile := filepath.Join(confDir, "config.yaml")
	
	return confFile, nil
}

func (w *WebServer) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", w.handleIndex)
	mux.HandleFunc("/ws", w.hub.ServeWS)
	mux.HandleFunc("/app.js", w.handleAppJS)
	mux.HandleFunc("/api/config", w.handleConfig)
	mux.HandleFunc("/api/history", w.handleHistory)
	mux.HandleFunc("/api/clear-history", w.handleClearHistory)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))
}

func (w *WebServer) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}

	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Write(data)
}

func (w *WebServer) handleAppJS(writer http.ResponseWriter, request *http.Request) {
	data, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	writer.Write(data)
}

func (w *WebServer) handleConfig(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case "GET":
		w.handleGetConfig(writer, request)
	case "POST":
		w.handleSaveConfig(writer, request)
	default:
		http.Error(writer, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (w *WebServer) handleGetConfig(writer http.ResponseWriter, request *http.Request) {
	// Get the correct config file path
	configFile, err := w.getConfigFilePath()
	if err != nil {
		http.Error(writer, fmt.Sprintf("Failed to get config file path: %v", err), http.StatusInternalServerError)
		return
	}
	
	data, err := os.ReadFile(configFile)
	if err != nil {
		http.Error(writer, fmt.Sprintf("Failed to read config file %s: %v", configFile, err), http.StatusInternalServerError)
		return
	}

	// Always return the raw YAML content
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("X-Config-Path", configFile) // Add header to show which path was used
	writer.Write(data)
}

func (w *WebServer) handleSaveConfig(writer http.ResponseWriter, request *http.Request) {
	// Read the request body
	body, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(writer, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer request.Body.Close()
	
	// Validate YAML syntax
	var yamlData interface{}
	if err := yaml.Unmarshal(body, &yamlData); err != nil {
		http.Error(writer, fmt.Sprintf("Invalid YAML format: %v", err), http.StatusBadRequest)
		return
	}
	
	// Get the correct config file path
	configFile, err := w.getConfigFilePath()
	if err != nil {
		http.Error(writer, fmt.Sprintf("Failed to get config file path: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Ensure config directory exists
	configDir := filepath.Dir(configFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		http.Error(writer, fmt.Sprintf("Failed to create config directory %s: %v", configDir, err), http.StatusInternalServerError)
		return
	}
	
	// Save to config file
	if err := os.WriteFile(configFile, body, 0644); err != nil {
		http.Error(writer, fmt.Sprintf("Failed to save config file to %s: %v", configFile, err), http.StatusInternalServerError)
		return
	}
	
	// Try to reload the configuration
	// Note: In a production environment, you might want to validate the config
	// against your expected schema before applying it
	
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	response := map[string]interface{}{
		"status": "success",
		"message": "Configuration saved successfully",
		"config_path": configFile,
	}
	json.NewEncoder(writer).Encode(response)
}

func (w *WebServer) handleHistory(writer http.ResponseWriter, request *http.Request) {
	if request.Method != "GET" {
		http.Error(writer, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse limit parameter with default value
	limitStr := request.URL.Query().Get("limit")
	limit := 50 // 默认返回50条
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			// 最大限制100条，防止返回过多数据
			if parsedLimit > 100 {
				limit = 100
			} else {
				limit = parsedLimit
			}
		}
	}

	// Get history from the websocket hub
	history, err := w.hub.GetHistory(limit)
	if err != nil {
		http.Error(writer, "Failed to get history", http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(writer).Encode(history); err != nil {
		http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func (w *WebServer) handleClearHistory(writer http.ResponseWriter, request *http.Request) {
	if request.Method != "POST" && request.Method != "DELETE" {
		http.Error(writer, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 清空历史记录
	if err := w.hub.ClearHistory(); err != nil {
		http.Error(writer, "Failed to clear history", http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	response := map[string]interface{}{
		"success": true,
		"message": "History cleared successfully",
	}
	
	if err := json.NewEncoder(writer).Encode(response); err != nil {
		http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}
