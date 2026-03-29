package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/cagri/reswe/internal/agent"
	"github.com/cagri/reswe/internal/models"
	"github.com/cagri/reswe/internal/provider"
	"github.com/cagri/reswe/internal/scanner"
	"github.com/cagri/reswe/internal/store"
)

type Server struct {
	store        store.Store
	hub          *Hub
	orchestrator *agent.Orchestrator
	registry     *provider.Registry
	scanner      *scanner.Scanner
	app          *fiber.App
	frontendFS   fs.FS
}

type Config struct {
	Port       int
	DBPath     string
	OllamaURL string
	FrontendFS fs.FS
}

func New(cfg Config) (*Server, error) {
	db, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	registry := provider.NewRegistry()
	ollama := provider.NewOllama(cfg.OllamaURL)
	registry.Register(ollama)

	hub := NewHub()
	sc := scanner.New()

	orch := agent.NewOrchestrator(db, registry, sc, func(msg models.WSMessage) {
		hub.Broadcast(msg)
	})

	s := &Server{
		store:        db,
		hub:          hub,
		orchestrator: orch,
		registry:     registry,
		scanner:      sc,
		frontendFS:   cfg.FrontendFS,
	}

	s.setupRoutes()
	return s, nil
}

func (s *Server) setupRoutes() {
	app := fiber.New(fiber.Config{
		BodyLimit:    50 * 1024 * 1024,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Accept", "Content-Type"},
	}))

	// API routes
	api := app.Group("/api")

	// Projects
	api.Get("/projects", s.handleListProjects)
	api.Post("/projects", s.handleCreateProject)
	api.Get("/projects/:id", s.handleGetProject)
	api.Put("/projects/:id", s.handleUpdateProject)
	api.Delete("/projects/:id", s.handleDeleteProject)

	// Repos
	api.Post("/projects/:id/reconcile", s.handleReconcile)
	api.Post("/projects/:id/repos", s.handleAddRepo)
	api.Get("/projects/:id/repos", s.handleListRepos)
	api.Delete("/repos/:repoId", s.handleDeleteRepo)
	api.Post("/projects/:id/scan-directory", s.handleScanDirectory)
	api.Post("/discover-repos", s.handleDiscoverRepos)
	api.Post("/analyze-folder", s.handleAnalyzeFolder)

	// Tasks
	api.Get("/projects/:id/tasks", s.handleListTasks)
	api.Post("/projects/:id/tasks", s.handleCreateTask)
	api.Get("/tasks/:taskId", s.handleGetTask)
	api.Put("/tasks/:taskId", s.handleUpdateTask)
	api.Delete("/tasks/:taskId", s.handleDeleteTask)

	// Agent phases info & prompt preview
	api.Get("/phases", s.handleGetPhases)
	api.Get("/tasks/:taskId/preview/:phase", s.handlePreviewPrompt)

	// Agent actions
	api.Post("/tasks/:taskId/plan", s.handlePlan)
	api.Post("/tasks/:taskId/plan/chat", s.handlePlanChat)
	api.Post("/tasks/:taskId/plan/answer", s.handlePlanAnswer)
	api.Get("/tasks/:taskId/plan/messages", s.handleListPlanMessages)
	api.Get("/tasks/:taskId/questions", s.handleGetPendingQuestions)
	api.Post("/tasks/:taskId/execute", s.handleExecute)

	// Chat sessions
	api.Get("/tasks/:taskId/sessions", s.handleListSessions)
	api.Get("/sessions/:sessionId", s.handleGetSession)
	api.Post("/sessions/:sessionId/restore", s.handleRestoreSession)
	api.Post("/clarifications/:clarificationId/answer", s.handleAnswer)

	// Agent status & control
	api.Get("/tasks/:taskId/agent-status", s.handleGetAgentStatus)
	api.Post("/tasks/:taskId/cancel", s.handleCancelAgent)
	api.Get("/agents/active", s.handleGetActiveAgents)

	// Persisted agent runs & steps
	api.Get("/tasks/:taskId/runs", s.handleListAgentRuns)
	api.Get("/tasks/:taskId/runs/latest", s.handleGetLatestRun)
	api.Get("/runs/:runId", s.handleGetAgentRun)

	// Providers
	api.Get("/providers", s.handleListProviders)

	// Exclude Rules (global settings)
	api.Get("/exclude-rules", s.handleListExcludeRules)
	api.Post("/exclude-rules", s.handleCreateExcludeRule)
	api.Put("/exclude-rules/:id", s.handleUpdateExcludeRule)
	api.Delete("/exclude-rules/:id", s.handleDeleteExcludeRule)

	// Project exclude config
	api.Get("/projects/:id/exclude-config", s.handleGetProjectExcludeConfig)
	api.Post("/projects/:id/exclude-override", s.handleSetProjectExcludeOverride)
	api.Delete("/projects/:id/exclude-override", s.handleDeleteProjectExcludeOverride)
	api.Post("/projects/:id/custom-patterns", s.handleAddProjectCustomPattern)
	api.Delete("/custom-patterns/:patternId", s.handleDeleteProjectCustomPattern)

	// System / Utilities
	api.Post("/pick-directory", s.handlePickDirectory)

	// WebSocket
	app.Get("/ws", s.hub.HandleWS)

	// Frontend — serve embedded SPA
	if s.frontendFS != nil {
		app.Get("/*", s.serveFrontend)
	}

	s.app = app
}

func (s *Server) Start(ctx context.Context, port int) error {
	addr := fmt.Sprintf(":%d", port)

	go func() {
		<-ctx.Done()
		log.Println("shutting down server...")
		s.app.Shutdown()
	}()

	log.Printf("reSwe server starting on http://localhost%s", addr)
	return s.app.Listen(addr)
}

func (s *Server) Close() error {
	return s.store.Close()
}

// serveFrontend serves the embedded SPA. Tries the exact path first, falls back to index.html.
func (s *Server) serveFrontend(c fiber.Ctx) error {
	path := c.Path()
	if path == "/" {
		path = "/index.html"
	}

	// Try to open the requested file
	f, err := s.frontendFS.Open(path[1:]) // strip leading /
	if err == nil {
		f.Close()
		data, err := fs.ReadFile(s.frontendFS, path[1:])
		if err != nil {
			return c.Status(500).SendString("read error")
		}
		c.Set("Content-Type", getMimeType(path))
		return c.Send(data)
	}

	// SPA fallback — serve index.html for any non-file route
	data, err := fs.ReadFile(s.frontendFS, "index.html")
	if err != nil {
		return c.Status(500).SendString("index.html not found")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.Send(data)
}

func getMimeType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(path, ".woff"):
		return "font/woff"
	default:
		return "application/octet-stream"
	}
}

// Helper functions

func writeJSON(c fiber.Ctx, status int, v interface{}) error {
	if v == nil {
		return c.Status(status).JSON([]interface{}{})
	}
	// Go nil slices serialize as JSON null — use reflect to catch and return []
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return c.Status(status).JSON([]interface{}{})
	}
	return c.Status(status).JSON(v)
}

func writeError(c fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

// Bridge helpers
func models_ws_error(taskID int64, errMsg string) models.WSMessage {
	return models.WSMessage{
		Type:    models.WSTypeAgentError,
		TaskID:  taskID,
		Payload: map[string]interface{}{"error": errMsg},
	}
}

func (s *Server) orchestrator_registry_list() []string {
	return s.registry.List()
}

// unused but keep for reference
var _ = json.Marshal
var _ = http.StatusOK
