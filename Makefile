BINARY := devswitch
VERSION ?= 0.1.0

.PHONY: build test vet lint install

build:
	go build -ldflags "-X github.com/RBen19/devswitch/internal/cli.version=$(VERSION)" -o $(BINARY) ./cmd/devswitch

test:
	go test ./...

vet:
	go vet ./...

install:
	go install -ldflags "-X github.com/RBen19/devswitch/internal/cli.version=$(VERSION)" ./cmd/devswitch
