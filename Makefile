BINARY := bin/alert-copilot
IMAGE  ?= ghcr.io/rakaprad/alert-copilot:dev

.PHONY: all build test vet lint run sample docker clean

all: vet test build

build:
	CGO_ENABLED=0 go build -trimpath -o $(BINARY) ./cmd/alert-copilot

test:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# Loads .env if present, then runs the service locally.
run:
	set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/alert-copilot

sample:
	./scripts/send-sample.sh

docker:
	docker build -t $(IMAGE) .

clean:
	rm -rf bin
