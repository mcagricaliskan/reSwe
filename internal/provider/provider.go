package provider

import (
	"context"

	"github.com/cagri/reswe/internal/models"
)

// StreamCallback is called for each chunk of streaming response
type StreamCallback func(chunk string)

// Provider defines the interface for AI providers
type Provider interface {
	// Name returns the provider name
	Name() string

	// Chat sends a chat request and returns the full response
	Chat(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error)

	// ChatStream sends a chat request and streams the response via callback
	ChatStream(ctx context.Context, req models.ChatRequest, cb StreamCallback) (*models.ChatResponse, error)

	// ListModels returns available models
	ListModels(ctx context.Context) ([]string, error)

	// Ping checks if the provider is reachable
	Ping(ctx context.Context) error
}

// Registry manages available providers
type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
