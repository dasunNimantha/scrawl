package main

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Entry struct {
	ID        string
	Title     string
	Body      string
	CreatedAt time.Time
	TimeAgo   string
}

var tmpl *template.Template

func init() {
	funcMap := template.FuncMap{
		"timeago": func(t time.Time) string {
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return "just now"
			case d < time.Hour:
				m := int(d.Minutes())
				if m == 1 {
					return "1 min ago"
				}
				return strconv.Itoa(m) + " mins ago"
			case d < 24*time.Hour:
				h := int(d.Hours())
				if h == 1 {
					return "1 hour ago"
				}
				return strconv.Itoa(h) + " hours ago"
			case d < 30*24*time.Hour:
				days := int(d.Hours() / 24)
				if days == 1 {
					return "1 day ago"
				}
				return strconv.Itoa(days) + " days ago"
			default:
				return t.Format("Jan 2, 2006")
			}
		},
	}
	tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))
}

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	entries, err := getRecentEntries(0)
	if err != nil {
		log.Println("error fetching entries:", err)
		http.Error(w, "Internal Server Error", 500)
		return
	}

	tmpl.ExecuteTemplate(w, "index.html", map[string]any{
		"Entries": entries,
	})
}

func handleCreate(w http.ResponseWriter, r *http.Request) {
	title := r.FormValue("title")
	body := r.FormValue("body")

	if title == "" || body == "" {
		http.Error(w, "title and body are required", 400)
		return
	}

	id := generateID()

	_, err := db.Exec("INSERT INTO entries (id, title, body) VALUES (?, ?, ?)", id, title, body)
	if err != nil {
		log.Println("error creating entry:", err)
		http.Error(w, "Internal Server Error", 500)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		entries, _ := getRecentEntries(0)
		tmpl.ExecuteTemplate(w, "created-response", map[string]any{
			"ID":      id,
			"Entries": entries,
		})
		return
	}

	http.Redirect(w, r, "/e/"+id, http.StatusSeeOther)
}

func handleList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}

	entries, err := getRecentEntries(page)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		tmpl.ExecuteTemplate(w, "entry-list", map[string]any{
			"Entries": entries,
			"Page":    page,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	http.Error(w, "use HTMX", 400)
}

func handleView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var entry Entry
	var createdAt string
	err := db.QueryRow("SELECT id, title, body, created_at FROM entries WHERE id = ?", id).
		Scan(&entry.ID, &entry.Title, &entry.Body, &createdAt)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	entry.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	tmpl.ExecuteTemplate(w, "view.html", entry)
}

func getRecentEntries(page int) ([]Entry, error) {
	limit := 20
	offset := page * limit

	rows, err := db.Query(
		"SELECT id, title, body, created_at FROM entries ORDER BY created_at DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Title, &e.Body, &createdAt); err != nil {
			continue
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		entries = append(entries, e)
	}
	return entries, nil
}
