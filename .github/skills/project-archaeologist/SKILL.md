---
name: project-archaeologist
description:
  Reverse-engineers a codebase into comprehensive, visual documentation. Use this skill whenever a user says they're lost in their own project, want to understand how a codebase works, need an architecture overview, want to document a service or folder, or says things like "I don't know what my files do", "I need a visual overview", "document my project", "map out the codebase", "create docs for my repo", or similar. Supports JS/TS, Go, Rust, C++, Python, Docker, docker-compose, YAML config files, and existing Markdown files. Produces multiple Markdown files with Mermaid diagrams — one index/overview file plus one deep-dive file per service or major layer.
---

# Project Archaeologist

You are a senior software architect reverse-engineering an existing codebase into
clear, visual, living documentation. Your job is to understand the project as it
actually is — not as it might ideally be — and surface that understanding in a way
that helps any developer immediately grasp what's going on.

## The Goal

Produce a set of interconnected Markdown files (with Mermaid diagrams) that give
both a bird's-eye view and a deep per-service/per-layer view of the project.
Think of it as building the documentation that *should have existed all along*.

---

## User Context

The user may provide context alongside their request — e.g. "this is a billing
service that handles subscriptions", "focus on the auth flow", "the `payments/`
folder is the most important part", or "we use event sourcing for orders".

Take this seriously. User context is ground truth — it tells you what the person
already knows, what they care most about, and where to focus your energy. Use it to:
- **Prioritize** which services or flows to document most deeply
- **Validate** your findings against what the user described (and call out any gaps
  or discrepancies you find)
- **Fill in intent** when code alone doesn't make the "why" clear

If the user gives no context at all, proceed with a full scan and document
everything equally.

---

## Phase 1 — Scan & Understand

Before writing anything, build a complete mental model of the project. Do this
systematically:

### 1a. Directory & file inventory

```bash
find . -type f | grep -Ev "node_modules|\.git|dist|build|__pycache__|\.cache|/testing/" | sort
```

Always exclude any folder named `testing` (at any depth in the project tree) from
both scanning and documentation. Test code is not production logic — documenting it
would add noise and dilute the signal. If you notice tests referencing behavior that
isn't obvious from the production code, you can reference that insight in the
edge-cases section without reading the test files directly.

Also read the root-level files first: `README.md`, `package.json`, `go.mod`,
`Cargo.toml`, `Makefile`, `docker-compose.yml`, `*.yaml`, `*.yml`, `.env.example`.
These tell you the project's purpose, dependencies, and runtime shape before you
dive into code.

### 1b. Understand the shape

Look at the top-level directory structure to identify:
- **Services / applications** — each major runnable unit (API server, worker,
  CLI tool, frontend app, etc.)
- **Shared libraries / packages** — internal shared code
- **Infrastructure / config** — Docker, k8s, CI/CD, environment configs
- **Data layer** — database schemas, migrations, ORM models

### 1c. Read strategically — depth-first per service

For each service or major module, read files in this order:
1. Entry points (`main.go`, `index.ts`, `app.py`, `cmd/`, `src/main.*`)
2. Route / handler definitions
3. Core business logic files (largest, most-imported, or most-referenced)
4. Data models / schema files
5. Configuration and environment handling
6. External integrations (API clients, SDKs, message queues)
7. Tests — they reveal intended behavior and edge cases

When reading, collect:
- What does this file/function *do* in plain English?
- What are its inputs and outputs?
- What other files/services does it call?
- What external systems does it talk to (DBs, queues, third-party APIs)?
- Any edge cases, error handling patterns, or known quirks worth noting?

### 1d. Parse existing docs and configs

Read all `.md` files (not just README), all `docker-compose*.yml` files, all
`*.yaml` / `*.yml` config files (k8s manifests, CI pipelines, env configs).
These often reveal deployment topology, environment differences, and intended
architecture that the code alone doesn't show.

---

## Phase 2 — Plan the Output Structure

Before writing docs, decide on the file structure:

```
docs/
├── INDEX.md              ← always created; project overview + master diagram
├── <service-a>.md        ← one file per service or major domain
├── <service-b>.md
├── data-models.md        ← if there's a meaningful data layer
├── api-contracts.md      ← if there are inter-service or external APIs
└── dependencies.md       ← if the dependency graph is complex enough to warrant it
```

Create a file per service/layer when it has meaningful depth. Avoid creating
stub files — if a layer is simple, fold it into INDEX.md instead.

---

## Phase 3 — Write the Docs

### INDEX.md — The Master Overview

Always create this first. It must contain:

**1. Project header with tech stack badges**

Use shield.io-style badges inline in Markdown to show at a glance what
technologies are in play. Example:

```markdown
![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?logo=typescript&logoColor=white)
![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)
```

**2. Project summary** (3-5 sentences)

What does this project do? Who uses it? What problem does it solve?
Write this in plain English, based on what you actually found in the code —
not what the README says the project *should* do.

**3. High-level architecture Mermaid diagram**

Show all major components and how they connect. Use `graph TD` or `C4Context`
style. Label all arrows with the protocol/mechanism (HTTP, gRPC, AMQP, SQL, etc.).

```mermaid
graph TD
    Client["Browser / Client"]
    API["API Server (Go)"]
    Worker["Background Worker"]
    DB[(PostgreSQL)]
    Redis[(Redis Cache)]
    Queue["Message Queue (RabbitMQ)"]

    Client -->|"HTTPS REST"| API
    API -->|"SQL"| DB
    API -->|"Publish job"| Queue
    API -->|"Cache read/write"| Redis
    Queue -->|"Consume"| Worker
    Worker -->|"SQL"| DB
```

Be precise. Only show connections that actually exist in the code.

**4. Table of contents** linking to each service/layer doc

```markdown
## Docs

| Section | Description |
|---|---|
| [API Server](./api-server.md) | Core REST API, auth, routing |
| [Worker](./worker.md) | Background job processing |
| [Data Models](./data-models.md) | Database schema and relationships |
```

**5. Tech stack summary table**

```markdown
| Layer | Technology | Version | Notes |
|---|---|---|---|
| API | Go | 1.22 | Gin framework |
| Database | PostgreSQL | 15 | Migrations via golang-migrate |
| Cache | Redis | 7 | Session + rate limiting |
| Frontend | React + TypeScript | 18 / 5.x | Vite build |
| Container | Docker Compose | — | Local dev only |
```

---

### Per-Service / Per-Layer Docs

Each service doc follows this structure:

#### 1. Purpose & Responsibility

One paragraph: what this service does, what it owns, and what it does NOT do.
Draw clear boundaries.

#### 2. Entry Point & Startup Sequence

Describe how the service starts. What does `main()` / `index.ts` do?
What config does it read? What connections does it establish?
Use a sequence diagram if startup order matters:

```mermaid
sequenceDiagram
    participant main
    participant config
    participant db
    participant server

    main->>config: Load env vars & validate
    main->>db: Open connection pool
    main->>server: Register routes
    main->>server: Listen on :8080
```

#### 3. Architecture & Internal Structure

Show how the internal layers/modules relate to each other.
For a backend service this is typically: handler → service → repository → DB.

```mermaid
graph TD
    Router["HTTP Router"] --> AuthMiddleware
    AuthMiddleware --> Handlers["Route Handlers"]
    Handlers --> Services["Business Logic / Services"]
    Services --> Repos["Repositories"]
    Repos --> DB[(Database)]
    Services --> Cache[(Redis)]
    Services --> ExtAPI["External APIs"]
```

#### 4. File / Module Map

A table of every significant file with a plain-English description of what it does:

| File | Purpose |
|---|---|
| `cmd/api/main.go` | Entry point; wires together config, DB, router |
| `internal/auth/jwt.go` | JWT generation and validation; handles refresh logic |
| `internal/user/service.go` | User business logic: create, update, deactivation |

Include every file that contains meaningful logic. Skip auto-generated files,
test fixtures, and trivial config files.

#### 5. Data Flow — Key Operations

Pick the 2-4 most important operations (e.g. "User login", "Create order",
"Process payment") and trace them end-to-end through the code:

```mermaid
sequenceDiagram
    participant Client
    participant Handler as POST /auth/login
    participant AuthService
    participant UserRepo
    participant DB
    participant Redis

    Client->>Handler: {email, password}
    Handler->>AuthService: Login(email, password)
    AuthService->>UserRepo: FindByEmail(email)
    UserRepo->>DB: SELECT * FROM users WHERE email=?
    DB-->>UserRepo: user row
    AuthService->>AuthService: bcrypt.CompareHash(hash, password)
    AuthService->>Redis: SET session:{userId} ttl=24h
    AuthService-->>Handler: JWT token
    Handler-->>Client: 200 {token}
```

Reference the actual function names and file paths where they live.

#### 6. API Contracts (if applicable)

For services that expose HTTP/gRPC/message-queue interfaces, document them:

**HTTP endpoints:**

| Method | Path | Auth | Request | Response | Notes |
|---|---|---|---|---|---|
| POST | `/auth/login` | None | `{email, password}` | `{token, expiresAt}` | Rate-limited: 5 req/min |
| GET | `/users/:id` | JWT | — | User object | 404 if not found |

**Message queue events (if applicable):**

| Event | Producer | Consumer | Payload | Notes |
|---|---|---|---|---|
| `order.created` | API | Worker | `{orderId, userId, items[]}` | Triggers fulfillment |

#### 7. Dependencies & Imports Graph

Show what this service depends on, both internal and external:

```mermaid
graph LR
    Service["api-server"]

    subgraph Internal
        SharedLib["shared/utils"]
        Models["shared/models"]
    end

    subgraph External
        PG["PostgreSQL"]
        Redis["Redis"]
        Stripe["Stripe API"]
        SendGrid["SendGrid"]
    end

    Service --> SharedLib
    Service --> Models
    Service --> PG
    Service --> Redis
    Service --> Stripe
    Service --> SendGrid
```

#### 8. Database / Data Models (in data-models.md or inline)

For each table/collection/entity, document:
- Fields with types and constraints
- Relationships (FK references, embedded docs, etc.)
- Indexes and why they exist
- Any soft-delete, audit, or versioning patterns

Use an ER diagram:

```mermaid
erDiagram
    USERS {
        uuid id PK
        string email UK
        string password_hash
        timestamp created_at
        timestamp deleted_at "soft delete"
    }
    ORDERS {
        uuid id PK
        uuid user_id FK
        string status
        decimal total
        timestamp created_at
    }
    ORDER_ITEMS {
        uuid id PK
        uuid order_id FK
        uuid product_id FK
        int quantity
        decimal unit_price
    }

    USERS ||--o{ ORDERS : "places"
    ORDERS ||--o{ ORDER_ITEMS : "contains"
```

#### 9. Edge Cases, Gotchas & Known Complexity

This section is what separates useful documentation from useless documentation.
For each significant edge case or non-obvious behavior you found, document it:

- What is the case?
- Where in the code is it handled?
- Why does it exist? (if inferable)
- What would break if it weren't there?

Examples of things to surface here:
- Retry logic and backoff strategies
- Race conditions and how they're guarded (mutexes, transactions, idempotency keys)
- Graceful shutdown sequences
- Hard-coded limits or thresholds
- Workarounds for third-party API quirks
- Files / functions that are load-bearing but look innocuous

#### 10. Architecture Decisions & Explanations

This section makes the documentation genuinely educational — not just a map of
what exists, but an explanation of *why* it's built that way and *how* it actually
works under the hood.

Whenever you encounter a non-trivial architectural pattern, design decision, or
language feature being used deliberately, explain it. The target reader is a
competent developer who may not be familiar with this particular pattern or
language idiom — write for them.

**The rule:** the first time a concept appears in the docs, explain it. If
`sync.Pool` shows up in the file map, don't just note it — explain what a pool is,
why you'd use one, and what this code specifically gains from it. Subsequent
references can just link back.

**What to look for and explain:**

*Concurrency patterns (especially Go):*
- **Goroutines** — explain what they are (lightweight threads managed by the Go
  runtime, not OS threads), why they're cheap to spawn, and what this code uses
  them for specifically
- **`sync.Pool`** — explain that pools reduce GC pressure by reusing allocated
  objects rather than discarding them; note what object is being pooled and at
  what call volume this starts to matter
- **`sync.WaitGroup`** — explain the fan-out/fan-in pattern it enables
- **Channels** — explain buffered vs unbuffered and which is used here and why
- **`context.Context`** — explain that it carries cancellation signals and
  deadlines across goroutine boundaries, and trace how it flows through this code

*Resource management:*
- **Connection pools** (DB, Redis, HTTP) — explain that opening a connection is
  expensive (TCP handshake, auth), so pools keep a set warm and lend them out;
  note the configured pool size and what happens when it's exhausted
- **Worker pools** — explain the bounded concurrency pattern: why you'd limit
  parallelism even when goroutines are cheap (backpressure, DB connection limits,
  downstream rate limits)

*Resilience patterns:*
- **Circuit breakers** — explain the three states (closed/open/half-open) and
  what failure threshold triggers opening
- **Exponential backoff with jitter** — explain why plain retry loops cause
  thundering herd and how jitter spreads the load
- **Idempotency keys** — explain why distributed systems need them (retries can
  cause duplicate side effects) and how this code implements the check

*Data & consistency patterns:*
- **Optimistic locking / versioning** — explain the compare-and-swap approach and
  when it beats pessimistic locking
- **Eventual consistency** — explain what operation accepts a stale read and why
  that's an acceptable trade-off here
- **Soft deletes** — explain the `deleted_at` pattern, why hard deletes are
  sometimes avoided (audit trails, foreign key safety), and any query implications

*Performance patterns:*
- **Caching strategy** — explain what is cached, for how long, what the
  invalidation strategy is, and what stale data would mean for correctness
- **Batch processing** — explain why batching reduces round-trips and what the
  trade-off on latency is
- **Lazy loading vs eager loading** — explain the N+1 query problem if relevant

**Format for each explanation:**

Write it as an inline callout right where the concept first appears — not buried
in a separate appendix. Use plain prose, not just a definition. Connect the
general concept to the specific code:

> **What is a `sync.Pool` and why is it used here?**
> A `sync.Pool` is a cache of reusable objects managed by the Go runtime.
> Normally, every request that needs a `bytes.Buffer` would allocate one from
> the heap — fine for low traffic, but at high request rates this creates a lot
> of short-lived garbage, which triggers frequent GC pauses. The pool keeps a
> set of pre-allocated buffers that goroutines can borrow and return. In this
> service (`internal/render/pool.go`), it's used to reuse JSON encoding buffers
> across requests. At the load this service handles (~5k req/s), benchmarking
> showed a ~30% reduction in GC pause time after introducing the pool.

Not every explanation needs that level of depth — calibrate to how non-obvious
the pattern is. A `sync.Mutex` needs one sentence. A custom sharded lock
implementation needs a paragraph.

---

## Phase 4 — Diagram Quality Rules

Every Mermaid diagram must:
- Have meaningful node labels (not just `A → B`)
- Label all edges with the mechanism (HTTP, SQL, event name, function call, etc.)
- Use subgraphs to group related nodes when the diagram has >6 nodes
- Be correct — only show connections that actually exist in the code

Prefer `graph TD` (top-down) for architecture and dependency diagrams.
Prefer `sequenceDiagram` for request/data flow traces.
Use `erDiagram` for data models.
Use `flowchart LR` for pipelines and CI/CD flows.

---

## Phase 5 — Output & File Naming

Save all documentation files into a `docs/` folder at the project root (or
wherever makes sense given the existing structure — e.g. alongside an existing
`docs/` folder).

Name files descriptively: `api-server.md`, `worker.md`, `data-models.md`,
`docker-infrastructure.md`, etc.

After creating all files, present them to the user and give a brief summary:
- How many services/layers were documented
- Which diagrams were created
- Anything notable you discovered (surprising complexity, undocumented behavior,
  potential issues) — this is valuable signal for the developer

---

## Language & Stack-Specific Tips

See the **Stack-Specific Scanning Tips** section at the bottom of this file for
detailed guidance on JS/TS, Go, Rust, C++, Docker, and YAML/config files —
what to look for first in each stack, common patterns to watch for, and gotchas
that are easy to miss.

For advanced Mermaid diagram patterns, see the **Mermaid Diagram Patterns**
section at the bottom of this file.
-e 
---

# Stack-Specific Scanning Tips

## JavaScript / TypeScript

**Start here:**
- `package.json` — scripts, dependencies, workspace structure, engines field
- `tsconfig.json` — module resolution, paths aliases, compilation targets
- `.env.example` — every required environment variable and what it does

**Read in this order:**
1. Framework entry point (`src/index.ts`, `src/app.ts`, `server.ts`, `next.config.js`)
2. Route definitions (Express: `app.use()` chains; Next.js: `pages/` or `app/` dir)
3. Middleware stack — document each middleware and what it does/guards
4. Service/business logic layer
5. Data access layer (ORM models, raw query files)

**Things to watch for:**
- **Barrel files** (`index.ts` re-exporting from a directory) — these reveal the
  intended public API of a module. A barrel that re-exports 20 things from 20 files
  is a module boundary, not just a convenience.
- **`async/await` error handling** — look for unhandled promise rejections.
  Missing `try/catch` around `await` in route handlers is a gotcha.
- **Circular imports** — TypeScript won't always catch these at compile time.
  If you see imports that seem oddly indirect, suspect a circular dependency workaround.
- **Environment-dependent behavior** — search for `process.env.NODE_ENV` to find
  all places where dev vs prod behavior diverges.
- **Type assertions (`as SomeType`)** — these are places where the developer opted
  out of type safety. Often worth noting.

**Framework-specific:**
- *Express*: middleware order matters enormously. Document it left-to-right.
- *Next.js*: `getServerSideProps` vs `getStaticProps` vs RSC — each has different
  caching and runtime implications worth documenting.
- *NestJS*: decorators are the architecture. Read module files (`*.module.ts`) to
  understand DI wiring.

---

## Go

**Start here:**
- `go.mod` — module name and Go version
- `go.sum` — (skip, but note if it's missing or stale)
- Top-level `Makefile` — reveals build, test, migration, and deployment commands

**Directory conventions:**
- `cmd/` — one subdirectory per binary/executable
- `internal/` — private packages (can't be imported by external modules)
- `pkg/` — public packages (intended for external use)
- `api/` or `proto/` — interface definitions (OpenAPI, protobuf)

**Read in this order:**
1. `cmd/<name>/main.go` — wiring: config loading, dependency injection, server startup
2. Router/handler registration
3. Middleware chain
4. Service layer (`internal/<domain>/service.go`)
5. Repository layer (`internal/<domain>/repository.go` or `store.go`)
6. Model/domain types (`internal/<domain>/domain.go` or `types.go`)

**Things to watch for:**
- **`context.Context` propagation** — if a function doesn't accept `ctx`, it can't
  be cancelled or timed out. Missing ctx is a gotcha, especially in DB calls.
- **`init()` functions** — can have surprising side effects (registering drivers,
  setting global state). Always document `init()` functions.
- **Goroutine lifecycle** — any `go func()` should have a clear owner that waits
  for it (via WaitGroup or channel). Orphaned goroutines are a leak.
- **Error wrapping** — `fmt.Errorf("... %w", err)` vs bare `err` return affects
  whether callers can use `errors.Is()` / `errors.As()`.
- **Graceful shutdown** — look for `os.Signal` channel + `context.WithCancel` pattern.
  Document the shutdown sequence if it exists.
- **Struct embedding** — promoted methods can be surprising. If a struct embeds
  another, the embedded type's methods appear as if they're the outer struct's own.

---

## Rust

**Start here:**
- `Cargo.toml` — workspace members, features, dependency versions
- `Cargo.lock` — (skip reading, but note if it's checked in; it should be for binaries)
- `src/lib.rs` vs `src/main.rs` — library, binary, or both?

**Things to watch for:**
- **`pub` visibility** — `pub`, `pub(crate)`, `pub(super)` reveal the intended API
  surface. Non-pub items are implementation details.
- **`impl Trait for Type` blocks** — these are often where the core logic lives.
  Search for `impl` blocks systematically; they won't always be in the obvious file.
- **Error types** — custom error enums (often `#[derive(Error)]` from `thiserror`)
  tell you what failure modes the author considered. Document the variants.
- **`unsafe` blocks** — always document these. What invariant is being upheld manually?
- **`Arc<Mutex<T>>`** — shared mutable state. Document what's inside and why it needs
  to be shared across threads.
- **Async runtime** — is it `tokio`, `async-std`, or something else? The runtime
  choice affects executor behavior.
- **Feature flags** — `#[cfg(feature = "...")]` gating means behavior varies by
  compile-time flags. Document which features exist and what they enable.

---

## C++

**Start here:**
- `CMakeLists.txt` — build targets, linked libraries, include paths
- `Makefile` (if present) — build commands
- Header files (`.h`, `.hpp`) — these are the API; read them before the `.cpp` files

**Things to watch for:**
- **Virtual method hierarchies** — find base classes with `virtual` methods; they
  reveal the intended extension points and polymorphism model.
- **Include guards vs `#pragma once`** — note if the codebase is inconsistent.
- **Memory management** — raw `new`/`delete` vs `std::unique_ptr`/`shared_ptr`.
  Document ownership semantics for any type that holds heap-allocated resources.
- **Global / static state** — global variables and static class members are
  initialization-order hazards. Document them.
- **Template heavy code** — templates can make data flow hard to follow.
  Document the primary template instantiations that actually get used at runtime.
- **Platform ifdefs** — `#ifdef _WIN32`, `#ifdef __linux__` etc. reveal
  platform-specific behavior branches. Document them.

---

## Docker / Docker Compose

**For each service in `docker-compose.yml`, document:**

| Field | What to look for |
|---|---|
| `image` / `build` | Is it using a prebuilt image or a local Dockerfile? |
| `ports` | What's exposed and on what host port? |
| `volumes` | Which paths are persisted? Which are bind-mounts for dev? |
| `environment` | What env vars does it need? Cross-reference with `.env.example` |
| `depends_on` | Startup ordering — note if health checks are used |
| `networks` | Which services can talk to each other? |
| `command` / `entrypoint` | Is the default CMD being overridden? Why? |

**For multi-stage Dockerfiles:**

Document each stage and its purpose:
- Stage 1 (`builder`): what it installs and builds
- Stage 2 (`runner`): what it copies from the builder and why (size optimization)
- Note what's deliberately excluded from the final image

**Common gotchas:**
- `depends_on` only waits for the container to *start*, not for the service inside
  to be *ready*. If the app has retry logic for DB connections, note it.
- Bind mounts in dev (`./src:/app/src`) mean local edits are reflected immediately,
  but this disappears in production. Note which volumes are dev-only.
- Named volumes persist between `docker-compose down` runs; anonymous volumes don't.
  Document which databases/queues use named volumes.

---

## YAML / Config Files

**CI/CD pipelines (GitHub Actions, GitLab CI, CircleCI, etc.):**
- Document the trigger conditions (push, PR, schedule, manual)
- Document each stage/job and what it does
- Note any environment-specific deployments (staging on merge to `main`,
  production on tag push)
- Surface any secrets referenced — these tell you what external services are used

**Kubernetes manifests:**

| Resource | What to document |
|---|---|
| `Deployment` | Replicas, image, resource requests/limits, env vars, health checks |
| `Service` | Type (ClusterIP/NodePort/LoadBalancer), ports, selector |
| `Ingress` | Hostnames, paths, TLS, annotations (rate limiting, auth) |
| `ConfigMap` | What config is externalized here and why |
| `Secret` | What secrets are referenced (not values — just names and purposes) |
| `HorizontalPodAutoscaler` | Min/max replicas, scaling metric |

**Environment / feature flag configs:**
- Document every environment variable in `.env.example` with type, default, and effect
- Document any feature flag system — which flags exist, what they control, how to
  toggle them

---

## Existing Markdown / Documentation

When `.md` files already exist in the project:
- Read them all — they contain the author's mental model, which may differ from
  the code reality
- Note discrepancies between what the docs say and what the code actually does
- Reference existing docs from your new docs (link to them, don't duplicate them)
- If existing docs are badly out of date, call this out explicitly in your INDEX.md
-e 
---

# Mermaid Diagram Patterns

Advanced patterns for documenting complex architectures.

---

## Architecture Diagrams (`graph TD`)

### Microservices with API Gateway

```mermaid
graph TD
    Client["Client (Browser / Mobile)"]
    GW["API Gateway / Load Balancer"]

    subgraph Services
        AuthSvc["Auth Service\n:8001"]
        UserSvc["User Service\n:8002"]
        OrderSvc["Order Service\n:8003"]
        NotifSvc["Notification Service\n:8004"]
    end

    subgraph Data
        AuthDB[(Auth DB\nPostgres)]
        UserDB[(User DB\nPostgres)]
        OrderDB[(Order DB\nMongo)]
        Cache[(Redis\nCache)]
    end

    subgraph Messaging
        Queue["Message Broker\n(RabbitMQ / Kafka)"]
    end

    Client -->|"HTTPS"| GW
    GW -->|"JWT verify"| AuthSvc
    GW -->|"REST"| UserSvc
    GW -->|"REST"| OrderSvc
    AuthSvc --> AuthDB
    UserSvc --> UserDB
    UserSvc --> Cache
    OrderSvc --> OrderDB
    OrderSvc -->|"order.created"| Queue
    Queue -->|"consume"| NotifSvc
```

### Monolith with internal layers

```mermaid
graph TD
    subgraph "HTTP Layer"
        Router["Router / Middleware"]
        Handlers["Route Handlers"]
    end

    subgraph "Business Layer"
        Services["Services"]
        Domain["Domain Models"]
    end

    subgraph "Data Layer"
        Repos["Repositories"]
        Migrations["DB Migrations"]
    end

    subgraph "External"
        DB[(Database)]
        Cache[(Cache)]
        ExtAPI["Third-party APIs"]
    end

    Router --> Handlers
    Handlers --> Services
    Services --> Domain
    Services --> Repos
    Services --> ExtAPI
    Repos --> DB
    Services --> Cache
```

---

## Sequence Diagrams — Common Patterns

### Auth flow with token refresh

```mermaid
sequenceDiagram
    participant C as Client
    participant A as API
    participant Auth as Auth Service
    participant DB

    C->>A: GET /protected (Bearer: expired-token)
    A->>Auth: ValidateToken(token)
    Auth-->>A: 401 TokenExpired
    A-->>C: 401 {error: "token_expired"}

    C->>A: POST /auth/refresh (refreshToken)
    A->>Auth: RefreshToken(refreshToken)
    Auth->>DB: SELECT session WHERE refresh_token=?
    DB-->>Auth: session row
    Auth->>Auth: Generate new access + refresh tokens
    Auth->>DB: UPDATE session SET refresh_token=new_token
    Auth-->>A: {accessToken, refreshToken}
    A-->>C: 200 {accessToken, refreshToken}
```

### Async job processing

```mermaid
sequenceDiagram
    participant Client
    participant API
    participant Queue
    participant Worker
    participant DB
    participant Email

    Client->>API: POST /orders
    API->>DB: INSERT order (status=pending)
    API->>Queue: Publish order.created {orderId}
    API-->>Client: 202 Accepted {orderId}

    Note over Queue,Worker: Async — may be seconds or minutes later

    Queue->>Worker: Deliver order.created
    Worker->>DB: SELECT order WHERE id=?
    Worker->>Worker: Process order logic
    Worker->>DB: UPDATE order SET status=fulfilled
    Worker->>Email: Send confirmation email
    Worker->>Queue: ACK message
```

### Optimistic locking / conflict resolution

```mermaid
sequenceDiagram
    participant C1 as Client A
    participant C2 as Client B
    participant API
    participant DB

    C1->>API: GET /doc/123 → {content, version: 5}
    C2->>API: GET /doc/123 → {content, version: 5}

    C1->>API: PUT /doc/123 {content: "A's edit", version: 5}
    API->>DB: UPDATE doc SET content=? WHERE id=123 AND version=5
    DB-->>API: 1 row updated
    DB->>DB: version becomes 6
    API-->>C1: 200 OK {version: 6}

    C2->>API: PUT /doc/123 {content: "B's edit", version: 5}
    API->>DB: UPDATE doc SET content=? WHERE id=123 AND version=5
    DB-->>API: 0 rows updated (version mismatch)
    API-->>C2: 409 Conflict — please re-fetch and retry
```

---

## ER Diagrams — Common Patterns

### Multi-tenant SaaS

```mermaid
erDiagram
    TENANTS {
        uuid id PK
        string name
        string plan
        timestamp created_at
    }
    USERS {
        uuid id PK
        uuid tenant_id FK
        string email UK
        string role
        timestamp created_at
    }
    RESOURCES {
        uuid id PK
        uuid tenant_id FK
        uuid created_by FK
        string type
        jsonb data
        timestamp updated_at
    }

    TENANTS ||--o{ USERS : "has"
    TENANTS ||--o{ RESOURCES : "owns"
    USERS ||--o{ RESOURCES : "creates"
```

### Event sourcing / audit log

```mermaid
erDiagram
    AGGREGATES {
        uuid id PK
        string type
        int version
        timestamp updated_at
    }
    EVENTS {
        uuid id PK
        uuid aggregate_id FK
        string event_type
        int sequence
        jsonb payload
        uuid caused_by_user FK
        timestamp occurred_at
    }
    SNAPSHOTS {
        uuid id PK
        uuid aggregate_id FK
        int version
        jsonb state
        timestamp created_at
    }

    AGGREGATES ||--o{ EVENTS : "sourced by"
    AGGREGATES ||--o| SNAPSHOTS : "cached as"
```

---

## Flowcharts for Pipelines / CI/CD

```mermaid
flowchart LR
    Push["git push"] --> CI

    subgraph CI ["CI Pipeline"]
        Lint["Lint & Type Check"]
        Test["Unit Tests"]
        Build["Docker Build"]
        Scan["Security Scan"]

        Lint --> Test --> Build --> Scan
    end

    Scan --> Gate{All checks pass?}
    Gate -- Yes --> Deploy["Deploy to Staging"]
    Gate -- No --> Fail["Fail & Notify"]

    Deploy --> SmokeTest["Smoke Tests"]
    SmokeTest --> Approve{Manual approval}
    Approve -- Approved --> Prod["Deploy to Production"]
    Approve -- Rejected --> Rollback["Rollback Staging"]
```

---

## Tips for Large Diagrams

**When a diagram gets too crowded (>10 nodes):**
1. Split into two diagrams — one for the macro view, one zooming into a subsystem
2. Use subgraphs to visually cluster related nodes
3. Collapse a group of related services into a single node with a note that it's
   expanded in the service-specific doc

**Labeling conventions:**
- Databases: use `[(Name)]` syntax for cylinder shape
- External services: use `["Name (external)"]` or a different subgraph
- Queues/brokers: use `>"Name"]` (flag shape) or just a box with a label

**Keep diagrams honest:**
- Never show a connection that doesn't actually exist in the code
- If you're unsure whether two services communicate, leave the edge out and note
  the uncertainty in the text below the diagram
- Add a `Note` below complex diagrams explaining non-obvious edges
