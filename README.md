# claude code proxy

基于 golang 开发的通用 请求代理程序, 可以方便的查看 api 请求详情, 在使用 `claude code` 是派生此需求, 所以取名为 `ccproxy`

## 安装

release 下载 [macos](https://github.com/daodao97/claude-code-proxy/releases) | [windows](https://github.com/daodao97/claude-code-proxy/releases)

## macos 使用演示

![](./docs/web_home.png)

![](./docs/web_detail.png)

![](./docs/install.png)

![](./docs/start.png)

![](./docs/web.png)

![](./docs/notify.png)


## 修改配置文件

配置文件, 可以使用托盘按钮打开, 根据实际请求修改 代理配置

`macos` 

```
~/.ccproxy/config.yaml
```

## 配置 cc 环境变量

```
export ANTHROPIC_BASE_URL=http://localhost:9527
export ANTHROPIC_AUTH_TOKEN=aicoding-d0904095b6c795abb6b
```