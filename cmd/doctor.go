package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tuuu428/note-cli/internal/note"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "doctor [記事フォルダ]...",
		Short: "note の API が想定どおりに動くか確認する（読み取りだけ・投稿はしない）",
		Long: `note の非公式 API が、このツールの想定どおりの形で応答するかを確認します。
何も書き込まないので、何度実行しても安全です。

  1. ログイン情報が有効か（ユーザー情報の取得）
  2. 記事フォルダを指定した場合: そこから公開した記事の状態を読み直せるか

GitHub Actions で定期実行すれば、note の仕様変更に早めに気づけます。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, user, err := loggedInClient()
			if err != nil {
				return err
			}
			fmt.Printf("OK ユーザー情報: @%s\n", user.URLName)

			for _, dir := range args {
				state, err := note.LoadState(dir)
				if err != nil {
					return err
				}
				for name, rec := range state {
					if rec.Status != "published" {
						continue
					}
					got, err := client.GetNote(rec.Key)
					if err != nil {
						return fmt.Errorf("%s: %w", name, err)
					}
					fmt.Printf("OK 記事の読み直し: %s（status=%s price=%d）\n", name, got.Status, *got.Price)
					break // 1 本確認できれば十分
				}
			}
			fmt.Println("問題は見つかりませんでした")
			return nil
		},
	})
}
