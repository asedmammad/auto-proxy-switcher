# Auto Proxy Switcher

A lightweight Go application that automatically tests and switches between V2Ray proxies to maintain a stable connection.

## Features

- Automatic proxy testing and switching
- Support for VMESS and VLESS protocols
- Concurrent proxy testing
- Network connectivity monitoring
- Caching of working proxies
- Periodic proxy updates from a subscription URL

## Prerequisites

- Go 1.24 or higher
- V2Ray core installed and accessible from PATH
- Gnu/Linux (primary support)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/asedmammad/auto_proxy_switcher.git

2. Build the application:
git clone https://github.com/asedmammad/auto_proxy_switcher.git
cd auto_proxy_switchergo
go build -ldflags "-s -w"
``` 
## Usage
The application supports several command-line flags:

```bash
auto_proxy_switcher [flags]
Available flags:

-help: Show help message
-version: Show version information
-sub-url string: Subscription URL for proxy servers (default "https://some-sub-url.com/sub?max=100&type=vmess")
-target-url string: Target URL to check proxy connectivity (default "https://google.com")
-update-cache: Enable periodic proxy cache updates
-run-tester: Run proxy tester to find working proxies
-concurrent-checks int: Number of proxies to check concurrently (default 3)
```

### Example usage:

```bash
auto_proxy_switcher -run-tester -update-cache -concurrent-checks 5
```

## How It Works
1. The application loads proxy configurations from a subscription URL
2. It tests each proxy concurrently against a target URL
3. When a working proxy is found, it's configured as the active proxy
4. The application monitors network connectivity and automatically switches to another proxy if the current one fails
5. Proxy configurations are cached locally and updated periodically if enabled

## Configuration
The application stores its configuration and cache files in:

- Linux: `$HOME/.config/auto-proxy-switcher/`
- MacOS: `$HOME/Library/Application Support/auto-proxy-switcher/`
- Windows: `%APPDATA%\auto-proxy-switcher\`

## License
This project is open source and available under the MIT License.

## Contributing
Contributions are welcome! Please feel free to submit a Pull Request.
