# FlashDNS 🚀

A lightweight, high-performance caching DNS server built in Go for home networks. FlashDNS improves query response times and enhances privacy by caching DNS responses locally.

## Features

- ✅ **Fast Response Times**: Cached queries return in microseconds instead of milliseconds
- 🔒 **Enhanced Privacy**: Your devices only query your local server, reducing external DNS exposure
- ⚡ **Lightweight**: Minimal resource usage, perfect for home servers or Raspberry Pi
- 🎯 **Smart Caching**: Respects DNS TTL values for accurate results
- 📊 **Detailed Logging**: Logs cache hits/misses to `/var/log/dnsServer.log`
- 🛡️ **Memory Safe**: Built-in cache size limits prevent memory leaks
- 🔧 **Configurable**: Choose your upstream DNS provider and listening address

## How It Works

```
Your Device → FlashDNS (cache) → External DNS (Cloudflare/Google)
                ↓ if cached
              Returns immediately
```

1. Your device queries FlashDNS
2. If the domain is cached and not expired, FlashDNS returns it instantly
3. If not cached, FlashDNS queries the upstream DNS (like Cloudflare)
4. The response is cached with its TTL (Time To Live) for future requests
5. Result is returned to your device

## Installation

### Prerequisites

- Go 1.24 or higher
- Root/sudo access (DNS servers require port 53)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/jean0t/flashdns.git
cd flashdns

# Build
go build -o flashdns ./cmd/main.go

# Run (requires sudo for port 53)
sudo ./flashdns -s
```

### Quick Install

```bash
# Build and install to /usr/local/bin
go build -o flashdns ./cmd/main.go
sudo mv flashdns /usr/local/bin/
sudo chmod +x /usr/local/bin/flashdns
```

## Usage

### Basic Usage

```bash
# Start with default settings (listens on all interfaces, uses Cloudflare DNS)
sudo flashdns -s

# Specify custom listening address
sudo flashdns -a 192.168.1.100 -s

# Use Google DNS as upstream
sudo flashdns -d 8.8.8.8 -s

# Combine options
sudo flashdns -a 0.0.0.0 -d 1.1.1.1 -s
```

### Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `-s` | Start the server | `false` |
| `-a` | Address to listen on | `0.0.0.0` (all interfaces) |
| `-d` | Upstream DNS server | `1.1.1.1` (Cloudflare) |

### Popular Upstream DNS Providers

- **Cloudflare**: `1.1.1.1` (default)
- **Google**: `8.8.8.8`
- **Quad9**: `9.9.9.9`
- **OpenDNS**: `208.67.222.222`

## Configuration

### Configure Your Devices

After starting FlashDNS, configure your devices to use it as their DNS server:

**On Linux:**
```bash
# Edit /etc/resolv.conf
nameserver 192.168.1.100
```

**On Windows:**
- Control Panel → Network and Internet → Network Connections
- Right-click your connection → Properties
- Select IPv4 → Properties
- Use the following DNS server: `192.168.1.100`

**On macOS:**
- System Preferences → Network
- Select your connection → Advanced → DNS
- Add your FlashDNS server IP

**On Router (Recommended):**
Set FlashDNS as the primary DNS in your router's DHCP settings. This automatically configures all devices on your network.

### Running as a Service (systemd)

Create `/etc/systemd/system/flashdns.service`:

```ini
[Unit]
Description=FlashDNS Caching DNS Server
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/flashdns -a 0.0.0.0 -d 1.1.1.1 -s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable flashdns
sudo systemctl start flashdns
sudo systemctl status flashdns
```

## Project Structure

```
.
├── cmd
│   └── main.go              # Entry point
├── go.mod                   # Go module definition
├── internal
│   ├── cache
│   │   └── cache.go         # Caching logic with TTL support
│   ├── logger
│   │   └── logger.go        # Logging to /var/log/dnsServer.log
│   └── server
│       └── dnsServer.go     # DNS server implementation
├── LICENSE                  # MIT License
└── README.md               # This file
```

## Logs

FlashDNS logs all operations to `/var/log/dnsServer.log`:

```
2024-01-15 10:30:45 [INFO] DNS server listening on 0.0.0.0:53
2024-01-15 10:30:45 [INFO] Upstream DNS: 1.1.1.1:53
2024-01-15 10:30:50 [INFO] Cache MISS: google.com:1 - querying upstream
2024-01-15 10:30:51 [INFO] Cached google.com:1 with TTL: 300 seconds
2024-01-15 10:30:55 [INFO] Cache HIT: google.com:1
```

## Performance

Typical performance improvements:

- **Cached query**: 0.1-0.5 ms
- **Uncached query**: 10-50 ms (depends on upstream DNS)
- **Cache hit rate**: 70-90% for typical home usage

## Security & Privacy

FlashDNS enhances privacy by:
- Reducing the number of DNS queries visible to external DNS providers
- Keeping frequently accessed domains cached locally
- Allowing you to choose privacy-focused upstream DNS providers

**Note**: FlashDNS does not provide DNS-over-HTTPS (DoH) or DNS-over-TLS (DoT) because the goal is to be used in the internal network, typically your own house, although support is coming up next. It queries upstream DNS servers using standard UDP.

## Contributing

Contributions are welcome! Here's how you can help:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Setup

```bash
# Clone your fork
git clone https://github.com/jean0t/flash-dns.git
cd flashdns

# Install dependencies
go mod download

# Run tests (if available)
go test ./...

# Build
go build -o flashdns ./cmd/main.go
```

## Roadmap

- [ ] Encryptation (DNS-over-TLS)
- [ ] Web dashboard for cache statistics
- [ ] IPv6 support
- [ ] Configuration file support
- [ ] Docker image

## FAQ

**Q: Why do I need sudo/root access?**  
A: DNS servers traditionally use port 53, which requires elevated privileges on Unix systems.

**Q: Can I use this on a Raspberry Pi?**  
A: Absolutely! FlashDNS is lightweight and perfect for Raspberry Pi or similar devices.

**Q: Will this work with my existing router?**  
A: Yes, just point your router's DNS settings to your FlashDNS server IP.

**Q: What happens if FlashDNS goes down?**  
A: Your devices will lose DNS resolution. Consider setting a fallback DNS server in your device/router settings.

**Q: Does this support IPv6?**  
A: Currently, FlashDNS handles AAAA records (IPv6 addresses) but listens only on IPv4.

## Troubleshooting

**Server won't start:**
```bash
# Check if port 53 is already in use
sudo lsof -i :53

# Check logs
sudo tail -f /var/log/dnsServer.log
```

**No cache hits:**
- Ensure your devices are actually using FlashDNS as their DNS server
- Check logs to see if queries are reaching the server
- Verify the server is running: `sudo systemctl status flashdns`

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with Go's standard library
- Inspired by the need for faster, more private home DNS resolution
- Thanks to all contributors!

## Author

Your Name - [@jean0t](https://github.com/jean0t)

## Support

If you find FlashDNS useful, please consider:
- ⭐ Starring the repository
- 🐛 Reporting bugs
- 💡 Suggesting new features
- 🤝 Contributing code

---

**Made with ❤️ for faster, more private internet browsing**