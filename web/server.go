package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"

	"ccproxy/config"
	"ccproxy/websocket"
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

func (w *WebServer) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", w.handleIndex)
	mux.HandleFunc("/ws", w.hub.ServeWS)
	mux.HandleFunc("/app.js", w.handleAppJS)
	mux.HandleFunc("/api/config", w.handleConfig)
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
	if request.Method != "GET" {
		http.Error(writer, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try to read the original YAML config file
	configFile := "config.yaml"
	data, err := os.ReadFile(configFile)
	if err != nil {
		// If we can't read the file, fallback to JSON encoding of the config struct
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(writer).Encode(w.config); err != nil {
			http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		return
	}

	// Return the raw YAML content
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Write(data)
}
