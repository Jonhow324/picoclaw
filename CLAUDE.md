# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

PicoClaw is a lightweight personal AI assistant written primarily in Go. The main `picoclaw` Cobra CLI runs/configures the agent gateway; a separately built WebUI launcher embeds a React/Vite dashboard and manages that gateway. The Go module is `github.com/sipeed/picoclaw`, pinned to Go 1.25.10 (`GOTOOLCHAIN=local`).

For WebUI work, use Node.js 22+ and pnpm 10.33.0+; the frontend's `packageManager` field is authoritative. Do not substitute npm or yarn.

## Common commands

Run these from the repository root unless stated otherwise:

```bash
# Install and verify Go dependencies
make deps

# Build the core CLI (runs go generate first)
make build

# Build the embedded WebUI launcher (builds frontend into web/backend/dist)
make build-launcher

# Build both platform variants or clean artifacts
make build-all
make clean

# Run the core binary; append CLI arguments with ARGS
make run ARGS='status'

# Entire Go test suite (core plus web backend; frontend lint is run by web tests)
make test

# One focused core test. Include project build tags when invoking Go directly.
go test -tags goolm,stdjson -run '^TestName$' -v ./pkg/session/

# One focused Web backend test
(cd web/backend && go test -run '^TestName$' -v ./api/)

# Docker-backed integration suites
make integration-test

# Formatting, static analysis, lint, and full local CI-style check
make fmt
make vet
make lint
golangci-lint run --build-tags goolm,stdjson
make check
```

Web development commands:

```bash
# Install locked frontend dependencies
(cd web/frontend && pnpm install --frozen-lockfile)

# Start Vite only; API and WebSocket requests proxy to localhost:18800
(cd web/frontend && pnpm dev)

# Build a production frontend, or start frontend + backend together
(cd web/frontend && pnpm build)
make -C web dev

# Start only the Go launcher backend (expects a built core binary)
make -C web dev-backend

# Frontend checks: lint is read-only; check rewrites formatting and fixes ESLint issues
(cd web/frontend && pnpm lint)
(cd web/frontend && pnpm format)
(cd web/frontend && pnpm check)
```

The root Makefile uses `CGO_ENABLED=0` and build tags `goolm,stdjson` by default. Preserve these tags for direct `go test`, `go vet`, and `go build` calls unless deliberately testing another build variant. Docker integration tests require Docker Compose.

## Architecture

### Core runtime

- `cmd/picoclaw/main.go` constructs the root Cobra command. Subcommands in `cmd/picoclaw/internal/` own CLI-facing configuration, onboarding, agent, gateway, MCP, cron, skills, model, auth, migration, status, and update flows.
- `pkg/config/` is the source of truth for persisted configuration. `LoadConfig` performs version migration, merges environment variables, normalizes/validates channels and models, and supplies a default workspace. Use its load/save APIs rather than parsing configuration ad hoc.
- The gateway creates a `pkg/bus.MessageBus`, a provider chosen from `pkg/providers/`, a `pkg/channels.Manager`, and `pkg/agent.AgentLoop`. Channels publish inbound messages to the bus; `AgentLoop.Run` consumes them, resolves agent/session state, invokes the LLM and tools, then publishes outbound messages back through the bus for channels to deliver. The bus also decouples streaming through the channel manager's `StreamDelegate`.
- `pkg/agent/` is the central turn-processing layer. `NewAgentLoop` builds the multi-agent registry, fallback/rate-limit chain, state/runtime-event infrastructure, context manager, commands, and shared tool registries. It is the correct integration point for behavior that applies across agent turns.
- `pkg/providers/` maps `ModelConfig` protocol/auth settings to `LLMProvider` implementations. Add provider behavior through the factory/protocol path rather than scattering provider-specific handling in the agent loop.
- `pkg/channels/` contains messaging adapters and their lifecycle manager. Channel implementations translate external events into bus messages and consume outbound bus messages. Channel configuration is initialized centrally by `pkg/config`.
- Supporting cross-cutting packages include `pkg/session/` and `pkg/memory/` for persistent conversation state, `pkg/tools/` and `pkg/mcp/` for built-in/MCP capabilities, `pkg/skills/` for skill discovery/installation, `pkg/events/` for runtime events, `pkg/evolution/` for self-improvement flow, and `pkg/seahorse/` for context compaction.

### Web launcher and frontend

- `web/backend/main.go` is a separate Go binary (`picoclaw-launcher`). It resolves/onboards the app config, configures launcher authentication/listeners, builds an HTTP mux, registers `web/backend/api` routes, serves embedded frontend assets, and can auto-start the PicoClaw gateway.
- `web/backend/api/` owns REST handlers that read/update the same application config and manage gateway, models, channels, sessions, logs, MCP, OAuth, and launcher settings. Add API routes through `Handler.RegisterRoutes`; preserve its launcher auth and middleware boundaries.
- `web/frontend/` is React 19 + TypeScript + Vite + TanStack Router + Tailwind v4. File-based routes live in `src/routes/`; API clients are in `src/api/`; shared state is Jotai in `src/store/`; UI/features/hooks are separated under `src/components/`, `src/features/`, and `src/hooks/`.
- All frontend HTTP calls must use `launcherFetch()` from `src/api/http.ts`, which handles launcher authentication redirects. Do not bypass it. Use Jotai action functions rather than mutating atoms directly, Tailwind classes rather than inline styles, and `@tabler/icons-react` for icons.
- `pnpm build:backend` writes the frontend bundle to `web/backend/dist`, which the launcher embeds. `make -C web dev` first builds a development core binary, runs the backend on port 18800, and starts Vite; Vite proxies API and WebSocket traffic there.

## Repository conventions

- Go tests commonly use `stretchr/testify`, table-driven cases, and may use `t.Parallel()`.
- Go formatting and lint configuration are defined by the root Makefile and `.golangci.yaml`; use `make fmt` instead of ad-hoc formatting commands when preparing a broad Go change.
- Frontend source uses two-space indentation, LF line endings, and UTF-8 (`web/frontend/.editorconfig`).
- Integration suites are discovered under `integration/suites/`; see `integration/README.md` before adding or modifying one.
- The legacy `pico` channel is only for backward client compatibility; do not use it for new channel functionality.
