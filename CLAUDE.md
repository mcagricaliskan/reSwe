# CLAUDE.md

## DB Reset

After any database schema change (new tables, altered columns, migrations), remind the user to reset the DB:

```
rm -f ~/.reswe/data.db && ./bin/reswe
```

## Build

```
go build -o bin/reswe . && cd frontend && npm run build && cd ..
```

## Tech Stack

- Backend: Go + Fiber v3 + SQLite
- Frontend: React 19 + TypeScript + Vite + shadcn/ui + Tailwind
- Single binary with embedded frontend (`go:embed`)
