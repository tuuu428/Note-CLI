package article

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// 本文中のこの行から下が有料部分になります。<!-- 有料 --> でも OK。
var paidMarkerRe = regexp.MustCompile(`^\s*<!--\s*(paid|有料|ここから有料)\s*-->\s*$`)

// HasPaidMarker は本文に有料ラインの目印があるかを返します。
func HasPaidMarker(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if paidMarkerRe.MatchString(line) {
			return true
		}
	}
	return false
}

// HTML は note のエディタが扱う形に変換した本文です。
type HTML struct {
	Full      string // 本文全体
	Free      string // 無料部分
	Paid      string // 有料部分（無料記事なら空）
	Separator string // 無料部分の最後の段落の ID（有料ラインの位置）。無料記事なら空
	Length    int    // 文字数
}

var md = goldmark.New(
	goldmark.WithExtensions(extension.Strikethrough, extension.Linkify),
	// 1 回の改行をそのまま改行（<br>）として扱う。テキストファイルの見た目どおりになる
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

var openTagRe = regexp.MustCompile(`^<([a-z0-9]+)`)

// ToHTML は Markdown / テキストの本文を note 用の HTML に変換します。
//
// note のエディタは段落ごとに name と id（ランダムな ID）を持っています。
// 有料ラインの位置もこの ID で指定するので、段落ごとに ID を振ります。
func ToHTML(body string) (HTML, error) {
	src := []byte(body)
	doc := md.Parser().Parse(text.NewReader(src))

	var out HTML
	var full, free, paid strings.Builder
	inPaid := false

	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if isPaidMarker(n, src) {
			inPaid = true
			continue
		}
		normalizeHeading(n)

		var buf bytes.Buffer
		if err := md.Renderer().Render(&buf, src, n); err != nil {
			return out, err
		}
		id := newID()
		block := openTagRe.ReplaceAllString(strings.TrimSpace(buf.String()),
			fmt.Sprintf(`<$1 name="%s" id="%s"`, id, id))

		full.WriteString(block)
		if inPaid {
			paid.WriteString(block)
		} else {
			free.WriteString(block)
			out.Separator = id
		}
	}

	out.Full, out.Free, out.Paid = full.String(), free.String(), paid.String()
	if !inPaid {
		out.Separator = ""
	}
	out.Length = len([]rune(stripTags(out.Full)))
	return out, nil
}

func isPaidMarker(n ast.Node, src []byte) bool {
	if n.Kind() != ast.KindHTMLBlock {
		return false
	}
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(src))
	}
	return paidMarkerRe.MatchString(strings.TrimSpace(b.String()))
}

// note の見出しは「大見出し(h2)」と「小見出し(h3)」の 2 種類だけなので合わせる。
func normalizeHeading(n ast.Node) {
	h, ok := n.(*ast.Heading)
	if !ok {
		return
	}
	if h.Level <= 2 {
		h.Level = 2
	} else {
		h.Level = 3
	}
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

// newID は UUID v4 形式のランダムな ID を作ります。
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

var localImageRe = regexp.MustCompile(`!\[[^\]]*\]\((?:\./|\.\./|/)?[^):]+\)`)

// LocalImages は本文中のローカル画像参照（未対応）を返します。警告表示に使います。
func LocalImages(body string) []string {
	return localImageRe.FindAllString(body, -1)
}
