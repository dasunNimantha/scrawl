package main

import (
	"database/sql"
	"embed"
	"log"
	"net/http"
	"os"

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
		dbPath = "textbin.db"
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
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	log.Printf("textbin running on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
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
