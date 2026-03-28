.PHONY: dev build clean frontend frontend-dev deps

# Default target
all: build

# Install Go dependencies
deps:
	go mod tidy

# Build frontend
frontend:
	cd frontend && npm install && npm run build

# Run frontend dev server (hot reload)
frontend-dev:
	cd frontend && npm run dev

# Build everything into a single binary
build: frontend deps
	go build -o bin/reswe .

# Run in development mode (frontend served separately)
dev: deps
	go run . -dev -port 8080

# Run with embedded frontend
run: build
	./bin/reswe

# Clean build artifacts
clean:
	rm -rf bin/ frontend/dist/ frontend/node_modules/

# Build for all platforms
release: frontend deps
	GOOS=darwin GOARCH=arm64 go build -o bin/reswe-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o bin/reswe-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build -o bin/reswe-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build -o bin/reswe-windows-amd64.exe .
