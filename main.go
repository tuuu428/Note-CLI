// note-cli: ローカルの記事ファイルを note.com に投稿するコマンドラインツール
package main

import (
	"os"

	"github.com/tuuu428/note-cli/cmd"
)

// version は `go build -ldflags "-X main.version=v1.2.3"` で上書きされます（GoReleaser が自動で行う）。
var version = "dev"

func main() {
	if err := cmd.Execute(version); err != nil {
		os.Exit(1)
	}
}
