# Contributing to NyxProxy-OSS

Thank you for your interest in contributing to NyxProxy-OSS! This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful, inclusive, and constructive in all interactions.

## How to Contribute

### Reporting Bugs

Before creating a bug report:
1. Check existing [issues](https://github.com/ikenxuan/nyxproxy-oss/issues)
2. Verify you're using the latest version
3. Collect relevant information (logs, config, system details)

Create a bug report with:
- Clear title and description
- Steps to reproduce
- Expected vs actual behavior
- Environment details (OS, Go version, etc.)
- Relevant logs and configuration (remove sensitive data!)

### Suggesting Features

Feature requests are welcome! Please:
1. Check if it's already requested
2. Explain the use case
3. Describe the proposed solution
4. Consider backward compatibility

### Pull Requests

1. **Fork and Clone**
   ```bash
   git clone https://github.com/YOUR-USERNAME/nyxproxy-oss.git
   cd nyxproxy-oss
   ```

2. **Create a Branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make Changes**
   - Write clean, readable code
   - Follow existing code style
   - Add tests if applicable
   - Update documentation

4. **Test Your Changes**
   ```bash
   gofmt -w .
   go vet ./...
   go test ./...

   go build -o nyxproxy ./cmd/proxy
   ./nyxproxy
   ```

5. **Commit**
   ```bash
   git add .
   git commit -m "Add feature: description"
   ```

   Good commit messages:
   - Use present tense ("Add feature" not "Added feature")
   - Be descriptive but concise
   - Reference issues if applicable (#123)

6. **Push and Create PR**
   ```bash
   git push origin feature/your-feature-name
   ```
   
   Then create a Pull Request on GitHub.

## Development Setup

### Prerequisites

- Go 1.27 or later
- Linux (the proxy manages Linux network interfaces)
- Node 24 LTS and pnpm 12.14.0 for the web console
- Git

### Build and Run

```bash
# Clone the repository
git clone https://github.com/ikenxuan/nyxproxy-oss.git
cd nyxproxy-oss

# Install dependencies
go mod download

# Build the embedded web console
pnpm --dir web install --frozen-lockfile
pnpm --dir web build

# Build the Go binary
go build -o nyxproxy ./cmd/proxy

# Run
./nyxproxy
```

### Frontend Development

`pnpm --dir web dev` starts the Next.js development server. It rewrites `/api`
requests to the backend at `http://127.0.0.1:18081` (override with
`NYXPROXY_API`), so the backend must be running first and the console must be
enabled in its `config.yaml` (`web.enabled: true`). The default console address
is `127.0.0.1:18081` in `config.example.yaml`.

Local frontend development also requires `web.trust_proxy_headers: true` in the
backend configuration. The Next rewrite changes the `Host` header, and the
backend's Origin check honors `X-Forwarded-Host` only when that flag is enabled;
without it, login and configuration changes fail with an origin mismatch.

### Project Structure

```
nyxproxy-oss/
├── cmd/
│   ├── proxy/          # Main proxy application
├── internal/
│   └── config/         # Configuration management
├── pkg/
│   ├── https/          # HTTPS proxy implementation
│   ├── socks5/         # SOCKS5 proxy implementation
│   ├── network/        # Network interface utilities
│   ├── metrics/        # Metrics collection
│   └── monitoring/     # HTTP monitoring server
├── docs/               # Documentation
├── .github/
│   └── workflows/      # CI/CD workflows
├── config.example.yaml # Example configuration
├── go.mod              # Go module definition
└── README.md           # Main documentation
```

## Coding Standards

### Go Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Use meaningful variable names
- Add comments for exported functions
- Keep functions focused and small

### Code Example

```go
// GetOutboundIP returns the preferred outbound IP for the given interface and protocol
func GetOutboundIP(interfaceName string, protocol int) (net.IP, error) {
    if interfaceName == "" {
        return getAutoOutboundIP(protocol)
    }
    
    // Implementation...
}
```

### Error Handling

Always handle errors explicitly:

```go
// Good
if err != nil {
    return fmt.Errorf("failed to create proxy: %w", err)
}

// Bad
_ = someFunction() // ignoring errors
```

### Configuration Changes

When adding configuration options:
1. Update `internal/config/config.go`
2. Update `config.example.yaml`
3. Update `docs/CONFIG.md`

## Testing

Currently, testing is manual. Automated tests are welcome contributions!

### Manual Testing Checklist

- [ ] SOCKS5 proxy works
- [ ] HTTPS proxy works
- [ ] Authentication works
- [ ] Interface selection works
- [ ] IPv4/IPv6 selection works
- [ ] Monitoring endpoints respond
- [ ] Config file loads and validation works

## Documentation

When contributing, please update:
- Code comments
- README.md (if user-facing changes)
- docs/CONFIG.md (if config changes)
- docs/MONITORING.md (if monitoring changes)

## Release Process

Maintainers handle releases. The process:

1. Update version in code
2. Create git tag: `git tag v1.0.0`
3. Push tag: `git push origin v1.0.0`
4. GitHub Actions builds and creates release
5. Update release notes on GitHub

## Questions?

Feel free to:
- Open an issue for questions
- Start a discussion
- Reach out to maintainers

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

Thank you for contributing to NyxProxy-OSS! 🎉
