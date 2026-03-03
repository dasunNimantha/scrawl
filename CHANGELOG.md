# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0] - 2026-03-03

### Added

- Optional password protection for entries using bcrypt hashing
- Lock icon in sidebar for protected entries
- Unlock form with error feedback for incorrect passwords
- Password verification on view, edit, delete, and download actions
- `POST /e/{id}/unlock` endpoint for password verification
- 14 new integration tests for password-related flows (49 total)
- Mobile-responsive improvements for editor and view layouts

## [1.2.0] - 2026-03-03

### Added

- Live word/character count in editor footer
- Download as `.txt` button on view page (`GET /e/{id}/download`)
- Expiring entries with optional TTL (1 hour, 1 day, 7 days, 30 days — default: never)
- Background cleanup goroutine for expired entries (runs every 10 minutes)
- Graceful schema migration for `expires_at` column

## [1.1.0] - 2026-03-03

### Added

- SVG favicon
- Docker Hub image push on release (multi-arch: amd64 + arm64)

### Fixed

- Gzip middleware now sets Content-Type header correctly for HTML rendering
- Simplified textarea placeholder text

## [1.0.0] - 2026-03-02

### Added

- Create, view, edit, and delete text entries
- Shareable links for every entry (`/e/{id}`)
- Sidebar with recent entries and search filter
- Copy Link and Copy Text buttons on view page
- Keyboard shortcuts: `Ctrl+Enter` save, `Tab` indent, `Esc` close modal
- Dark theme with clean, modern UI
- Gzip compression middleware with Content-Type detection
- Security headers (CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy)
- Input sanitization (control character stripping, title length cap)
- Body size limit (128KB)
- ID format validation
- HTMX embedded locally (no CDN dependency)
- Cache headers for static assets
- SQLite with WAL mode
- Single binary deployment (embedded templates and static assets)
- Dockerfile with multi-stage Alpine build
- GitHub Actions CI pipeline and release workflow
- Dependabot with auto-merge for Go modules and GitHub Actions
- MIT license

[1.3.0]: https://github.com/dasunNimantha/scrawl/releases/tag/v1.3.0
[1.2.0]: https://github.com/dasunNimantha/scrawl/releases/tag/v1.2.0
[1.1.0]: https://github.com/dasunNimantha/scrawl/releases/tag/v1.1.0
[1.0.0]: https://github.com/dasunNimantha/scrawl/releases/tag/v1.0.0
