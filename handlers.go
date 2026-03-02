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

	entries, err := listEntries(0)
	if err != nil {
		log.Println("error fetching entries:", err)
		http.Error(w, "Internal Server Error", 500)
		return
	}

	tmpl.ExecuteTemplate(w, "index.html", map[string]any{
		"Entries":   entries,
		"ViewEntry": nil,
	})
}

const maxBodySize = 128 * 1024 // 128KB

func handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

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

	entry := Entry{ID: id, Title: title, Body: body, CreatedAt: time.Now()}
	entries, _ := listEntries(0)

	if r.Header.Get("HX-Request") == "true" {
		tmpl.ExecuteTemplate(w, "created-response", map[string]any{
			"ID":      id,
			"Entry":   entry,
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

	entries, err := listEntries(page)
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

	entry, err := getEntry(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		tmpl.ExecuteTemplate(w, "view-content", entry)
		return
	}

	entries, _ := listEntries(0)
	tmpl.ExecuteTemplate(w, "index.html", map[string]any{
		"Entries":   entries,
		"ViewEntry": entry,
	})
}

func handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	title := r.FormValue("title")
	body := r.FormValue("body")

	if title == "" || body == "" {
		http.Error(w, "title and body are required", 400)
		return
	}

	result, err := db.Exec("UPDATE entries SET title = ?, body = ? WHERE id = ?", title, body, id)
	if err != nil {
		log.Println("error updating entry:", err)
		http.Error(w, "Internal Server Error", 500)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.NotFound(w, r)
		return
	}

	entry, _ := getEntry(id)

	if r.Header.Get("HX-Request") == "true" {
		entries, _ := listEntries(0)
		tmpl.ExecuteTemplate(w, "edited-response", map[string]any{
			"Entry":   entry,
			"Entries": entries,
		})
		return
	}

	http.Redirect(w, r, "/e/"+id, http.StatusSeeOther)
}

func handleEditForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	entry, err := getEntry(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	tmpl.ExecuteTemplate(w, "edit-form", entry)
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	result, err := db.Exec("DELETE FROM entries WHERE id = ?", id)
	if err != nil {
		log.Println("error deleting entry:", err)
		http.Error(w, "Internal Server Error", 500)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.NotFound(w, r)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		entries, _ := listEntries(0)
		tmpl.ExecuteTemplate(w, "deleted-response", map[string]any{
			"Entries": entries,
		})
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// listEntries returns entries without body for sidebar display.
func listEntries(page int) ([]Entry, error) {
	limit := 20
	offset := page * limit

	rows, err := db.Query(
		"SELECT id, title, created_at FROM entries ORDER BY created_at DESC, rowid DESC LIMIT ? OFFSET ?",
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
		if err := rows.Scan(&e.ID, &e.Title, &createdAt); err != nil {
			continue
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		entries = append(entries, e)
	}
	return entries, nil
}

// getEntry returns a single entry with full body.
func getEntry(id string) (*Entry, error) {
	var entry Entry
	var createdAt string
	err := db.QueryRow("SELECT id, title, body, created_at FROM entries WHERE id = ?", id).
		Scan(&entry.ID, &entry.Title, &entry.Body, &createdAt)
	if err != nil {
		return nil, err
	}
	entry.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &entry, nil
}
