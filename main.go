// note-cli: ローカルの記事ファイルを note.com に投稿するコマンドラインツール
package main

import (
	"os"

	"github.com/tuuu428/note-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
