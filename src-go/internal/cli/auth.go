package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
)

func newLoginCommand(app *App) *cobra.Command {
	var email string
	var password string
	var mfa string
	command := &cobra.Command{
		Use:   "login",
		Short: "Log in to an Obsidian account",
		RunE: func(cmd *cobra.Command, args []string) error {
			email = viper.GetString("email")
			password = viper.GetString("password")

			if email == "" || password == "" {
				storedEmail, storedPassword, err := configpkg.LoadCredentials()
				if err != nil {
					return err
				}
				if email == "" && storedEmail != "" {
					email = storedEmail
				}
				if password == "" && storedPassword != "" {
					password = storedPassword
				}
			}
			if email == "" {
				fmt.Print("Email: ")
				fmt.Scanln(&email)
			}
			if password == "" {
				fmt.Print("Password: ")
				pass, err := readPassword(app.stdin)
				if err != nil {
					return err
				}
				password = pass
			}
			if email == "" || password == "" {
				return fmt.Errorf("both --email and --password are required")
			}
			if err := configpkg.SaveCredentials(email, password); err != nil {
				return err
			}
			response, err := app.client().SignIn(cmd.Context(), email, password, mfa)
			if err != nil {
				return err
			}
			if err := configpkg.SaveAuthToken(response.Token); err != nil {
				return err
			}
			writeLines(app.stdout, fmt.Sprintf("Logged in as %s <%s>", response.Name, response.Email))
			return nil
		},
	}
	command.Flags().StringVar(&email, "email", "", "account email")
	command.Flags().StringVar(&password, "password", "", "account password")
	command.Flags().StringVar(&mfa, "mfa", "", "MFA code")

	viper.BindPFlag("email", command.Flags().Lookup("email"))
	viper.BindPFlag("password", command.Flags().Lookup("password"))
	viper.BindEnv("email", "OBSIDIAN_EMAIL")
	viper.BindEnv("password", "OBSIDIAN_PASSWORD")

	return command
}

func newLogoutCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of the current account",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := configpkg.LoadAuthToken()
			if err != nil {
				return err
			}
			if token != "" {
				_ = app.client().SignOut(cmd.Context(), token)
			}
			if err := configpkg.ClearAuthToken(); err != nil {
				return err
			}
			if err := configpkg.ClearCredentials(); err != nil {
				return err
			}
			writeLines(app.stdout, "Logged out.")
			return nil
		},
	}
}

var _ = func() {
	cobra.OnInitialize(func() {
		viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	})
}
