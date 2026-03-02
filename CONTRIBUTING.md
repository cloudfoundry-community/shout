# Contributing to Shout!

Thank you for your interest in contributing to Shout!

## Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- GNU Make

## Getting Started

1. Fork this repository
2. Clone your fork:
   ```bash
   git clone https://github.com/<your-username>/shout.git
   cd shout
   ```
3. Create a feature branch:
   ```bash
   git checkout -b my-new-feature
   ```

## Development Workflow

Build and test using the Go Makefile:

```bash
make -f Makefile-go build    # Build the binary
make -f Makefile-go test     # Run fmt + vet + tests with race detection
make -f Makefile-go coverage # Generate coverage report
```

### Code Quality

All submissions must pass:

- `go fmt` — standard formatting
- `go vet` — static analysis
- `go test -race` — tests with race detection

The `make -f Makefile-go test` target runs all three automatically.

### Security Scanning

```bash
make -f Makefile-go security  # Run gosec + govulncheck + trivy
```

## Submitting Changes

1. Ensure all tests pass: `make -f Makefile-go test`
2. Commit your changes with a clear message describing the **why**
3. Push to your fork: `git push origin my-new-feature`
4. Open a Pull Request against the `main` branch

### Pull Request Guidelines

- Keep PRs focused on a single change
- Include tests for new functionality
- Update documentation if behavior changes
- Reference related issues in the PR description

## Reporting Bugs

Open a [GitHub Issue](https://github.com/cloudfoundry-community/shout/issues) with:

- Steps to reproduce
- Expected vs actual behavior
- Version information (`shout --version`)

## Security Vulnerabilities

**Do not open a public issue for security vulnerabilities.**
See [SECURITY.md](SECURITY.md) for responsible disclosure instructions.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
