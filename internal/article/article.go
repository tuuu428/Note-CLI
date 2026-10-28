// Package article は、ローカルに置いた記事ファイル（.md / .txt）を読み込み、
// 「設定（フロントマター）」と「本文」に分けて扱うためのパッケージです。
package article

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// 公開状態
const (
	StatusDraft     = "draft"     // 下書き
	StatusPublished = "published" // 公開
)

// DefaultsFileName は、同じフォルダの記事すべてに効く「共通設定」ファイルの名前です。
const DefaultsFileName = "note-defaults.yaml"

// Settings は記事ファイル先頭の `---` で囲まれた部分（フロントマター）に書ける設定です。
// ポインタ型（*bool など）は「書かれていない」と「false と書かれた」を区別するために使っています。
type Settings struct {
	Title  string   `yaml:"title"`
	Tags   []string `yaml:"tags"`
	Status string   `yaml:"status"` // draft / published

	// 有料記事。0 なら無料。有料にする場合は本文に <!-- paid --> の行を入れて、そこから下を有料部分にする。
	Price  *int  `yaml:"price"`
	Refund *bool `yaml:"refund"` // 返金を許可するか（有料記事のみ）

	// 追加するマガジン。"m1234abcd" のようなキー、または数値IDで指定。
	Magazines []string `yaml:"magazines"`

	// メンバーシップ限定公開にするプランのキー（実験的）。
	MembershipPlans []string `yaml:"membership_plans"`

	// 見出し画像（アイキャッチ）。記事ファイルからの相対パス。例: ./cover.png
	Eyecatch string `yaml:"eyecatch"`

	Comments *bool `yaml:"comments"` // コメントを受け付けるか
	Notify   *bool `yaml:"notify"`   // フォロワーに通知するか

	// note の API に直接送る追加項目。note 側の仕様変更に CLI の更新なしで対応するための「逃げ道」。
	Extra map[string]any `yaml:"extra"`
}

// Article は読み込んだ記事 1 本分です。
type Article struct {
	Path     string
	Settings Settings
	Body     string // フロントマターを除いた本文（Markdown またはプレーンテキスト）
}

var frontMatterRe = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---\r?\n?`)

// Load は記事ファイルを読み込みます。同じフォルダに note-defaults.yaml があれば、
// それを共通設定として先に適用し、記事ごとの設定で上書きします。
func Load(path string) (*Article, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")) // Windows のメモ帳が付ける BOM を除去

	var s Settings
	if err := loadDefaults(filepath.Dir(path), &s); err != nil {
		return nil, err
	}

	body := string(raw)
	if m := frontMatterRe.FindStringSubmatch(body); m != nil {
		var own Settings
		if err := yaml.Unmarshal([]byte(m[1]), &own); err != nil {
			return nil, fmt.Errorf("%s: フロントマターの書き方が正しくありません: %w", path, err)
		}
		merge(&s, own)
		body = body[len(m[0]):]
	} else if s.Title == "" {
		// フロントマターが無いテキストファイルは「1 行目 = タイトル」とみなす
		first, rest, _ := strings.Cut(body, "\n")
		s.Title = strings.TrimSpace(strings.TrimLeft(first, "# "))
		body = rest
	}

	if s.Status == "" {
		s.Status = StatusDraft
	}
	return &Article{Path: path, Settings: s, Body: strings.TrimSpace(body)}, nil
}

func loadDefaults(dir string, s *Settings) error {
	raw, err := os.ReadFile(filepath.Join(dir, DefaultsFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(raw, s); err != nil {
		return fmt.Errorf("%s の書き方が正しくありません: %w", DefaultsFileName, err)
	}
	return nil
}

// merge は記事ごとの設定 own を、共通設定 dst の上に重ねます（書かれている項目だけ上書き）。
func merge(dst *Settings, own Settings) {
	if own.Title != "" {
		dst.Title = own.Title
	}
	if own.Tags != nil {
		dst.Tags = own.Tags
	}
	if own.Status != "" {
		dst.Status = own.Status
	}
	if own.Price != nil {
		dst.Price = own.Price
	}
	if own.Refund != nil {
		dst.Refund = own.Refund
	}
	if own.Magazines != nil {
		dst.Magazines = own.Magazines
	}
	if own.MembershipPlans != nil {
		dst.MembershipPlans = own.MembershipPlans
	}
	if own.Eyecatch != "" {
		dst.Eyecatch = own.Eyecatch
	}
	if own.Comments != nil {
		dst.Comments = own.Comments
	}
	if own.Notify != nil {
		dst.Notify = own.Notify
	}
	if own.Extra != nil {
		if dst.Extra == nil {
			dst.Extra = map[string]any{}
		}
		for k, v := range own.Extra {
			dst.Extra[k] = v
		}
	}
}

// PriceValue は価格（未指定なら 0 = 無料）を返します。
func (s Settings) PriceValue() int {
	if s.Price == nil {
		return 0
	}
	return *s.Price
}

// BoolOr は *bool が未指定なら def を返します。
func BoolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// Validate は投稿前に設定の矛盾をチェックします。問題があればすべてまとめて返します。
func (a *Article) Validate() error {
	var errs []string
	s := a.Settings
	if strings.TrimSpace(s.Title) == "" {
		errs = append(errs, "タイトルがありません（title: を書くか、1 行目にタイトルを書いてください）")
	}
	if a.Body == "" {
		errs = append(errs, "本文が空です")
	}
	if s.Status != StatusDraft && s.Status != StatusPublished {
		errs = append(errs, fmt.Sprintf("status は draft か published にしてください（今: %q）", s.Status))
	}
	if len(s.Tags) > 10 {
		errs = append(errs, "タグは 10 個までにしてください")
	}
	price := s.PriceValue()
	hasMarker := HasPaidMarker(a.Body)
	switch {
	case price != 0 && (price < 100 || price > 50000):
		errs = append(errs, "価格は 0（無料）か 100〜50000 円にしてください")
	case price > 0 && !hasMarker:
		errs = append(errs, "有料記事は、本文の「ここから有料」にしたい位置に <!-- paid --> の行を入れてください")
	case price == 0 && hasMarker:
		errs = append(errs, "本文に <!-- paid --> がありますが price が 0 です（有料にするなら price を設定）")
	}
	errs = append(errs, a.checkImages()...)
	if len(errs) > 0 {
		return fmt.Errorf("%s:\n  - %s", a.Path, strings.Join(errs, "\n  - "))
	}
	return nil
}

// 画像として送れる形式（note のエディタが受け付けるもの）
var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

// ImagePath は本文やフロントマターに書いた画像の場所を、実際のファイルパスに直します。
// 相対パスは記事ファイルがあるフォルダが基準です。"画像%201.png" のような URL 形式も読めます。
func (a *Article) ImagePath(ref string) string {
	if u, err := url.PathUnescape(ref); err == nil {
		ref = u
	}
	if filepath.IsAbs(ref) {
		return ref
	}
	return filepath.Join(filepath.Dir(a.Path), filepath.FromSlash(ref))
}

func (a *Article) checkImages() []string {
	var errs []string
	refs := ImageRefs(a.Body)
	if a.Settings.Eyecatch != "" {
		refs = append(refs, a.Settings.Eyecatch)
	}
	for _, ref := range refs {
		p := a.ImagePath(ref)
		if !imageExts[strings.ToLower(filepath.Ext(p))] {
			errs = append(errs, fmt.Sprintf("画像 %s は対応していない形式です（png / jpg / gif / webp）", ref))
			continue
		}
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			errs = append(errs, fmt.Sprintf("画像 %s が見つかりません（%s）", ref, p))
		}
	}
	return errs
}

// Visibility は公開範囲を人間向けの日本語で説明します（確認表示用）。
func (s Settings) Visibility() string {
	if s.Status == StatusDraft {
		return "下書き（自分だけ）"
	}
	parts := []string{"公開"}
	if p := s.PriceValue(); p > 0 {
		parts = append(parts, fmt.Sprintf("有料 %d円（<!-- paid --> から下）", p))
	} else {
		parts = append(parts, "無料")
	}
	if len(s.MembershipPlans) > 0 {
		parts = append(parts, "メンバーシップ限定: "+strings.Join(s.MembershipPlans, ", "))
	}
	if len(s.Magazines) > 0 {
		parts = append(parts, "マガジン: "+strings.Join(s.Magazines, ", "))
	}
	if s.Eyecatch != "" {
		parts = append(parts, "見出し画像あり")
	}
	if !BoolOr(s.Comments, true) {
		parts = append(parts, "コメント不可")
	}
	if !BoolOr(s.Notify, true) {
		parts = append(parts, "通知なし")
	}
	return strings.Join(parts, " / ")
}

// IsArticleFile は CLI が記事として扱うファイルかを判定します（README.md や隠しファイルは除く）。
func IsArticleFile(name string) bool {
	base := filepath.Base(name)
	ext := strings.ToLower(filepath.Ext(base))
	return (ext == ".md" || ext == ".txt") && !strings.HasPrefix(base, ".") && !strings.EqualFold(base, "README.md")
}
