# Repository Guidelines

## Project Structure & Module Organization
`main.go` boots the app and wires the Fiber server. Backend code lives under `internal/`: `server/` for HTTP and WebSocket handlers, `agent/` for orchestration logic, `store/` for SQLite access, `provider/` for Ollama integration, `scanner/` for repo scanning, and `models/` for shared types. The React frontend lives in `frontend/src`, with route pages under `pages/`, reusable UI under `components/`, and browser utilities under `lib/`. Built artifacts land in `bin/` and `frontend/dist/`.

## Build, Test, and Development Commands
Use `make dev` to run the Go server in development mode and serve the frontend separately. Use `make frontend-dev` for the Vite hot-reload UI. Use `make build` to install frontend deps, build the SPA, and produce `bin/reswe`. Use `make run` or `make start` to launch the embedded binary. Use `cd frontend && npm run lint` to check the TypeScript/React code. Use `go test ./...` for backend tests when adding or changing Go logic.

## Coding Style & Naming Conventions
Follow default Go formatting with `gofmt`; keep packages lowercase and group related files by feature (`handlers_task.go`, `orchestrator.go`). In the frontend, use TypeScript with 2-space indentation, PascalCase for components and page files, and `use...` camelCase for hooks. Prefer the existing `@/` import alias for frontend modules. Linting is handled by ESLint 9 in `frontend/eslint.config.js`.

## Testing Guidelines
There are currently no committed automated test files, so new behavior should include tests where practical. For backend changes, add table-driven `*_test.go` files next to the package being changed and run `go test ./...`. Frontend test tooling is not configured yet; at minimum, verify affected flows with `npm run lint`, `make build`, and a manual pass in `make frontend-dev`.

## Commit & Pull Request Guidelines
Match the existing Conventional Commit style: `feat: ...`, `fix: ...`, `chore: ...`. Keep PRs scoped, explain user-visible changes, and list local verification steps. Include screenshots for UI changes and note any config or schema impact. If you change the SQLite schema, tell reviewers to reset the local DB with `rm -f ~/.reswe/data.db && ./bin/reswe`.

## Configuration Notes
The app defaults to port `16147`, stores data in `~/.reswe/data.db`, and expects Ollama at `http://localhost:11434` unless overridden with flags.
