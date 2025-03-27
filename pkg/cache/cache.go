package cache

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/asedmammad/auto_proxy_switcher/pkg/net"
)

const (
	cacheFileName = "data_cache.txt"
	maxBufferSize = 10 * 1024 * 1024 // 10MB
)

type retryConfig struct {
	maxRetries      int
	backoffDuration time.Duration
}

// Cache handles storing and retrieving data using a simple text file.
type Cache struct {
	FilePath string
	retry    retryConfig
}

// NewCache creates a new instance of Cache with the specified file path for caching.
func NewCache(dir string) *Cache {
	return &Cache{
		FilePath: filepath.Join(dir, cacheFileName),
		retry: retryConfig{
			maxRetries:      3,
			backoffDuration: time.Second * 3,
		},
	}
}

// Set appends new data to the cache file.
func (c *Cache) Set(data string) error {
	if err := c.ensureDirectoryExists(); err != nil {
		return err
	}

	return c.writeToFile(data)
}

func (c *Cache) ensureDirectoryExists() error {
	if err := os.MkdirAll(filepath.Dir(c.FilePath), 0755); err != nil {
		log.Printf("Error creating directory structure: %v", err)
		return fmt.Errorf("failed to create directory structure: %w", err)
	}
	return nil
}

func (c *Cache) writeToFile(data string) error {
	f, err := os.OpenFile(c.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Error opening or creating cache file: %v", err)
		return fmt.Errorf("failed to open or create cache file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(data + "\n"); err != nil {
		log.Printf("Error writing data to cache file: %v", err)
		return fmt.Errorf("failed to write data to cache file: %w", err)
	}

	log.Printf("Data written to cache file successfully")
	return nil
}

// Get retrieves all cached data from the file.
func (c *Cache) Get() ([]string, error) {
	f, err := os.Open(c.FilePath)
	if err != nil {
		log.Printf("Error opening cache file: %v", err)
		return nil, fmt.Errorf("failed to open cache file: %w", err)
	}
	defer f.Close()

	return c.readLines(f)
}

func (c *Cache) readLines(f *os.File) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(f)
	buf := make([]byte, maxBufferSize)
	scanner.Buffer(buf, maxBufferSize)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading cache file: %v", err)
		return nil, fmt.Errorf("error reading cache file: %w", err)
	}

	log.Printf("Cache data retrieved successfully")
	return lines, nil
}

// Update retrieves data from the URL and updates the cache file with the new data.
func (c *Cache) Update(url string) error {
	log.Printf("Updating cache with URL: %s", url)
	
	requestData, err := c.fetchAndDecodeData(url)
	if err != nil {
		return err
	}

	log.Printf("Data fetched successfully, updating cache...")
	if err := c.Set(requestData); err != nil {
		log.Printf("Error updating cache file: %v", err)
		return fmt.Errorf("failed to update cache file: %w", err)
	}

	log.Printf("Cache updated successfully")
	return nil
}

func (c *Cache) fetchAndDecodeData(url string) (string, error) {
	client := net.NewHTTPClient()
	requestData, err := c.fetchRequestWithRetries(client, url)
	if err != nil {
		return "", fmt.Errorf("failed to get data from URL: %w", err)
	}

	decodedData, err := c.decodeBase64Data(requestData)
	if err != nil {
		return "", err
	}

	return string(decodedData), nil
}

func (c *Cache) decodeBase64Data(data string) ([]byte, error) {
	decodedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		log.Printf("Error decoding data from URL: %v", err)
		return nil, fmt.Errorf("failed to decode data from URL: %w", err)
	}
	log.Printf("Decoded data from URL: %s", decodedData)
	return decodedData, nil
}

func (c *Cache) fetchRequestWithRetries(client *net.HTTPClient, url string) (string, error) {
	var data string
	var lastErr error

	for retryCount := 0; retryCount < c.retry.maxRetries; retryCount++ {
		log.Printf("Attempt %d: Fetching data from URL: %s", retryCount+1, url)
		data, lastErr = client.SendRequest(url)
		if lastErr == nil {
			return data, nil
		}
		log.Printf("Attempt %d: Error fetching data from URL: %v", retryCount+1, lastErr)
		time.Sleep(c.retry.backoffDuration)
	}

	return "", lastErr
}
