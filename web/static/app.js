class LogViewer {
    constructor() {
        this.ws = null;
        this.logs = [];
        this.isPaused = false;
        this.autoScroll = true;
        this.maxLogs = 1000;
        this.wasConnected = false;
        this.lastLogTime = 0;
        this.config = null;
        this.latencySum = 0;
        this.latencyCount = 0;
        
        // Config editor state
        this.isEditingConfig = false;
        this.originalConfigYaml = '';

        this.initElements();
        this.bindEvents();
        this.loadConfig();
        this.loadHistory();
        this.connect();
        this.initKeyboardShortcuts();
    }

    initElements() {
        this.connectionStatus = document.getElementById('connectionStatus');
        this.connectionText = document.getElementById('connectionText');
        this.logsContainer = document.getElementById('logsContainer');
        this.clearBtn = document.getElementById('clearBtn');
        this.pauseBtn = document.getElementById('pauseBtn');
        this.autoScrollBtn = document.getElementById('autoScrollBtn');
        this.modal = document.getElementById('logModal');
        this.modalBody = document.getElementById('modalBody');
        this.closeModal = document.getElementById('closeModal');
        
        // Statistics elements
        this.totalRequestsEl = document.getElementById('totalRequests');
        this.successRequestsEl = document.getElementById('successRequests');
        this.errorRequestsEl = document.getElementById('errorRequests');
        this.uptimeEl = document.getElementById('uptime');
        this.avgLatencyEl = document.getElementById('avgLatency');
        
        // Config elements
        this.proxyAddressEl = document.getElementById('proxyAddress');
        this.configBtn = document.getElementById('configBtn');
        this.configModal = document.getElementById('configModal');
        this.configModalBody = document.getElementById('configModalBody');
        this.closeConfigModal = document.getElementById('closeConfigModal');
        this.editConfigBtn = document.getElementById('editConfigBtn');
        this.saveConfigBtn = document.getElementById('saveConfigBtn');
        this.cancelEditBtn = document.getElementById('cancelEditBtn');
    }

    bindEvents() {
        this.clearBtn.addEventListener('click', () => this.clearLogs());
        this.pauseBtn.addEventListener('click', () => this.togglePause());
        this.autoScrollBtn.addEventListener('click', () => this.toggleAutoScroll());
        
        window.addEventListener('beforeunload', () => {
            if (this.ws) {
                this.ws.close();
            }
        });

        // Modal events
        this.closeModal.addEventListener('click', () => this.hideModal());
        this.modal.addEventListener('click', (e) => {
            if (e.target === this.modal) {
                this.hideModal();
            }
        });
        
        // Config modal events
        this.configBtn.addEventListener('click', () => this.showConfigModal());
        this.closeConfigModal.addEventListener('click', () => this.hideConfigModal());
        this.configModal.addEventListener('click', (e) => {
            if (e.target === this.configModal) {
                this.hideConfigModal();
            }
        });
        
        // Config edit events
        this.editConfigBtn.addEventListener('click', () => this.enableConfigEdit());
        this.saveConfigBtn.addEventListener('click', () => this.saveConfig());
        this.cancelEditBtn.addEventListener('click', () => this.cancelConfigEdit());
        
        // ESC key to close modal
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                if (this.modal.classList.contains('show')) {
                    this.hideModal();
                } else if (this.configModal.classList.contains('show')) {
                    this.hideConfigModal();
                }
            }
        });
    }

    async loadConfig() {
        try {
            const response = await fetch('/api/config');
            if (response.ok) {
                const contentType = response.headers.get('content-type');
                if (contentType && contentType.includes('application/json')) {
                    // Handle JSON response (fallback)
                    this.config = await response.json();
                } else {
                    // Handle YAML/text response
                    this.configYaml = await response.text();
                    // For proxy address, we still need some basic config info
                    // Try to parse basic server info from YAML
                    this.parseBasicConfigFromYaml(this.configYaml);
                }
                this.updateProxyAddress();
            } else {
                this.proxyAddressEl.textContent = '配置加载失败';
            }
        } catch (error) {
            console.error('Failed to load config:', error);
            this.proxyAddressEl.textContent = '配置加载失败';
        }
    }

    parseBasicConfigFromYaml(yamlText) {
        // Simple YAML parsing for basic server info
        const lines = yamlText.split('\n');
        const config = { server: {} };
        
        let currentSection = null;
        for (const line of lines) {
            const trimmed = line.trim();
            if (trimmed.startsWith('server:')) {
                currentSection = 'server';
            } else if (currentSection === 'server') {
                if (trimmed.startsWith('host:')) {
                    config.server.host = trimmed.split(':')[1].trim().replace(/"/g, '');
                } else if (trimmed.startsWith('port:')) {
                    config.server.port = trimmed.split(':')[1].trim().replace(/"/g, '');
                } else if (!trimmed.startsWith(' ') && trimmed.includes(':')) {
                    currentSection = null;
                }
            }
        }
        
        this.config = { Server: config.server };
    }

    updateProxyAddress() {
        if (this.config && this.config.Server) {
            const protocol = window.location.protocol === 'https:' ? 'https' : 'http';
            const host = this.config.Server.Host === '0.0.0.0' ? window.location.hostname : this.config.Server.Host;
            const port = this.config.Server.Port;
            this.proxyAddressEl.textContent = `${protocol}://${host}:${port}`;
        } else {
            this.proxyAddressEl.textContent = '配置未加载';
        }
    }

    async loadHistory(limit = 50) {
        try {
            const response = await fetch(`/api/history?limit=${limit}`);
            if (response.ok) {
                const history = await response.json();
                console.log(`加载了 ${history.length} 条历史消息`);
                
                // 按时间顺序添加历史消息（后端已返回倒序，最新的在前）
                for (const logData of history) {
                    this.addLog(logData, true); // 第二个参数表示这是历史消息
                    if (logData.stats) {
                        this.updateStats(logData.stats);
                    }
                }
                
                // 如果有历史消息，移除空状态
                if (history.length > 0) {
                    const emptyState = this.logsContainer.querySelector('.empty-state');
                    if (emptyState) {
                        emptyState.remove();
                    }
                }
            } else {
                console.warn('无法加载历史消息:', response.status);
            }
        } catch (error) {
            console.error('加载历史消息失败:', error);
        }
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.updateConnectionStatus(true);
        };

        this.ws.onmessage = (event) => {
            if (!this.isPaused) {
                const logData = JSON.parse(event.data);
                this.addLog(logData);
                this.updateStats(logData.stats);
            }
        };

        this.ws.onclose = () => {
            this.updateConnectionStatus(false);
            setTimeout(() => this.connect(), 3000);
        };

        this.ws.onerror = () => {
            this.updateConnectionStatus(false);
        };
    }

    updateConnectionStatus(connected) {
        if (connected) {
            this.connectionStatus.classList.add('connected');
            this.connectionText.textContent = '✅ 已连接';
            if (!this.wasConnected) {
                this.showNotification('连接成功', 'success');
                this.wasConnected = true;
            }
        } else {
            this.connectionStatus.classList.remove('connected');
            this.connectionText.textContent = '❌ 连接断开';
            if (this.wasConnected) {
                this.showNotification('连接断开，正在重连...', 'error');
                this.wasConnected = false;
            }
        }
    }

    addLog(logData, isHistorical = false) {
        if (isHistorical) {
            // 对于历史消息，直接按顺序添加到末尾（后端已排序）
            this.logs.push(logData);
        } else {
            // 对于新消息，添加到开头
            this.logs.unshift(logData);
        }
        
        // Track latency for average calculation
        this.trackLatency(logData);
        
        if (this.logs.length > this.maxLogs) {
            // 如果超过限制，保留最新的消息
            this.logs = this.logs.slice(0, this.maxLogs);
        }

        this.renderLogs();
        
        // 只为新消息显示通知
        if (!isHistorical) {
            this.showNewLogNotification();
        }
    }

    renderLogs() {
        if (this.logs.length === 0) {
            this.logsContainer.innerHTML = `
                <div class="empty-state">
                    <h3>暂无日志数据</h3>
                    <p>当有请求通过代理服务器时，日志将在这里显示</p>
                </div>
            `;
            return;
        }

        const logsHtml = this.logs.map(log => this.renderLogEntry(log)).join('');
        this.logsContainer.innerHTML = logsHtml;

        if (this.autoScroll) {
            this.logsContainer.scrollTop = 0;
        }

        this.bindLogEvents();
    }

    updateStats(stats) {
        if (!stats) return;
        
        this.totalRequestsEl.textContent = stats.total_requests.toLocaleString();
        this.successRequestsEl.textContent = stats.success_requests.toLocaleString();
        this.errorRequestsEl.textContent = stats.error_requests.toLocaleString();
        
        // Calculate and display uptime
        if (stats.start_time) {
            const startTime = new Date(stats.start_time);
            const now = new Date();
            const uptimeMs = now - startTime;
            const uptimeStr = this.formatUptime(uptimeMs);
            this.uptimeEl.textContent = uptimeStr;
        }
    }

    formatUptime(ms) {
        const seconds = Math.floor(ms / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        const days = Math.floor(hours / 24);
        
        if (days > 0) {
            return `${days}d ${hours % 24}h ${minutes % 60}m`;
        } else if (hours > 0) {
            return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
        } else if (minutes > 0) {
            return `${minutes}m ${seconds % 60}s`;
        } else {
            return `${seconds}s`;
        }
    }

    renderLogEntry(log) {
        const statusClass = this.getStatusClass(log.status_code);
        const methodClass = log.method.toLowerCase();
        const isStreaming = this.isStreamingResponse(log);
        
        // Format connection info
        const connectionInfo = this.formatConnectionInfo(log);
        
        // Debug log to check target_url
        console.log('Log entry:', {
            path: log.path,
            target_url: log.target_url,
            method: log.method
        });
        
        return `
            <div class="log-entry" data-log-id="${Date.now()}-${Math.random()}" style="animation-delay: 0.1s">
                <div class="log-header">
                    <span class="method ${methodClass}">${log.method}</span>
                    <span class="status-code ${statusClass}">${log.status_code}</span>
                    <span class="streaming-badge ${isStreaming ? 'streaming' : 'non-streaming'}">${isStreaming ? '✨ 流式' : '📄 非流'}</span>
                    ${this.formatRoutingInfo(log)}
                    <span class="duration">⏱️ ${log.duration}</span>
                    ${connectionInfo}
                    <span class="timestamp">🕰️ ${log.timestamp}</span>
                    <div class="log-actions">
                        <button class="btn btn-sm copy-json" title="复制JSON">📋</button>
                        <button class="btn btn-sm show-details">🔍 详情</button>
                    </div>
                </div>
            </div>
        `;
    }

    renderLogDetails(log) {
        let details = '';

        // Connection Metrics section (new)
        if (this.hasConnectionMetrics(log)) {
            details += `
                <div class="detail-section">
                    <div class="detail-title" data-section="connection-metrics">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>🔗 连接指标</span>
                        </div>
                        <button class="copy-section-btn" data-copy-type="connection-metrics">📋 复制</button>
                    </div>
                    <div class="detail-content" data-section-content="connection-metrics">${this.formatConnectionMetricsDetails(log)}</div>
                </div>
            `;
        }

        if (log.target_url) {
            details += `
                <div class="detail-section">
                    <div class="detail-title" data-section="target-url">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>🎯 真实目标地址</span>
                        </div>
                        <button class="copy-section-btn" data-copy-type="target-url">📋 复制</button>
                    </div>
                    <div class="detail-content" data-section-content="target-url">${log.target_url}</div>
                </div>
            `;
        }

        if (log.remote_addr) {
            details += `
                <div class="detail-section">
                    <div class="detail-title" data-section="remote-addr">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>🌐 客户端地址</span>
                        </div>
                        <button class="copy-section-btn" data-copy-type="remote-addr">📋 复制</button>
                    </div>
                    <div class="detail-content" data-section-content="remote-addr">${log.remote_addr}</div>
                </div>
            `;
        }

        if (log.request_headers && Object.keys(log.request_headers).length > 0) {
            details += `
                <div class="detail-section">
                    <div class="detail-title" data-section="request-headers">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>📤 请求头</span>
                        </div>
                        <button class="copy-section-btn" data-copy-type="request-headers">📋 复制</button>
                    </div>
                    <div class="detail-content" data-section-content="request-headers">${JSON.stringify(log.request_headers, null, 2)}</div>
                </div>
            `;
        }

        if (log.response_headers && Object.keys(log.response_headers).length > 0) {
            details += `
                <div class="detail-section">
                    <div class="detail-title" data-section="response-headers">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>📥 响应头</span>
                        </div>
                        <button class="copy-section-btn" data-copy-type="response-headers">📋 复制</button>
                    </div>
                    <div class="detail-content" data-section-content="response-headers">${JSON.stringify(log.response_headers, null, 2)}</div>
                </div>
            `;
        }

        if (log.request_body) {
            details += `
                <div class="detail-section">
                    <div class="detail-title" data-section="request-body">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>📝 请求体</span>
                        </div>
                        <button class="copy-section-btn" data-copy-type="request-body">📋 复制</button>
                    </div>
                    <div class="detail-content" data-section-content="request-body">${this.escapeHtml(log.request_body)}</div>
                </div>
            `;
        }

        if (log.response_body) {
            const isBinary = log.response_body.startsWith('[BINARY DATA');
            const isStreamingLog = this.isStreamingResponse(log);
            
            details += `
                <div class="detail-section">
                    <div class="detail-title" data-section="response-body">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>📄 响应体</span>
                        </div>
                        <div style="display: flex; gap: 0.5rem; align-items: center;">
                            <button class="copy-section-btn" data-copy-type="response-body">📋 复制</button>
                            ${isStreamingLog ? '<button class="aggregate-stream-btn" data-log-id="stream-toggle">🔗 聚合流式响应</button>' : ''}
                        </div>
                    </div>
                    <div class="detail-content${isBinary ? ' binary-data' : ''}" data-section-content="response-body" data-original-content="${this.escapeHtml(log.response_body)}">${this.escapeHtml(log.response_body)}</div>
                </div>
            `;
        }

        if (log.error) {
            details += `
                <div class="detail-section">
                    <div class="detail-title" data-section="error">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>❌ 错误信息</span>
                        </div>
                        <button class="copy-section-btn" data-copy-type="error">📋 复制</button>
                    </div>
                    <div class="detail-content" data-section-content="error" style="color: #e74c3c;">${log.error}</div>
                </div>
            `;
        }

        return details;
    }

    bindLogEvents() {
        const detailsBtns = this.logsContainer.querySelectorAll('.show-details');
        detailsBtns.forEach((btn, index) => {
            btn.addEventListener('click', (e) => {
                e.preventDefault();
                this.showModal(this.logs[index]);
            });
        });

        const copyBtns = this.logsContainer.querySelectorAll('.copy-json');
        copyBtns.forEach((btn, index) => {
            btn.addEventListener('click', (e) => {
                e.preventDefault();
                this.copyLogAsJSON(this.logs[index]);
            });
        });
    }

    getStatusClass(statusCode) {
        if (statusCode >= 200 && statusCode < 300) return 'success';
        if (statusCode >= 400 && statusCode < 600) return 'error';
        if (statusCode >= 300 && statusCode < 400) return 'warning';
        return '';
    }

    async clearLogs() {
        // Add confirmation with smooth animation
        if (this.logs.length === 0) {
            this.showNotification('没有日志可清空', 'info');
            return;
        }
        
        // 先调用后端接口清理历史日志
        try {
            const response = await fetch('/api/clear-history', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
            });
            
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            
            const result = await response.json();
            console.log('后端清理结果:', result);
            
            // Animate out existing logs
            const logEntries = this.logsContainer.querySelectorAll('.log-entry');
            logEntries.forEach((entry, index) => {
                setTimeout(() => {
                    entry.style.opacity = '0';
                    entry.style.transform = 'translateX(-100%)';
                }, index * 50);
            });
            
            // Clear after animation
            setTimeout(() => {
                this.logs = [];
                // Reset latency statistics
                this.latencySum = 0;
                this.latencyCount = 0;
                this.updateAverageLatency();
                this.renderLogs();
                this.showNotification(`已清空 ${logEntries.length} 条日志`, 'success');
            }, Math.min(logEntries.length * 50 + 300, 1000));
            
        } catch (error) {
            console.error('清空历史日志失败:', error);
            this.showNotification('清空历史日志失败，请重试', 'error');
            
            // 如果后端接口失败，仍然执行前端的清空操作
            const logEntries = this.logsContainer.querySelectorAll('.log-entry');
            logEntries.forEach((entry, index) => {
                setTimeout(() => {
                    entry.style.opacity = '0';
                    entry.style.transform = 'translateX(-100%)';
                }, index * 50);
            });
            
            setTimeout(() => {
                this.logs = [];
                this.latencySum = 0;
                this.latencyCount = 0;
                this.updateAverageLatency();
                this.renderLogs();
                this.showNotification(`已清空前端 ${logEntries.length} 条日志`, 'info');
            }, Math.min(logEntries.length * 50 + 300, 1000));
        }
        
        // Add haptic feedback style animation
        this.clearBtn.style.transform = 'scale(0.95)';
        setTimeout(() => {
            this.clearBtn.style.transform = 'scale(1)';
        }, 100);
    }

    togglePause() {
        this.isPaused = !this.isPaused;
        this.pauseBtn.innerHTML = this.isPaused ? '▶️ 继续' : '⏸️ 暂停';
        this.pauseBtn.classList.toggle('active', this.isPaused);
        
        // Add haptic feedback style animation
        this.pauseBtn.style.transform = 'scale(0.95)';
        setTimeout(() => {
            this.pauseBtn.style.transform = 'scale(1)';
        }, 100);
        
        this.showNotification(this.isPaused ? '日志已暂停' : '日志已恢复', 'info');
    }

    toggleAutoScroll() {
        this.autoScroll = !this.autoScroll;
        this.autoScrollBtn.classList.toggle('active', this.autoScroll);
        this.autoScrollBtn.innerHTML = this.autoScroll ? '📜 自动滚动' : '✋ 手动滚动';
        
        // Add haptic feedback style animation
        this.autoScrollBtn.style.transform = 'scale(0.95)';
        setTimeout(() => {
            this.autoScrollBtn.style.transform = 'scale(1)';
        }, 100);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    isStreamingResponse(log) {
        // Check if response content type indicates streaming
        if (log.response_headers) {
            const contentType = log.response_headers['Content-Type'] || log.response_headers['content-type'] || '';
            if (contentType.includes('text/event-stream') || 
                contentType.includes('application/x-ndjson') ||
                contentType.includes('application/stream+json')) {
                return true;
            }
        }
        
        // Check if request headers indicate streaming
        if (log.request_headers) {
            const accept = log.request_headers['Accept'] || log.request_headers['accept'] || '';
            const stainlessHelper = log.request_headers['X-Stainless-Helper-Method'] || 
                                   log.request_headers['x-stainless-helper-method'] || '';
            
            if (accept.includes('text/event-stream') || stainlessHelper === 'stream') {
                return true;
            }
        }
        
        // Check if request body indicates streaming
        if (log.request_body && log.request_body.includes('"stream":true')) {
            return true;
        }
        
        return false;
    }

    formatConnectionInfo(log) {
        let connectionInfo = '';
        
        // Show only first byte time if available
        if (log.first_byte_duration) {
            connectionInfo += `<span class="connection-metric first-byte" title="首字节延迟">🏃 ${log.first_byte_duration}</span>`;
        }
        
        return connectionInfo;
    }

    formatTargetURL(url) {
        try {
            const parsedUrl = new URL(url);
            // Show hostname + port (if not default) + path for better readability
            let hostname = parsedUrl.hostname;
            const path = parsedUrl.pathname;
            const port = parsedUrl.port;
            
            // Add port if it's not the default port for the protocol
            if (port && 
                !((parsedUrl.protocol === 'http:' && port === '80') || 
                  (parsedUrl.protocol === 'https:' && port === '443'))) {
                hostname = `${hostname}:${port}`;
            }
            
            // If path is too long, truncate it
            if (path.length > 30) {
                return `${hostname}${path.substring(0, 27)}...`;
            }
            return `${hostname}${path}`;
        } catch (e) {
            // Fallback to showing the original URL if parsing fails
            return url.length > 40 ? url.substring(0, 37) + '...' : url;
        }
    }

    formatRoutingInfo(log) {
        const sourcePath = `${log.path}${log.query ? '?' + log.query : ''}`;
        
        console.log('formatRoutingInfo called with:', {
            sourcePath,
            target_url: log.target_url,
            hasTarget: !!log.target_url
        });
        
        if (log.target_url) {
            const targetPath = this.formatTargetURL(log.target_url);
            const result = `<span class="routing-info"><span class="source-path">${sourcePath}</span><span class="routing-arrow">→</span><span class="target-path" title="目标地址: ${log.target_url}">${targetPath}</span></span>`;
            console.log('Returning routing info:', result);
            return result;
        } else {
            const result = `<span class="path">${sourcePath}</span>`;
            console.log('Returning simple path:', result);
            return result;
        }
    }

    hasConnectionMetrics(log) {
        return log.dns_lookup_duration || log.connect_duration || 
               log.tls_handshake_duration || log.first_byte_duration || 
               log.upstream_latency || log.total_latency || 
               log.connection_reused !== undefined;
    }

    formatConnectionMetricsDetails(log) {
        let metrics = [];
        
        if (log.dns_lookup_duration) {
            metrics.push(`DNS解析时间: ${log.dns_lookup_duration}`);
        }
        if (log.connect_duration) {
            metrics.push(`TCP连接时间: ${log.connect_duration}`);
        }
        if (log.tls_handshake_duration) {
            metrics.push(`TLS握手时间: ${log.tls_handshake_duration}`);
        }
        if (log.first_byte_duration) {
            metrics.push(`首字节延迟: ${log.first_byte_duration}`);
        }
        if (log.upstream_latency) {
            metrics.push(`上游服务延迟: ${log.upstream_latency}`);
        }
        if (log.total_latency) {
            metrics.push(`总延迟: ${log.total_latency}`);
        }
        if (log.connection_reused !== undefined) {
            metrics.push(`连接复用: ${log.connection_reused ? '是' : '否'}`);
        }
        
        return metrics.join('\n');
    }

    trackLatency(logData) {
        // Extract latency from upstream_latency or duration
        let latencyMs = 0;
        
        if (logData.upstream_latency) {
            latencyMs = this.parseLatencyToMs(logData.upstream_latency);
        } else if (logData.duration) {
            latencyMs = this.parseLatencyToMs(logData.duration);
        }
        
        if (latencyMs > 0) {
            this.latencySum += latencyMs;
            this.latencyCount++;
            
            // Update average latency display
            this.updateAverageLatency();
        }
    }

    parseLatencyToMs(latencyStr) {
        // Parse duration strings like "123.456ms", "1.234s", "12.3µs" etc.
        const match = latencyStr.match(/^(\d+\.?\d*)(µs|ms|s|m|h)/);
        if (!match) return 0;
        
        const value = parseFloat(match[1]);
        const unit = match[2];
        
        switch (unit) {
            case 'µs': return value / 1000;
            case 'ms': return value;
            case 's': return value * 1000;
            case 'm': return value * 60 * 1000;
            case 'h': return value * 60 * 60 * 1000;
            default: return 0;
        }
    }

    updateAverageLatency() {
        if (this.latencyCount === 0) {
            this.avgLatencyEl.textContent = '--';
            return;
        }
        
        const avgMs = this.latencySum / this.latencyCount;
        let displayText;
        
        if (avgMs < 1) {
            displayText = `${(avgMs * 1000).toFixed(0)}µs`;
        } else if (avgMs < 1000) {
            displayText = `${avgMs.toFixed(1)}ms`;
        } else {
            displayText = `${(avgMs / 1000).toFixed(2)}s`;
        }
        
        this.avgLatencyEl.textContent = displayText;
    }

    async copyLogAsJSON(log) {
        try {
            const jsonString = JSON.stringify(log, null, 2);
            
            if (navigator.clipboard && window.isSecureContext) {
                await navigator.clipboard.writeText(jsonString);
                this.showNotification('JSON已复制到剪贴板', 'success');
            } else {
                // Fallback for older browsers
                this.fallbackCopyTextToClipboard(jsonString);
            }
        } catch (err) {
            console.error('复制失败:', err);
            this.showNotification('复制失败', 'error');
        }
    }

    fallbackCopyTextToClipboard(text) {
        const textArea = document.createElement('textarea');
        textArea.value = text;
        textArea.style.position = 'fixed';
        textArea.style.left = '-999999px';
        textArea.style.top = '-999999px';
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();
        
        try {
            const successful = document.execCommand('copy');
            if (successful) {
                this.showNotification('JSON已复制到剪贴板', 'success');
            } else {
                this.showNotification('复制失败', 'error');
            }
        } catch (err) {
            console.error('复制失败:', err);
            this.showNotification('复制失败', 'error');
        }
        
        document.body.removeChild(textArea);
    }

    showNotification(message, type = 'info') {
        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        notification.textContent = message;
        
        document.body.appendChild(notification);
        
        // Show notification
        setTimeout(() => {
            notification.classList.add('show');
        }, 100);
        
        // Hide and remove notification
        setTimeout(() => {
            notification.classList.remove('show');
            setTimeout(() => {
                if (document.body.contains(notification)) {
                    document.body.removeChild(notification);
                }
            }, 300);
        }, 2000);
    }

    showModal(log) {
        const details = this.renderLogDetails(log);
        this.modalBody.innerHTML = details;
        this.modal.classList.add('show');
        document.body.style.overflow = 'hidden';
        
        // Bind stream aggregation events
        this.bindStreamAggregationEvents(log);
        
        // Bind copy section events
        this.bindCopySectionEvents(log);
        
        // Bind collapse section events
        this.bindCollapseSectionEvents();
        
        // Add entrance animation for modal content
        const modalContent = this.modal.querySelector('.modal-content');
        modalContent.style.transform = 'translateY(-30px) scale(0.95)';
        modalContent.style.opacity = '0';
        
        setTimeout(() => {
            modalContent.style.transform = 'translateY(0) scale(1)';
            modalContent.style.opacity = '1';
        }, 50);
    }

    hideModal() {
        const modalContent = this.modal.querySelector('.modal-content');
        modalContent.style.transform = 'translateY(-20px) scale(0.95)';
        modalContent.style.opacity = '0';
        
        setTimeout(() => {
            this.modal.classList.remove('show');
            document.body.style.overflow = '';
        }, 200);
    }

    showConfigModal() {
        if (!this.configYaml && !this.config) {
            this.showNotification('配置未加载', 'error');
            return;
        }

        const configHtml = this.renderConfigDetails();
        this.configModalBody.innerHTML = configHtml;
        this.configModal.classList.add('show');
        document.body.style.overflow = 'hidden';
        
        // Update button states
        this.updateConfigButtonStates();
        
        // Bind events for config modal
        this.bindConfigModalEvents();
        
        // Add entrance animation for modal content
        const modalContent = this.configModal.querySelector('.modal-content');
        modalContent.style.transform = 'translateY(-30px) scale(0.95)';
        modalContent.style.opacity = '0';
        
        setTimeout(() => {
            modalContent.style.transform = 'translateY(0) scale(1)';
            modalContent.style.opacity = '1';
        }, 50);
    }

    hideConfigModal() {
        const modalContent = this.configModal.querySelector('.modal-content');
        modalContent.style.transform = 'translateY(-20px) scale(0.95)';
        modalContent.style.opacity = '0';
        
        setTimeout(() => {
            this.configModal.classList.remove('show');
            document.body.style.overflow = '';
        }, 200);
    }

    renderConfigDetails() {
        const content = this.configYaml || '# 配置加载中...';
        this.originalConfigYaml = content;
        
        if (this.isEditingConfig) {
            return `
                <div class="config-editor">
                    <div class="config-editor-header">
                        <h4 class="config-editor-title">📝 编辑 YAML 配置文件</h4>
                    </div>
                    <textarea class="config-editor-textarea" id="configTextarea" placeholder="请输入 YAML 配置内容...">${this.escapeHtml(content)}</textarea>
                    <div class="config-status" id="configStatus">
                        <span>✏️ 编辑模式 - 修改完成后请点击保存</span>
                    </div>
                </div>
            `;
        } else {
            return `
                <div class="detail-section">
                    <div class="detail-title" data-section="config">
                        <div class="detail-title-text">
                            <span class="collapse-icon">▼</span>
                            <span>📋 YAML 配置文件</span>
                        </div>
                        <button class="copy-section-btn" data-copy-type="config">📋 复制</button>
                    </div>
                    <div class="config-display" data-section-content="config">${this.escapeHtml(content)}</div>
                </div>
            `;
        }
    }

    showNewLogNotification() {
        const now = Date.now();
        if (now - this.lastLogTime > 5000) { // Only show if more than 5 seconds since last log
            const notification = document.createElement('div');
            notification.className = 'log-pulse';
            notification.style.cssText = `
                position: fixed;
                top: 50%;
                right: 2rem;
                width: 12px;
                height: 12px;
                background: linear-gradient(135deg, #34c759 0%, #30d158 100%);
                border-radius: 50%;
                box-shadow: 0 0 0 0 rgba(52, 199, 89, 0.7);
                animation: pulse-new-log 2s ease-out;
                z-index: 1000;
            `;
            
            document.body.appendChild(notification);
            
            setTimeout(() => {
                if (document.body.contains(notification)) {
                    document.body.removeChild(notification);
                }
            }, 2000);
        }
        this.lastLogTime = now;
    }

    bindStreamAggregationEvents(log) {
        const aggregateBtn = this.modalBody.querySelector('.aggregate-stream-btn');
        if (aggregateBtn) {
            aggregateBtn.addEventListener('click', () => {
                this.toggleStreamAggregation(log, aggregateBtn);
            });
        }
    }

    bindCopySectionEvents(log) {
        const copyBtns = this.modalBody.querySelectorAll('.copy-section-btn');
        copyBtns.forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.preventDefault();
                this.copySectionContent(log, btn);
            });
        });
    }

    bindCollapseSectionEvents() {
        const detailTitles = this.modalBody.querySelectorAll('.detail-title[data-section]');
        detailTitles.forEach(title => {
            title.addEventListener('click', (e) => {
                // 避免点击复制按钮时触发折叠
                if (e.target.classList.contains('copy-section-btn') || e.target.closest('.copy-section-btn')) {
                    return;
                }
                this.toggleSectionCollapse(title);
            });
        });
    }

    toggleSectionCollapse(titleElement) {
        const sectionName = titleElement.getAttribute('data-section');
        const contentElement = this.modalBody.querySelector(`[data-section-content="${sectionName}"]`);
        const collapseIcon = titleElement.querySelector('.collapse-icon');
        
        if (!contentElement || !collapseIcon) return;
        
        if (contentElement.classList.contains('collapsed')) {
            // 展开
            contentElement.classList.remove('collapsed');
            collapseIcon.classList.remove('collapsed');
            collapseIcon.textContent = '▼';
        } else {
            // 折叠
            contentElement.classList.add('collapsed');
            collapseIcon.classList.add('collapsed');
            collapseIcon.textContent = '▶';
        }
    }

    bindConfigModalEvents() {
        // Bind collapse section events for config modal
        const detailTitles = this.configModalBody.querySelectorAll('.detail-title[data-section]');
        detailTitles.forEach(title => {
            title.addEventListener('click', (e) => {
                // 避免点击复制按钮时触发折叠
                if (e.target.classList.contains('copy-section-btn') || e.target.closest('.copy-section-btn')) {
                    return;
                }
                this.toggleConfigSectionCollapse(title);
            });
        });
        
        // Bind copy events
        const copyBtns = this.configModalBody.querySelectorAll('.copy-section-btn');
        copyBtns.forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.preventDefault();
                this.copyConfigContent(btn);
            });
        });
    }

    bindConfigCollapseSectionEvents() {
        const detailTitles = this.configModalBody.querySelectorAll('.detail-title[data-section]');
        detailTitles.forEach(title => {
            title.addEventListener('click', (e) => {
                this.toggleConfigSectionCollapse(title);
            });
        });
    }

    toggleConfigSectionCollapse(titleElement) {
        const sectionName = titleElement.getAttribute('data-section');
        const contentElement = this.configModalBody.querySelector(`[data-section-content="${sectionName}"]`);
        const collapseIcon = titleElement.querySelector('.collapse-icon');
        
        if (!contentElement || !collapseIcon) return;
        
        if (contentElement.classList.contains('collapsed')) {
            // 展开
            contentElement.classList.remove('collapsed');
            collapseIcon.classList.remove('collapsed');
            collapseIcon.textContent = '▼';
        } else {
            // 折叠
            contentElement.classList.add('collapsed');
            collapseIcon.classList.add('collapsed');
            collapseIcon.textContent = '▶';
        }
    }

    async copySectionContent(log, button) {
        const copyType = button.getAttribute('data-copy-type');
        let content = '';
        
        switch (copyType) {
            case 'connection-metrics':
                content = this.formatConnectionMetricsDetails(log);
                break;
            case 'target-url':
                content = log.target_url || '';
                break;
            case 'remote-addr':
                content = log.remote_addr || '';
                break;
            case 'request-headers':
                content = JSON.stringify(log.request_headers, null, 2);
                break;
            case 'response-headers':
                content = JSON.stringify(log.response_headers, null, 2);
                break;
            case 'request-body':
                content = log.request_body || '';
                break;
            case 'response-body':
                // Check if we should copy aggregated content or original
                const responseSection = button.closest('.detail-section');
                const aggregatedDiv = responseSection.querySelector('.aggregated-content');
                const originalDiv = responseSection.querySelector('.detail-content[data-original-content]');
                
                if (aggregatedDiv && aggregatedDiv.style.display !== 'none') {
                    // Copy aggregated content (text only, without info header)
                    const aggregatedText = aggregatedDiv.textContent;
                    // Remove the info line "✨ 已聚合 X 个流式事件"
                    const lines = aggregatedText.split('\n');
                    content = lines.slice(1).join('\n').trim();
                } else {
                    content = log.response_body || '';
                }
                break;
            case 'error':
                content = log.error || '';
                break;
            case 'aggregated-content':
                // Copy aggregated content (text only, without info header)
                const aggregatedContainer = button.closest('.aggregated-content');
                if (aggregatedContainer) {
                    const aggregatedText = aggregatedContainer.textContent;
                    // Remove the info line "✨ 已聚合 X 个流式事件"
                    const lines = aggregatedText.split('\n');
                    content = lines.slice(1).join('\n').trim();
                }
                break;
            default:
                content = '';
        }
        
        if (!content) {
            this.showNotification('没有内容可复制', 'warn');
            return;
        }
        
        try {
            if (navigator.clipboard && window.isSecureContext) {
                await navigator.clipboard.writeText(content);
                this.showNotification('内容已复制到剪贴板', 'success');
                
                // Visual feedback on button
                const originalText = button.textContent;
                button.textContent = '✅ 已复制';
                button.style.color = '#34c759';
                
                setTimeout(() => {
                    button.textContent = originalText;
                    button.style.color = '';
                }, 1500);
            } else {
                // Fallback for older browsers
                this.fallbackCopyTextToClipboard(content);
            }
        } catch (err) {
            console.error('复制失败:', err);
            this.showNotification('复制失败', 'error');
        }
    }

    toggleStreamAggregation(log, button) {
        const responseSection = button.closest('.detail-section');
        if (!responseSection) return;

        // Prevent rapid clicking during transition
        if (button.disabled) return;
        button.disabled = true;
        
        // Add loading state
        const originalText = button.textContent;
        button.textContent = '⏳ 处理中...';
        button.style.pointerEvents = 'none';

        let originalDiv = responseSection.querySelector('.detail-content[data-original-content]');
        let aggregatedDiv = responseSection.querySelector('.aggregated-content');
        
        const isAggregated = button.classList.contains('active');
        
        // Function to restore button state
        const restoreButton = (newText, isActive) => {
            setTimeout(() => {
                button.textContent = newText;
                button.disabled = false;
                button.style.pointerEvents = 'auto';
                
                if (isActive) {
                    button.classList.add('active');
                } else {
                    button.classList.remove('active');
                }
            }, 450); // Slightly longer than transition time
        };
        
        if (isAggregated) {
            // Switch to original content with smooth transition
            if (aggregatedDiv) {
                // Fade out aggregated content
                aggregatedDiv.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
                aggregatedDiv.style.opacity = '0';
                aggregatedDiv.style.transform = 'translateY(-10px)';
                
                setTimeout(() => {
                    aggregatedDiv.style.display = 'none';
                }, 300);
            }
            
            if (originalDiv) {
                // Fade in original content
                originalDiv.style.display = 'block';
                originalDiv.style.opacity = '0';
                originalDiv.style.transform = 'translateY(10px)';
                
                // Force reflow
                originalDiv.offsetHeight;
                
                originalDiv.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
                originalDiv.style.opacity = '1';
                originalDiv.style.transform = 'translateY(0)';
            }
            
            restoreButton('🔗 聚合流式响应', false);
            this.showNotification('已切换到原始响应', 'info');
        } else {
            // Switch to aggregated content
            if (!aggregatedDiv) {
                // Create aggregated content for the first time
                const aggregatedContent = this.aggregateStreamingResponse(log.response_body);
                
                if (aggregatedContent.success) {
                    aggregatedDiv = document.createElement('div');
                    aggregatedDiv.className = 'aggregated-content';
                    aggregatedDiv.style.display = 'none'; // Initially hidden
                    
                    // Only show debug info if there were issues or user needs it
                    let debugInfoHtml = '';
                    if (aggregatedContent.debugInfo && aggregatedContent.debugInfo.length > 0 && aggregatedContent.eventCount === 0) {
                        // Only show debug info if aggregation failed
                        debugInfoHtml = `
<details style="margin-top: 0.5rem; font-size: 0.7rem; color: #8e8e93;">
<summary style="cursor: pointer;">🔍 调试信息 (${aggregatedContent.debugInfo.length} 条)</summary>
<div style="margin-top: 0.5rem; white-space: pre-wrap; font-family: monospace;">${aggregatedContent.debugInfo.join('\n')}</div>
</details>`;
                    }
                    
                    aggregatedDiv.innerHTML = `
<div class="aggregation-info" style="display: flex; justify-content: space-between; align-items: center;">
<span>✨ 已聚合 ${aggregatedContent.eventCount} 个流式事件${aggregatedContent.text ? ` (${aggregatedContent.text.length} 个字符)` : ''}</span>
<button class="copy-section-btn" data-copy-type="aggregated-content" style="font-size: 0.65rem; padding: 0.2rem 0.4rem;">📋 复制</button>
</div>
${aggregatedContent.text ? this.escapeHtml(aggregatedContent.text.trim()) : '<em style="color: #8e8e93;">没有找到可聚合的文本内容</em>'}${debugInfoHtml}`;
                    
                    // Insert after the original content div
                    if (originalDiv) {
                        originalDiv.parentNode.insertBefore(aggregatedDiv, originalDiv.nextSibling);
                    } else {
                        responseSection.appendChild(aggregatedDiv);
                    }
                    
                    // Bind copy events for the new aggregated content
                    const aggregatedCopyBtn = aggregatedDiv.querySelector('.copy-section-btn');
                    if (aggregatedCopyBtn) {
                        aggregatedCopyBtn.addEventListener('click', (e) => {
                            e.preventDefault();
                            this.copySectionContent(log, aggregatedCopyBtn);
                        });
                    }
                    
                    this.showNotification(`成功聚合 ${aggregatedContent.eventCount} 个流式事件`, 'success');
                } else {
                    const errorMsg = aggregatedContent.error ? 
                        `解析失败: ${aggregatedContent.error}` : 
                        '无法解析流式响应格式';
                    this.showNotification(errorMsg, 'error');
                    
                    // 显示调试信息
                    if (aggregatedContent.debugInfo) {
                        console.log('聚合失败的调试信息:', aggregatedContent.debugInfo);
                    }
                    
                    // Restore button state on error
                    restoreButton('🔗 聚合流式响应', false);
                    return;
                }
            }
            
            // Show aggregated content, hide original with smooth transition
            if (originalDiv) {
                // Fade out original content
                originalDiv.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
                originalDiv.style.opacity = '0';
                originalDiv.style.transform = 'translateY(-10px)';
                
                setTimeout(() => {
                    originalDiv.style.display = 'none';
                }, 300);
            }
            
            if (aggregatedDiv) {
                // Delay showing aggregated content until original fades out
                setTimeout(() => {
                    aggregatedDiv.style.display = 'block';
                    
                    // Add smooth transition effect
                    aggregatedDiv.style.opacity = '0';
                    aggregatedDiv.style.transform = 'translateY(10px)';
                    
                    // Force reflow
                    aggregatedDiv.offsetHeight;
                    
                    // Add transition styles
                    aggregatedDiv.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
                    aggregatedDiv.style.opacity = '1';
                    aggregatedDiv.style.transform = 'translateY(0)';
                }, 150); // Start showing aggregated content halfway through original fade out
            }
            
            restoreButton('📜 显示原始响应', true);
        }
    }

    aggregateStreamingResponse(responseBody) {
        try {
            // console.log('开始聚合流式响应:', responseBody.substring(0, 200) + '...');
            
            const lines = responseBody.split('\n');
            let aggregatedText = '';
            let eventCount = 0;
            let currentEvent = null;
            let debugInfo = [];
            
            for (const line of lines) {
                const trimmedLine = line.trim();
                
                // Skip empty lines
                if (!trimmedLine) continue;
                
                // Parse Server-Sent Events format
                if (trimmedLine.startsWith('event:')) {
                    currentEvent = trimmedLine.substring(6).trim();
                    debugInfo.push(`Event: ${currentEvent}`);
                } else if (trimmedLine.startsWith('data:')) {
                    const dataContent = trimmedLine.substring(5).trim();
                    debugInfo.push(`Data line: ${dataContent.substring(0, 100)}...`);
                    
                    try {
                        // Try to parse as JSON
                        const jsonData = JSON.parse(dataContent);
                        
                        // Handle Claude API streaming format
                        if (jsonData.type === 'content_block_delta' && 
                            jsonData.delta && 
                            jsonData.delta.type === 'text_delta' && 
                            jsonData.delta.text) {
                            aggregatedText += jsonData.delta.text;
                            eventCount++;
                            debugInfo.push(`Found Claude text: "${jsonData.delta.text}"`);
                        }
                        // Handle OpenAI streaming format
                        else if (jsonData.choices && jsonData.choices[0] && 
                                jsonData.choices[0].delta && 
                                jsonData.choices[0].delta.content) {
                            aggregatedText += jsonData.choices[0].delta.content;
                            eventCount++;
                            debugInfo.push(`Found OpenAI content: "${jsonData.choices[0].delta.content}"`);
                        }
                        // Handle Anthropic message format
                        else if (jsonData.type === 'message_delta' && 
                                jsonData.delta && 
                                jsonData.delta.text) {
                            aggregatedText += jsonData.delta.text;
                            eventCount++;
                            debugInfo.push(`Found Anthropic message: "${jsonData.delta.text}"`);
                        }
                        // Handle generic content field
                        else if (jsonData.content && typeof jsonData.content === 'string') {
                            aggregatedText += jsonData.content;
                            eventCount++;
                            debugInfo.push(`Found generic content: "${jsonData.content}"`);
                        }
                        // Handle generic text field
                        else if (jsonData.text && typeof jsonData.text === 'string') {
                            aggregatedText += jsonData.text;
                            eventCount++;
                            debugInfo.push(`Found generic text: "${jsonData.text}"`);
                        }
                        // Handle direct string content
                        else if (typeof jsonData === 'string' && jsonData.trim()) {
                            aggregatedText += jsonData;
                            eventCount++;
                            debugInfo.push(`Found direct string: "${jsonData}"`);
                        }
                        // Handle object with unknown structure - try to find text content
                        else {
                            const textContent = this.extractTextFromObject(jsonData);
                            if (textContent) {
                                aggregatedText += textContent;
                                eventCount++;
                                debugInfo.push(`Found extracted text: "${textContent}"`);
                            } else {
                                debugInfo.push(`Unhandled JSON structure: ${JSON.stringify(jsonData).substring(0, 100)}...`);
                            }
                        }
                    } catch (parseError) {
                        // If not JSON, treat as plain text
                        if (dataContent && dataContent !== '[DONE]' && dataContent.trim()) {
                            aggregatedText += dataContent + '\n';
                            eventCount++;
                            debugInfo.push(`Found plain text: "${dataContent}"`);
                        }
                    }
                }
                // Handle lines that might not follow SSE format strictly
                else if (trimmedLine.startsWith('{') && trimmedLine.endsWith('}')) {
                    try {
                        const jsonData = JSON.parse(trimmedLine);
                        const textContent = this.extractTextFromObject(jsonData);
                        if (textContent) {
                            aggregatedText += textContent;
                            eventCount++;
                            debugInfo.push(`Found JSON line text: "${textContent}"`);
                        }
                    } catch (e) {
                        // Ignore malformed JSON
                        debugInfo.push(`Malformed JSON line: ${trimmedLine.substring(0, 50)}...`);
                    }
                }
            }
            
            // console.log('聚合调试信息:', debugInfo);
            // console.log(`聚合结果: ${eventCount} 个事件, ${aggregatedText.length} 个字符`);
            
            return {
                success: eventCount > 0,
                text: aggregatedText,
                eventCount: eventCount,
                debugInfo: debugInfo.slice(-10) // Keep last 10 debug entries
            };
        } catch (error) {
            console.error('Error aggregating streaming response:', error);
            return { success: false, text: '', eventCount: 0, error: error.message };
        }
    }

    extractTextFromObject(obj) {
        // Recursively search for text content in object
        if (typeof obj === 'string' && obj.trim()) {
            return obj;
        }
        
        if (typeof obj !== 'object' || obj === null) {
            return null;
        }
        
        // Common text field names to check
        const textFields = ['text', 'content', 'message', 'data', 'value', 'output'];
        
        for (const field of textFields) {
            if (obj[field] && typeof obj[field] === 'string' && obj[field].trim()) {
                return obj[field];
            }
        }
        
        // Check for nested delta structures
        if (obj.delta) {
            const deltaText = this.extractTextFromObject(obj.delta);
            if (deltaText) return deltaText;
        }
        
        // Check for choices array (OpenAI format)
        if (obj.choices && Array.isArray(obj.choices) && obj.choices.length > 0) {
            const choiceText = this.extractTextFromObject(obj.choices[0]);
            if (choiceText) return choiceText;
        }
        
        return null;
    }

    initKeyboardShortcuts() {
        document.addEventListener('keydown', (e) => {
            // Only handle shortcuts when not typing in an input
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
                return;
            }

            switch(e.key) {
                case 'c':
                case 'C':
                    if (e.metaKey || e.ctrlKey) return; // Avoid conflict with copy
                    this.clearLogs();
                    e.preventDefault();
                    break;
                case ' ':
                    this.togglePause();
                    e.preventDefault();
                    break;
                case 's':
                case 'S':
                    this.toggleAutoScroll();
                    e.preventDefault();
                    break;
                case 'Escape':
                    if (this.modal.classList.contains('show')) {
                        this.hideModal();
                    } else if (this.configModal.classList.contains('show')) {
                        this.hideConfigModal();
                    }
                    break;
            }
        });
    }
    
    enableConfigEdit() {
        this.isEditingConfig = true;
        this.updateConfigButtonStates();
        
        // Re-render the modal content in edit mode
        const configHtml = this.renderConfigDetails();
        this.configModalBody.innerHTML = configHtml;
        this.bindConfigModalEvents();
        
        // Focus on textarea
        const textarea = document.getElementById('configTextarea');
        if (textarea) {
            textarea.focus();
            // Add syntax validation
            textarea.addEventListener('input', () => this.validateYamlSyntax(textarea.value));
        }
        
        this.showNotification('已进入编辑模式', 'info');
    }
    
    cancelConfigEdit() {
        this.isEditingConfig = false;
        this.updateConfigButtonStates();
        
        // Re-render the modal content in display mode
        const configHtml = this.renderConfigDetails();
        this.configModalBody.innerHTML = configHtml;
        this.bindConfigModalEvents();
        
        this.showNotification('已取消编辑', 'info');
    }
    
    async saveConfig() {
        const textarea = document.getElementById('configTextarea');
        if (!textarea) {
            this.showNotification('编辑器未找到', 'error');
            return;
        }
        
        const newConfigYaml = textarea.value;
        
        // Validate YAML syntax
        if (!this.isValidYaml(newConfigYaml)) {
            this.showNotification('YAML 格式错误，请检查语法', 'error');
            return;
        }
        
        try {
            // Show saving state
            this.saveConfigBtn.textContent = '🔄 保存中...';
            this.saveConfigBtn.disabled = true;
            
            const response = await fetch('/api/config', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/x-yaml',
                },
                body: newConfigYaml
            });
            
            if (response.ok) {
                // Update local config
                this.configYaml = newConfigYaml;
                this.originalConfigYaml = newConfigYaml;
                
                // Parse basic config info for proxy address
                this.parseBasicConfigFromYaml(newConfigYaml);
                this.updateProxyAddress();
                
                // Exit edit mode
                this.isEditingConfig = false;
                this.updateConfigButtonStates();
                
                // Re-render in display mode
                const configHtml = this.renderConfigDetails();
                this.configModalBody.innerHTML = configHtml;
                this.bindConfigModalEvents();
                
                this.showNotification('配置保存成功', 'success');
            } else {
                const errorText = await response.text();
                this.showNotification(`保存失败: ${errorText}`, 'error');
            }
        } catch (error) {
            console.error('Save config failed:', error);
            this.showNotification(`保存失败: ${error.message}`, 'error');
        } finally {
            // Restore button state
            this.saveConfigBtn.textContent = '💾 保存';
            this.saveConfigBtn.disabled = false;
        }
    }
    
    updateConfigButtonStates() {
        if (this.isEditingConfig) {
            this.editConfigBtn.style.display = 'none';
            this.saveConfigBtn.style.display = 'inline-block';
            this.cancelEditBtn.style.display = 'inline-block';
        } else {
            this.editConfigBtn.style.display = 'inline-block';
            this.saveConfigBtn.style.display = 'none';
            this.cancelEditBtn.style.display = 'none';
        }
    }
    
    validateYamlSyntax(yamlContent) {
        const statusEl = document.getElementById('configStatus');
        if (!statusEl) return;
        
        if (this.isValidYaml(yamlContent)) {
            statusEl.className = 'config-status';
            statusEl.innerHTML = '<span>✅ YAML 语法正确</span>';
        } else {
            statusEl.className = 'config-status error';
            statusEl.innerHTML = '<span>⚠️ YAML 语法错误，请检查格式</span>';
        }
    }
    
    isValidYaml(yamlString) {
        try {
            // Basic YAML validation - check for common syntax errors
            const lines = yamlString.split('\n');
            
            for (let i = 0; i < lines.length; i++) {
                const line = lines[i];
                const trimmed = line.trim();
                
                // Skip empty lines and comments
                if (!trimmed || trimmed.startsWith('#')) continue;
                
                // Check indentation consistency
                const indent = line.length - line.trimStart().length;
                if (indent % 2 !== 0 && indent > 0) {
                    // YAML should use consistent 2-space indentation
                    return false;
                }
                
                // Basic colon check for key-value pairs
                if (trimmed.includes(':') && !trimmed.startsWith('-')) {
                    const colonIndex = trimmed.indexOf(':');
                    const afterColon = trimmed.substring(colonIndex + 1).trim();
                    // Allow empty values or quoted strings
                    if (afterColon && !afterColon.startsWith('"') && !afterColon.startsWith("'") && 
                        !/^[a-zA-Z0-9\s\-_\.\[\]{}]+$/.test(afterColon)) {
                        // This is a very basic check - in a real implementation, 
                        // you'd want to use a proper YAML parser
                    }
                }
            }
            
            return true; // Basic validation passed
        } catch (error) {
            return false;
        }
    }
    
    async copyConfigContent(button) {
        const content = this.configYaml || JSON.stringify(this.config, null, 2);
        
        if (!content) {
            this.showNotification('没有内容可复制', 'warning');
            return;
        }
        
        try {
            if (navigator.clipboard && window.isSecureContext) {
                await navigator.clipboard.writeText(content);
                this.showNotification('配置内容已复制到剪贴板', 'success');
                
                // Visual feedback on button
                const originalText = button.textContent;
                button.textContent = '✅ 已复制';
                button.style.color = '#34c759';
                
                setTimeout(() => {
                    button.textContent = originalText;
                    button.style.color = '';
                }, 1500);
            } else {
                this.fallbackCopyTextToClipboard(content);
            }
        } catch (err) {
            console.error('复制失败:', err);
            this.showNotification('复制失败', 'error');
        }
    }
}

// Add CSS for new log pulse animation
const style = document.createElement('style');
style.textContent = `
    @keyframes pulse-new-log {
        0% {
            transform: scale(0);
            box-shadow: 0 0 0 0 rgba(52, 199, 89, 0.7);
        }
        70% {
            transform: scale(1);
            box-shadow: 0 0 0 10px rgba(52, 199, 89, 0);
        }
        100% {
            transform: scale(1);
            box-shadow: 0 0 0 0 rgba(52, 199, 89, 0);
        }
    }
`;
document.head.appendChild(style);

document.addEventListener('DOMContentLoaded', () => {
    new LogViewer();
});