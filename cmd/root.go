// Package cmd は `note-cli xxx` の各コマンドを定義します（Cobra というライブラリを使用）。
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "note-cli",
	Short: "ローカルの記事ファイルを note.com に投稿する CLI",
	Long: `ローカルの .md / .txt ファイルを note.com に下書き保存・公開する CLI です。

公開範囲（下書き / 公開 / 有料 / メンバーシップ限定 / マガジン追加 / コメント可否 / 通知）は
記事ファイル先頭の「フロントマター」か、フォルダの note-defaults.yaml で設定します。

※ note には公式 API がないため、非公式 API を使っています。仕様変更で動かなくなることがあります。`,
	SilenceUsage: true,
}

// Execute は main から呼ばれる入口です。version は GoReleaser が完成品を作るときに埋め込みます。
func Execute(version string) error {
	rootCmd.Version = version
	return rootCmd.Execute()
}
