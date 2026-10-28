package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const template = `---
title: %s
tags: []
# draft = 下書き / published = 公開
status: draft
# 0 = 無料。有料にするなら 100〜50000 を指定し、本文に <!-- paid --> の行を入れる
price: 0
# refund: false          # 有料記事の返金を許可するか
# magazines: []          # 追加するマガジンのキー（例: m1234abcd）
# membership_plans: []   # メンバーシップ限定にするプランのキー（実験的）
comments: true           # コメントを受け付けるか
notify: true             # フォロワーに通知するか
---

ここに本文を書きます。Markdown が使えます。

## 見出し

<!-- paid -->
（price を設定すると、この行から下が有料部分になります。無料記事ならこの行は消してください）
`

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "new <ファイル名> [タイトル]",
		Short: "記事ファイルのひな形を作る",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, title := args[0], "タイトル"
			if len(args) == 2 {
				title = args[1]
			}
			// O_EXCL: 同じ名前のファイルが既にあれば上書きせずエラーにする
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := fmt.Fprintf(f, template, title); err != nil {
				return err
			}
			fmt.Println("作成しました:", path)
			return nil
		},
	})
}
