# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[1.1.0]: https://github.com/dasunNimantha/scrawl/releases/tag/v1.1.0
[1.0.0]: https://github.com/dasunNimantha/scrawl/releases/tag/v1.0.0
