# Contributing to docmap

Thanks for your interest in contributing!

## Development Setup

```bash
# Clone the repo
git clone https://github.com/JordanCoin/docmap.git
cd docmap

# Build the binary
go build -o docmap .

# Test it
./docmap README.md
./docmap .
```

## Running Tests

```bash
go test ./...
```

## Code Style

- Run `gofmt` before committing
- Run `go vet ./...` to check for issues
- Keep functions focused and readable

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and linting
5. Commit with a clear message
6. Push to your fork
7. Open a Pull Request

## What Makes a Good PR

- **Focused**: One feature or fix per PR
- **Tested**: Include tests for new functionality
- **Documented**: Update README if needed
- **Clean**: No unrelated changes

## Reporting Issues

When reporting bugs, include:

- docmap version (`docmap --version`)
- OS and version
- Steps to reproduce
- Expected vs actual behavior
- Sample markdown file if relevant

## Feature Requests

We love feature ideas! Please:

- Check existing issues first
- Describe the use case
- Explain why it's useful for LLM/human documentation navigation

## Code of Conduct

Be kind. Be respectful. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
