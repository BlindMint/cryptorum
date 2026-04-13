# Cryptorum

A personal digital library application for self-hosting. Organize, read, and manage your ebook, comic, and audiobook collection from a single, streamlined interface.

## Features

- **Multiple Format Support**: EPUB, PDF, CBZ/CBR (comics), MP3/M4B/M4A (audiobooks)
- **Library Organization**: Organize books by custom libraries and shelves
- **Built-in Readers**: Read books directly in the app with dedicated readers for each format
- **Full-Text Search**: Find books quickly with SQLite FTS5 search
- **Reading Progress**: Track your reading progress across all books
- **Speed Reader**: RSVP word-at-a-time reading mode for text formats
- **Single-User Design**: Simple authentication with password protection

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Books stored as files on your server
- **ebook-convert** (from [Calibre](https://calibre-ebook.com/)) for text-ebook processing in the built-in readers

### Installing ebook-convert

For local development of text-based ebook processing, install Calibre on your system:

```bash
# Debian/Ubuntu
sudo apt-get install calibre

# macOS
brew install calibre

# Or download from https://calibre-ebook.com/download
```

### Configuration

Create a `config.yaml` file:

```yaml
server:
  port: 6060
  data_path: /data

auth:
  mode: password            # Use "none" to disable authentication
  username: your_username
  password_hash: "$2a$10$..."  # bcrypt hash of your password
  session_duration: 720h

libraries:
  - name: Books
    paths:
      - /books/fiction
      - /books/nonfiction
  - name: Comics
    paths:
      - /books/comics

bookdrop:
  path: /bookdrop         # Drop files here for auto-import

metadata:
  providers:
    - google_books
    - open_library
  auto_fetch_on_import: true
```

### Generating a Password Hash

Generate a bcrypt hash for your password:

```bash
# Using Go
go run golang.org/x/crypto/bcrypt your-password

# Or use an online bcrypt generator and copy the hash
```

### Docker Deployment

```bash
# Build and start
docker compose up -d

# View logs
docker compose logs -f
```

The app will be available at `http://localhost:6060` (or your configured port).

The Docker image installs a prebuilt Calibre binary and uses `ebook-convert` to preprocess
text-first ebooks into a cached canonical EPUB package that powers both continuous and
paginated reading modes.

## Volume Mounts

| Path | Description |
|------|-------------|
| `/data` | SQLite database, covers, cache |
| `/books` | Your book library files |
| `/bookdrop` | Auto-import folder |

## Authentication

By default, authentication is enabled. Set `auth.mode: none` in config.yaml to disable.

**Default credentials** (change these!):
- Username: `samurai`
- Password: `password`

## Development

```bash
# Backend
cd backend
go build -o cryptorum ./cmd/server
./cryptorum

# Frontend
cd frontend
npm install
npm run dev
```

## Tech Stack

- **Backend**: Go + chi router + SQLite
- **Frontend**: SvelteKit + Tailwind CSS
- **Readers**: epub.js, PDF.js

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE).
