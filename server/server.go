package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ccproxy/config"
	"ccproxy/middleware"
	"ccproxy/proxy"
	"ccproxy/web"
	"ccproxy/websocket"
)

type Server struct {
	config    *config.Config
	handler   *proxy.ProxyHandler
	server    *http.Server
	webServer *http.Server
	hub       *websocket.Hub
}

func NewServer(cfg *config.Config) *Server {
	// 创建数据目录
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	hub, err := websocket.NewHub(cfg.WebSocket.BroadcastSize, dataDir)
	if err != nil {
		log.Fatalf("Failed to create websocket hub: %v", err)
	}
	go hub.Run()

	handler := proxy.NewProxyHandler(cfg)
	loggerHandler := middleware.NewLoggerMiddleware(handler, hub, cfg)

	proxyMux := http.NewServeMux()
	proxyMux.Handle("/", loggerHandler)

	server := createHTTPServer(fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port), proxyMux, cfg)

	webMux := http.NewServeMux()
	webServer := web.NewWebServer(hub, cfg)
	webServer.SetupRoutes(webMux)

	webServerInstance := createHTTPServer(fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Web.Port), webMux, cfg)

	return &Server{
		config:    cfg,
		handler:   handler,
		server:    server,
		webServer: webServerInstance,
		hub:       hub,
	}
}

func (s *Server) Start() error {
	log.Printf("Starting proxy server on %s", s.server.Addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start proxy server: %v", err)
		}
	}()

	if s.config.Web.Enabled && s.webServer != nil {
		log.Printf("Starting web interface on %s", s.webServer.Addr)
		go func() {
			if err := s.webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start web server: %v", err)
			}
		}()
	}

	s.waitForShutdown()
	return nil
}

func createHTTPServer(addr string, handler http.Handler, cfg *config.Config) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: handler,
		// Remove read/write timeouts to support long-running streaming requests
		// ReadTimeout:  time.Duration(cfg.Server.Timeouts.Read) * time.Second,
		// WriteTimeout: time.Duration(cfg.Server.Timeouts.Write) * time.Second,
		IdleTimeout: time.Duration(cfg.Server.Timeouts.Idle) * time.Second,
	}
}

func (s *Server) waitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.config.Server.Timeouts.Shutdown)*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("Proxy server forced to shutdown: %v", err)
	} else {
		log.Println("Proxy server gracefully stopped")
	}

	if s.webServer != nil {
		if err := s.webServer.Shutdown(ctx); err != nil {
			log.Printf("Web server forced to shutdown: %v", err)
		} else {
			log.Println("Web server gracefully stopped")
		}
	}
}
