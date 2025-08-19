package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ccproxy/config"
)

// URLHealth stores health status and response time for a URL
type URLHealth struct {
	URL            string
	IsHealthy      bool
	ResponseTime   time.Duration
	LastCheck      time.Time
	ErrorCount     int
	TotalChecks    int
	SuccessChecks  int
	AverageTime    time.Duration
	MinTime        time.Duration
	MaxTime        time.Duration
}

// HealthChecker manages health checks for multiple URLs
type HealthChecker struct {
	urlHealthMap map[string]*URLHealth
	mutex        sync.RWMutex
	client       *http.Client
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		urlHealthMap: make(map[string]*URLHealth),
		client: &http.Client{
			Timeout: 5 * time.Second, // 5 second timeout for health checks
		},
	}
}

// StartHealthChecks starts periodic health checks for all target URLs
func (hc *HealthChecker) StartHealthChecks(targets []config.ProxyTarget) {
	for _, target := range targets {
		for _, url := range target.TargetURLs {
			hc.initializeURLHealth(url)
			go hc.runPeriodicHealthCheck(url, target.HealthCheckPath, target.HealthCheckDelay)
		}
	}
}

// initializeURLHealth initializes health status for a URL
func (hc *HealthChecker) initializeURLHealth(url string) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	
	if _, exists := hc.urlHealthMap[url]; !exists {
		hc.urlHealthMap[url] = &URLHealth{
			URL:        url,
			IsHealthy:  true, // Assume healthy initially
			LastCheck:  time.Now(),
			ErrorCount: 0,
		}
	}
}

// runPeriodicHealthCheck runs health checks at specified intervals
func (hc *HealthChecker) runPeriodicHealthCheck(url, healthPath string, delaySeconds int) {
	ticker := time.NewTicker(time.Duration(delaySeconds) * time.Second)
	defer ticker.Stop()

	// Run initial health check
	hc.checkURLHealth(url, healthPath)

	for range ticker.C {
		hc.checkURLHealth(url, healthPath)
	}
}

// checkURLHealth performs a health check on a specific URL
func (hc *HealthChecker) checkURLHealth(baseURL, healthPath string) {
	// Try multiple health check strategies
	isHealthy, responseTime, errorMsg := hc.performHealthCheck(baseURL, healthPath)
	hc.updateHealthStatus(baseURL, isHealthy, responseTime, errorMsg)
}

// performHealthCheck tries different strategies to determine if a URL is healthy
func (hc *HealthChecker) performHealthCheck(baseURL, healthPath string) (bool, time.Duration, string) {
	start := time.Now()
	
	// Strategy 1: Try the configured health check path
	if healthPath != "" && healthPath != "/" {
		if healthy, responseTime, _ := hc.tryHealthCheckURL(baseURL, healthPath, start); healthy {
			return true, responseTime, ""
		}
	}
	
	// Strategy 2: Try root path "/"
	if healthPath != "/" {
		if healthy, responseTime, _ := hc.tryHealthCheckURL(baseURL, "/", start); healthy {
			return true, responseTime, ""
		}
	}
	
	// Strategy 3: Try common health check endpoints
	commonPaths := []string{"/health", "/ping", "/status", "/api/health"}
	for _, path := range commonPaths {
		if path != healthPath { // Skip if already tried
			if healthy, responseTime, _ := hc.tryHealthCheckURL(baseURL, path, start); healthy {
				return true, responseTime, ""
			}
		}
	}
	
	// Strategy 4: Accept 404 as healthy for API endpoints (server is responsive)
	// Some APIs don't have a proper health endpoint but are still functional
	if _, responseTime, statusCode := hc.tryHealthCheckURL(baseURL, healthPath, start); statusCode == 404 {
		// 404 means the server is responding, just no health endpoint
		return true, responseTime, ""
	}
	
	// If all strategies fail, mark as unhealthy
	finalResponseTime := time.Since(start)
	return false, finalResponseTime, "all health check strategies failed"
}

// tryHealthCheckURL attempts a health check on a specific URL path
func (hc *HealthChecker) tryHealthCheckURL(baseURL, path string, startTime time.Time) (bool, time.Duration, int) {
	// Build health check URL
	healthURL := baseURL
	if path != "/" && path != "" {
		// Ensure baseURL doesn't end with / and path starts with /
		baseURL = strings.TrimSuffix(baseURL, "/")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		healthURL = baseURL + path
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Reduced timeout
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return false, time.Since(startTime), 0
	}

	resp, err := hc.client.Do(req)
	responseTime := time.Since(startTime)

	if err != nil {
		return false, responseTime, 0
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx status codes as healthy
	isHealthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	return isHealthy, responseTime, resp.StatusCode
}

// updateHealthStatus updates the health status for a URL
func (hc *HealthChecker) updateHealthStatus(url string, isHealthy bool, responseTime time.Duration, errorMsg string) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	health, exists := hc.urlHealthMap[url]
	if !exists {
		health = &URLHealth{
			URL:     url,
			MinTime: time.Hour, // Initialize with large value
		}
		hc.urlHealthMap[url] = health
	}

	previousHealth := health.IsHealthy
	health.IsHealthy = isHealthy
	health.ResponseTime = responseTime
	health.LastCheck = time.Now()
	health.TotalChecks++

	// Update response time statistics
	if responseTime > 0 {
		if health.MinTime > responseTime || health.MinTime == 0 {
			health.MinTime = responseTime
		}
		if health.MaxTime < responseTime {
			health.MaxTime = responseTime
		}
		
		if isHealthy {
			health.SuccessChecks++
			// Calculate rolling average for successful requests only
			if health.SuccessChecks == 1 {
				health.AverageTime = responseTime
			} else {
				// Simple moving average
				health.AverageTime = (health.AverageTime*time.Duration(health.SuccessChecks-1) + responseTime) / time.Duration(health.SuccessChecks)
			}
		}
	}

	if isHealthy {
		health.ErrorCount = 0
		// Only log recovery from unhealthy state with performance info
		if !previousHealth {
			successRate := float64(health.SuccessChecks) / float64(health.TotalChecks) * 100
			log.Printf("[INFO] URL %s recovered (success rate: %.1f%%, avg: %v, min: %v, max: %v)", 
				url, successRate, health.AverageTime, health.MinTime, health.MaxTime)
		}
	} else {
		health.ErrorCount++
		// Only log the first failure and every 10th failure to reduce noise
		if previousHealth || health.ErrorCount%10 == 0 {
			successRate := float64(health.SuccessChecks) / float64(health.TotalChecks) * 100
			if errorMsg == "" {
				log.Printf("[WARN] URL %s unhealthy (failure #%d, success rate: %.1f%%)", 
					url, health.ErrorCount, successRate)
			} else {
				log.Printf("[WARN] URL %s unhealthy: %s (failure #%d, success rate: %.1f%%)", 
					url, errorMsg, health.ErrorCount, successRate)
			}
		}
	}
}

// GetFastestHealthyURL returns the fastest responding healthy URL from a list
func (hc *HealthChecker) GetFastestHealthyURL(urls []string) string {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	var fastestURL string
	var fastestTime time.Duration = time.Hour // Initialize with a very large value

	healthyCount := 0
	var healthyURLs []string
	var urlStats []string
	
	for _, url := range urls {
		health, exists := hc.urlHealthMap[url]
		if !exists {
			// If we don't have health data yet, assume it's healthy and use it
			log.Printf("[INFO] No health data for %s yet, using as default", url)
			return url
		}

		statusStr := "unhealthy"
		responseTimeToUse := health.ResponseTime
		
		if health.IsHealthy {
			statusStr = "healthy"
			healthyCount++
			healthyURLs = append(healthyURLs, url)
			
			// Use average time if available, otherwise use last response time
			if health.AverageTime > 0 {
				responseTimeToUse = health.AverageTime
			}
			
			if responseTimeToUse < fastestTime {
				fastestTime = responseTimeToUse
				fastestURL = url
			}
		}
		
		// Build comprehensive stats string
		statsStr := fmt.Sprintf("%s(%s", url, statusStr)
		if health.TotalChecks > 0 {
			successRate := float64(health.SuccessChecks) / float64(health.TotalChecks) * 100
			if health.AverageTime > 0 {
				statsStr += fmt.Sprintf(",avg:%v,rate:%.0f%%", health.AverageTime, successRate)
			} else {
				statsStr += fmt.Sprintf(",last:%v,rate:%.0f%%", health.ResponseTime, successRate)
			}
		} else {
			statsStr += fmt.Sprintf(",last:%v", health.ResponseTime)
		}
		statsStr += ")"
		
		urlStats = append(urlStats, statsStr)
	}

	// Log detailed selection process for multiple URLs
	if len(urls) > 1 {
		log.Printf("[INFO] URL selection from %d candidates: %v", 
			len(urls), strings.Join(urlStats, ", "))
			
		if healthyCount > 1 {
			log.Printf("[INFO] Found %d healthy URLs, selecting fastest: %s (avg: %v)", 
				healthyCount, fastestURL, fastestTime)
		} else if healthyCount == 1 {
			log.Printf("[INFO] Only 1 healthy URL available: %s (avg: %v)", fastestURL, fastestTime)
		}
	}

	// If no healthy URLs found, return the first one as fallback
	if healthyCount == 0 && len(urls) > 0 {
		log.Printf("[WARN] No healthy URLs found in %d candidates, using first URL as fallback: %s", 
			len(urls), urls[0])
		return urls[0]
	}

	return fastestURL
}

// GetURLHealth returns the health status of a specific URL
func (hc *HealthChecker) GetURLHealth(url string) *URLHealth {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	if health, exists := hc.urlHealthMap[url]; exists {
		// Return a copy to avoid race conditions
		healthCopy := *health
		return &healthCopy
	}
	return nil
}

// GetAllHealthStatuses returns health status for all monitored URLs
func (hc *HealthChecker) GetAllHealthStatuses() map[string]*URLHealth {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	result := make(map[string]*URLHealth)
	for url, health := range hc.urlHealthMap {
		// Return copies to avoid race conditions
		healthCopy := *health
		result[url] = &healthCopy
	}
	return result
}