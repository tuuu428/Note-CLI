package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tuuu428/note-cli/internal/note"
	"golang.org/x/term"
)

func init() {
	var useCookie bool
	login := &cobra.Command{
		Use:   "login",
		Short: "note にログインする（ログイン情報はこのPCに保存）",
		Long: `note にログインします。2 つの方法があります。

  1) メールアドレス + パスワード（既定）
  2) --cookie : ブラウザでログイン済みの Cookie「_note_session_v5」の値を貼り付ける
     （ロボット対策でパスワードログインが失敗するときはこちら）

ログイン情報は自分だけが読める権限でこのPCに保存されます。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := bufio.NewReader(os.Stdin)
			client := note.NewClient("")

			var session string
			if useCookie {
				fmt.Print("_note_session_v5 の値: ")
				v, err := readSecret(in)
				if err != nil {
					return err
				}
				session = v
			} else {
				fmt.Print("メールアドレス（または note ID）: ")
				email, _ := in.ReadString('\n')
				fmt.Print("パスワード: ")
				pw, err := readSecret(in)
				if err != nil {
					return err
				}
				session, err = client.SignIn(strings.TrimSpace(email), pw)
				if err != nil {
					return err
				}
			}

			client.Session = session
			user, err := client.CurrentUser()
			if err != nil {
				return err
			}
			path, err := note.SaveSession(session)
			if err != nil {
				return err
			}
			fmt.Printf("ログインしました: %s（@%s）\n保存先: %s\n", user.Nickname, user.URLName, path)
			return nil
		},
	}
	login.Flags().BoolVar(&useCookie, "cookie", false, "ブラウザの Cookie でログインする")

	logout := &cobra.Command{
		Use:   "logout",
		Short: "保存したログイン情報を削除する",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := note.DeleteSession(); err != nil {
				return err
			}
			fmt.Println("ログアウトしました")
			return nil
		},
	}

	whoami := &cobra.Command{
		Use:   "whoami",
		Short: "ログイン中のアカウントを表示する",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, user, err := loggedInClient()
			if err != nil {
				return err
			}
			fmt.Printf("%s（@%s） https://note.com/%s\n", user.Nickname, user.URLName, user.URLName)
			return nil
		},
	}

	rootCmd.AddCommand(login, logout, whoami)
}

// readSecret はパスワードなどを画面に表示せずに読み込みます。
func readSecret(in *bufio.Reader) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println()
		return strings.TrimSpace(string(b)), err
	}
	s, err := in.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

func loggedInClient() (*note.Client, *note.User, error) {
	session, err := note.LoadSession()
	if err != nil {
		return nil, nil, err
	}
	client := note.NewClient(session)
	user, err := client.CurrentUser()
	if err != nil {
		return nil, nil, err
	}
	return client, user, nil
}
