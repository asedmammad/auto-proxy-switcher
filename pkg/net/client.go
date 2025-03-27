package net

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Constants for HTTP client configuration
const (
	DefaultTimeout        = 7 * time.Second
	DefaultRetries        = 3
	DefaultDialTimeout    = 5 * time.Second
	DefaultKeepAlive      = 30 * time.Second
	DefaultMaxIdleConns   = 100
	DefaultIdleConnTimeout = 90 * time.Second
	DefaultTLSTimeout     = 5 * time.Second
	DefaultContinueTimeout = 1 * time.Second
    UserAgent = "auto-proxy-switcher/1.0"
)

// HTTPClient defines methods for making HTTP requests.
type HTTPClient struct{
	// HTTP client configuration
	Timeout    time.Duration
	MaxRetries int
	Transport  *http.Transport
}

// NewHTTPClient creates a new instance of HTTPClient with default configuration.
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		Timeout:    DefaultTimeout,
		MaxRetries: DefaultRetries,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   DefaultDialTimeout,
				KeepAlive: DefaultKeepAlive,
			}).DialContext,
			MaxIdleConns:          DefaultMaxIdleConns,
			IdleConnTimeout:       DefaultIdleConnTimeout,
			TLSHandshakeTimeout:   DefaultTLSTimeout,
			ExpectContinueTimeout: DefaultContinueTimeout,
		},
	}
}

// SendRequest sends an HTTP GET request to the specified URL and returns the response body as a string.
func (c *HTTPClient) createHTTPClient(timeout time.Duration, transport *http.Transport) *http.Client {
    log.Printf("Creating HTTP client with timeout: %v", timeout)
    if transport == nil {
        log.Printf("Using default transport as none was provided")
        transport = c.Transport
    }
    if transport == nil {
        log.Printf("WARNING: Both provided and default transport are nil")
    }
    
    return &http.Client{
        Timeout:   timeout,
        Transport: transport,
    }
}

func (c *HTTPClient) createRequest(ctx context.Context, method, url string) (*http.Request, error) {
    log.Printf("Creating request: method=%s, url=%s, ctx=%v", method, url, ctx != nil)
    var req *http.Request
    var err error
    
    if ctx != nil {
        req, err = http.NewRequestWithContext(ctx, method, url, nil)
    } else {
        req, err = http.NewRequest(method, url, nil)
    }
    
    if err != nil {
        log.Printf("Failed to create request: %v", err)
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    req.Header.Set("User-Agent", UserAgent)
    log.Printf("Request created successfully with User-Agent: %s", UserAgent)
    return req, nil
}

func (c *HTTPClient) SendRequest(url string) (string, error) {
    log.Printf("SendRequest called with URL: %s", url)
    
    if url == "" {
        log.Printf("Error: Empty URL provided")
        return "", fmt.Errorf("URL cannot be empty")
    }
    
    log.Printf("Creating HTTP client with timeout: %v", c.Timeout)
    client := c.createHTTPClient(c.Timeout, nil)
    
    log.Printf("Creating request object")
    req, err := c.createRequest(nil, http.MethodGet, url)
    if err != nil {
        log.Printf("Failed to create request: %v", err)
        return "", err
    }
    
    log.Printf("Executing HTTP request")
    resp, err := client.Do(req)
    if err != nil {
        log.Printf("Error during HTTP request: %v", err)
        return "", fmt.Errorf("error during HTTP request: %w", err)
    }
    defer resp.Body.Close()

    log.Printf("Response received with status code: %d", resp.StatusCode)
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        log.Printf("Unexpected status code: %d", resp.StatusCode)
        return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }

    log.Printf("Reading response body")
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Printf("Error reading response body: %v", err)
        return "", fmt.Errorf("error reading response body: %w", err)
    }

    log.Printf("Successfully completed request, response length: %d bytes", len(body))
    return string(body), nil
}

func (c *HTTPClient) handleProxyResponse(resp *http.Response, attempt int) error {
    if resp.Body != nil {
        _, _ = io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf("attempt %d: unexpected status code: %d", attempt+1, resp.StatusCode)
    }

    return nil
}

func (c *HTTPClient) performProxyAttempt(proxyClient *http.Client, req *http.Request, attempt int, proxyURL *url.URL) error {
    if attempt > 0 {
        backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
        log.Printf("Backing off for %v before retry", backoff)
        time.Sleep(backoff)
    }

    log.Printf("Testing proxy %s, attempt %d/%d", proxyURL.String(), attempt+1, c.MaxRetries)
    
    resp, err := proxyClient.Do(req)
    if err != nil {
        return fmt.Errorf("attempt %d failed: %w", attempt+1, err)
    }
    
    return c.handleProxyResponse(resp, attempt)
}

func (c *HTTPClient) TestProxy(targetURL string, proxyURL *url.URL, timeout time.Duration, retries ...int) (bool, error) {
    if targetURL == "" {
        return false, fmt.Errorf("target URL cannot be empty")
    }
    if proxyURL == nil {
        return false, fmt.Errorf("proxy URL cannot be nil")
    }

    maxRetries := c.MaxRetries
    if len(retries) > 0 && retries[0] > 0 {
        maxRetries = retries[0]
    }

    if timeout == 0 {
        timeout = c.Timeout
    }

    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    transport := c.Transport.Clone()
    transport.Proxy = http.ProxyURL(proxyURL)
    proxyClient := c.createHTTPClient(timeout, transport)

    req, err := c.createRequest(ctx, http.MethodGet, targetURL)
    if err != nil {
        return false, err
    }

    var lastErr error
    for attempt := 0; attempt < maxRetries; attempt++ {
        if err := c.performProxyAttempt(proxyClient, req, attempt, proxyURL); err != nil {
            lastErr = err
            log.Printf("Proxy test failed: %v", lastErr)
            continue
        }

        log.Printf("Proxy test successful: %s", proxyURL.String())
        return true, nil
    }

    return false, lastErr
}
