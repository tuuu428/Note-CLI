package note

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ---- ログイン情報の保存 ----

// SessionPath はログイン情報の保存場所です（例: ~/.config/note-cli/session.json）。
func SessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "note-cli", "session.json"), nil
}

// SaveSession はログイン情報を「自分だけが読める」権限（0600）で保存します。
func SaveSession(session string) (string, error) {
	path, err := SessionPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]string{"session": session})
	return path, os.WriteFile(path, b, 0o600)
}

// LoadSession はログイン情報を読み込みます。環境変数 NOTE_SESSION があればそちらを優先します。
func LoadSession() (string, error) {
	if s := os.Getenv("NOTE_SESSION"); s != "" {
		return s, nil
	}
	path, err := SessionPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("ログインしていません。先に `note-cli login` を実行してください")
	}
	if err != nil {
		return "", err
	}
	var v struct{ Session string }
	if err := json.Unmarshal(b, &v); err != nil || v.Session == "" {
		return "", fmt.Errorf("%s が壊れています。`note-cli login` をやり直してください", path)
	}
	return v.Session, nil
}

// DeleteSession はログイン情報を削除します。
func DeleteSession() error {
	path, err := SessionPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ---- 投稿済み記事の記録 ----

// StateFileName は、フォルダ内のどのファイルがどの note 記事になったかの記録ファイルです。
// これがあるので、同じファイルをもう一度投稿すると「新しい記事」ではなく「更新」になります。
const StateFileName = ".note-cli.json"

type Posted struct {
	ID     int64  `json:"id"`
	Key    string `json:"key"`
	Status string `json:"status"`
	URL    string `json:"url"`

	Images   map[string]string `json:"images,omitempty"`   // 画像の中身のハッシュ値 → アップロード後の URL
	Eyecatch string            `json:"eyecatch,omitempty"` // 設定済みの見出し画像のハッシュ値
}

// Hash はファイルの中身から「指紋」（SHA-256）を作ります。中身が 1 バイトでも違えば別の値になります。
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type State map[string]Posted // キーはファイル名

func LoadState(dir string) (State, error) {
	b, err := os.ReadFile(filepath.Join(dir, StateFileName))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return nil, err
	}
	st := State{}
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("%s が壊れています: %w", StateFileName, err)
	}
	return st, nil
}

func (st State) Save(dir string) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, StateFileName), append(b, '\n'), 0o644)
}
