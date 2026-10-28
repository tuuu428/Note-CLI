package note

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tuuu428/note-cli/internal/article"
)

// 本物の note の代わりに、テスト用の偽サーバーを立てて通信内容を確認します。
func TestDraftAndPublishFlow(t *testing.T) {
	var published map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, _ := r.Cookie("_note_session_v5"); c == nil || c.Value != "sess" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-XSRF-TOKEN") != "tok" {
			t.Errorf("%s %s: XSRF ヘッダーがない", r.Method, r.URL.Path)
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v2/current_user":
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "tok"})
			io.WriteString(w, `{"data":{"id":42,"urlname":"me","nickname":"私"}}`)
		case r.Method == "POST" && r.URL.Path == "/api/v1/text_notes":
			if r.URL.Query().Get("user_id") != "42" {
				t.Errorf("user_id = %q", r.URL.Query().Get("user_id"))
			}
			io.WriteString(w, `{"data":{"id":7,"key":"nabc"}}`)
		case r.Method == "POST" && r.URL.Path == "/api/v1/text_notes/draft_save":
			if r.URL.Query().Get("id") != "7" {
				t.Errorf("draft id = %q", r.URL.Query().Get("id"))
			}
			io.WriteString(w, `{"data":{}}`)
		case r.Method == "PUT" && r.URL.Path == "/api/v1/text_notes/7":
			json.NewDecoder(r.Body).Decode(&published)
			io.WriteString(w, `{"data":{}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient("sess")
	c.BaseURL = srv.URL
	user, err := c.CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	n, err := c.CreateDraft(user.ID, "題")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SaveDraft(n.ID, "題", "<p>x</p>", 1); err != nil {
		t.Fatal(err)
	}

	price := 300
	a := &article.Article{Body: "無料\n\n<!-- paid -->\n\n有料", Settings: article.Settings{
		Title: "題", Status: "published", Tags: []string{"Go", "#日記"}, Price: &price,
		Magazines: []string{"m123abc", "99"}, Comments: ptr(false),
		Extra: map[string]any{"index": true},
	}}
	h, _ := article.ToHTML(a.Body)
	if err := c.Publish(n.ID, PublishPayload(a, h)); err != nil {
		t.Fatal(err)
	}

	if published["status"] != "published" || published["price"] != float64(300) || published["disable_comment"] != true {
		t.Errorf("payload: %v", published)
	}
	if tags := published["hashtags"].([]any); tags[0] != "#Go" || tags[1] != "#日記" {
		t.Errorf("hashtags: %v", tags)
	}
	if published["separator"] != h.Separator || published["pay_body"] != h.Paid {
		t.Errorf("paid split not sent: %v", published)
	}
	if ids := published["magazine_ids"].([]any); len(ids) != 1 || ids[0] != float64(99) {
		t.Errorf("magazine_ids: %v", ids)
	}
	if published["index"] != true {
		t.Error("extra で上書きできていない")
	}
}

func TestUnauthorizedHasHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient("bad")
	c.BaseURL = srv.URL
	_, err := c.CurrentUser()
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 401 {
		t.Fatalf("got %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
