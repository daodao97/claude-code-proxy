.PHONY: build run clean test deps

# 构建可执行文件
build:
	go build -o ccproxy

# 运行服务
run: build
	./ccproxy

# 运行服务并指定配置文件
run-config: build
	./ccproxy -config=config.yaml

# 清理构建文件
clean:
	rm -f ccproxy

# 安装依赖
deps:
	go mod tidy

# 格式化代码
fmt:
	go fmt ./...

# 检查代码
vet:
	go vet ./...

# 运行测试
test: build
	./test.sh

# 交叉编译 Linux 版本
build-linux:
	GOOS=linux GOARCH=amd64 go build -o ccproxy-linux

# 交叉编译 Windows 版本
build-windows:
	GOOS=windows GOARCH=amd64 go build -o ccproxy.exe

# 构建所有平台
build-all: build build-linux build-windows

# 开发模式（实时重载需要安装 air）
dev:
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "请先安装 air: go install github.com/cosmtrek/air@latest"; \
		go run main.go; \
	fi

# 构建托盘应用
build-tray:
	cd tray && make build

# 运行托盘应用
run-tray: build build-tray
	cd tray && make run

# 安装托盘应用
install-tray: build build-tray
	cd tray && make install

# 构建完整套件（主程序 + 托盘应用）
build-suite: build build-tray

# 清理所有构建文件
clean-all: clean
	cd tray && make clean

# 启动托盘应用（带主程序检查）
start-tray: build-suite
	./start-tray.sh