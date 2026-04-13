# Repository Guidelines

## Project Structure & Module Organization

Cryptorum is a self-hosted digital library app with a Go API and SvelteKit frontend.

- `backend/` is the Go module. The server entrypoint is `backend/cmd/server`; reusable packages live in `backend/internal`; migrations and sqlc queries are under `backend/internal/db`.
- `frontend/` is the SvelteKit app. Routes are in `frontend/src/routes`, shared code in `frontend/src/lib`, and public assets in `frontend/static`.
- `Dockerfile` and `docker-compose.yml` package the full app.
- Runtime data belongs in `data/`, `books/`, `bookdrop/`, and caches, not source changes.

## Build, Test, and Development Commands

- `cd backend && go run ./cmd/server` starts the API using `config.yaml`.
- `cd backend && go build -o cryptorum ./cmd/server` builds the API binary.
- `cd backend && go test ./...` runs Go tests and compile checks.
- `cd frontend && npm install` installs frontend dependencies.
- `cd frontend && npm run dev` starts the SvelteKit dev server.
- `cd frontend && npm run check` runs Svelte and TypeScript validation.
- `cd frontend && npm run build` builds the static frontend.
- `docker compose up -d --build` builds and starts the containerized app.
- `./test.sh` runs the smoke check; it expects frontend build artifacts.

## Coding Style & Naming Conventions

Format Go with `gofmt`; use short, lowercase package names. Keep HTTP handlers in `backend/cmd/server` unless behavior is reusable, then move it to `backend/internal/<domain>`. Add schema changes as numbered migrations in `backend/internal/db/migrations` and keep SQL in `backend/internal/db/queries`.

Use Svelte 5, TypeScript, 2-space indentation, and TypeScript semicolons. Name components `PascalCase.svelte`; name stores with camelCase under `frontend/src/lib/stores`.

## Testing Guidelines

Add Go tests beside the package under test as `*_test.go` with `TestName` functions. Prefer table-driven tests for scanner, metadata, auth, and database behavior.

No frontend test runner is configured; use `npm run check` and `npm run build`. Add focused tests when introducing one.

## Commit & Pull Request Guidelines

Recent commits use concise, imperative summaries such as `Fix progress tracking` and `Add bulk metadata support`. Start with a verb, keep the subject specific, and group related changes.

Pull requests should describe the user-visible change, list validation commands, mention schema or config changes, and include screenshots for UI changes.

## Security & Configuration Tips

Do not commit real credentials, private library files, generated databases, or cache contents. Treat `config.yaml`, cookies, book data, and Calibre cache output as local deployment state.
