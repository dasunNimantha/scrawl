# scrawl

A blazingly fast, no-nonsense text sharing app. Drop in formatted text with emojis, indentation, and structure — get a shareable link instantly.

Single binary. No accounts. No bloat.

![View entry](screenshots/view-entry.png)

## Features

- **Instant sharing** — create an entry, get a link
- **Formatting preserved** — emojis, indentation, tabs, whitespace all stay intact
- **Sidebar** — recent entries always visible, searchable by title
- **Full CRUD** — create, view, edit, delete entries
- **Copy buttons** — copy shareable link or entry text to clipboard
- **Keyboard shortcuts** — `Ctrl+Enter` save, `Tab` indent, `Esc` close modal
- **Gzip compression** — responses compressed on the fly
- **Single binary** — Go executable with embedded assets, nothing else needed
- **Dark theme** — easy on the eyes

![Editor](screenshots/editor.png)

## Quick Start

```bash
git clone https://github.com/YOUR_USERNAME/scrawl.git
cd scrawl
go build -o scrawl .
./scrawl
```

Open [http://localhost:8080](http://localhost:8080).

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `DB_PATH` | `scrawl.db` | SQLite database file path |

```bash
PORT=3000 DB_PATH=/data/scrawl.db ./scrawl
```

## Tech Stack

- **Backend** — Go (stdlib `net/http`, `html/template`, `embed`)
- **Database** — SQLite (via `mattn/go-sqlite3`, WAL mode)
- **Frontend** — Vanilla HTML/CSS/JS + HTMX (embedded locally, no CDN)
- **Dependencies** — 1 (go-sqlite3)

## Build

Requires Go 1.22+ and a C compiler (for SQLite).

```bash
# development
go run .

# production binary (stripped)
CGO_ENABLED=1 go build -ldflags="-s -w" -o scrawl .

# run tests
go test -v ./...
```

## Project Structure

```
scrawl/
├── main.go              # server, routes, middleware
├── handlers.go          # HTTP handlers, DB queries
├── handlers_test.go     # integration tests
├── go.mod
├── templates/
│   └── index.html       # all templates (single file)
└── static/
    ├── style.css
    └── htmx.min.js
```

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Home page with editor |
| `POST` | `/api/entries` | Create entry |
| `GET` | `/e/{id}` | View entry |
| `PUT` | `/api/entries/{id}` | Update entry |
| `DELETE` | `/api/entries/{id}` | Delete entry |
| `GET` | `/api/entries` | List entries (HTMX) |
| `GET` | `/api/entries/{id}/edit` | Edit form (HTMX) |

## License

[MIT](LICENSE)
