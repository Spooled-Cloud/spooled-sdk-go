# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### How to Report

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to: **security@spooled.cloud**

Include the following information:

- Type of vulnerability (e.g., authentication bypass, injection, etc.)
- Full paths of source file(s) related to the vulnerability
- Location of the affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact assessment

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 1 week
- **Fix Timeline**: Depends on severity, typically 30-90 days
- **Public Disclosure**: After fix is released and users have time to update

### What to Expect

1. Confirmation that we received your report
2. Assessment of the vulnerability and its impact
3. A plan for addressing the vulnerability
4. Credit in the security advisory (if desired)

### Bug Bounty

We currently do not have a formal bug bounty program, but we deeply appreciate responsible disclosure and will acknowledge contributors in our security advisories.

## Security Best Practices

When using the Spooled Go SDK:

1. **Protect your API keys**: Never commit API keys to version control
2. **Use environment variables**: Store credentials in environment variables
3. **Rotate keys regularly**: Periodically rotate API keys
4. **Use minimal permissions**: Create API keys with only necessary permissions
5. **Keep updated**: Always use the latest SDK version

### Example: Secure Configuration

```go
// Good: Use environment variables
client, err := spooled.NewClient(
    spooled.WithAPIKey(os.Getenv("SPOOLED_API_KEY")),
)

// Bad: Hardcoded credentials
client, err := spooled.NewClient(
    spooled.WithAPIKey("sp_live_xxxxx"), // DON'T DO THIS
)
```

## Security Features

The SDK includes several security features:

- **TLS by default**: All connections use TLS encryption
- **Automatic token refresh**: JWT tokens are refreshed automatically
- **Request signing**: API requests are authenticated per-request
- **No credential logging**: Credentials are never logged
