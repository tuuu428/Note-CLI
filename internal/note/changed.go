package note

import (
	"errors"
	"fmt"
	"sort"
)

// APIChangedError は「note の API の仕様が変わったかもしれない」ことを表すエラーです。
//
// 非公式 API は予告なく変わります。変わったまま投稿を続けると、
// 「公開したつもりが下書きのまま」「有料のつもりが無料」のような事故になりかねません。
// そこで、応答の形が想定と違ったらこのエラーを返し、残りの投稿もすべて止めます。
type APIChangedError struct {
	Endpoint string // どの窓口で起きたか
	Detail   string // 何が想定と違ったか
}

func (e *APIChangedError) Error() string {
	return fmt.Sprintf(`note の API 仕様が変わった可能性があるため、処理を停止しました。
  窓口: %s
  内容: %s
  → note の画面で記事の状態を確認してください。
  → 'note-cli doctor' で API の状態を確認できます。
  → 直し方は docs/LEARNING.md の「API が変わったとき」を参照してください。`, e.Endpoint, e.Detail)
}

// IsAPIChanged は err が API 仕様変更のエラーかを判定します。
func IsAPIChanged(err error) bool {
	var e *APIChangedError
	return errors.As(err, &e)
}

// 404（見つからない）/ 405（その方法は使えない）/ 410（廃止）は、窓口自体が変わったサイン。
func changedStatus(code int) bool {
	return code == 404 || code == 405 || code == 410
}

// Expectation は公開後に「本当にその設定になったか」を確かめるための期待値です。
type Expectation struct {
	Status string
	Price  int
}

// Verify は公開後に記事を読み直し、状態と価格が送った通りになっているか確認します。
// 違っていれば、note が送った項目を黙って無視した（= 仕様が変わった）とみなして停止します。
func (c *Client) Verify(key string, want Expectation) error {
	got, err := c.GetNote(key)
	if err != nil {
		return err
	}
	if got.Status != want.Status || *got.Price != want.Price {
		return &APIChangedError{
			Endpoint: "PUT /api/v1/text_notes/{id}",
			Detail: fmt.Sprintf("送った設定が反映されていません（期待: status=%s price=%d / 実際: status=%s price=%d）",
				want.Status, want.Price, got.Status, *got.Price),
		}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
