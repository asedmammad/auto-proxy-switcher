package switcher

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/asedmammad/auto_proxy_switcher/internal/config"
	"github.com/asedmammad/auto_proxy_switcher/pkg/cache"
)

// Constants for application configuration
const (
	AppVersion      = "1.0.0"
	AppName         = "auto_proxy_switcher"
	CacheUpdateFreq = 1 * time.Hour
)

// printUsage prints the command-line usage information
func printUsage() {
	fmt.Printf("Usage of %s:\n", AppName)
	flag.PrintDefaults()
	os.Exit(1)
}

// setupCleanupHandler sets up signal handlers for graceful shutdown
func setupCleanupHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-c
		log.Println("Received shutdown signal, cleaning up...")
		Shutdown()
		os.Exit(0)
	}()
}

// initializeCache creates and initializes the cache if needed
func initializeCache(cfg *config.Config, appConfigPath string) (*cache.Cache, error) {
    c := cache.NewCache(appConfigPath)
    if c == nil {
        return nil, fmt.Errorf("failed to create cache manager")
    }

    if cfg.RunTester {
        if _, err := os.Stat(c.FilePath); os.IsNotExist(err) {
            log.Printf("Cache file doesn't exist, creating initial cache...")
            if err := c.Update(cfg.SubscriptionUrl); err != nil {
                return nil, fmt.Errorf("failed to create initial cache: %v", err)
            }
            log.Printf("Initial cache created successfully")
        }
    }
    return c, nil
}

// startCacheUpdater starts periodic cache updates
func startCacheUpdater(c *cache.Cache, subscriptionUrl string) {
    ticker := time.NewTicker(CacheUpdateFreq)
    defer ticker.Stop()

    // Initial update
    if err := performCacheUpdate(c, subscriptionUrl); err != nil {
        log.Printf("Error in initial cache update: %v", err)
    }

    // Periodic updates
    for range ticker.C {
        if err := performCacheUpdate(c, subscriptionUrl); err != nil {
            log.Printf("Error in periodic cache update: %v", err)
        }
    }
}

// performCacheUpdate executes a single cache update
func performCacheUpdate(c *cache.Cache, subscriptionUrl string) error {
    log.Printf("Performing cache update...")
    if err := c.Update(subscriptionUrl); err != nil {
        return err
    }
    log.Printf("Cache updated successfully")
    return nil
}

// Start initializes and runs the application
func Start() {
    log.Printf("Starting application...")
    
    cfg := config.ParseArgs()

    if cfg.ShowHelp {
        printUsage()
        return
    }

    if cfg.ShowVersion {
        log.Printf("Version: %s", AppVersion)
        return
    }

    setupCleanupHandler()

    // Initialize application paths
    confDir, err := os.UserConfigDir()
    if err != nil {
        log.Fatalf("Failed to get user config directory: %v", err)
    }
    appConfigPath := path.Join(confDir, AppName)
    
    // Initialize cache
    c, err := initializeCache(cfg, appConfigPath)
    if err != nil {
        log.Fatalf("Cache initialization failed: %v", err)
    }

    // Start background tasks
    if cfg.UpdateCache {
        go startCacheUpdater(c, cfg.SubscriptionUrl)
    }

    if cfg.RunTester {
        log.Printf("Starting proxy monitoring with target URL: %s and %d concurrent checks", 
            cfg.ProxyTargetUrl, cfg.ConcurrentChecks)
        MonitorNetworkAndTestProxy(c, cfg.ProxyTargetUrl, cfg.ConcurrentChecks)
    }

    // Keep the main goroutine alive if background tasks are running
    if cfg.RunTester || cfg.UpdateCache {
        select {}
    }
}
