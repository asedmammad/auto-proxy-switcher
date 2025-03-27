package switcher

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"sync"
	"time"

	"github.com/asedmammad/auto_proxy_switcher/internal/config"
	"github.com/asedmammad/auto_proxy_switcher/pkg/cache"
	"github.com/asedmammad/auto_proxy_switcher/pkg/net"
	"github.com/asedmammad/auto_proxy_switcher/pkg/v2ray"
)

// Constants for configuration values
const (
	ProxyRetryInterval  = 5 * time.Minute
	NetworkCheckTimeout = 5 * time.Second
	ProxyTestTimeout    = 7 * time.Second
	BaseProxyPort       = 7778 // Base port for local proxy
	ActiveProxyBasePort = 7000
)

// GetLocalProxyAddress returns a unique local proxy address based on index
func GetLocalProxyAddress(index int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", BaseProxyPort+index)
}

var (
	activeProxy     *v2ray.ProxyConfig
	activeProxyLock sync.RWMutex
	activeV2Ray     *v2ray.V2RayProcess
	v2rayLock       sync.Mutex
	// Add a context that can be used to cancel ongoing operations when shutting down
	ctx, cancelFunc = context.WithCancel(context.Background())
	// Channel to limit concurrent proxy tests
	proxyTestSemaphore chan struct{}
)

// GetActiveProxy returns the currently active working proxy
func GetActiveProxy() *v2ray.ProxyConfig {
	activeProxyLock.RLock()
	defer activeProxyLock.RUnlock()
	return activeProxy
}

// setActiveProxy sets the active working proxy
func setActiveProxy(proxy *v2ray.ProxyConfig) {
	activeProxyLock.Lock()
	defer activeProxyLock.Unlock()
	activeProxy = proxy
}

// Shutdown gracefully stops all running processes
func Shutdown() {
	// Cancel the context to signal all goroutines to stop
	cancelFunc()

	// Stop the active V2Ray process
	stopCurrentV2Ray()
}

// stopV2Ray stops a V2Ray process
func stopV2Ray(cmd *exec.Cmd) {
	if cmd != nil {
		if err := v2ray.StopV2Ray(cmd); err != nil {
			log.Printf("Failed to stop v2ray: %v", err)
		}
	}
}

// stopCurrentV2Ray stops the currently running V2Ray process
func stopCurrentV2Ray() {
	v2rayLock.Lock()
	defer v2rayLock.Unlock()

	if activeV2Ray != nil && activeV2Ray.Cmd != nil {
		stopV2Ray(activeV2Ray.Cmd)
		activeV2Ray = nil
	}
}

// ProxyTester handles proxy testing and management
type ProxyTester struct {
	proxyCache         *cache.Cache
	targetURL          string
	httpClient         *net.HTTPClient
	proxyTestSemaphore chan struct{}
	retryTicker        *time.Ticker
}

// NewProxyTester creates a new ProxyTester instance
func NewProxyTester(proxyCache *cache.Cache, targetURL string, concurrentChecks int) *ProxyTester {
	if concurrentChecks <= 0 {
		concurrentChecks = config.DefaultConcurrentChecks
	}

	return &ProxyTester{
		proxyCache:         proxyCache,
		targetURL:          targetURL,
		httpClient:         net.NewHTTPClient(),
		proxyTestSemaphore: make(chan struct{}, concurrentChecks),
		retryTicker:        time.NewTicker(ProxyRetryInterval),
	}
}

// testSingleProxy tests a single proxy configuration with a specific port index and returns whether it works
// In testSingleProxy method
func (pt *ProxyTester) testSingleProxy(parsedURL *v2ray.ProxyConfig, portIndex int) bool {
	if parsedURL == nil {
		log.Println("Error: proxy config is nil")
		return false
	}

	log.Printf("Starting test for proxy: %s (port index: %d)", parsedURL.Name, portIndex)

	select {
	case <-ctx.Done():
		log.Printf("Test cancelled for proxy: %s", parsedURL.Name)
		return false
	default:
	}

	configPath, cmd, err := pt.startV2RayProcess(parsedURL, portIndex)
	if err != nil {
		return false
	}
	defer stopV2Ray(cmd)

	if success := pt.testProxyConnection(parsedURL, portIndex); !success {
		return false
	}

	return pt.activateWorkingProxy(parsedURL, cmd, configPath)
}

// In startV2RayProcess method
func (pt *ProxyTester) startV2RayProcess(proxy *v2ray.ProxyConfig, portIndex int) (string, *exec.Cmd, error) {
	port := BaseProxyPort + portIndex
	log.Printf("Attempting to start V2Ray process for proxy %s on port %d", proxy.Name, port)

	configFileName := fmt.Sprintf("config_%d.json", port)
	configPath, err := v2ray.CreateConfigWithPort(proxy, port, configFileName)
	if err != nil {
		log.Printf("Failed to create v2ray config for %s: %v", proxy.Name, err)
		return "", nil, err
	}

	log.Println("Starting v2ray process for proxy:", proxy.Name, "on port", port)
	cmd, err := v2ray.RunV2Ray(configPath)
	if err != nil {
		log.Printf("Failed to start v2ray for %s: %v", proxy.Name, err)
		return "", nil, err
	}

	log.Printf("Successfully started V2Ray process for proxy %s on port %d", proxy.Name, port)
	return configPath, cmd, nil
}

// In testProxyConnection method
func (pt *ProxyTester) testProxyConnection(proxy *v2ray.ProxyConfig, portIndex int) bool {
	log.Printf("Testing connection for proxy %s using port index %d", proxy.Name, portIndex)

	proxyURL, err := url.Parse(GetLocalProxyAddress(portIndex))
	if err != nil {
		log.Printf("Failed to parse proxy URL for %s: %v", proxy.Name, err)
		return false
	}

	log.Printf("Attempting to connect to target URL via proxy %s", proxy.Name)
	success, err := pt.httpClient.TestProxy(pt.targetURL, proxyURL, ProxyTestTimeout, 1)
	if err != nil {
		log.Printf("Connection test failed for proxy %s: %v", proxy.Name, err)
		return false
	}

	if success {
		log.Printf("Connection test successful for proxy %s", proxy.Name)
	} else {
		log.Printf("Connection test failed for proxy %s (no error but unsuccessful)", proxy.Name)
	}
	return success
}

// In testProxies method
func (pt *ProxyTester) testProxies() {
	log.Println("Starting proxy testing sequence")

	select {
	case <-ctx.Done():
		log.Println("Proxy testing cancelled: context done")
		return
	default:
	}

	if !net.HasNetworkConnectionWithTimeout(NetworkCheckTimeout) {
		log.Println("No network connection available, skipping proxy tests")
		return
	}

	log.Println("Network connection confirmed, proceeding with proxy tests")
	// First try to load and test the last working proxy
	lastProxy, err := config.LoadLastWorkingProxy()
	if err != nil {
		log.Printf("Failed to load last working proxy: %v", err)
	} else if lastProxy != nil {
		log.Printf("Testing last working proxy: %s", lastProxy.Name)
		parsedProxy := &v2ray.ProxyConfig{
			Protocol: lastProxy.Protocol,
			Name:     lastProxy.Name,
			Add:      lastProxy.Add,
			Host:     lastProxy.Host,
			ID:       lastProxy.ID,
			Net:      lastProxy.Net,
			Path:     lastProxy.Path,
			Port:     lastProxy.Port,
			SNI:      lastProxy.SNI,
			Security: lastProxy.Security,
		}

		// Acquire semaphore slot
		pt.proxyTestSemaphore <- struct{}{}
		if pt.testSingleProxy(parsedProxy, 0) {
			<-pt.proxyTestSemaphore // Release semaphore slot
			return
		}
		<-pt.proxyTestSemaphore // Release semaphore slot

		log.Println("Last working proxy is no longer working, testing all proxies...")
	}

	proxies, err := pt.proxyCache.Get()
	if err != nil {
		log.Printf("Failed to get proxies from cache: %v", err)
		return
	}
	log.Printf("Retrieved %d proxies from cache for testing", len(proxies))

	// Create a channel to signal when a working proxy is found
	foundWorkingProxy := make(chan bool, 1)

	// Create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Process proxies concurrently
	for i, proxyStr := range proxies {
		// Check if we already found a working proxy
		select {
		case <-foundWorkingProxy:
			// A working proxy was found, no need to test more
			log.Println("A working proxy was found, stopping other tests")
			return
		default:
			// Continue testing
		}

		// Parse the proxy URL
		log.Println("Parsing proxy URL")
		parsedURL, err := v2ray.ParseURL(proxyStr)
		if err != nil {
			log.Printf("Failed to parse proxy URL %s: %v", proxyStr, err)
			continue
		}

		// Acquire a slot from the semaphore (blocks if all slots are in use)
		// Update this line to use pt.proxyTestSemaphore instead of global proxyTestSemaphore
		log.Println("Acquiring proxy test semaphore")
		pt.proxyTestSemaphore <- struct{}{}
		log.Println("Proxy test semaphore acquired")

		// Increment the wait group counter
		wg.Add(1)

		// Start a goroutine to test this proxy
		go func(proxy *v2ray.ProxyConfig, index int) {
			defer wg.Done()
			defer func() {
				<-pt.proxyTestSemaphore
				log.Printf("Released semaphore slot for proxy %s", proxy.Name)
			}()

			log.Printf("Testing proxy %s (%d/%d)", proxy.Name, index+1, len(proxies))
			if pt.testSingleProxy(proxy, index%cap(pt.proxyTestSemaphore)) {
				log.Printf("Found working proxy: %s (index: %d)", proxy.Name, index)
				select {
				case foundWorkingProxy <- true:
					log.Println("Successfully signaled working proxy found")
				default:
					log.Println("Working proxy already found, signal not sent")
				}
			}
		}(parsedURL, i)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Check if we found a working proxy
	select {
	case <-foundWorkingProxy:
		// We found a working proxy
	default:
		log.Println("No working proxy found")
	}
}

// MonitorNetworkAndTestProxy starts monitoring network changes and tests proxies when network is available
func MonitorNetworkAndTestProxy(proxyCache *cache.Cache, targetURL string, concurrentChecks int) {
	log.Printf("Initializing proxy monitoring with target URL: %s", targetURL)
	if proxyCache == nil {
		log.Println("Error: proxy cache is nil")
		return
	}

	proxyTester := NewProxyTester(proxyCache, targetURL, concurrentChecks)
	log.Printf("Setting up proxy tester with %d concurrent checks", concurrentChecks)

	proxyTester.testProxies()

	// Start a goroutine for periodic checking when no working proxy is available
	go func() {
		defer proxyTester.retryTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-proxyTester.retryTicker.C:
				if GetActiveProxy() == nil {
					log.Println("No working proxy available, retrying proxy tests...")
					proxyTester.testProxies()
				}
			}
		}
	}()

	// Start network change listener
	net.ListenNetworkChanges(func(isUp bool) {
		log.Printf("Network status change detected: isUp=%v", isUp)
		// Check if context is canceled before proceeding
		select {
		case <-ctx.Done():
			return
		default:
			// Continue processing
		}

		hasInternetAccess := net.HasNetworkConnectionWithTimeout(NetworkCheckTimeout)

		if hasInternetAccess {
			lastProxy := GetActiveProxy()
			if lastProxy != nil {
				log.Printf("Network is up, testing last working proxy: %s", lastProxy.Name)

				// Acquire semaphore slot
				proxyTester.proxyTestSemaphore <- struct{}{}
				if proxyTester.testSingleProxy(lastProxy, 0) {
					<-proxyTester.proxyTestSemaphore // Release semaphore slot
					return
				}
				<-proxyTester.proxyTestSemaphore // Release semaphore slot

				log.Println("Last working proxy failed, testing all proxies...")
			} else {
				log.Println("Network is up, no last working proxy, testing all proxies...")
			}
			proxyTester.testProxies()
		} else {
			log.Println("Network is down")
			stopCurrentV2Ray()
		}
	})
}

func (pt *ProxyTester) activateWorkingProxy(proxy *v2ray.ProxyConfig, cmd *exec.Cmd, configPath string) bool {
	log.Printf("Found working proxy: %s", proxy.Name)

	stopCurrentV2Ray()

	configFileName := fmt.Sprintf("config_active_%d.json", ActiveProxyBasePort)
	newConfigPath, err := v2ray.CreateConfigWithPort(proxy, ActiveProxyBasePort, configFileName)
	if err != nil {
		log.Printf("Failed to create v2ray config for %s: %v", proxy.Name, err)
		return false
	}

	v2rayLock.Lock()
	activeV2Ray = &v2ray.V2RayProcess{
		ConfigPath: newConfigPath,
		Cmd:        cmd,
		StartTime:  time.Now(),
	}
	v2rayLock.Unlock()

	setActiveProxy(proxy)
	pt.saveLastWorkingProxy(proxy)
	return true
}

func (pt *ProxyTester) saveLastWorkingProxy(proxy *v2ray.ProxyConfig) {
	lastProxy := &config.LastWorkingProxy{
		Protocol: proxy.Protocol,
		Name:     proxy.Name,
		Add:      proxy.Add,
		Host:     proxy.Host,
		ID:       proxy.ID,
		Net:      proxy.Net,
		Path:     proxy.Path,
		Port:     proxy.Port,
		SNI:      proxy.SNI,
		Security: proxy.Security,
	}
	if err := config.SaveLastWorkingProxy(lastProxy); err != nil {
		log.Printf("Failed to save last working proxy: %v", err)
	}
}
