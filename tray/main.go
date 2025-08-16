package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ccproxy/config"
	"ccproxy/middleware"
	"ccproxy/proxy"
	"ccproxy/web"
	"ccproxy/websocket"

	"github.com/daodao97/xgo/xlog"
	"github.com/emersion/go-autostart"
	"github.com/fsnotify/fsnotify"
	notify "github.com/getlantern/notifier"
	"github.com/getlantern/systray"
	"github.com/skratchdot/open-golang/open"
	"gopkg.in/yaml.v2"
)

//go:embed icon/icon.png
var icon []byte

//go:embed icon/icon_off.png
var iconOff []byte

//go:embed icon/icon.ico
var iconWin []byte

//go:embed icon/icon_off.ico
var iconOffWin []byte

var home string
var confDir string
var confFile string

type CCProxy struct {
	config      *config.Config
	proxyServer *http.Server
	webServer   *http.Server
	hub         *websocket.Hub
	ctx         context.Context
	cancel      context.CancelFunc
	Running     bool
}

type AppConfig struct {
	AutoStart  bool   `yaml:"auto_start"`
	StartProxy bool   `yaml:"start_proxy"`
	ConfigFile string `yaml:"config_file"`
}

var ccproxy *CCProxy
var appConfig = &AppConfig{
	ConfigFile: "config.yaml",
}

type Menu struct {
	Title   string
	OnClick func(m *systray.MenuItem)
}

func main() {
	defer func() {
		if ccproxy != nil {
			ccproxy.Stop()
		}
	}()

	home, _ = os.UserHomeDir()
	confDir = filepath.Join(home, ".ccproxy")
	confFile = filepath.Join(confDir, "config.yaml")

	// 确保配置目录存在
	if err := os.MkdirAll(confDir, 0755); err != nil {
		log.Fatal("Failed to create config directory:", err)
	}

	loadAppConfig()
	ccproxy = &CCProxy{}

	systray.Run(onReady, onExit)
}

func onReady() {
	_icon := icon
	_iconOff := iconOff

	if runtime.GOOS == "windows" {
		_icon = iconWin
		_iconOff = iconOffWin
	}

	systray.SetTemplateIcon(_iconOff, _iconOff)
	systray.SetTooltip("CC Proxy - HTTP代理服务器")

	var restartMenu *systray.MenuItem

	startProxy := func(m *systray.MenuItem) {
		err := ccproxy.Start()
		if err != nil {
			showNotification("CC Proxy 启动失败", err.Error())
			return
		}
		m.SetTitle("停止代理")
		systray.SetTemplateIcon(_icon, _icon)
		if restartMenu != nil {
			restartMenu.Show()
		}
	}

	stopProxy := func(m *systray.MenuItem) {
		ccproxy.Stop()
		m.SetTitle("启动代理")
		systray.SetTemplateIcon(_iconOff, _iconOff)
		if restartMenu != nil {
			restartMenu.Hide()
		}
	}

	// 主代理控制菜单
	proxyMenu := addMenu(&Menu{
		Title: "启动代理",
		OnClick: func(m *systray.MenuItem) {
			m.Disable()
			if ccproxy.Running {
				stopProxy(m)
			} else {
				startProxy(m)
			}
			m.Enable()
		},
	})

	// 如果配置了自动启动代理
	if appConfig.StartProxy {
		proxyMenu.Disable()
		startProxy(proxyMenu)
		proxyMenu.Enable()
	}

	// 重启代理菜单
	restartProxy := func() {
		stopProxy(proxyMenu)
		startProxy(proxyMenu)
	}

	restartMenu = addMenu(&Menu{
		Title: "重启代理",
		OnClick: func(m *systray.MenuItem) {
			if ccproxy.Running {
				restartProxy()
			} else {
				startProxy(proxyMenu)
			}
		},
	})
	restartMenu.Hide()

	// 添加分隔符
	systray.AddSeparator()

	// 打开 Web 界面
	addMenu(&Menu{
		Title: "打开监控界面",
		OnClick: func(m *systray.MenuItem) {
			cfg, err := loadProxyConfig()
			if err != nil {
				showNotification("配置加载失败", err.Error())
				return
			}
			webPort := "8081"
			if cfg.Web.Enabled && cfg.Web.Port != "" {
				webPort = cfg.Web.Port
			}
			url := fmt.Sprintf("http://localhost:%s", webPort)
			if err := open.Run(url); err != nil {
				showNotification("打开失败", "无法打开监控界面")
			}
		},
	})

	// 编辑配置文件
	addMenu(&Menu{
		Title: "编辑配置",
		OnClick: func(m *systray.MenuItem) {
			err := open.RunWith(confFile, "Visual Studio Code")
			if err != nil {
				if err := open.Run(confDir); err != nil {
					showNotification("打开失败", "无法打开配置目录")
				}
			}
		},
	})

	systray.AddSeparator()

	// 开机自启动
	addCheckboxMenu(&Menu{
		Title: "开机自启动",
		OnClick: func(m *systray.MenuItem) {
			app := &autostart.App{
				Name:        "ccproxy-tray",
				DisplayName: "CC Proxy",
				Exec:        []string{os.Args[0]},
			}

			if !m.Checked() {
				m.Check()
				if err := app.Enable(); err != nil {
					log.Printf("启用开机自启动失败: %v", err)
					m.Uncheck()
					return
				}
			} else {
				m.Uncheck()
				if err := app.Disable(); err != nil {
					log.Printf("禁用开机自启动失败: %v", err)
					m.Check()
					return
				}
			}

			appConfig.AutoStart = m.Checked()
			saveAppConfig()
		},
	}, appConfig.AutoStart)

	// 启动时启动代理
	addCheckboxMenu(&Menu{
		Title: "启动时启动代理",
		OnClick: func(m *systray.MenuItem) {
			if !m.Checked() {
				m.Check()
			} else {
				m.Uncheck()
			}
			appConfig.StartProxy = m.Checked()
			saveAppConfig()
		},
	}, appConfig.StartProxy)

	systray.AddSeparator()

	// 关于菜单
	addMenu(&Menu{
		Title: "关于 CC Proxy",
		OnClick: func(m *systray.MenuItem) {
			_ = open.Run("https://github.com/daodao97/claude-code-proxy")
		},
	})

	// 退出菜单
	addMenu(&Menu{
		Title: "退出",
		OnClick: func(m *systray.MenuItem) {
			ccproxy.Stop()
			systray.Quit()
		},
	})

	// 启动配置文件监控
	go watchConfigFile(restartProxy)
}

func onExit() {
	fmt.Println("CC Proxy 托盘应用退出")
}

func (cp *CCProxy) Start() error {
	if cp.Running {
		xlog.Info("CC Proxy 已启动")
		return nil
	}

	// 加载配置
	cfg, err := config.LoadConfig(confFile)
	if err != nil {
		xlog.Error("加载配置失败", xlog.Err(err))
		return fmt.Errorf("加载配置失败: %v", err)
	}

	cp.config = cfg
	cp.ctx, cp.cancel = context.WithCancel(context.Background())

	// 创建 WebSocket Hub
	cp.hub = websocket.NewHub(cfg.WebSocket.BroadcastSize)
	go cp.hub.Run()

	// 创建代理处理器
	handler := proxy.NewProxyHandler(cfg)
	loggerHandler := middleware.NewLoggerMiddleware(handler, cp.hub, cfg)

	// 创建代理服务器
	proxyMux := http.NewServeMux()
	proxyMux.Handle("/", loggerHandler)

	cp.proxyServer = &http.Server{
		Addr:        fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:     proxyMux,
		IdleTimeout: time.Duration(cfg.Server.Timeouts.Idle) * time.Second,
	}

	// 用于检测启动状态的通道
	proxyStarted := make(chan error, 1)
	webStarted := make(chan error, 1)

	// 启动代理服务器
	go func() {
		xlog.Info("启动代理服务器", xlog.String("addr", cp.proxyServer.Addr))
		if err := cp.proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			xlog.Error("代理服务器启动失败", xlog.Err(err))
			proxyStarted <- err
			return
		}
	}()

	// 检测代理服务器是否启动成功
	go func() {
		time.Sleep(200 * time.Millisecond) // 等待启动
		if _, err := http.Get(fmt.Sprintf("http://%s:%s/health", cfg.Server.Host, cfg.Server.Port)); err == nil {
			proxyStarted <- nil // 启动成功
		} else {
			// 如果没有 health 端点，尝试连接端口
			conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port))
			if err != nil {
				proxyStarted <- err
			} else {
				conn.Close()
				proxyStarted <- nil
			}
		}
	}()

	// 如果启用了 Web 界面，创建 Web 服务器
	var webServerEnabled bool
	if cfg.Web.Enabled {
		webServerEnabled = true
		webMux := http.NewServeMux()
		webServer := web.NewWebServer(cp.hub, cfg)
		webServer.SetupRoutes(webMux)

		cp.webServer = &http.Server{
			Addr:        fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Web.Port),
			Handler:     webMux,
			IdleTimeout: time.Duration(cfg.Server.Timeouts.Idle) * time.Second,
		}

		// 启动 Web 服务器
		go func() {
			xlog.Info("启动Web服务器", xlog.String("addr", cp.webServer.Addr))
			if err := cp.webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				xlog.Error("Web服务器启动失败", xlog.Err(err))
				webStarted <- err
				return
			}
		}()

		// 检测 Web 服务器是否启动成功
		go func() {
			time.Sleep(200 * time.Millisecond)
			if _, err := http.Get(fmt.Sprintf("http://%s:%s/", cfg.Server.Host, cfg.Web.Port)); err != nil {
				webStarted <- err
			} else {
				webStarted <- nil
			}
		}()
	} else {
		webStarted <- nil // Web 服务器未启用，标记为成功
	}

	// 等待启动结果
	var startupErrors []string

	// 检查代理服务器启动结果
	if err := <-proxyStarted; err != nil {
		startupErrors = append(startupErrors, fmt.Sprintf("代理服务器启动失败: %v", err))
	}

	// 检查 Web 服务器启动结果
	if webServerEnabled {
		if err := <-webStarted; err != nil {
			startupErrors = append(startupErrors, fmt.Sprintf("Web服务器启动失败: %v", err))
		}
	}

	// 如果有启动错误，清理并返回错误
	if len(startupErrors) > 0 {
		cp.cleanup()
		errorMsg := strings.Join(startupErrors, "; ")
		showNotification("CC Proxy 启动失败", errorMsg)
		return fmt.Errorf(errorMsg)
	}

	// 所有服务器都启动成功
	cp.Running = true
	xlog.Info("CC Proxy 已启动", xlog.String("host", cfg.Server.Host), xlog.String("port", cfg.Server.Port))
	showNotification("CC Proxy 已启动", fmt.Sprintf("代理服务器运行在 http://%s:%s", cfg.Server.Host, cfg.Server.Port))

	return nil
}

func (cp *CCProxy) Stop() {
	if !cp.Running {
		return
	}

	xlog.Info("正在停止 CC Proxy...")

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 停止代理服务器
	if cp.proxyServer != nil {
		if err := cp.proxyServer.Shutdown(ctx); err != nil {
			xlog.Error("停止代理服务器失败", xlog.Err(err))
		} else {
			xlog.Info("代理服务器已停止")
		}
	}

	// 停止 Web 服务器
	if cp.webServer != nil {
		if err := cp.webServer.Shutdown(ctx); err != nil {
			xlog.Error("停止Web服务器失败", xlog.Err(err))
		} else {
			xlog.Info("Web服务器已停止")
		}
	}

	// 取消上下文
	if cp.cancel != nil {
		cp.cancel()
	}

	cp.Running = false
	cp.proxyServer = nil
	cp.webServer = nil
	cp.hub = nil
	cp.cancel = nil

	showNotification("CC Proxy 已停止", "代理服务器已停止运行")
}

func (cp *CCProxy) cleanup() {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 清理代理服务器
	if cp.proxyServer != nil {
		cp.proxyServer.Shutdown(ctx)
		cp.proxyServer = nil
	}

	// 清理 Web 服务器
	if cp.webServer != nil {
		cp.webServer.Shutdown(ctx)
		cp.webServer = nil
	}

	// 取消上下文
	if cp.cancel != nil {
		cp.cancel()
		cp.cancel = nil
	}

	cp.hub = nil
	cp.Running = false
}

func addMenu(menu *Menu) *systray.MenuItem {
	item := systray.AddMenuItem(menu.Title, menu.Title)
	if menu.OnClick != nil {
		go func() {
			for {
				<-item.ClickedCh
				menu.OnClick(item)
			}
		}()
	}
	return item
}

func addCheckboxMenu(menu *Menu, checked bool) *systray.MenuItem {
	item := addMenu(menu)
	if checked {
		item.Check()
	}
	return item
}

func loadProxyConfig() (*config.Config, error) {
	return config.LoadConfig(confFile)
}

func loadAppConfig() {
	appConfigFile := filepath.Join(confDir, "app.yaml")
	data, err := os.ReadFile(appConfigFile)
	if err != nil {
		// 如果文件不存在，使用默认配置
		return
	}

	if err := yaml.Unmarshal(data, appConfig); err != nil {
		log.Printf("解析应用配置失败: %v", err)
	}
}

func saveAppConfig() {
	appConfigFile := filepath.Join(confDir, "app.yaml")
	data, err := yaml.Marshal(appConfig)
	if err != nil {
		log.Printf("序列化应用配置失败: %v", err)
		return
	}

	if err := os.WriteFile(appConfigFile, data, 0644); err != nil {
		log.Printf("保存应用配置失败: %v", err)
	}
}

func showNotification(title, message string) {
	n := notify.NewNotifications()
	err := n.Notify(&notify.Notification{
		Title:   title,
		Message: message,
		Sender:  "com.ccproxy.tray",
	})
	if err != nil {
		log.Printf("显示通知失败: %v", err)
	}
}

func watchConfigFile(restartCallback func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("创建文件监控失败: %v", err)
		return
	}
	defer watcher.Close()

	// 检查配置文件是否存在，如果不存在则创建默认配置
	if _, err := os.Stat(confFile); os.IsNotExist(err) {
		log.Printf("配置文件不存在，创建默认配置: %s", confFile)
		if err := createDefaultConfig(); err != nil {
			log.Printf("创建默认配置失败: %v", err)
			return
		}
	}

	if err := watcher.Add(confFile); err != nil {
		log.Printf("监控配置文件失败: %v", err)
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if strings.Contains(event.String(), "WRITE") && ccproxy.Running {
				showNotification("配置文件已更改", "点击通知重启代理服务")
				// 可以在这里添加自动重启逻辑
				// restartCallback()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("文件监控错误: %v", err)
		}
	}
}

func createDefaultConfig() error {
	defaultConfig := `server:
  host: "0.0.0.0"
  port: "9527"

web:
  port: "9528"
  enabled: true
  max_logs: 1000  # Maximum logs to keep in web interface

proxy:
  timeout: 30           # Proxy request timeout in seconds
  max_retries: 3        # Maximum number of retry attempts
  retry_delay: 1000     # Delay between retries in milliseconds
  targets:
    - path: "/v1/*"
      target_url: "https://api.aicoding.sh"
      methods: ["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "CONNECT", "TRACE", "ANY"]
      headers:
        X-Forwarded-For: "proxy"

websocket:
  buffer_size: 1024     # WebSocket read buffer size in bytes
  broadcast_size: 1000  # WebSocket broadcast channel buffer size

logging:
  level: "info"
  file: ""`

	return os.WriteFile(confFile, []byte(defaultConfig), 0644)
}
