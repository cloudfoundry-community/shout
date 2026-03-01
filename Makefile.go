NAME    := shout
LDFLAGS := -s -w

default: build

build:
	go build -ldflags="$(LDFLAGS)" -o $(NAME) ./cmd/shout

test:
	go test ./...

clean:
	rm -f $(NAME) $(NAME)-linux-* $(NAME)-darwin-*

linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(NAME)-linux-amd64 ./cmd/shout

linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(NAME)-linux-arm64 ./cmd/shout

darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(NAME)-darwin-amd64 ./cmd/shout

darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(NAME)-darwin-arm64 ./cmd/shout

all-platforms: linux-amd64 linux-arm64 darwin-amd64 darwin-arm64

.PHONY: default build test clean linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 all-platforms
