// Package note は note.com とやり取り（HTTP 通信）する部分です。
//
// note には公式の公開 API がありません。ここではブラウザの note エディタが裏で使っている
// 非公式の API を、先駆者の調査結果をもとに呼び出しています。note 側の変更で動かなく
// なる可能性がある点に注意してください。
package note

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const DefaultBaseURL = "https://note.com"

// ブラウザからのアクセスに見せるための User-Agent（note はこれが無いと拒否することがある）
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// Client は note の API を呼ぶための道具です。
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Session string // _note_session_v5 Cookie の値（ログインしている証拠）
	xsrf    string // 書き込み時に必要な XSRF トークン（なりすまし対策の合言葉）
}

func NewClient(session string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Session: session,
	}
}

// APIError は note から返ってきたエラーです。
type APIError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *APIError) Error() string {
	hint := ""
	switch e.Status {
	case 401, 403:
		hint = "（ログインの有効期限切れかもしれません。`note-cli login` をやり直してください）"
	case 404:
		hint = "（note 側の API が変わった可能性があります）"
	case 422:
		hint = "（送った設定値を note が受け付けませんでした。`--dry-run` で中身を確認してください）"
	}
	body := e.Body
	if len(body) > 300 {
		body = body[:300] + "..."
	}
	return fmt.Sprintf("note API エラー %s %s: %d %s %s", e.Method, e.Path, e.Status, body, hint)
}

func (c *Client) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	cookie := "_note_session_v5=" + c.Session
	if c.xsrf != "" {
		cookie += "; XSRF-TOKEN=" + url.QueryEscape(c.xsrf)
	}
	req.Header.Set("Cookie", cookie)
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://editor.note.com")
		req.Header.Set("Referer", "https://editor.note.com/")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		if c.xsrf != "" {
			req.Header.Set("X-XSRF-TOKEN", c.xsrf)
		}
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	c.captureXSRF(res)

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return &APIError{Method: method, Path: path, Status: res.StatusCode, Body: string(raw)}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("note の応答を読めませんでした: %w", err)
		}
	}
	return nil
}

// captureXSRF は応答に XSRF トークンが含まれていれば覚えておきます。
func (c *Client) captureXSRF(res *http.Response) {
	if t := res.Header.Get("X-XSRF-TOKEN"); t != "" {
		c.xsrf, _ = url.QueryUnescape(t)
		return
	}
	for _, ck := range res.Cookies() {
		if ck.Name == "XSRF-TOKEN" {
			c.xsrf, _ = url.QueryUnescape(ck.Value)
		}
	}
}

// User はログイン中のユーザーです。
type User struct {
	ID       int64  `json:"id"`
	URLName  string `json:"urlname"`
	Nickname string `json:"nickname"`
}

// CurrentUser はログイン中のユーザー情報を取得します（XSRF トークンもここで受け取る）。
func (c *Client) CurrentUser() (*User, error) {
	var res struct {
		Data *User `json:"data"`
	}
	if err := c.do(http.MethodGet, "/api/v2/current_user", nil, &res); err != nil {
		return nil, err
	}
	if res.Data == nil || res.Data.ID == 0 {
		return nil, fmt.Errorf("ログインしていません（セッションが無効です）。`note-cli login` を実行してください")
	}
	return res.Data, nil
}

// SignIn はメールアドレスとパスワードでログインし、セッションの値を返します。
// note 側でロボット対策（reCAPTCHA）が出ると失敗するので、その場合は Cookie 方式を使ってください。
func (c *Client) SignIn(email, password string) (string, error) {
	cred, _ := json.Marshal(map[string]string{"login": email, "password": password})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1/sessions/sign_in", bytes.NewReader(cred))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return "", &APIError{Method: "POST", Path: "/api/v1/sessions/sign_in", Status: res.StatusCode, Body: string(raw)}
	}
	for _, ck := range res.Cookies() {
		if ck.Name == "_note_session_v5" {
			return ck.Value, nil
		}
	}
	var body struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &body) == nil && body.Data.Token != "" {
		return body.Data.Token, nil
	}
	return "", fmt.Errorf("ログイン応答にセッションが含まれていませんでした。Cookie 方式（--cookie）を試してください")
}

// Note は作成した記事の識別子です。ID は数値、Key は URL に出る "n1234abcd" 形式。
type Note struct {
	ID  int64  `json:"id"`
	Key string `json:"key"`
}

// CreateDraft は空の下書きを新しく作ります。
func (c *Client) CreateDraft(userID int64, title string) (*Note, error) {
	var res struct {
		Data Note `json:"data"`
	}
	in := map[string]any{"body": "<p></p>", "body_length": 0, "name": title, "index": false, "is_lead_form": false}
	if err := c.do(http.MethodPost, fmt.Sprintf("/api/v1/text_notes?user_id=%d", userID), in, &res); err != nil {
		return nil, err
	}
	if res.Data.ID == 0 {
		return nil, fmt.Errorf("下書きを作成できませんでした（ID が返ってきませんでした）")
	}
	if res.Data.Key == "" {
		res.Data.Key = fmt.Sprintf("n%d", res.Data.ID)
	}
	return &res.Data, nil
}

// SaveDraft は下書きのタイトルと本文を保存します。
// ※ タグ・価格・マガジンなどは下書き保存では反映されず、公開時（Publish）にだけ反映されます。
func (c *Client) SaveDraft(id int64, title, bodyHTML string, bodyLength int) error {
	in := map[string]any{"body": bodyHTML, "body_length": bodyLength, "name": title, "index": false, "is_lead_form": false}
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/text_notes/draft_save?id=%d&is_temp_saved=true", id), in, nil)
}

// Publish は記事を公開します（公開済み記事の更新にも使います）。
// note の API は「一部だけ更新」ができないので、payload にはすべての項目を入れて送ります。
func (c *Client) Publish(id int64, payload map[string]any) error {
	return c.do(http.MethodPut, fmt.Sprintf("/api/v1/text_notes/%d", id), payload, nil)
}

// NoteURL は記事の公開 URL を返します。
func NoteURL(urlname, key string) string {
	return fmt.Sprintf("https://note.com/%s/n/%s", urlname, key)
}

// EditURL は記事の編集画面の URL を返します。
func EditURL(key string) string {
	return fmt.Sprintf("https://editor.note.com/notes/%s/edit/", key)
}
