package note

import (
	"strconv"
	"strings"

	"github.com/tuuu428/note-cli/internal/article"
)

// PublishPayload は記事の設定から、公開 API（PUT /api/v1/text_notes/{id}）に送る中身を組み立てます。
//
// 項目名は先駆者がブラウザの通信を調べて分かったものです。status / name / body / hashtags /
// price / free_body / pay_body / separator / magazine_ids は複数の調査で確認されています。
// メンバーシップ（circle_permissions / limited）は確認が少なく実験的です。
// 合わない項目があればフロントマターの extra: で上書きできます。
func PublishPayload(a *article.Article, h article.HTML) map[string]any {
	s := a.Settings

	tags := make([]string, 0, len(s.Tags))
	for _, t := range s.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "#") {
			t = "#" + t // note のハッシュタグは「#」付きで送る
		}
		tags = append(tags, t)
	}

	magazineIDs, magazineKeys := []int64{}, []string{}
	for _, m := range s.Magazines {
		if n, err := strconv.ParseInt(m, 10, 64); err == nil {
			magazineIDs = append(magazineIDs, n)
		} else {
			magazineKeys = append(magazineKeys, m)
		}
	}

	plans := s.MembershipPlans
	if plans == nil {
		plans = []string{}
	}

	price := s.PriceValue()
	freeBody, payBody := h.Full, ""
	var separator any // 無料記事なら null
	if price > 0 {
		freeBody, payBody, separator = h.Free, h.Paid, h.Separator
	}

	p := map[string]any{
		"status":                  s.Status,
		"name":                    s.Title,
		"body":                    h.Full,
		"body_length":             h.Length,
		"free_body":               freeBody,
		"pay_body":                payBody,
		"separator":               separator,
		"price":                   price,
		"is_refund":               price > 0 && article.BoolOr(s.Refund, false),
		"hashtags":                tags,
		"magazine_ids":            magazineIDs,
		"magazine_keys":           magazineKeys,
		"circle_permissions":      plans,
		"limited":                 len(plans) > 0,
		"disable_comment":         !article.BoolOr(s.Comments, true),
		"send_notifications_flag": article.BoolOr(s.Notify, true),
		"author_ids":              []int64{},
		"image_keys":              []string{},
		"index":                   false,
		"is_lead_form":            false,
	}
	for k, v := range s.Extra {
		p[k] = v
	}
	return p
}
