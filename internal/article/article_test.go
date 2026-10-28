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
	h, err := ToHTML("# 見出し\n\n無料です\n\n<!-- paid -->\n\n有料です\n#### 小")
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
	h, err := ToHTML("一行目\n二行目")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.Full, "<br") || h.Separator != "" {
		t.Errorf("unexpected: %+v", h)
	}
}

func TestLocalImages(t *testing.T) {
	got := LocalImages("![a](./img.png) ![b](https://example.com/x.png)")
	if len(got) != 1 {
		t.Errorf("ローカル画像だけを検出するべき: %v", got)
	}
}
