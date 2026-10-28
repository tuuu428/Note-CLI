package article

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFrontMatter(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "a.md", "---\ntitle: こんにちは\ntags: [日記, Go]\nstatus: published\ncomments: false\n---\n本文です\n")
	a, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Settings.Title != "こんにちは" || a.Body != "本文です" || len(a.Settings.Tags) != 2 {
		t.Fatalf("unexpected: %+v", a)
	}
	if BoolOr(a.Settings.Comments, true) {
		t.Error("comments: false が反映されていない")
	}
	if err := a.Validate(); err != nil {
		t.Error(err)
	}
}

func TestLoadPlainTextUsesFirstLineAsTitle(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "a.txt", "\xef\xbb\xbf私のタイトル\n\n一行目\n二行目\n")
	a, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Settings.Title != "私のタイトル" || a.Settings.Status != StatusDraft {
		t.Fatalf("unexpected settings: %+v", a.Settings)
	}
	if a.Body != "一行目\n二行目" {
		t.Fatalf("unexpected body: %q", a.Body)
	}
}

func TestDefaultsAreOverriddenPerFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, DefaultsFileName, "status: published\ntags: [共通]\nnotify: false\n")
	p := write(t, dir, "a.md", "---\ntitle: T\ntags: [個別]\n---\nbody\n")
	a, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s := a.Settings
	if s.Status != StatusPublished || s.Tags[0] != "個別" || BoolOr(s.Notify, true) {
		t.Fatalf("defaults not merged correctly: %+v", s)
	}
}

func TestValidatePaid(t *testing.T) {
	price := 500
	cases := []struct {
		name  string
		price *int
		body  string
		ok    bool
	}{
		{"free", nil, "a", true},
		{"paid with marker", &price, "a\n\n<!-- paid -->\n\nb", true},
		{"paid without marker", &price, "a", false},
		{"marker without price", nil, "a\n<!-- 有料 -->\nb", false},
	}
	for _, c := range cases {
		a := &Article{Path: "x", Body: c.body, Settings: Settings{Title: "t", Status: StatusPublished, Price: c.price}}
		if err := a.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: got err=%v", c.name, err)
		}
	}
	bad := 50
	a := &Article{Path: "x", Body: "a\n<!-- paid -->\nb", Settings: Settings{Title: "t", Status: StatusPublished, Price: &bad}}
	if err := a.Validate(); err == nil {
		t.Error("50円は不正な価格としてエラーになるべき")
	}
}

func TestToHTMLSplitsPaidSection(t *testing.T) {
	h, err := ToHTML("# 見出し\n\n無料です\n\n<!-- paid -->\n\n有料です\n#### 小", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h.Free, "<h2 name=") || !strings.Contains(h.Free, "無料です") {
		t.Errorf("free part: %s", h.Free)
	}
	if !strings.Contains(h.Paid, "有料です") || !strings.Contains(h.Paid, "<h3 ") || strings.Contains(h.Free, "有料です") {
		t.Errorf("paid part: %s", h.Paid)
	}
	if h.Separator == "" || !strings.Contains(h.Free, `id="`+h.Separator+`">無料です`) {
		t.Errorf("separator %q should point at the last free paragraph: %s", h.Separator, h.Free)
	}
	if strings.Contains(h.Full, "paid") {
		t.Error("有料ラインの目印は本文に残さない")
	}
}

func TestToHTMLHardWraps(t *testing.T) {
	h, err := ToHTML("一行目\n二行目", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.Full, "<br") || h.Separator != "" {
		t.Errorf("unexpected: %+v", h)
	}
}

func TestImages(t *testing.T) {
	body := "前\n\n![猫の写真](./img/cat.png)\n\n文中の ![](dog.jpg) と ![](https://example.com/x.png)\n\n![](./img/cat.png)"
	refs := ImageRefs(body)
	if len(refs) != 2 || refs[0] != "./img/cat.png" || refs[1] != "dog.jpg" {
		t.Fatalf("ローカル画像だけを重複なしで返すべき: %v", refs)
	}

	h, err := ToHTML(body, map[string]string{"./img/cat.png": "https://assets.example/cat.png"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.Full, `<img src="https://assets.example/cat.png" alt="" width="620" height="auto"><figcaption>猫の写真</figcaption></figure>`) {
		t.Errorf("画像だけの段落は note の figure になるべき: %s", h.Full)
	}
	if !strings.Contains(h.Full, `src="upload://dog.jpg"`) || !strings.Contains(h.Full, `src="https://example.com/x.png"`) {
		t.Errorf("文中の画像: %s", h.Full)
	}
}

func TestValidateImages(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ok.png", "x")
	write(t, dir, "doc.pdf", "x")
	p := write(t, dir, "a.md", "---\ntitle: t\neyecatch: missing.png\n---\n![](ok.png)\n\n![](doc.pdf)\n\n![](%E7%84%A1.png)\n")
	a, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	err = a.Validate()
	if err == nil {
		t.Fatal("エラーになるべき")
	}
	msg := err.Error()
	for _, want := range []string{"missing.png が見つかりません", "doc.pdf は対応していない形式", "%E7%84%A1.png が見つかりません"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q が含まれていない: %s", want, msg)
		}
	}
	if strings.Contains(msg, "ok.png") {
		t.Errorf("存在する画像はエラーにしない: %s", msg)
	}
}
