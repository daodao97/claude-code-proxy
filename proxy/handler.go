package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"ccproxy/config"
)

type ProxyHandler struct {
	config *config.Config
	client *http.Client
}

func NewProxyHandler(cfg *config.Config) *ProxyHandler {
	return &ProxyHandler{
		config: cfg,
		client: &http.Client{
			// No timeout for proxy client to support long-running requests
			// including streaming responses, file uploads, and AI model inference
		},
	}
}

func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Enhanced logging with request context
	requestInfo := p.getRequestInfo(r)
	log.Printf("[INFO] Incoming request: %s", requestInfo)

	target := p.findTarget(r.URL.Path, r.Method)
	if target == nil {
		log.Printf("[WARN] No matching target found for %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	targetURL, err := p.buildTargetURL(r.URL, target)
	if err != nil {
		log.Printf("[ERROR] Failed to build target URL for %s: %v (Original path: %s, Target: %s)",
			r.URL.Path, err, r.URL.String(), target.TargetURL)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	log.Printf("[INFO] Routing %s %s -> %s", r.Method, r.URL.Path, targetURL)
	ctx := context.WithValue(r.Context(), "target_url", targetURL)
	r = r.WithContext(ctx)

	if err := p.forwardRequestWithRetry(w, r, target); err != nil {
		log.Printf("[ERROR] Failed to forward request to %s after all retries: %v (Client: %s, UserAgent: %s)",
			targetURL, err, r.RemoteAddr, r.Header.Get("User-Agent"))
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
}

func (p *ProxyHandler) getRequestInfo(r *http.Request) string {
	return fmt.Sprintf("%s %s from %s (UA: %s, ContentLength: %d)",
		r.Method, r.URL.String(), r.RemoteAddr,
		r.Header.Get("User-Agent"), r.ContentLength)
}

func (p *ProxyHandler) findTarget(path, method string) *config.ProxyTarget {
	for _, target := range p.config.Proxy.Targets {
		if p.matchPath(path, target.Path) && p.matchMethod(method, target.Methods) {
			return &target
		}
	}
	return nil
}

func (p *ProxyHandler) matchPath(requestPath, targetPath string) bool {
	if strings.HasSuffix(targetPath, "*") {
		prefix := strings.TrimSuffix(targetPath, "*")
		return strings.HasPrefix(requestPath, prefix)
	}
	return requestPath == targetPath
}

func (p *ProxyHandler) matchMethod(method string, allowedMethods []string) bool {
	if len(allowedMethods) == 0 {
		return true
	}

	for _, allowedMethod := range allowedMethods {
		if strings.ToUpper(method) == strings.ToUpper(allowedMethod) {
			return true
		}
	}
	return false
}

func (p *ProxyHandler) forwardRequestWithRetry(w http.ResponseWriter, r *http.Request, target *config.ProxyTarget) error {
	var lastErr error
	maxRetries := p.config.Proxy.MaxRetries
	retryDelay := time.Duration(p.config.Proxy.RetryDelay) * time.Millisecond

	targetURL, err := p.buildTargetURL(r.URL, target)
	if err != nil {
		return fmt.Errorf("failed to build target URL: %w", err)
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[WARN] Retrying request to %s (attempt %d/%d) after %dms delay",
				targetURL, attempt, maxRetries, p.config.Proxy.RetryDelay)
			time.Sleep(retryDelay)
		}

		startTime := time.Now()
		err := p.forwardRequest(w, r, target)
		duration := time.Since(startTime)

		if err == nil {
			if attempt > 0 {
				log.Printf("[INFO] Request succeeded on retry %d to %s (took %v)", attempt, targetURL, duration)
			}
			return nil
		}

		lastErr = err

		// Check if the error is retryable
		if !p.isRetryableError(err) {
			log.Printf("[ERROR] Non-retryable error for %s after %v: %v", targetURL, duration, err)
			break
		}

		log.Printf("[WARN] Retryable error for %s (attempt %d/%d, took %v): %v",
			targetURL, attempt+1, maxRetries+1, duration, err)
	}

	return fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
}

func (p *ProxyHandler) isRetryableError(err error) bool {
	errStr := err.Error()
	// Common retryable errors
	retryableErrors := []string{
		"unexpected EOF",
		"connection reset by peer",
		"no such host",
		"timeout",
		"network is unreachable",
		"connection refused",
		"temporary failure",
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), retryableErr) {
			return true
		}
	}
	return false
}
