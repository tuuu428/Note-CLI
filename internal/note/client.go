// Package note は note.com とやり取り（HTTP 通信）する部分です。
//
// note には公式の公開 API がありません。ここではブラウザの note エディタが裏で使っている
// 非公式の API を、先駆者の調査結果をもとに呼び出しています。
//
// note 側の仕様が変わったときに「よく分からないまま投稿が進む」のを防ぐため、
// 応答の形が想定と違ったら APIChangedError を返してその場で停止します（changed.go）。
package note

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
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
		HTTP:    &http.Client{Timeout: 60 * time.Second},
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
	case 422:
		hint = "（送った設定値を note が受け付けませんでした。`--dry-run` で中身を確認してください）"
	}
	return fmt.Sprintf("note API エラー %s %s: %d %s %s", e.Method, e.Path, e.Status, shorten(e.Body), hint)
}

func shorten(s string) string {
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}

// doJSON は JSON を送って JSON を受け取ります。
func (c *Client) doJSON(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	return c.send(method, path, "application/json", body, out)
}

// send は note に 1 回リクエストを送ります。ログイン Cookie と XSRF トークンはここで付けます。
func (c *Client) send(method, path, contentType string, body io.Reader, out any) error {
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
		req.Header.Set("Content-Type", contentType)
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
		if changedStatus(res.StatusCode) {
			return &APIChangedError{Endpoint: method + " " + path,
				Detail: fmt.Sprintf("ステータス %d（窓口が無くなった・移動した可能性。記事を note 上で削除した場合もこうなります）: %s", res.StatusCode, shorten(string(raw)))}
		}
		return &APIError{Method: method, Path: path, Status: res.StatusCode, Body: string(raw)}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return &APIChangedError{Endpoint: method + " " + path,
				Detail: "応答が JSON ではありませんでした: " + shorten(string(raw))}
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
	const ep = "GET /api/v2/current_user"
	var res struct {
		Data *User `json:"data"`
	}
	if err := c.doJSON(http.MethodGet, "/api/v2/current_user", nil, &res); err != nil {
		return nil, err
	}
	if res.Data == nil {
		return nil, fmt.Errorf("ログインしていません（セッションが無効です）。`note-cli login` を実行してください")
	}
	if res.Data.ID == 0 || res.Data.URLName == "" {
		return nil, &APIChangedError{Endpoint: ep, Detail: "data.id / data.urlname がありません"}
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
		Data *Note `json:"data"`
	}
	in := map[string]any{"body": "<p></p>", "body_length": 0, "name": title, "index": false, "is_lead_form": false}
	path := fmt.Sprintf("/api/v1/text_notes?user_id=%d", userID)
	if err := c.doJSON(http.MethodPost, path, in, &res); err != nil {
		return nil, err
	}
	if res.Data == nil || res.Data.ID == 0 || res.Data.Key == "" {
		return nil, &APIChangedError{Endpoint: "POST /api/v1/text_notes", Detail: "作成した下書きの data.id / data.key が返ってきませんでした"}
	}
	return res.Data, nil
}

// SaveDraft は下書きのタイトルと本文を保存します。
// ※ タグ・価格・マガジンなどは下書き保存では反映されず、公開時（Publish）にだけ反映されます。
func (c *Client) SaveDraft(id int64, title, bodyHTML string, bodyLength int) error {
	in := map[string]any{"body": bodyHTML, "body_length": bodyLength, "name": title, "index": false, "is_lead_form": false}
	var res map[string]any
	return c.doJSON(http.MethodPost, fmt.Sprintf("/api/v1/text_notes/draft_save?id=%d&is_temp_saved=true", id), in, &res)
}

// Publish は記事を公開します（公開済み記事の更新にも使います）。
// note の API は「一部だけ更新」ができないので、payload にはすべての項目を入れて送ります。
func (c *Client) Publish(id int64, payload map[string]any) error {
	var res map[string]any
	return c.doJSON(http.MethodPut, fmt.Sprintf("/api/v1/text_notes/%d", id), payload, &res)
}

// Published は公開後に note から読み直した記事の状態です。
type Published struct {
	Status string `json:"status"`
	Price  *int   `json:"price"`
}

// GetNote は記事の現在の状態を取得します（公開できたかの確認に使う）。
func (c *Client) GetNote(key string) (*Published, error) {
	const ep = "GET /api/v3/notes/{key}"
	var res struct {
		Data *Published `json:"data"`
	}
	if err := c.doJSON(http.MethodGet, "/api/v3/notes/"+url.PathEscape(key), nil, &res); err != nil {
		return nil, err
	}
	if res.Data == nil || res.Data.Status == "" || res.Data.Price == nil {
		return nil, &APIChangedError{Endpoint: ep, Detail: "data.status / data.price がありません"}
	}
	return res.Data, nil
}

// ---- 画像 ----

// UploadImage は本文用の画像をアップロードし、note 上の画像 URL を返します。
//
// 手順は 2 段階です（ブラウザの note エディタと同じ）:
//  1. note に「アップロードしたい」と伝え、保存先（Amazon S3）への一時的な許可証をもらう
//  2. その許可証を付けて、画像を S3 に直接送る
func (c *Client) UploadImage(filename string, data []byte) (string, error) {
	const ep = "POST /api/v3/images/upload/presigned_post"
	body, ctype, err := multipartBody([][2]string{{"filename", filename}}, "", "", nil)
	if err != nil {
		return "", err
	}
	var res struct {
		Data *struct {
			URL    string         `json:"url"`
			Action string         `json:"action"`
			Post   map[string]any `json:"post"`
		} `json:"data"`
	}
	if err := c.send(http.MethodPost, "/api/v3/images/upload/presigned_post", ctype, body, &res); err != nil {
		return "", err
	}
	d := res.Data
	if d == nil || d.URL == "" || d.Action == "" || len(d.Post) == 0 {
		return "", &APIChangedError{Endpoint: ep, Detail: "data.url / data.action / data.post がありません"}
	}

	// S3 は「file 以外の項目を先、file を最後」に送る決まり
	fields := make([][2]string, 0, len(d.Post))
	for _, k := range sortedKeys(d.Post) {
		if v := fmt.Sprint(d.Post[k]); d.Post[k] != nil && v != "" {
			fields = append(fields, [2]string{k, v})
		}
	}
	s3Body, s3Type, err := multipartBody(fields, "file", filename, data)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, d.Action, s3Body)
	if err != nil {
		return "", &APIChangedError{Endpoint: ep, Detail: "data.action が URL ではありません: " + d.Action}
	}
	req.Header.Set("Content-Type", s3Type)
	s3Res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer s3Res.Body.Close()
	if s3Res.StatusCode >= 300 {
		raw, _ := io.ReadAll(s3Res.Body)
		return "", fmt.Errorf("画像の保存先（S3）へのアップロードに失敗しました: %d %s", s3Res.StatusCode, shorten(string(raw)))
	}
	return d.URL, nil
}

// UploadEyecatch は記事の見出し画像（アイキャッチ）を設定します。
func (c *Client) UploadEyecatch(noteID int64, filename string, data []byte) error {
	body, ctype, err := multipartBody([][2]string{{"note_id", fmt.Sprint(noteID)}}, "file", filename, data)
	if err != nil {
		return err
	}
	var res map[string]any
	return c.send(http.MethodPost, "/api/v1/image_upload/note_eyecatch", ctype, body, &res)
}

// multipartBody はファイル送信用の「multipart/form-data」形式の中身を作ります。
func multipartBody(fields [][2]string, fileField, filename string, data []byte) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range fields {
		if err := w.WriteField(f[0], f[1]); err != nil {
			return nil, "", err
		}
	}
	if fileField != "" {
		h := textproto.MIMEHeader{}
		safe := strings.NewReplacer(`"`, "", "\r", "", "\n", "").Replace(filename)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fileField, safe))
		h.Set("Content-Type", http.DetectContentType(data))
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}

// NoteURL は記事の公開 URL を返します。
func NoteURL(urlname, key string) string {
	return fmt.Sprintf("https://note.com/%s/n/%s", urlname, key)
}

// EditURL は記事の編集画面の URL を返します。
func EditURL(key string) string {
	return fmt.Sprintf("https://editor.note.com/notes/%s/edit/", key)
}
