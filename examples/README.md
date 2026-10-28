# サンプル

| ファイル | 内容 |
|---|---|
| `note-defaults.yaml` | このフォルダ全体の共通設定 |
| `01-free.md` | 無料で公開する記事 |
| `02-paid.md` | 有料・マガジン追加・コメント不可の記事 |
| `03-memo.txt` | フロントマターなしのテキスト（1 行目がタイトル、下書き） |
| `04-images.md` | 本文の画像と見出し画像（`images/` フォルダ） |

```sh
note-cli check examples/
note-cli post examples/ --dry-run
```
