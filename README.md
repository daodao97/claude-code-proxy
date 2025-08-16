# CC Proxy

一个简单高效的 Go 语言 HTTP 代理服务器，用于将接口请求转发到目标服务。

## 功能特性

### 核心功能
- 🎯 **灵活路径匹配**: 支持通配符路径匹配 (`/api/*`)
- 🔄 **HTTP 方法过滤**: 支持指定允许的 HTTP 方法
- 📋 **自定义请求头**: 为转发请求添加自定义头部
- 📊 **实时监控**: WebSocket 实时日志监控界面
- 🔄 **智能重试**: 支持请求失败重试机制
- 📝 **详细日志**: 完整的请求/响应日志记录
- ⚡ **优雅关闭**: 支持优雅关闭和信号处理
- 📄 **YAML 配置**: 简洁的 YAML 配置文件

### 托盘应用 (新增)
- 🎛️ **图形界面**: 系统托盘图形化管理
- 🚀 **一键启停**: 托盘菜单快速启动/停止代理
- 📊 **监控面板**: 一键打开 Web 监控界面
- ⚙️ **配置管理**: 快速编辑配置文件
- 🔄 **自动重启**: 配置变更自动提示重启
- 📱 **系统通知**: 重要事件系统通知
- 🏃 **开机自启**: 支持开机自动启动

## 安装运行

### 方式一：托盘应用（推荐）

1. **克隆项目**：
```bash
git clone https://github.com/daodao97/ccproxy.git
cd ccproxy
```

2. **构建并启动托盘应用**：
```bash
make start-tray
```

3. **使用托盘应用**：
   - 在系统托盘中找到 CC Proxy 图标
   - 右键点击图标访问菜单
   - 点击"启动代理"开始服务
   - 点击"打开监控界面"查看实时日志

### 方式二：命令行方式

1. **克隆项目**：
```bash
git clone https://github.com/daodao97/ccproxy.git
cd ccproxy
```

2. **安装依赖**：
```bash
make deps
```

3. **配置服务**：
编辑 `config.yaml` 文件，配置代理规则

4. **运行服务**：
```bash
make run
```

或指定配置文件：
```bash
go run main.go -config=custom-config.yaml
```

5. **构建可执行文件**：
```bash
make build
./ccproxy
```

### 构建选项

```bash
# 构建主程序
make build

# 构建托盘应用
make build-tray

# 构建完整套件
make build-suite

# 构建所有平台版本
make build-all

# 安装托盘应用到系统
make install-tray
```

## 配置说明

### 服务器配置
- `server.host`: 监听地址（默认: 0.0.0.0）
- `server.port`: 监听端口（默认: 8080）

### 代理规则
- `path`: 匹配路径（支持 * 通配符）
- `target_url`: 目标服务地址
- `methods`: 允许的 HTTP 方法（可选）
- `headers`: 自定义请求头（可选）

### 示例配置
```yaml
server:
  host: "0.0.0.0"
  port: "8080"

proxy:
  targets:
    - path: "/api/v1/*"
      target_url: "http://localhost:3000"
      methods: ["GET", "POST", "PUT", "DELETE"]
      headers:
        X-Forwarded-For: "proxy"
```

## 路径匹配规则

- 精确匹配: `/health` 只匹配 `/health`
- 通配符匹配: `/api/*` 匹配 `/api/users`, `/api/orders` 等
- 请求参数会自动传递到目标服务

## 特性说明

1. **自动请求转发**: 保持原始请求的方法、头部和参数
2. **响应透传**: 完整传递目标服务的响应状态码、头部和内容
3. **错误处理**: 友好的错误响应和日志记录
4. **优雅关闭**: 支持 SIGINT/SIGTERM 信号优雅关闭服务
5. **访问日志**: 记录所有请求的详细信息

## 项目结构

```
ccproxy/
├── main.go              # 程序入口
├── config/              # 配置管理
│   └── config.go
├── server/              # HTTP 服务器
│   └── server.go
├── proxy/               # 代理核心逻辑
│   ├── handler.go
│   └── forward.go
├── middleware/          # 中间件
│   └── logging.go
├── config.yaml          # 配置文件
└── README.md
```