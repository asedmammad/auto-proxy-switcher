package v2ray

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Constants for configuration values
const (
	DefaultHTTPPort     = 7778
	DefaultUserLevel    = 8
	DefaultEncryption   = "none"
	DefaultTag          = "proxy"
	DefaultHTTPTag      = "http"
	DefaultV2RayTimeout = 10 * time.Second
	DefaultTempDir      = "/tmp/v2ray"
	DefaultConfigFile   = "config.json"
)

// V2RayProcess represents a running v2ray instance
type V2RayProcess struct {
	ConfigPath string
	Cmd        *exec.Cmd
	StartTime  time.Time
}

type InboundSettings struct {
	UserLevel int `json:"userLevel"`
}

type Inbound struct {
	Port     int             `json:"port"`
	Protocol string          `json:"protocol"`
	Settings InboundSettings `json:"settings"`
	Tag      string          `json:"tag"`
}

type V2RayConfig struct {
	Inbounds  []Inbound  `json:"inbounds"`
	Outbounds []Outbound `json:"outbounds"`
}

type Outbound struct {
	Protocol       string         `json:"protocol"`
	Settings       Settings       `json:"settings"`
	StreamSettings StreamSettings `json:"streamSettings"`
	Tag            string         `json:"tag"`
}

type Settings struct {
	Vnext []VNext `json:"vnext,omitempty"`
}

type VNext struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Users   []User `json:"users"`
}

type User struct {
	ID         string `json:"id"`
	Encryption string `json:"encryption" default:"none"`
}

type StreamSettings struct {
	Network     string      `json:"network"`
	Security    string      `json:"security"`
	WSSettings  WSSettings  `json:"wsSettings"`
	TLSSettings TLSSettings `json:"tlsSettings"`
}

type WSSettings struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type TLSSettings struct {
	ServerName string `json:"serverName"`
}

// ProxyConfig represents the decoded base64 proxy configuration
type ProxyConfig struct {
	Protocol string `json:"-"` // Protocol type (e.g., vmess)
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

// UnmarshalJSON implements custom JSON unmarshaling for ProxyConfig
func (p *ProxyConfig) UnmarshalJSON(data []byte) error {
	// Create an auxiliary type to avoid recursive UnmarshalJSON calls
	type Aux struct {
		Name     string      `json:"name"`
		Add      string      `json:"add"`
		Host     string      `json:"host"`
		ID       string      `json:"id"`
		Net      string      `json:"net"`
		Path     string      `json:"path"`
		Port     json.Number `json:"port"` // Use json.Number to handle both string and number
		SNI      string      `json:"sni"`
		Security string      `json:"security"`
	}

	var aux Aux
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // Enable json.Number parsing
	if err := dec.Decode(&aux); err != nil {
		return err
	}

	// Convert port to integer
	port, err := aux.Port.Int64()
	if err != nil {
		return fmt.Errorf("failed to convert port to integer: %v", err)
	}

	// Copy values to the actual ProxyConfig
	p.Name = aux.Name
	p.Add = aux.Add
	p.Host = aux.Host
	p.ID = aux.ID
	p.Net = aux.Net
	p.Path = aux.Path
	p.Port = int(port)
	p.SNI = aux.SNI
	p.Security = aux.Security

	return nil
}

// ParseURL decodes a proxy URL and returns the proxy configuration
func ParseURL(proxyURL string) (*ProxyConfig, error) {
	// Extract protocol and remaining part
	parts := strings.Split(proxyURL, "://")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid proxy URL format")
	}

	protocol := parts[0]
	if protocol != "vmess" && protocol != "vless" {
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	var config ProxyConfig
	config.Protocol = protocol

	// Extract fragment (name) if present
	remaining := parts[1]
	fragmentParts := strings.Split(remaining, "#")
	if len(fragmentParts) > 1 {
		name, err := url.QueryUnescape(fragmentParts[1])
		if err != nil {
			return nil, fmt.Errorf("failed to decode proxy name: %v", err)
		}
		config.Name = name
		remaining = fragmentParts[0]
	}

	var err error
	switch protocol {
	case "vmess":
		err = parseVMESSConfig(&config, remaining)
	case "vless":
		err = parseVLESSConfig(&config, remaining)
	}

	if err != nil {
		return nil, err
	}

	return &config, nil
}

// parseVMESSConfig handles VMESS-specific configuration parsing
func parseVMESSConfig(config *ProxyConfig, data string) error {
	decodedBytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Errorf("failed to decode base64 URL: %v", err)
	}

	if err := json.Unmarshal(decodedBytes, config); err != nil {
		return fmt.Errorf("failed to parse JSON config: %v", err)
	}

	return nil
}

// parseVLESSConfig handles VLESS-specific configuration parsing
func parseVLESSConfig(config *ProxyConfig, data string) error {
	// Extract user info and server address
	userInfoAndRest := strings.Split(data, "@")
	if len(userInfoAndRest) != 2 {
		return fmt.Errorf("invalid vless URL format")
	}

	// Set the ID (user info)
	config.ID = userInfoAndRest[0]

	// Parse server address and parameters
	serverAndParams := strings.Split(userInfoAndRest[1], "?")
	if len(serverAndParams) != 2 {
		return fmt.Errorf("invalid vless URL format: missing parameters")
	}

	if err := parseVLESSServerAddress(config, serverAndParams[0]); err != nil {
		return err
	}

	if err := parseVLESSParameters(config, serverAndParams[1]); err != nil {
		return err
	}

	return nil
}

// parseVLESSServerAddress parses the server address and port for VLESS configuration
func parseVLESSServerAddress(config *ProxyConfig, serverAddr string) error {
	serverParts := strings.Split(serverAddr, ":")
	if len(serverParts) != 2 {
		return fmt.Errorf("invalid server address format")
	}

	config.Add = serverParts[0]
	port, err := strconv.Atoi(serverParts[1])
	if err != nil {
		return fmt.Errorf("invalid port number: %v", err)
	}
	config.Port = port

	return nil
}

// parseVLESSParameters parses the query parameters for VLESS configuration
func parseVLESSParameters(config *ProxyConfig, paramsStr string) error {
	params := strings.Split(paramsStr, "&")
	for _, param := range params {
		kv := strings.Split(param, "=")
		if len(kv) != 2 {
			continue
		}

		value, err := url.QueryUnescape(kv[1])
		if err != nil {
			return fmt.Errorf("failed to decode parameter value: %v", err)
		}

		switch kv[0] {
		case "type":
			config.Net = value
		case "security":
			config.Security = value
		case "path":
			config.Path = value
		case "host":
			config.Host = value
		case "sni":
			config.SNI = value
		}
	}
	return nil
}

// CreateConfigWithPort creates a V2Ray configuration file for the given proxy configuration with a custom HTTP port
func CreateConfigWithPort(proxyConfig *ProxyConfig, httpPort int, configFileName string) (string, error) {
	// Validate input
	if proxyConfig == nil {
		return "", fmt.Errorf("proxy config cannot be nil")
	}
	if proxyConfig.Protocol == "" {
		return "", fmt.Errorf("protocol not specified in proxy config")
	}
	if httpPort <= 0 {
		return "", fmt.Errorf("invalid HTTP port: %d", httpPort)
	}

	config := V2RayConfig{
		Inbounds: []Inbound{
			{
				Port:     httpPort,
				Protocol: "http",
				Settings: InboundSettings{
					UserLevel: DefaultUserLevel,
				},
				Tag: DefaultHTTPTag,
			},
		},
		Outbounds: []Outbound{
			{
				Protocol: proxyConfig.Protocol,
				Settings: Settings{
					Vnext: []VNext{
						{
							Address: proxyConfig.Add,
							Port:    proxyConfig.Port,
							Users: []User{
								{ID: proxyConfig.ID, Encryption: DefaultEncryption},
							},
						},
					},
				},
				StreamSettings: StreamSettings{
					Network:  proxyConfig.Net,
					Security: proxyConfig.Security,
					WSSettings: WSSettings{
						Path: proxyConfig.Path,
						Headers: map[string]string{
							"Host": proxyConfig.Host,
						},
					},
					TLSSettings: TLSSettings{
						ServerName: proxyConfig.SNI,
					},
				},
				Tag: DefaultTag,
			},
		},
	}

	// Create a unique config file name based on the port
	configFileName = fmt.Sprintf("config_%d.json", httpPort)

	// Create temporary directory if it doesn't exist
	if err := os.MkdirAll(DefaultTempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Use the provided config filename
	configPath := filepath.Join(DefaultTempDir, configFileName)
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, configJSON, 0644); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	return configPath, nil
}

// CreateConfig creates a V2Ray configuration file for the given proxy configuration
func CreateConfig(proxyConfig *ProxyConfig) (string, error) {
	// Use the default HTTP port and default config filename
	return CreateConfigWithPort(proxyConfig, DefaultHTTPPort, DefaultConfigFile)
}

// waitForV2Ray attempts to connect to V2Ray's HTTP port until it succeeds or times out
func waitForV2Ray(port int, timeout time.Duration) error {
	endTime := time.Now().Add(timeout)
	address := fmt.Sprintf("127.0.0.1:%d", port)
	const pollInterval = 100 * time.Millisecond

	for time.Now().Before(endTime) {
		conn, err := net.DialTimeout("tcp", address, pollInterval)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("v2ray failed to start on port %d within %v", port, timeout)
}

// extractPortFromConfigPath extracts the port number from the config file path
func extractPortFromConfigPath(configPath string) (int, error) {
	// Extract the filename from the path
	fileName := filepath.Base(configPath)
	
	// Check if it's the default config file
	if fileName == DefaultConfigFile {
		return DefaultHTTPPort, nil
	}
	
	// Try to extract the port number from the filename (format: config_PORT.json)
	parts := strings.Split(fileName, "_")
	if len(parts) != 2 {
		return DefaultHTTPPort, nil // Fall back to default port if format is unexpected
	}
	
	// Extract the port number part (removing the .json extension)
	portStr := strings.TrimSuffix(parts[1], ".json")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return DefaultHTTPPort, fmt.Errorf("failed to parse port from config filename: %w", err)
	}
	
	return port, nil
}

// RunV2Ray starts V2Ray with the given configuration file and waits for it to be ready
func RunV2Ray(configPath string) (*exec.Cmd, error) {
	cmd := exec.Command("v2ray", "run", "-config", configPath)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start v2ray: %w", err)
	}

	// Extract the port from the config file path
	port, err := extractPortFromConfigPath(configPath)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("failed to extract port from config path: %w", err)
	}
	
	// Wait for V2Ray to be ready on the specific port
	if err := waitForV2Ray(port, DefaultV2RayTimeout); err != nil {
		// If V2Ray doesn't become ready, kill the process and return error
		_ = cmd.Process.Kill() // Ignore error on kill as we're already handling an error
		return nil, fmt.Errorf("v2ray failed to become ready: %w", err)
	}

	return cmd, nil
}

// StopV2Ray stops the running V2Ray process
func StopV2Ray(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	
	// First try graceful termination
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		// If graceful termination fails, force kill
		return cmd.Process.Kill()
	}
	
	// Wait for process to exit with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	
	select {
	case <-time.After(3 * time.Second):
		// Process didn't exit in time, force kill
		return cmd.Process.Kill()
	case err := <-done:
		return err
	}
}
