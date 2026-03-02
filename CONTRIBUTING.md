# Contributing to Shout!

Thank you for your interest in contributing to Shout!

## Prerequisites

### Common Lisp (develop branch)

- [SBCL](http://www.sbcl.org/) built with `--fancy` (or Roswell/Homebrew SBCL)
- GNU Make

### Go rewrite (norm/go-shout branch)

- [Go 1.26+](https://go.dev/dl/)
- GNU Make

> **Note:** The Go rewrite is the future direction for Shout! New features should
> target the Go codebase. The Lisp codebase on `develop` receives security fixes
> and maintenance only.

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

### Common Lisp (develop branch)

```bash
make quicklisp    # Set up Quicklisp (uses vendored deps if available)
make libs         # Install Lisp dependencies
make shout        # Build standalone executable
make test         # Run test suite (prove framework)
make coverage     # Generate coverage report
```

Run in development without compiling:
```bash
sbcl --script run.lisp
```

### Go rewrite (norm/go-shout branch)

```bash
make -f Makefile-go build    # Build the binary
make -f Makefile-go test     # Run fmt + vet + tests with race detection
make -f Makefile-go coverage # Generate coverage report
```

### Code Quality

All submissions must pass the relevant test suite:

- **Lisp:** `make test`
- **Go:** `make -f Makefile-go test` (runs `go fmt` + `go vet` + `go test -race`)

### Security Scanning

```bash
make -f Makefile-go security  # Run gosec + govulncheck + trivy (Go only)
```

## Submitting Changes

1. Ensure all tests pass for the branch you're targeting
2. Commit your changes with a clear message describing the **why**
3. Push to your fork: `git push origin my-new-feature`
4. Open a Pull Request against the appropriate branch (`develop` for Lisp, `main` for Go)

### Pull Request Guidelines

- Keep PRs focused on a single change
- Include tests for new functionality
- Update documentation if behavior changes
- Reference related issues in the PR description

## Reporting Bugs

Open a [GitHub Issue](https://github.com/cloudfoundry-community/shout/issues) with:

- Steps to reproduce
- Expected vs actual behavior
- Version information (`shout --version` or `GET /info`)

## Security Vulnerabilities

**Do not open a public issue for security vulnerabilities.**
See [SECURITY.md](SECURITY.md) for responsible disclosure instructions.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
