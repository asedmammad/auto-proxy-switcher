package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Constants for configuration
const (
	ConfigDirName        = "auto-proxy-switcher"
	LastWorkingProxyFile = "last_working_proxy.json"
	DefaultDirPerms      = 0755
	DefaultFilePerms     = 0644
)

// Config holds the application's configuration.
type Config struct {
	ShowHelp         bool
	ShowVersion      bool
	SubscriptionUrl  string
	ProxyTargetUrl   string
	UpdateCache      bool
	RunTester        bool
	ConcurrentChecks int
}

// LastWorkingProxy represents the last known working proxy configuration
type LastWorkingProxy struct {
	Protocol string `json:"protocol"`
	Name     string `json:"name"`
	Add      string `json:"add"`
	Host     string `json:"host"`
	ID       string `json:"id"`
	Net      string `json:"net"`
	Path     string `json:"path"`
	Port     int    `json:"port"`
	SNI      string `json:"sni"`
	Security string `json:"security"`
}

// GetConfigDir returns the path to the config directory, creating it if it doesn't exist
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", ConfigDirName)
	if err := os.MkdirAll(configDir, DefaultDirPerms); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	return configDir, nil
}

// SaveLastWorkingProxy saves the last working proxy configuration to a file
func SaveLastWorkingProxy(proxy *LastWorkingProxy) error {
	if proxy == nil {
		return fmt.Errorf("proxy cannot be nil")
	}

	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	data, err := json.MarshalIndent(proxy, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal proxy data: %w", err)
	}

	filePath := filepath.Join(configDir, LastWorkingProxyFile)
	if err := os.WriteFile(filePath, data, DefaultFilePerms); err != nil {
		return fmt.Errorf("failed to write proxy data to file: %w", err)
	}

	return nil
}

// LoadLastWorkingProxy loads the last working proxy configuration from file
func LoadLastWorkingProxy() (*LastWorkingProxy, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	filePath := filepath.Join(configDir, LastWorkingProxyFile)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Not finding a last working proxy is not an error condition
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read proxy data from file: %w", err)
	}

	var proxy LastWorkingProxy
	if err := json.Unmarshal(data, &proxy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proxy data: %w", err)
	}

	return &proxy, nil
}

// Default configuration values
const (
	DefaultSubscriptionUrl  = "https://some-sub-url.com/sub?max=100&type=vmess"
	DefaultProxyTargetUrl   = "https://google.com"
	DefaultConcurrentChecks = 3
)

// ParseArgs parses command line arguments and returns a Config struct
func ParseArgs() *Config {
	var cfg Config

	// Define command line flags
	flag.BoolVar(&cfg.ShowHelp, "help", false, "Show help message")
	flag.BoolVar(&cfg.ShowVersion, "version", false, "Show version")
	flag.StringVar(&cfg.SubscriptionUrl, "sub-url", DefaultSubscriptionUrl, "Subscription URL for proxy servers")
	flag.StringVar(&cfg.ProxyTargetUrl, "target-url", DefaultProxyTargetUrl, "Target URL to check the proxy against")
	flag.BoolVar(&cfg.UpdateCache, "update-cache", false, "Update proxy cache periodically")
	flag.BoolVar(&cfg.RunTester, "run-tester", false, "Run proxy tester to find working proxies")
	flag.IntVar(&cfg.ConcurrentChecks, "concurrent-checks", DefaultConcurrentChecks, "Number of proxies to check concurrently")

	// Parse the flags
	flag.Parse()

	return &cfg
}
