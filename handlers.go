package main

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

type Entry struct {
	ID           string
	Title        string
	Body         string
	CreatedAt    time.Time
	ExpiresAt    *time.Time
	PasswordHash *string
	Locked       bool // set by listEntries for sidebar icon
}

func (e *Entry) IsLocked() bool {
	return e.PasswordHash != nil && *e.PasswordHash != ""
}

func hashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

func verifyPassword(entry *Entry, password string) bool {
	if !entry.IsLocked() {
		return true
	}
	return bcrypt.CompareHashAndPassword([]byte(*entry.PasswordHash), []byte(password)) == nil
}

var tmpl *template.Template

var validID = regexp.MustCompile(`^[0-9a-f]{8}$`)

const maxTitleLen = 200

func init() {
	funcMap := template.FuncMap{
		"dict": func(pairs ...any) map[string]any {
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs)-1; i += 2 {
				m[pairs[i].(string)] = pairs[i+1]
			}
			return m
		},
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

// sanitize strips control characters (except tab, newline, carriage return).
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
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

var ttlOptions = map[string]time.Duration{
	"1h":  time.Hour,
	"1d":  24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

func parseTTL(val string) *time.Time {
	if d, ok := ttlOptions[val]; ok {
		t := time.Now().Add(d)
		return &t
	}
	return nil
}

func handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	title := sanitize(strings.TrimSpace(r.FormValue("title")))
	body := sanitize(r.FormValue("body"))

	if title == "" || body == "" {
		http.Error(w, "title and body are required", 400)
		return
	}

	if len(title) > maxTitleLen {
		title = title[:maxTitleLen]
	}

	id := generateID()
	expiresAt := parseTTL(r.FormValue("ttl"))

	var expiresStr *string
	if expiresAt != nil {
		s := expiresAt.Format(time.RFC3339)
		expiresStr = &s
	}

	var pwHash *string
	if pw := r.FormValue("password"); pw != "" {
		h, err := hashPassword(pw)
		if err != nil {
			log.Println("error hashing password:", err)
			http.Error(w, "Internal Server Error", 500)
			return
		}
		pwHash = &h
	}

	_, err := db.Exec("INSERT INTO entries (id, title, body, expires_at, password_hash) VALUES (?, ?, ?, ?, ?)", id, title, body, expiresStr, pwHash)
	if err != nil {
		log.Println("error creating entry:", err)
		http.Error(w, "Internal Server Error", 500)
		return
	}

	entry := Entry{ID: id, Title: title, Body: body, CreatedAt: time.Now(), ExpiresAt: expiresAt, PasswordHash: pwHash}
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
	if !validID.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	entry, err := getEntry(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if entry.IsLocked() {
		data := map[string]any{"Entry": entry}
		if r.Header.Get("HX-Request") == "true" {
			tmpl.ExecuteTemplate(w, "unlock-form", data)
			return
		}
		entries, _ := listEntries(0)
		data["Entries"] = entries
		tmpl.ExecuteTemplate(w, "index.html", map[string]any{
			"Entries":     entries,
			"LockedEntry": entry,
		})
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		tmpl.ExecuteTemplate(w, "view-content", map[string]any{
			"Entry": entry,
		})
		return
	}

	entries, _ := listEntries(0)
	tmpl.ExecuteTemplate(w, "index.html", map[string]any{
		"Entries":   entries,
		"ViewEntry": entry,
	})
}

func handleUnlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	entry, err := getEntry(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	password := r.FormValue("password")
	if !verifyPassword(entry, password) {
		tmpl.ExecuteTemplate(w, "unlock-form", map[string]any{
			"Entry": entry,
			"Error": "Incorrect password",
		})
		return
	}

	tmpl.ExecuteTemplate(w, "view-content", map[string]any{
		"Entry":    entry,
		"Password": password,
	})
}

func handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	entry, err := getEntry(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if entry.IsLocked() && !verifyPassword(entry, r.FormValue("password")) {
		http.Error(w, "incorrect password", 403)
		return
	}

	title := sanitize(strings.TrimSpace(r.FormValue("title")))
	body := sanitize(r.FormValue("body"))

	if title == "" || body == "" {
		http.Error(w, "title and body are required", 400)
		return
	}

	if len(title) > maxTitleLen {
		title = title[:maxTitleLen]
	}

	expiresAt := parseTTL(r.FormValue("ttl"))
	var expiresStr *string
	if expiresAt != nil {
		s := expiresAt.Format(time.RFC3339)
		expiresStr = &s
	}

	result, err := db.Exec("UPDATE entries SET title = ?, body = ?, expires_at = ? WHERE id = ?", title, body, expiresStr, id)
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

	entry, _ = getEntry(id)

	if r.Header.Get("HX-Request") == "true" {
		entries, _ := listEntries(0)
		tmpl.ExecuteTemplate(w, "edited-response", map[string]any{
			"Entry":    entry,
			"Entries":  entries,
			"Password": r.FormValue("password"),
		})
		return
	}

	http.Redirect(w, r, "/e/"+id, http.StatusSeeOther)
}

func handleEditForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	entry, err := getEntry(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	password := r.URL.Query().Get("password")
	if entry.IsLocked() && !verifyPassword(entry, password) {
		http.Error(w, "incorrect password", 403)
		return
	}

	tmpl.ExecuteTemplate(w, "edit-form", map[string]any{
		"Entry":    entry,
		"Password": password,
	})
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	entry, err := getEntry(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	password := r.URL.Query().Get("password")
	if password == "" && r.Body != nil {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		if vals, e := url.ParseQuery(string(b)); e == nil {
			password = vals.Get("password")
		}
	}

	if entry.IsLocked() && !verifyPassword(entry, password) {
		http.Error(w, "incorrect password", 403)
		return
	}

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

func handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	entry, err := getEntry(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if entry.IsLocked() && !verifyPassword(entry, r.URL.Query().Get("password")) {
		http.Error(w, "incorrect password", 403)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+entry.Title+".txt\"")
	w.Write([]byte(entry.Body))
}

// listEntries returns entries without body for sidebar display.
func listEntries(page int) ([]Entry, error) {
	limit := 20
	offset := page * limit

	rows, err := db.Query(
		"SELECT id, title, created_at, password_hash IS NOT NULL FROM entries WHERE expires_at IS NULL OR expires_at > datetime('now') ORDER BY created_at DESC, rowid DESC LIMIT ? OFFSET ?",
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
		if err := rows.Scan(&e.ID, &e.Title, &createdAt, &e.Locked); err != nil {
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
	var expiresAt *string
	err := db.QueryRow(
		"SELECT id, title, body, created_at, expires_at, password_hash FROM entries WHERE id = ? AND (expires_at IS NULL OR expires_at > datetime('now'))", id,
	).Scan(&entry.ID, &entry.Title, &entry.Body, &createdAt, &expiresAt, &entry.PasswordHash)
	if err != nil {
		return nil, err
	}
	entry.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if expiresAt != nil {
		t, _ := time.Parse(time.RFC3339, *expiresAt)
		entry.ExpiresAt = &t
	}
	entry.Locked = entry.IsLocked()
	return &entry, nil
}

// cleanupExpired removes entries past their expiration.
func cleanupExpired() {
	result, err := db.Exec("DELETE FROM entries WHERE expires_at IS NOT NULL AND expires_at <= datetime('now')")
	if err != nil {
		log.Println("cleanup error:", err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		log.Printf("cleaned up %d expired entries", n)
	}
}
