package article

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
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
	goldmark.WithRendererOptions(gmhtml.WithHardWraps()),
)

var openTagRe = regexp.MustCompile(`^<([a-z0-9]+)`)

// ToHTML は Markdown / テキストの本文を note 用の HTML に変換します。
//
// note のエディタは段落ごとに name と id（ランダムな ID）を持っています。
// 有料ラインの位置もこの ID で指定するので、段落ごとに ID を振ります。
//
// images は「本文に書いた画像の場所 → アップロード後の URL」の対応表です。
// 対応表に無いローカル画像は "upload://場所" という仮の URL になります（--dry-run 用）。
func ToHTML(body string, images map[string]string) (HTML, error) {
	src := []byte(body)
	doc := md.Parser().Parse(text.NewReader(src))
	replaceImageURLs(doc, images)

	var out HTML
	var full, free, paid strings.Builder
	inPaid := false

	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if isPaidMarker(n, src) {
			inPaid = true
			continue
		}
		normalizeHeading(n)

		id := newID()
		var block string
		if img := soleImage(n); img != nil {
			block = figure(id, img, src)
		} else {
			var buf bytes.Buffer
			if err := md.Renderer().Render(&buf, src, n); err != nil {
				return out, err
			}
			block = openTagRe.ReplaceAllString(strings.TrimSpace(buf.String()),
				fmt.Sprintf(`<$1 name="%s" id="%s"`, id, id))
		}

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

// ImageRefs は本文中のローカル画像（http で始まらないもの）の場所を、登場順・重複なしで返します。
func ImageRefs(body string) []string {
	src := []byte(body)
	doc := md.Parser().Parse(text.NewReader(src))
	var refs []string
	seen := map[string]bool{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if img, ok := n.(*ast.Image); ok && entering {
			dest := string(img.Destination)
			if isLocal(dest) && !seen[dest] {
				seen[dest] = true
				refs = append(refs, dest)
			}
		}
		return ast.WalkContinue, nil
	})
	return refs
}

func isLocal(dest string) bool {
	return !strings.HasPrefix(dest, "http://") && !strings.HasPrefix(dest, "https://") && !strings.HasPrefix(dest, "data:")
}

func replaceImageURLs(doc ast.Node, images map[string]string) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if img, ok := n.(*ast.Image); ok && entering {
			dest := string(img.Destination)
			if !isLocal(dest) {
				return ast.WalkContinue, nil
			}
			if u, ok := images[dest]; ok {
				img.Destination = []byte(u)
			} else {
				img.Destination = []byte("upload://" + dest)
			}
		}
		return ast.WalkContinue, nil
	})
}

// soleImage は「画像だけの段落」なら、その画像を返します。
func soleImage(n ast.Node) *ast.Image {
	if n.Kind() != ast.KindParagraph || n.ChildCount() != 1 {
		return nil
	}
	img, _ := n.FirstChild().(*ast.Image)
	return img
}

// figure は note の画像ブロックの形にします。![キャプション](画像) のキャプション部分は画像の下に表示されます。
func figure(id string, img *ast.Image, src []byte) string {
	caption := html.EscapeString(string(img.Text(src)))
	s := fmt.Sprintf(`<figure name="%s" id="%s"><img src="%s" alt="" width="620" height="auto">`,
		id, id, html.EscapeString(string(img.Destination)))
	if caption != "" {
		s += "<figcaption>" + caption + "</figcaption>"
	}
	return s + "</figure>"
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
