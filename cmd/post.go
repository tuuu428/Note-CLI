package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tuuu428/note-cli/internal/article"
	"github.com/tuuu428/note-cli/internal/note"
)

func init() {
	var (
		forcePublish bool
		forceDraft   bool
		dryRun       bool
		yes          bool
	)
	post := &cobra.Command{
		Use:   "post <ファイル または フォルダ>...",
		Short: "記事を note に投稿する（既定は下書き保存）",
		Long: `記事ファイルを note に投稿します。フォルダを指定すると中の .md / .txt をまとめて投稿します。

・フロントマターの status: published の記事だけが公開されます（それ以外は下書き）
・一度投稿したファイルは .note-cli.json に記録され、次からは同じ記事の「更新」になります
・公開する前には確認が出ます（--yes で省略）`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if forcePublish && forceDraft {
				return fmt.Errorf("--publish と --draft は同時に指定できません")
			}
			files, err := collectFiles(args)
			if err != nil {
				return err
			}

			// まず全部読み込んでチェック。1 つでも問題があれば何も投稿しない。
			arts := make([]*article.Article, 0, len(files))
			var problems []string
			for _, f := range files {
				a, err := article.Load(f)
				if err != nil {
					return err
				}
				if forcePublish {
					a.Settings.Status = article.StatusPublished
				}
				if forceDraft {
					a.Settings.Status = article.StatusDraft
				}
				if err := a.Validate(); err != nil {
					problems = append(problems, err.Error())
				}
				arts = append(arts, a)
			}
			if len(problems) > 0 {
				return fmt.Errorf("投稿前のチェックで問題が見つかりました:\n%s", strings.Join(problems, "\n"))
			}

			if dryRun {
				return printDryRun(arts)
			}

			client, user, err := loggedInClient()
			if err != nil {
				return err
			}

			if !yes && !confirmPublish(arts) {
				fmt.Println("中止しました")
				return nil
			}

			for i, a := range arts {
				if err := postOne(client, user, a); err != nil {
					if rest := len(arts) - i - 1; rest > 0 {
						fmt.Fprintf(os.Stderr, "※ エラーのため、残り %d 件は投稿していません\n", rest)
					}
					return fmt.Errorf("%s: %w", a.Path, err)
				}
			}
			return nil
		},
	}
	post.Flags().BoolVar(&forcePublish, "publish", false, "フロントマターに関係なく公開する")
	post.Flags().BoolVar(&forceDraft, "draft", false, "フロントマターに関係なく下書き保存にする")
	post.Flags().BoolVar(&dryRun, "dry-run", false, "投稿せず、送る内容を表示するだけ")
	post.Flags().BoolVarP(&yes, "yes", "y", false, "公開前の確認を省略する")

	check := &cobra.Command{
		Use:   "check <ファイル または フォルダ>...",
		Short: "投稿せずに記事ファイルの設定をチェックし、公開範囲を一覧表示する",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := collectFiles(args)
			if err != nil {
				return err
			}
			ng := 0
			for _, f := range files {
				a, err := article.Load(f)
				if err != nil {
					return err
				}
				if err := a.Validate(); err != nil {
					fmt.Println("NG", err)
					ng++
					continue
				}
				fmt.Printf("OK %s\n   「%s」 %s\n", f, a.Settings.Title, a.Settings.Visibility())
				if refs := article.ImageRefs(a.Body); len(refs) > 0 {
					fmt.Printf("   本文の画像 %d 枚: %s\n", len(refs), strings.Join(refs, ", "))
				}
			}
			if ng > 0 {
				return fmt.Errorf("%d 件のファイルに問題があります", ng)
			}
			return nil
		},
	}

	rootCmd.AddCommand(post, check)
}

// collectFiles は引数のファイル・フォルダから記事ファイルの一覧を作ります。
func collectFiles(args []string) ([]string, error) {
	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, arg)
			continue
		}
		entries, err := os.ReadDir(arg)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && article.IsArticleFile(e.Name()) {
				files = append(files, filepath.Join(arg, e.Name()))
			}
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("記事ファイル（.md / .txt）が見つかりません")
	}
	return files, nil
}

func confirmPublish(arts []*article.Article) bool {
	var pub []*article.Article
	for _, a := range arts {
		if a.Settings.Status == article.StatusPublished {
			pub = append(pub, a)
		}
	}
	if len(pub) == 0 {
		return true // 下書きだけなら確認不要
	}
	fmt.Println("次の記事を【公開】します:")
	for _, a := range pub {
		fmt.Printf("  - 「%s」 %s\n", a.Settings.Title, a.Settings.Visibility())
	}
	fmt.Print("よろしいですか？ [y/N]: ")
	ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	ans = strings.ToLower(strings.TrimSpace(ans))
	return ans == "y" || ans == "yes"
}

func printDryRun(arts []*article.Article) error {
	for _, a := range arts {
		h, err := article.ToHTML(a.Body, nil)
		if err != nil {
			return err
		}
		fmt.Printf("=== %s\n「%s」 %s\n", a.Path, a.Settings.Title, a.Settings.Visibility())
		for _, ref := range article.ImageRefs(a.Body) {
			fmt.Printf("画像をアップロード予定: %s（本文では upload://%s と表示）\n", ref, ref)
		}
		if a.Settings.Eyecatch != "" {
			fmt.Printf("見出し画像をアップロード予定: %s\n", a.Settings.Eyecatch)
		}
		if a.Settings.Status == article.StatusPublished {
			fmt.Println("公開 API に送る内容:")
			enc := json.NewEncoder(os.Stdout)
			enc.SetEscapeHTML(false) // <p> などを \u003c にせず読みやすく表示
			enc.SetIndent("", "  ")
			if err := enc.Encode(note.PublishPayload(a, h)); err != nil {
				return err
			}
		} else {
			fmt.Printf("下書き本文（HTML）:\n%s\n", h.Full)
		}
	}
	return nil
}

func postOne(client *note.Client, user *note.User, a *article.Article) error {
	dir, name := filepath.Dir(a.Path), filepath.Base(a.Path)
	state, err := note.LoadState(dir)
	if err != nil {
		return err
	}

	rec, exists := state[name]
	if exists && rec.Status == article.StatusPublished && a.Settings.Status == article.StatusDraft {
		return fmt.Errorf("この記事は公開済みです。下書きに戻すことはできないので、note の画面で操作してください（%s）", note.EditURL(rec.Key))
	}
	if !exists {
		n, err := client.CreateDraft(user.ID, a.Settings.Title)
		if err != nil {
			return err
		}
		rec = note.Posted{ID: n.ID, Key: n.Key, Status: article.StatusDraft}
	}
	// 記録はこまめに保存する（途中で失敗しても、次回は同じ記事・アップロード済み画像を再利用できる）
	save := func() error {
		state[name] = rec
		return state.Save(dir)
	}
	if err := save(); err != nil {
		return err
	}

	images, err := uploadImages(client, a, &rec, save)
	if err != nil {
		return err
	}
	h, err := article.ToHTML(a.Body, images)
	if err != nil {
		return err
	}
	if err := uploadEyecatch(client, a, &rec, save); err != nil {
		return err
	}

	if a.Settings.Status == article.StatusPublished {
		if err := client.Publish(rec.ID, note.PublishPayload(a, h)); err != nil {
			return err
		}
		// 本当に公開・価格設定されたか、note から読み直して確認する
		if err := client.Verify(rec.Key, note.Expectation{Status: article.StatusPublished, Price: a.Settings.PriceValue()}); err != nil {
			return err
		}
		rec.Status = article.StatusPublished
		rec.URL = note.NoteURL(user.URLName, rec.Key)
		fmt.Printf("公開しました: 「%s」 %s\n", a.Settings.Title, rec.URL)
	} else {
		if err := client.SaveDraft(rec.ID, a.Settings.Title, h.Full, h.Length); err != nil {
			return err
		}
		rec.URL = note.EditURL(rec.Key)
		fmt.Printf("下書き保存しました: 「%s」 %s\n", a.Settings.Title, rec.URL)
	}
	return save()
}

// uploadImages は本文の画像をアップロードし、「本文に書いた場所 → note 上の URL」の対応表を返します。
// 中身が同じ画像（ハッシュ値が同じ）は、前回アップロードした URL を使い回します。
func uploadImages(client *note.Client, a *article.Article, rec *note.Posted, save func() error) (map[string]string, error) {
	urls := map[string]string{}
	for _, ref := range article.ImageRefs(a.Body) {
		data, err := os.ReadFile(a.ImagePath(ref))
		if err != nil {
			return nil, err
		}
		hash := note.Hash(data)
		if u, ok := rec.Images[hash]; ok {
			urls[ref] = u
			continue
		}
		fmt.Printf("   画像をアップロード中: %s\n", ref)
		u, err := client.UploadImage(filepath.Base(a.ImagePath(ref)), data)
		if err != nil {
			return nil, err
		}
		if rec.Images == nil {
			rec.Images = map[string]string{}
		}
		rec.Images[hash] = u
		urls[ref] = u
		if err := save(); err != nil {
			return nil, err
		}
	}
	return urls, nil
}

// uploadEyecatch は見出し画像を設定します。前回と同じ画像なら何もしません。
func uploadEyecatch(client *note.Client, a *article.Article, rec *note.Posted, save func() error) error {
	if a.Settings.Eyecatch == "" {
		return nil
	}
	path := a.ImagePath(a.Settings.Eyecatch)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	hash := note.Hash(data)
	if rec.Eyecatch == hash {
		return nil
	}
	fmt.Printf("   見出し画像をアップロード中: %s\n", a.Settings.Eyecatch)
	if err := client.UploadEyecatch(rec.ID, filepath.Base(path), data); err != nil {
		return err
	}
	rec.Eyecatch = hash
	return save()
}
