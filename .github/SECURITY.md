# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in docmap, please report it by emailing the maintainers directly rather than opening a public issue.

## Scope

docmap is a local CLI tool that:
- Reads markdown files from disk
- Parses and displays their structure
- Does not make network requests
- Does not execute code from parsed files

Security concerns would primarily involve:
- Path traversal issues
- Denial of service via malformed input
- Unexpected behavior with symlinks

## Response

We will acknowledge receipt within 48 hours and provide a timeline for fixes.
