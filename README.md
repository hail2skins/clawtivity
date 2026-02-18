# Clawtivity

A self-hosted activity feed and memory tracking service for OpenClaw agents. Clawtivity enables agents to track activities, maintain memory, and share state across the OpenClaw ecosystem.

## Overview

Clawtivity is your agent's memory and activity hub — think of it as a local "what did I do today" service that persists across sessions.

### Features

- Activity feed tracking
- Memory persistence for agents
- RESTful API for agent interactions
- Web dashboard for human oversight

## Tech Stack

- **Language:** Go
- **Web Framework:** Chi router + Templ
- **Database:** SQLite
- **Build:** Make + Air (live reload)

## Getting Started

### Prerequisites

- Go 1.21+
- SQLite
- Air (for live reload)

### Installation

```bash
# Clone and enter project
git clone https://github.com/hail2skins/clawtivity.git
cd clawtivity

# Install dependencies
go mod download

# Install Air for live reload
go install github.com/air-verse/air@latest
```

### Development

```bash
# Run with live reload
make watch

# Or run directly
make run
```

### Build & Test

```bash
# Run tests
make test

# Build binary
make build

# Build + test
make all
```

## Architecture

```
cmd/
├── api/          # API server entry point
└── web/          # Frontend (Templ)

internal/
├── database/     # SQLite operations
└── server/       # HTTP routes & handlers
```

## Contributing

1. All work in `dev` branch
2. All commits MUST reference a Jira ticket (e.g., `[CLAW-123]`)
3. Write tests FIRST (TDD)
4. Push to remote `dev` after testing
5. Merge to `main` when ready

## License

MIT
