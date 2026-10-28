package note

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tuuu428/note-cli/internal/article"
)

// fakeNote は本物の note の代わりになるテスト用の偽サーバーです。
// routes に「窓口 → 応答」を登録しておくと、そのとおりに返します。
type fakeNote struct {
	t      *testing.T
	routes map[string]http.HandlerFunc
	got    map[string]map[string]any // 受け取った JSON
}

func newFake(t *testing.T) (*fakeNote, *Client) {
	f := &fakeNote{t: t, got: map[string]map[string]any{}, routes: map[string]http.HandlerFunc{
		"GET /api/v2/current_user": func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "tok"})
			io.WriteString(w, `{"data":{"id":42,"urlname":"me","nickname":"私"}}`)
		},
	}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := NewClient("sess")
	c.BaseURL = srv.URL
	return f, c
}

func (f *fakeNote) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if c, _ := r.Cookie("_note_session_v5"); c == nil || c.Value != "sess" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet && r.Header.Get("X-XSRF-TOKEN") != "tok" {
		f.t.Errorf("%s %s: XSRF ヘッダーがない", r.Method, r.URL.Path)
	}
	key := r.Method + " " + r.URL.Path
	h, ok := f.routes[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		f.got[key] = m
	}
	h(w, r)
}

func reply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }
}

func TestDraftAndPublishFlow(t *testing.T) {
	f, c := newFake(t)
	f.routes["POST /api/v1/text_notes"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user_id") != "42" {
			t.Errorf("user_id = %q", r.URL.Query().Get("user_id"))
		}
		io.WriteString(w, `{"data":{"id":7,"key":"nabc"}}`)
	}
	f.routes["POST /api/v1/text_notes/draft_save"] = reply(`{"data":{}}`)
	f.routes["PUT /api/v1/text_notes/7"] = reply(`{"data":{}}`)
	f.routes["GET /api/v3/notes/nabc"] = reply(`{"data":{"status":"published","price":300}}`)

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
	h, _ := article.ToHTML(a.Body, nil)
	if err := c.Publish(n.ID, PublishPayload(a, h)); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(n.Key, Expectation{Status: "published", Price: 300}); err != nil {
		t.Fatal(err)
	}

	published := f.got["PUT /api/v1/text_notes/7"]
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

func TestUploadImage(t *testing.T) {
	// 画像の保存先（S3）の偽物
	var s3Fields map[string]string
	var s3File []byte
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		s3Fields = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			s3Fields[k] = v[0]
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		s3File, _ = io.ReadAll(file)
		if r.Header.Get("Cookie") != "" {
			t.Error("S3 にログイン情報を送ってはいけない")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s3.Close()

	f, c := newFake(t)
	c.CurrentUser() // XSRF トークンを受け取る
	f.routes["POST /api/v3/images/upload/presigned_post"] = func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("filename") != "cat.png" {
			t.Errorf("filename = %q", r.FormValue("filename"))
		}
		io.WriteString(w, `{"data":{"url":"https://assets.note/cat.png","action":"`+s3.URL+`","post":{"key":"img/1","policy":"p","x-amz-signature":"s"}}}`)
	}

	u, err := c.UploadImage("cat.png", []byte("\x89PNG\r\n\x1a\nfake"))
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://assets.note/cat.png" {
		t.Errorf("url = %s", u)
	}
	if s3Fields["key"] != "img/1" || s3Fields["policy"] != "p" || string(s3File) != "\x89PNG\r\n\x1a\nfake" {
		t.Errorf("S3 に送った内容: %v %q", s3Fields, s3File)
	}
}

func TestUploadEyecatch(t *testing.T) {
	f, c := newFake(t)
	c.CurrentUser()
	f.routes["POST /api/v1/image_upload/note_eyecatch"] = func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("note_id") != "7" {
			t.Errorf("note_id = %q", r.FormValue("note_id"))
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"data":{}}`)
	}
	if err := c.UploadEyecatch(7, "cover.png", []byte("img")); err != nil {
		t.Fatal(err)
	}
}

// ---- 「API が変わったら止まる」ことのテスト ----

func TestAPIChangedIsDetected(t *testing.T) {
	cases := map[string]struct {
		route, body string
		call        func(c *Client) error
	}{
		"窓口が無くなった(404)": {"", "", func(c *Client) error { _, err := c.CreateDraft(42, "t"); return err }},
		"応答の形が違う":       {"POST /api/v1/text_notes", `{"result":{"note_id":7}}`, func(c *Client) error { _, err := c.CreateDraft(42, "t"); return err }},
		"JSON ではない":     {"PUT /api/v1/text_notes/7", `<html>maintenance</html>`, func(c *Client) error { return c.Publish(7, map[string]any{}) }},
		"画像の応答が違う":      {"POST /api/v3/images/upload/presigned_post", `{"data":{"url":"x"}}`, func(c *Client) error { _, err := c.UploadImage("a.png", []byte("x")); return err }},
		"公開されていない":      {"GET /api/v3/notes/nabc", `{"data":{"status":"draft","price":0}}`, func(c *Client) error { return c.Verify("nabc", Expectation{Status: "published"}) }},
		"価格が無視された":      {"GET /api/v3/notes/nabc", `{"data":{"status":"published","price":0}}`, func(c *Client) error { return c.Verify("nabc", Expectation{Status: "published", Price: 500}) }},
		"記事の形が違う":       {"GET /api/v3/notes/nabc", `{"data":{"state":"published"}}`, func(c *Client) error { return c.Verify("nabc", Expectation{Status: "published"}) }},
		"ユーザーの形が違う":     {"GET /api/v2/current_user", `{"data":{"user_id":42}}`, func(c *Client) error { _, err := c.CurrentUser(); return err }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, c := newFake(t)
			c.xsrf = "tok"
			if tc.route != "" {
				f.routes[tc.route] = reply(tc.body)
			}
			err := tc.call(c)
			if !IsAPIChanged(err) {
				t.Fatalf("APIChangedError になるべき: %v", err)
			}
			if !strings.Contains(err.Error(), "停止しました") {
				t.Errorf("メッセージ: %v", err)
			}
		})
	}
}

func TestUnauthorizedIsNotAPIChange(t *testing.T) {
	_, c := newFake(t)
	c.Session = "bad"
	_, err := c.CurrentUser()
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 401 || IsAPIChanged(err) {
		t.Fatalf("got %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
