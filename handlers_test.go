package main

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) func() {
	t.Helper()
	dbPath := "test_scrawl_" + t.Name() + ".db"

	var err error
	db, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		t.Fatal("failed to open test db:", err)
	}
	db.SetMaxOpenConns(1)

	if err := initDB(); err != nil {
		t.Fatal("failed to init test db:", err)
	}

	return func() {
		db.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("POST /api/entries", handleCreate)
	mux.HandleFunc("GET /api/entries", handleList)
	mux.HandleFunc("GET /e/{id}", handleView)
	mux.HandleFunc("PUT /api/entries/{id}", handleEdit)
	mux.HandleFunc("GET /api/entries/{id}/edit", handleEditForm)
	mux.HandleFunc("DELETE /api/entries/{id}", handleDelete)
	mux.HandleFunc("GET /e/{id}/download", handleDownload)
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	return mux
}

func TestIndexPage(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal("GET / failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)

	assertContains(t, body, "<title>scrawl</title>", "page title")
	assertContains(t, body, `class="logo"`, "logo element")
	assertContains(t, body, `class="sidebar"`, "sidebar")
	assertContains(t, body, `class="input-title"`, "heading input")
	assertContains(t, body, `class="input-body"`, "body textarea")
	assertContains(t, body, `class="btn btn-primary"`, "save button")
	assertContains(t, body, "No entries yet", "empty state message")
	assertContains(t, body, "htmx", "HTMX script")
	assertContains(t, body, `id="search-input"`, "search input")
}

func TestCreateEntry_NonHTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	form := url.Values{"title": {"Test Title"}, "body": {"Test body content 🎉"}}
	resp, err := client.PostForm(srv.URL+"/api/entries", form)
	if err != nil {
		t.Fatal("POST /api/entries failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 303 {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/e/") {
		t.Fatalf("expected redirect to /e/{id}, got %q", loc)
	}

	id := strings.TrimPrefix(loc, "/e/")
	if len(id) != 8 {
		t.Fatalf("expected 8-char hex ID, got %q (len=%d)", id, len(id))
	}
}

func TestCreateEntry_HTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	form := url.Values{"title": {"HTMX Entry"}, "body": {"Content via HTMX"}}
	req, _ := http.NewRequest("POST", srv.URL+"/api/entries", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("HTMX POST failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	assertContains(t, body, "Saved!", "success message")
	assertContains(t, body, "Copy Link", "copy link button")
	assertContains(t, body, "HTMX Entry", "entry title in response")
	assertContains(t, body, "Content via HTMX", "entry body in response")
	assertContains(t, body, `class="share-link"`, "share link element")
}

func TestCreateEntry_Validation(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	tests := []struct {
		name  string
		title string
		body  string
	}{
		{"empty both", "", ""},
		{"empty title", "", "some body"},
		{"empty body", "some title", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{"title": {tt.title}, "body": {tt.body}}
			resp, err := http.PostForm(srv.URL+"/api/entries", form)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 400 {
				t.Fatalf("expected 400 for %s, got %d", tt.name, resp.StatusCode)
			}
		})
	}
}

func TestViewEntry_FullPage(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "View Test", "Body with\n  indentation\n    and 🎯 emojis")

	resp, err := http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal("GET /e/{id} failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	assertContains(t, body, "View Test", "entry title")
	assertContains(t, body, "indentation", "entry body")
	assertContains(t, body, `class="sidebar"`, "sidebar present on view page")
	assertContains(t, body, `class="view-title"`, "view title styling")
	assertContains(t, body, `class="view-body"`, "view body styling")
	assertContains(t, body, "Copy Link", "copy link button")
	assertContains(t, body, "Copy Text", "copy text button")
	assertContains(t, body, "Edit", "edit button")
}

func TestViewEntry_HTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "HTMX View", "HTMX body content")

	req, _ := http.NewRequest("GET", srv.URL+"/e/"+id, nil)
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("HTMX GET failed:", err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)

	assertContains(t, body, "HTMX View", "entry title")
	assertContains(t, body, "HTMX body content", "entry body")
	assertNotContains(t, body, `class="sidebar"`, "should NOT contain sidebar (partial response)")
	assertContains(t, body, `class="view-content"`, "view content wrapper")
}

func TestViewEntry_NotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/e/deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestViewEntry_InvalidID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/e/not-hex!")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for invalid ID, got %d", resp.StatusCode)
	}
}

func TestSidebarShowsEntries(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	createTestEntry(t, srv.URL, "First Entry", "body 1")
	createTestEntry(t, srv.URL, "Second Entry", "body 2")
	createTestEntry(t, srv.URL, "Third Entry", "body 3")

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)
	assertContains(t, body, "First Entry", "first entry in sidebar")
	assertContains(t, body, "Second Entry", "second entry in sidebar")
	assertContains(t, body, "Third Entry", "third entry in sidebar")
	assertNotContains(t, body, "No entries yet", "empty state should be gone")
}

func TestListEntries_HTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	createTestEntry(t, srv.URL, "List Test", "body")

	req, _ := http.NewRequest("GET", srv.URL+"/api/entries?page=0", nil)
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	assertContains(t, body, "List Test", "entry in list response")
	assertContains(t, body, `class="entry-item"`, "entry item element")
}

func TestListEntries_NonHTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/entries")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestStaticCSS(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	assertContains(t, body, "--bg:", "CSS variables")
	assertContains(t, body, ".sidebar", "sidebar styles")
	assertContains(t, body, ".view-body", "view-body styles")
}

func TestFormattingPreserved(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	formatted := "Line 1\n  Indented line\n    Double indent\n\n🎉 Emoji line\n\ttab indent"
	id := createTestEntry(t, srv.URL, "Formatting Test", formatted)

	resp, err := http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)
	assertContains(t, body, "Indented line", "indented content")
	assertContains(t, body, "Double indent", "double indented content")
	assertContains(t, body, "Emoji line", "emoji content")
	assertContains(t, body, `<pre class="view-body">`, "pre tag for formatting")
}

func TestMultipleEntriesOrder(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	createTestEntry(t, srv.URL, "Oldest", "body")
	createTestEntry(t, srv.URL, "Middle", "body")
	createTestEntry(t, srv.URL, "Newest", "body")

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)

	newestIdx := strings.Index(body, "Newest")
	middleIdx := strings.Index(body, "Middle")
	oldestIdx := strings.Index(body, "Oldest")

	if newestIdx == -1 || middleIdx == -1 || oldestIdx == -1 {
		t.Fatal("not all entries found in response")
	}

	if !(newestIdx < middleIdx && middleIdx < oldestIdx) {
		t.Fatalf("entries not in reverse chronological order: newest=%d, middle=%d, oldest=%d",
			newestIdx, middleIdx, oldestIdx)
	}
}

func TestEditEntry_HTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Original Title", "Original body")

	form := url.Values{"title": {"Updated Title"}, "body": {"Updated body content"}}
	req, _ := http.NewRequest("PUT", srv.URL+"/api/entries/"+id, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("PUT failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	assertContains(t, body, "Updated!", "update success message")
	assertContains(t, body, "Updated Title", "updated title in response")
	assertContains(t, body, "Updated body content", "updated body in response")
}

func TestEditEntry_NonHTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Edit Me", "original")

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	form := url.Values{"title": {"Edited"}, "body": {"edited body"}}
	req, _ := http.NewRequest("PUT", srv.URL+"/api/entries/"+id, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("PUT failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 303 {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}
}

func TestEditEntry_Validation(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Valid", "valid body")

	form := url.Values{"title": {""}, "body": {"body"}}
	req, _ := http.NewRequest("PUT", srv.URL+"/api/entries/"+id, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestEditEntry_NotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	form := url.Values{"title": {"x"}, "body": {"y"}}
	req, _ := http.NewRequest("PUT", srv.URL+"/api/entries/deadbeef", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestEditEntry_VerifyPersisted(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Before Edit", "old body")

	form := url.Values{"title": {"After Edit"}, "body": {"new body"}}
	req, _ := http.NewRequest("PUT", srv.URL+"/api/entries/"+id, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	viewResp, err := http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer viewResp.Body.Close()

	body := readBody(t, viewResp)
	assertContains(t, body, "After Edit", "updated title persisted")
	assertContains(t, body, "new body", "updated body persisted")
	assertNotContains(t, body, "Before Edit", "old title should be gone")
}

func TestEditForm(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Form Title", "Form body content")

	resp, err := http.Get(srv.URL + "/api/entries/" + id + "/edit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	assertContains(t, body, "Form Title", "pre-filled title")
	assertContains(t, body, "Form body content", "pre-filled body")
	assertContains(t, body, "Save Changes", "save button")
	assertContains(t, body, "Cancel", "cancel button")
	assertContains(t, body, "hx-put", "HTMX PUT attribute")
}

func TestDeleteEntry_HTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "To Delete", "will be deleted")

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/entries/"+id, nil)
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("DELETE failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	assertContains(t, body, "Entry deleted", "deleted confirmation message")
	assertContains(t, body, `class="input-title"`, "editor form shown after delete")
	assertNotContains(t, body, "To Delete", "deleted entry should not appear in sidebar")
}

func TestDeleteEntry_NonHTMX(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "To Delete 2", "will be deleted too")

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/entries/"+id, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("DELETE failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 303 {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "/" {
		t.Fatalf("expected redirect to /, got %q", loc)
	}
}

func TestDeleteEntry_NotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/entries/deadbeef", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteEntry_VerifyGone(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Gone Entry", "this will vanish")

	resp, err := http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("entry should exist before delete, got %d", resp.StatusCode)
	}

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/entries/"+id, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("entry should be gone after delete, got %d", resp.StatusCode)
	}
}

func TestDeleteButton_InViewPage(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Has Delete Button", "check for button")

	resp, err := http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)
	assertContains(t, body, "Delete", "delete button text")
	assertContains(t, body, "confirmDelete", "delete confirmation JS call")
	assertContains(t, body, "delete-modal", "delete confirmation modal")
}

func Test404(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestSidebarDoesNotContainBody(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	createTestEntry(t, srv.URL, "Slim Query Test", "THIS_UNIQUE_BODY_TEXT_SHOULD_NOT_APPEAR")

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)
	assertContains(t, body, "Slim Query Test", "title should appear")

	// The body text should NOT appear in the index page (sidebar only shows titles)
	// It may appear in template JS though, so we check the entries-list nav specifically
	// For a basic check: the index page should contain the title but the body text
	// should only appear if the entry is being viewed
	assertNotContains(t, body, "THIS_UNIQUE_BODY_TEXT_SHOULD_NOT_APPEAR", "body should not be in sidebar response")
}

func TestSecurityHeaders(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(securityHeaders(newMux()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
		"Referrer-Policy":       "strict-origin-when-cross-origin",
	}
	for name, expected := range headers {
		if got := resp.Header.Get(name); got != expected {
			t.Errorf("expected %s=%q, got %q", name, expected, got)
		}
	}

	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Error("expected Content-Security-Policy header")
	}
}

func TestInputSanitization(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "  Padded Title  ", "body with \x00 null")

	resp, err := http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)
	assertContains(t, body, "Padded Title", "title should be trimmed")
	assertNotContains(t, body, "\x00", "null bytes should be stripped")
}

func TestDownloadEntry(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Download Test", "downloadable content here")

	resp, err := http.Get(srv.URL + "/e/" + id + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content type, got %q", ct)
	}

	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Fatalf("expected attachment disposition, got %q", cd)
	}

	body := readBody(t, resp)
	if body != "downloadable content here" {
		t.Fatalf("expected raw body content, got %q", body)
	}
}

func TestDownloadEntry_NotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/e/deadbeef/download")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestCreateEntryWithTTL(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	form := url.Values{"title": {"Expiring"}, "body": {"will expire"}, "ttl": {"1h"}}
	resp, err := client.PostForm(srv.URL+"/api/entries", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 303 {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}

	id := strings.TrimPrefix(resp.Header.Get("Location"), "/e/")

	entry, err := getEntry(id)
	if err != nil {
		t.Fatal("failed to get entry:", err)
	}

	if entry.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
}

func TestCreateEntryWithoutTTL(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	form := url.Values{"title": {"Permanent"}, "body": {"no expiry"}}
	resp, err := client.PostForm(srv.URL+"/api/entries", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	id := strings.TrimPrefix(resp.Header.Get("Location"), "/e/")

	entry, err := getEntry(id)
	if err != nil {
		t.Fatal("failed to get entry:", err)
	}

	if entry.ExpiresAt != nil {
		t.Fatal("expected ExpiresAt to be nil for no-TTL entry")
	}
}

func TestExpiredEntryNotVisible(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := generateID()
	db.Exec("INSERT INTO entries (id, title, body, expires_at) VALUES (?, ?, ?, datetime('now', '-1 hour'))",
		id, "Expired", "gone")

	resp, err := http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for expired entry, got %d", resp.StatusCode)
	}
}

func TestExpiredEntryNotInSidebar(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	db.Exec("INSERT INTO entries (id, title, body, expires_at) VALUES (?, ?, ?, datetime('now', '-1 hour'))",
		"abcd1234", "Expired Sidebar", "gone")
	createTestEntry(t, srv.URL, "Visible Entry", "still here")

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)
	assertContains(t, body, "Visible Entry", "non-expired entry should appear")
	assertNotContains(t, body, "Expired Sidebar", "expired entry should not appear in sidebar")
}

func TestViewEntryShowsDownloadButton(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	id := createTestEntry(t, srv.URL, "Download Button", "content")

	resp, err := http.Get(srv.URL + "/e/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)
	assertContains(t, body, "Download", "download button")
}

func TestEditorShowsTTLSelect(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body := readBody(t, resp)
	assertContains(t, body, "ttl-select", "TTL select element")
	assertContains(t, body, "No expiry", "default no-expiry option")
}

// --- helpers ---

func createTestEntry(t *testing.T, baseURL, title, body string) string {
	t.Helper()
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	form := url.Values{"title": {title}, "body": {body}}
	resp, err := client.PostForm(baseURL+"/api/entries", form)
	if err != nil {
		t.Fatal("create entry failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 303 {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}

	return strings.TrimPrefix(resp.Header.Get("Location"), "/e/")
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("failed to read body:", err)
	}
	return string(b)
}

func assertContains(t *testing.T, body, substr, label string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Errorf("expected %s to contain %q", label, substr)
	}
}

func assertNotContains(t *testing.T, body, substr, label string) {
	t.Helper()
	if strings.Contains(body, substr) {
		t.Errorf("expected %s to NOT contain %q", label, substr)
	}
}
