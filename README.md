# note-cli

ローカルに書きためた記事ファイル（`.md` / `.txt`）を、コマンド 1 つで [note](https://note.com) に下書き保存・公開する CLI です。Go で書いています。

公開範囲（下書き／公開／有料／メンバーシップ限定／マガジン追加／コメント可否／通知）は、記事ごとまたはフォルダごとに細かく設定できます。

> ⚠️ **注意**: note には公式の公開 API がありません。このツールは、note のブラウザ版エディタが裏で使っている**非公式 API** を先駆者の調査をもとに呼び出しています。
> - note 側の仕様変更で突然動かなくなる可能性があります
> - 非公式 API や自動投稿を note が推奨していない可能性があります。大量投稿や短時間の連続投稿は避け、利用規約を確認したうえで自己責任で使ってください
> - まずは**下書き保存**で試し、note の画面で見た目を確認してから公開するのがおすすめです

技術の解説（非エンジニア向け）は [docs/LEARNING.md](docs/LEARNING.md) にあります。

## インストール

[Go](https://go.dev/dl/) を入れたあと:

```sh
go install github.com/tuuu428/note-cli@latest
```

またはこのリポジトリをダウンロードして:

```sh
go build -o note-cli .
```

## 使い方

### 1. ログイン

```sh
note-cli login            # メールアドレス + パスワード
note-cli login --cookie   # ↑で失敗する場合: ブラウザの Cookie を貼り付ける
note-cli whoami           # ログインできているか確認
```

`--cookie` の場合は、ブラウザで note にログインした状態で開発者ツール（F12）→「Application（アプリケーション）」→「Cookies」→ `https://note.com` の `_note_session_v5` の値をコピーして貼り付けます。

ログイン情報は自分だけが読める権限でこの PC（`~/.config/note-cli/session.json` など）に保存されます。**この値は他人に見せないでください**（あなたとして note を操作できてしまいます）。消すときは `note-cli logout`。

### 2. 記事を書く

```sh
note-cli new 2026-10-28.md "今日のタイトル"   # ひな形を作る
```

```markdown
---
title: 今日のタイトル
tags: [日記, Go]
status: draft        # draft = 下書き / published = 公開
price: 0             # 0 = 無料
comments: true       # コメントを受け付ける
notify: true         # フォロワーに通知する
---

本文を Markdown で書きます。

## 見出し
```

フロントマター（先頭の `---` で囲んだ部分）が無い `.txt` ファイルは、**1 行目がタイトル**、残りが本文になり、下書き扱いです。

### 3. チェックして投稿

```sh
note-cli check 記事フォルダ/            # 投稿せずに設定チェック & 公開範囲の一覧
note-cli post 記事.md --dry-run        # 送る内容を表示するだけ
note-cli post 記事.md                  # 投稿（status に従う）
note-cli post 記事フォルダ/             # フォルダ内の .md / .txt をまとめて投稿
note-cli post 記事.md --draft          # 設定に関係なく下書き保存
note-cli post 記事.md --publish        # 設定に関係なく公開
```

- 公開する記事があるときは、投稿前に一覧が表示され確認を求められます（`-y` で省略）
- 1 ファイルでも設定ミスがあると、**何も投稿せずに**止まります
- 投稿済みのファイルはフォルダ内の `.note-cli.json` に記録され、もう一度 `post` すると**同じ記事を更新**します（重複投稿しません）

## 公開範囲の設定一覧

| 項目 | 例 | 意味 |
|---|---|---|
| `title` | `今日のタイトル` | タイトル |
| `tags` | `[日記, Go]` | ハッシュタグ（`#` は自動で付きます。10 個まで） |
| `status` | `draft` / `published` | 下書き / 公開（既定は `draft`） |
| `price` | `300` | 有料記事の価格（100〜50000円）。本文に `<!-- paid -->` の行が必要 |
| `refund` | `true` | 有料記事の返金を許可するか |
| `magazines` | `[m1234abcd]` | 追加するマガジン（URL の `/m/` の後ろのキー、または数値 ID） |
| `membership_plans` | `[プランキー]` | メンバーシップ限定公開（**実験的**） |
| `comments` | `false` | コメントを受け付けない |
| `notify` | `false` | フォロワーに通知しない |
| `extra` | `{index: true}` | note の API に直接送る追加項目（上級者向け） |

### 有料記事

```markdown
---
title: 有料記事
status: published
price: 300
---

ここまでは無料で読めます。

<!-- paid -->

ここから先は購入者だけが読めます。
```

`<!-- paid -->`（`<!-- 有料 -->` でも可）の行から下が有料部分になります。

### フォルダ共通の設定

記事と同じフォルダに `note-defaults.yaml` を置くと、そのフォルダの全記事に効く既定値になります。記事ごとのフロントマターが優先されます。

```yaml
# note-defaults.yaml
tags: [日記]
notify: false
magazines: [m1234abcd]
```

サンプルは [examples/](examples/) にあります。

## できないこと・既知の制限

- **画像**: 本文中のローカル画像（`![](./img.png)`）はアップロードしません（警告を表示します）。投稿後に note の編集画面で追加してください
- **公開済み → 下書き** に戻す操作はできません（note の画面で操作してください）
- **予約投稿**・**メンバーシップ限定**は API 仕様の確認が不十分です。動かない場合は `--dry-run` で中身を見て、`extra:` で項目を調整してください
- タグ・価格・マガジンなどの設定は**公開時にだけ**反映されます（note の下書き保存 API はこれらを無視するため）

## 参考にした先駆者の情報

- [JY8752/note-cli](https://github.com/JY8752/note-cli) — Go + Cobra で作られた note 用 CLI（記事ファイル・画像の作成）
- [shimayuz/note-com-mcp](https://github.com/shimayuz/note-com-mcp) — 非公式 API での下書き作成・保存（`/api/v1/text_notes`、`draft_save`）、認証（Cookie + XSRF トークン）の実装
- [Go-555/note-post-mcp](https://github.com/Go-555/note-post-mcp) — ブラウザ自動操作（Playwright）による投稿
- [A Thorough Investigation of the Unofficial note API](https://note.com/marie_222/n/n6a10366298b0?hl=en) — 公開 API（`PUT /api/v1/text_notes/{id}`）の項目（`hashtags` / `price` / `free_body` / `pay_body` / `separator` / `magazine_ids`）の調査
- [AIエージェントがNote自動投稿を実装するまで：非公式APIとの奮闘記](https://note.com/a_g_e_n_t_b_o_t/n/n86da8314430b) — タグ・価格は公開時の PUT でのみ受け付けられる、という知見
- [自動投稿非推奨？：noteのAPIでの「投稿」…](https://note.com/akawibaku137/n/nc154955d0220) — 自動投稿についての注意

## 開発

```sh
go test ./...   # テスト（本物の note には接続しません）
go vet ./...
```
