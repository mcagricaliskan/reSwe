package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/cagri/reswe/internal/server"
)

func main() {
	port := flag.Int("port", 16147, "server port")
	dbPath := flag.String("db", "", "database path (default: ~/.reswe/data.db)")
	ollamaURL := flag.String("ollama-url", "http://localhost:11434", "Ollama API URL")
	dev := flag.Bool("dev", false, "development mode (serve frontend from filesystem)")
	flag.Parse()

	// Default DB path
	if *dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		dir := filepath.Join(home, ".reswe")
		os.MkdirAll(dir, 0755)
		*dbPath = filepath.Join(dir, "data.db")
	}

	cfg := server.Config{
		Port:       *port,
		DBPath:     *dbPath,
		OllamaURL:  *ollamaURL,
	}

	// Use embedded frontend unless in dev mode
	if !*dev {
		cfg.FrontendFS = getFrontendFS()
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	defer srv.Close()

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down...")
		cancel()
	}()

	if err := srv.Start(ctx, *port); err != nil {
		log.Printf("server stopped: %v", err)
	}
}
