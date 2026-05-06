# Cryptorum

A personal digital library application for self-hosting. Organize, read, and manage your ebook, comic, and audiobook collection from a single, streamlined interface.

## Features

- **Multiple Format Support**: EPUB/text ebooks, PDF, CBZ/CBR/CB7/CBT comics, and MP3/M4B/M4A/other audiobooks
- **Library Organization**: Organize books by custom libraries and shelves, with per-library discovery exclusions
- **Built-in Readers**: Read books directly in the app with dedicated readers for ebooks, PDFs, comics, audiobooks, and speed reading
- **EmbedPDF PDF Reader**: PDF reading is powered by EmbedPDF/PDFium, not PDF.js, with app-integrated progress, resume, search access, auto-hiding chrome, and theme-aware controls
- **Full-Text Search**: Find books quickly with SQLite FTS5 search
- **Reading Progress**: Track and resume progress across supported readers, with cover progress bars for opened books
- **Discovery**: Dashboard discovery and similar-book recommendations can exclude selected libraries while keeping those books searchable and readable
- **Cover Metadata**: Optional file-format chips on covers, including library/dashboard/shelf cards and similar books
- **Speed Reader**: RSVP word-at-a-time reading mode for text formats
- **Single-User Design**: Simple authentication with password protection

## Screenshots

A quick tour of the main library, reader, and settings views:

<p align="center">
  <img src="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/01-dashboard.png" alt="Cryptorum dashboard" width="900">
</p>

| Dashboard | Book Details | Reader |
|-----------|--------------|--------|
| <a href="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/01-dashboard.png"><img src="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/01-dashboard.png" alt="Dashboard" width="260"></a> | <a href="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/02-book-details.png"><img src="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/02-book-details.png" alt="Book details" width="260"></a> | <a href="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/03-reader.png"><img src="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/03-reader.png" alt="Reader" width="260"></a> |

| Settings | Reader Settings | Reading History | Statistics |
|----------|-----------------|-----------------|------------|
| <a href="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/04-settings-01.png"><img src="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/04-settings-01.png" alt="Settings" width="200"></a> | <a href="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/05-settings-02.png"><img src="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/05-settings-02.png" alt="Reader settings" width="200"></a> | <a href="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/06-reading-history.png"><img src="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/06-reading-history.png" alt="Reading history" width="200"></a> | <a href="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/07-statistics.png"><img src="https://raw.githubusercontent.com/BlindMint/cryptorum/refs/heads/master/docs/images/07-statistics.png" alt="Statistics" width="200"></a> |

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
  username: username
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

## Readers

Cryptorum includes separate reader experiences for each major format family:

- **EPUB/text ebooks** use epub.js plus the app's processed EPUB cache for continuous or paginated reading.
- **PDFs** use EmbedPDF's Svelte viewer, backed by PDFium WebAssembly. The current PDF reader does not use PDF.js.
- **Comics** use the app's CBX reader for archive formats such as CBZ, CBR, CB7, and CBT.
- **Audiobooks** use the app's audio reader for common audio formats.
- **Speed Reader** provides an RSVP-style mode for text-readable formats.

Reader controls are designed to stay out of the way while reading. PDF, EPUB, and comic
readers include auto-hiding top controls, center-tap show/hide behavior, draggable/tappable
progress sliders, manual fullscreen controls, and progress saving/resume support. PDF and
speed reader settings also include a "keep screen on" option for long mobile reading sessions.

## Volume Mounts

| Path | Description |
|------|-------------|
| `/data` | SQLite database, covers, cache |
| `/books` | Your book library files |
| `/bookdrop` | Auto-import folder |

## Authentication

By default, authentication is enabled. Set `auth.mode: none` in config.yaml to disable.

**Default credentials** (change these!):
- Username: `username`
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
- **Readers**: epub.js, EmbedPDF/PDFium, custom CBX/audio/speed readers
- **Processing**: Calibre `ebook-convert` for text-ebook normalization

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE).
