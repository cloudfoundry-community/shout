NAME    := shout
LDFLAGS := -s -w

default: build

build:
	go build -ldflags="$(LDFLAGS)" -o $(NAME) ./cmd/shout

test:
	go test -race ./cmd/... ./internal/... ./pkg/...

coverage:
	go test -coverprofile=coverage.out ./cmd/... ./internal/... ./pkg/...
	go tool cover -func=coverage.out

coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html

coverage-check:
	@go test -coverprofile=coverage.out ./cmd/... ./internal/... ./pkg/... > /dev/null 2>&1
	@TOTAL=$$(go tool cover -func=coverage.out | grep ^total: | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${TOTAL}%"; \
	if [ $$(echo "$${TOTAL} < 50" | bc) -eq 1 ]; then \
		echo "FAIL: coverage $${TOTAL}% is below 50% threshold"; \
		exit 1; \
	else \
		echo "OK: coverage meets 50% threshold"; \
	fi

clean:
	rm -f $(NAME) $(NAME)-linux-* $(NAME)-darwin-* coverage.out coverage.html

gosec:
	gosec ./...

govulncheck:
	govulncheck ./...

trivy:
	trivy fs --scanners vuln,secret,misconfig .

trivy-image:
	trivy image shout:latest

security: gosec govulncheck trivy

linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(NAME)-linux-amd64 ./cmd/shout

linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(NAME)-linux-arm64 ./cmd/shout

darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(NAME)-darwin-amd64 ./cmd/shout

darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(NAME)-darwin-arm64 ./cmd/shout

all-platforms: linux-amd64 linux-arm64 darwin-amd64 darwin-arm64

.PHONY: default build test clean coverage coverage-html coverage-check \
	gosec govulncheck trivy trivy-image security \
	linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 all-platforms
