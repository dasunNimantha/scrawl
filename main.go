package main

import (
	"compress/gzip"
	"database/sql"
	"embed"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

var db *sql.DB

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "scrawl.db"
	}

	var err error
	db, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		log.Fatal("failed to open database:", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)

	if err := initDB(); err != nil {
		log.Fatal("failed to initialize database:", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("POST /api/entries", handleCreate)
	mux.HandleFunc("GET /api/entries", handleList)
	mux.HandleFunc("GET /e/{id}", handleView)
	mux.HandleFunc("PUT /api/entries/{id}", handleEdit)
	mux.HandleFunc("GET /api/entries/{id}/edit", handleEditForm)
	mux.HandleFunc("DELETE /api/entries/{id}", handleDelete)
	mux.Handle("GET /static/", cacheStatic(http.FileServerFS(staticFS)))

	handler := securityHeaders(gzipHandler(mux))

	log.Printf("scrawl running on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func initDB() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS entries (
			id         TEXT PRIMARY KEY,
			title      TEXT NOT NULL,
			body       TEXT NOT NULL,
			created_at DATETIME DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_entries_created ON entries(created_at DESC);
	`)
	return err
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self'; img-src 'self'; font-src 'self'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	})
}

var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func gzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			gz.Close()
			gzipPool.Put(gz)
		}()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		next.ServeHTTP(gzipResponseWriter{Writer: gz, ResponseWriter: w}, r)
	})
}
