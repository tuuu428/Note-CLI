package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tuuu428/note-cli/internal/article"
	"github.com/tuuu428/note-cli/internal/note"
)

// 同じファイルを 2 回 post したとき、記事を作り直さず、変わっていない画像も再アップロードしないこと。
func TestPostTwiceReusesNoteAndImages(t *testing.T) {
	calls := map[string]int{}
	var s3 *httptest.Server
	s3 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls["s3"]++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s3.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		calls[key]++
		switch key {
		case "POST /api/v1/text_notes":
			io.WriteString(w, `{"data":{"id":7,"key":"nabc"}}`)
		case "POST /api/v3/images/upload/presigned_post":
			fmt.Fprintf(w, `{"data":{"url":"https://assets.note/%d.png","action":"%s","post":{"key":"k"}}}`, calls[key], s3.URL)
		case "POST /api/v1/image_upload/note_eyecatch", "POST /api/v1/text_notes/draft_save":
			io.WriteString(w, `{"data":{}}`)
		default:
			t.Errorf("unexpected %s", key)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "cat.png"), []byte("cat"), 0o644)
	os.WriteFile(filepath.Join(dir, "cover.png"), []byte("cover"), 0o644)
	path := filepath.Join(dir, "a.md")
	os.WriteFile(path, []byte("---\ntitle: t\neyecatch: cover.png\n---\n![](cat.png)\n"), 0o644)

	client := note.NewClient("sess")
	client.BaseURL = srv.URL
	user := &note.User{ID: 42, URLName: "me"}

	for i := 0; i < 2; i++ {
		a, err := article.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Validate(); err != nil {
			t.Fatal(err)
		}
		if err := postOne(client, user, a); err != nil {
			t.Fatal(err)
		}
	}

	if calls["POST /api/v1/text_notes"] != 1 {
		t.Errorf("記事は 1 回だけ作るべき: %v", calls)
	}
	if calls["POST /api/v3/images/upload/presigned_post"] != 1 || calls["s3"] != 1 || calls["POST /api/v1/image_upload/note_eyecatch"] != 1 {
		t.Errorf("同じ画像は再アップロードしないべき: %v", calls)
	}
	if calls["POST /api/v1/text_notes/draft_save"] != 2 {
		t.Errorf("本文は毎回保存するべき: %v", calls)
	}

	// 画像を差し替えたら、もう一度アップロードされる
	os.WriteFile(filepath.Join(dir, "cat.png"), []byte("new cat"), 0o644)
	a, _ := article.Load(path)
	if err := postOne(client, user, a); err != nil {
		t.Fatal(err)
	}
	if calls["s3"] != 2 {
		t.Errorf("中身が変わった画像は再アップロードするべき: %v", calls)
	}
	st, _ := os.ReadFile(filepath.Join(dir, note.StateFileName))
	if !strings.Contains(string(st), "https://assets.note/2.png") {
		t.Errorf("記録ファイル: %s", st)
	}
}
