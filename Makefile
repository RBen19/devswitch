BINARY := devswitch
VERSION ?= dev

.PHONY: build test vet install release demo

build:
	go build -ldflags "-X github.com/RBen19/devswitch/internal/cli.version=$(VERSION)" -o $(BINARY) ./cmd/devswitch

test:
	go test ./...
	python3 scripts/test_installer.py
	python3 scripts/test_release.py

vet:
	go vet ./...

install:
	go install -ldflags "-X github.com/RBen19/devswitch/internal/cli.version=$(VERSION)" ./cmd/devswitch

release:
	./scripts/release.sh "$(VERSION)"

demo: build
	vhs docs/demo.tape
