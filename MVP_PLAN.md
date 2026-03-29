# reSwe - SWE Agent Orchestrator MVP Plan

## Vision

A lightweight, cross-platform SWE agent tool with a web UI that orchestrates AI providers to research, plan, and implement software engineering tasks across codebases.

---

## Tech Stack

| Layer | Choice | Why |
|-------|--------|-----|
| Backend | **Go 1.22+** | Single binary, cross-platform, great concurrency |
| Frontend | **React 19 + TypeScript + Vite + shadcn/ui + Tailwind** | Rich component ecosystem, strong typing, fast DX |
| Database | **SQLite** via `modernc.org/sqlite` | Pure Go (no CGO), zero config, portable |
| HTTP | **Fiber v3** | Fast, Express-like API, good middleware support |
| Real-time | **WebSocket** via Fiber WebSocket | Streaming agent output to UI |
| AI (v1) | **Ollama REST API** | Local, free, no auth, simple HTTP |
| Build | **Vite** (frontend) + `go:embed` | Fast builds, single binary output |

---

## Core Data Model

```
Project
  ├── id, name, description, created_at
  ├── repos[] (paths to local repositories)
  └── tasks[]
        ├── id, title, description, status, created_at
        ├── enhanced_description (AI-improved)
        ├── research_notes (AI research output)
        ├── implementation_plan (AI-generated plan)
        └── executions[] (implementation attempts)
              ├── provider, model, status
              ├── files_changed[]
              └── log[] (streaming output)
```

---

## Architecture

```
┌─────────────────────────────────────┐
│           Svelte Web UI             │
│  (Projects / Tasks / Agent Output)  │
└──────────┬──────────────────────────┘
           │ HTTP + WebSocket
┌──────────▼──────────────────────────┐
│           Go Backend                │
│                                     │
│  ┌─────────┐  ┌──────────────────┐  │
│  │ REST API │  │ WebSocket Hub    │  │
│  └────┬────┘  └───────┬──────────┘  │
│       │               │             │
│  ┌────▼───────────────▼──────────┐  │
│  │      Agent Orchestrator       │  │
│  │       (plan / execute)        │  │
│  └────────────┬──────────────────┘  │
│               │                     │
│  ┌────────────▼──────────────────┐  │
│  │     AI Provider Interface     │  │
│  │  ┌────────┐ ┌──────┐ ┌─────┐ │  │
│  │  │ Ollama │ │Claude│ │ GPT │ │  │
│  │  └────────┘ └──────┘ └─────┘ │  │
│  └───────────────────────────────┘  │
│               │                     │
│  ┌────────────▼──────────────────┐  │
│  │    Codebase Scanner           │  │
│  │  (file tree, file reader,     │  │
│  │   .gitignore, simple search)  │  │
│  └───────────────────────────────┘  │
│               │                     │
│  ┌────────────▼──────────────────┐  │
│  │    SQLite Storage             │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘
```

---

## Project Structure

```
reSwe/
├── main.go                     # Entry point, starts server
├── go.mod
├── go.sum
├── Makefile                    # Build commands
│
├── internal/
│   ├── server/
│   │   ├── server.go           # HTTP server setup, routes
│   │   ├── handlers_project.go # Project CRUD endpoints
│   │   ├── handlers_task.go    # Task CRUD + agent trigger endpoints
│   │   ├── handlers_settings.go # Settings endpoints
│   │   ├── picker.go           # Model picker
│   │   └── websocket.go        # WebSocket hub for streaming
│   │
│   ├── models/
│   │   └── models.go           # All data structs
│   │
│   ├── store/
│   │   ├── store.go            # DB interface
│   │   └── sqlite.go           # SQLite implementation
│   │
│   ├── agent/
│   │   ├── orchestrator.go     # Agent workflow coordinator
│   │   ├── loop.go             # ReAct agent loop
│   │   ├── plan.go             # Planning agent logic
│   │   ├── execute.go          # Execution agent logic
│   │   ├── tools.go            # Agent tool definitions
│   │   ├── prompts.go          # System prompts
│   │   └── tracker.go          # Progress tracking
│   │
│   ├── provider/
│   │   ├── provider.go         # AI provider interface
│   │   ├── ollama.go           # Ollama implementation
│   │   ├── claude.go           # Claude API (future)
│   │   └── openai.go           # OpenAI API (future)
│   │
│   └── scanner/
│       └── scanner.go          # Codebase file tree + reader
│
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   ├── src/
│   │   ├── App.tsx             # Root layout + routing
│   │   ├── main.tsx            # Entry
│   │   ├── lib/
│   │   │   └── api.ts          # HTTP + WebSocket client
│   │   ├── pages/
│   │   │   ├── Projects.tsx    # Project list + create
│   │   │   ├── Project.tsx     # Single project view
│   │   │   ├── ProjectSettings.tsx # Project settings
│   │   │   ├── Task.tsx        # Task detail + agent interaction
│   │   │   └── Settings.tsx    # AI provider config
│   │   └── components/ui/      # shadcn/ui components
│   └── dist/                   # Built output (embedded in Go)
│
└── embedded.go                 # go:embed directive for frontend/dist
```

---

## API Endpoints

### Projects
```
GET    /api/projects           # List all projects
POST   /api/projects           # Create project
GET    /api/projects/:id       # Get project details
PUT    /api/projects/:id       # Update project
DELETE /api/projects/:id       # Delete project
POST   /api/projects/:id/repos # Add repo path to project
```

### Tasks
```
GET    /api/projects/:id/tasks      # List tasks for project
POST   /api/projects/:id/tasks      # Create task
GET    /api/tasks/:id               # Get task details
PUT    /api/tasks/:id               # Update task
DELETE /api/tasks/:id               # Delete task
```

### Agent Actions
```
POST   /api/tasks/:id/plan          # Trigger planning agent
POST   /api/tasks/:id/execute       # Trigger execution agent
POST   /api/tasks/:id/chat          # Chat to clarify and refine plan
```

### System
```
GET    /api/providers               # List available AI providers
POST   /api/providers/test          # Test provider connection
WS     /ws                          # WebSocket for streaming updates
```

---

## Agent Workflow

Everything is plan-based. There are two phases: **Plan** and **Execute**. If the user needs clarification or wants to refine the approach, they chat with the agent to update the plan before executing.

### Phase 1: Plan
- AI scans repos in project (file tree, key files like README, package.json, go.mod)
- AI analyzes the codebase and task description using a ReAct loop with tools
- Generates a step-by-step implementation plan
- Lists files to create/modify, dependencies, risks
- User reviews the plan — if anything is unclear, user **chats** to clarify and the plan gets updated
- User approves the final plan

### Phase 2: Execute
- AI follows the approved plan
- Uses a ReAct loop with tools (read files, write files, search, etc.)
- Reads specific files, generates diffs/new files
- Changes shown in UI for review before applying
- User can accept/reject each change

---

## MVP Milestones

### M1: Foundation
- [x] Go project init, SQLite schema, basic CRUD API
- [x] Codebase scanner (file tree with .gitignore)
- [x] Ollama provider with streaming
- [x] Frontend skeleton: project list, create project

### M2: Task + Agent Infrastructure
- [x] Task CRUD in UI
- [x] WebSocket streaming infrastructure
- [x] ReAct agent loop with tool use
- [x] Settings page for provider config

### M3: Plan + Chat
- [ ] Planning agent with plan display
- [ ] Chat to clarify and refine plan
- [ ] Plan approval UI (accept/edit/reject)

### M4: Execute + Polish
- [ ] Execution agent that generates file changes
- [ ] Diff view in UI for reviewing changes
- [ ] Apply/reject changes
- [ ] Build pipeline: single binary with embedded frontend

---

## Key Design Decisions

### Why Ollama first?
- No API keys needed for getting started
- Runs locally, fast iteration during development
- REST API is simple: `POST /api/chat` with streaming
- Models like `qwen2.5-coder:7b` or `codellama` work well

### Why NOT fancy indexing for MVP?
- Embeddings, vector DBs, AST parsing = weeks of work
- Simple approach: file tree + read relevant files + let AI figure it out
- The AI context window is big enough for most repos if we're selective
- Can add RAG/embeddings in v2 if needed

### Context Strategy (MVP)
1. Scan file tree (names only) → send as context
2. AI identifies relevant files from tree
3. Read those files → send content as context
4. Two-pass approach keeps token usage reasonable

### Single Binary Distribution
```
go build -o reswe .
# That's it. One file. Runs anywhere.
```

---

## Prerequisites

- Go 1.22+
- Node.js 20+ (for frontend dev, not needed at runtime)
- Ollama installed and running (for AI features)

---

## Future (Post-MVP)

- Claude / OpenAI / Gemini providers
- Git integration (auto-branch, auto-commit)
- Semantic code search with embeddings
- Multi-agent parallel execution
- Task templates
- Team collaboration (shared server mode)
- VS Code extension that connects to reSwe
