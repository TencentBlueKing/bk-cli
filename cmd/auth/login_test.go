/*
 * TencentBlueKing is pleased to support the open source community by making
 * 蓝鲸智云 - bk-cli (BlueKing - Cli) available.
 * Copyright (C) Tencent. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://opensource.org/licenses/MIT
 *
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * We undertake not to change the open source license (MIT license) applicable
 * to the current version of the project delivered to anyone in the future.
 */

package auth_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	authcmd "github.com/TencentBlueKing/bk-cli/cmd/auth"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

// newTestRoot creates a root command wired with auth subcommands and the --context flag.
func newTestRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "bk-cli",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("context", "", "Override active context")
	root.AddCommand(authcmd.NewAuthCmd())
	return root
}

func executeCmd(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func executeCmdWithStderr(root *cobra.Command, args ...string) (string, string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	os.Stderr = w

	runErr := root.Execute()

	Expect(w.Close()).To(Succeed())
	os.Stderr = origStderr

	stderr, readErr := io.ReadAll(r)
	Expect(readErr).NotTo(HaveOccurred())
	Expect(r.Close()).To(Succeed())

	return buf.String(), string(stderr), runErr
}

var _ = Describe("Auth Commands", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-auth-test-*")
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)

		cfg := &config.Config{
			BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
			UserKey:      "bk_token",
		}
		Expect(config.CreateContext("default", cfg)).To(Succeed())
		Expect(config.SetActiveContext("default")).To(Succeed())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
		os.Unsetenv("BK_CLI_CONFIG_DIR")
	})

	Describe("login", func() {
		BeforeEach(func() {
			for _, name := range []string{
				"BK_CLI_BK_APP_CODE", "BK_CLI_BK_APP_SECRET", "BK_CLI_BK_TOKEN",
				"BK_CLI_BK_TICKET", "BK_CLI_ACCESS_TOKEN", "BK_CLI_PERSONAL_TOKEN",
			} {
				GinkgoT().Setenv(name, "")
			}
		})

		DescribeTable(
			"resolves credential flags and environment without changing the selected identity",
			func(values map[string]string, flags []string, expected credential.Credential) {
				for name, value := range values {
					GinkgoT().Setenv("BK_CLI_"+name, value)
				}
				root := newTestRoot()
				out, stderr, err := executeCmdWithStderr(
					root,
					append([]string{"auth", "login"}, flags...)...)
				Expect(err).NotTo(HaveOccurred())
				var envelope map[string]any
				Expect(json.Unmarshal([]byte(out), &envelope)).To(Succeed())
				Expect(envelope["ok"]).To(BeTrue())
				Expect(stderr).To(BeEmpty())
				key, err := credential.DeriveKey()
				Expect(err).NotTo(HaveOccurred())
				cred, err := credential.LoadFromFile(config.CredentialsPath("default"), key)
				Expect(err).NotTo(HaveOccurred())
				Expect(*cred).To(Equal(expected))
				for _, value := range values {
					Expect(out + stderr).NotTo(ContainSubstring(value))
				}
			},
			Entry(
				"standalone environment personal token",
				map[string]string{"PERSONAL_TOKEN": "env-personal"},
				nil,
				credential.Credential{
					Type:          credential.TypePersonalToken,
					PersonalToken: "env-personal",
				},
			),
			Entry(
				"explicit personal token ignores all environment fields",
				map[string]string{
					"BK_APP_CODE": " env-app\n", "BK_APP_SECRET": " env-secret\n",
					"BK_TOKEN": " env-token\n", "BK_TICKET": " env-ticket\n",
					"ACCESS_TOKEN": " env-access\n", "PERSONAL_TOKEN": " env-personal\n",
				},
				[]string{"--personal_token=flag-personal"},
				credential.Credential{
					Type:          credential.TypePersonalToken,
					PersonalToken: "flag-personal",
				},
			),
			Entry("explicit access token ignores environment personal token", map[string]string{
				"PERSONAL_TOKEN": " env-personal\n",
			}, []string{"--access_token=flag-access"},
				credential.Credential{Type: credential.TypeAccessToken, AccessToken: "flag-access"}),
			Entry("explicit app code ignores environment personal token", map[string]string{
				"BK_APP_SECRET":  "env-secret",
				"BK_TOKEN":       "env-token",
				"PERSONAL_TOKEN": " env-personal\n",
			}, []string{"--bk_app_code=flag-app"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "flag-app",
				BkAppSecret: "env-secret",
				BkToken:     "env-token",
			}),
			Entry("explicit app secret ignores environment personal token", map[string]string{
				"BK_APP_CODE":    "env-app",
				"BK_TICKET":      "env-ticket",
				"PERSONAL_TOKEN": " env-personal\n",
			}, []string{"--bk_app_secret=flag-secret"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "flag-secret",
				BkTicket:    "env-ticket",
			}),
			Entry("explicit user token ignores environment personal token", map[string]string{
				"BK_APP_CODE":    "env-app",
				"BK_APP_SECRET":  "env-secret",
				"PERSONAL_TOKEN": " env-personal\n",
			}, []string{"--bk_token=flag-token"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "env-secret",
				BkToken:     "flag-token",
			}),
			Entry("explicit user ticket ignores environment personal token", map[string]string{
				"BK_APP_CODE":    "env-app",
				"BK_APP_SECRET":  "env-secret",
				"PERSONAL_TOKEN": " env-personal\n",
			}, []string{"--bk_ticket=flag-ticket"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "env-secret",
				BkTicket:    "flag-ticket",
			}),
			Entry(
				"standalone environment access token",
				map[string]string{"ACCESS_TOKEN": "env-access"},
				nil,
				credential.Credential{Type: credential.TypeAccessToken, AccessToken: "env-access"},
			),
			Entry("environment app token", map[string]string{
				"BK_APP_CODE": "env-app", "BK_APP_SECRET": "env-secret", "BK_TOKEN": "env-token",
			}, nil, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "env-secret",
				BkToken:     "env-token",
			}),
			Entry("environment app ticket", map[string]string{
				"BK_APP_CODE": "env-app", "BK_APP_SECRET": "env-secret", "BK_TICKET": "env-ticket",
			}, nil, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "env-secret",
				BkTicket:    "env-ticket",
			}),
			Entry("explicit token ignores stale access token and ticket", map[string]string{
				"BK_APP_CODE": "env-app", "BK_APP_SECRET": "env-secret", "BK_TOKEN": "env-token",
				"BK_TICKET": "env-ticket", "ACCESS_TOKEN": "env-access",
			}, []string{"--bk_token=flag-token"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "env-secret",
				BkToken:     "flag-token",
			}),
			Entry("explicit ticket ignores stale access token and token", map[string]string{
				"BK_APP_CODE": "env-app", "BK_APP_SECRET": "env-secret", "BK_TOKEN": "env-token",
				"BK_TICKET": "env-ticket", "ACCESS_TOKEN": "env-access",
			}, []string{"--bk_ticket=flag-ticket"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "env-secret",
				BkTicket:    "flag-ticket",
			}),
			Entry("explicit access token ignores all environment app fields", map[string]string{
				"BK_APP_CODE": "env-app", "BK_APP_SECRET": "env-secret", "BK_TOKEN": "env-token",
				"BK_TICKET": "env-ticket", "ACCESS_TOKEN": "env-access",
			}, []string{"--access_token=flag-access"},
				credential.Credential{Type: credential.TypeAccessToken, AccessToken: "flag-access"}),
			Entry("all nonempty flags preserve access token precedence", nil,
				[]string{
					"--bk_app_code=flag-app",
					"--bk_app_secret=flag-secret",
					"--bk_token=flag-token",
					"--bk_ticket=flag-ticket",
					"--access_token=flag-access",
				},
				credential.Credential{Type: credential.TypeAccessToken, AccessToken: "flag-access"}),
			Entry("explicit ticket preserves precedence over explicit token", nil,
				[]string{
					"--bk_app_code=flag-app",
					"--bk_app_secret=flag-secret",
					"--bk_token=flag-token",
					"--bk_ticket=flag-ticket",
				}, credential.Credential{
					Type:        credential.TypeAppUser,
					BkAppCode:   "flag-app",
					BkAppSecret: "flag-secret",
					BkTicket:    "flag-ticket",
				}),
			Entry("explicit app identity chooses an environment token", map[string]string{
				"BK_APP_CODE":   "env-app",
				"BK_APP_SECRET": "env-secret",
				"BK_TOKEN":      "env-token",
				"ACCESS_TOKEN":  "env-access",
			}, []string{"--bk_app_code=flag-app"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "flag-app",
				BkAppSecret: "env-secret",
				BkToken:     "env-token",
			}),
			Entry("explicit app secret chooses an environment ticket", map[string]string{
				"BK_APP_CODE": "env-app", "BK_APP_SECRET": "env-secret", "BK_TICKET": "env-ticket",
			}, []string{"--bk_app_secret=flag-secret"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "flag-secret",
				BkTicket:    "env-ticket",
			}),
			Entry(
				"explicit app secret overrides whitespace in the same environment field",
				map[string]string{
					"BK_APP_CODE":   "env-app",
					"BK_APP_SECRET": " env-secret\n",
					"BK_TOKEN":      "env-token",
				},
				[]string{"--bk_app_secret=flag-secret"},
				credential.Credential{
					Type:        credential.TypeAppUser,
					BkAppCode:   "env-app",
					BkAppSecret: "flag-secret",
					BkToken:     "env-token",
				},
			),
			Entry("explicit token ignores whitespace in unused environment modes", map[string]string{
				"BK_APP_CODE": "env-app", "BK_APP_SECRET": "env-secret", "BK_TOKEN": " env-token\n",
				"BK_TICKET": " env-ticket\n", "ACCESS_TOKEN": " env-access\n",
			}, []string{"--bk_token=flag-token"}, credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "env-app",
				BkAppSecret: "env-secret",
				BkToken:     "flag-token",
			}),
			Entry("explicit access token ignores whitespace in all environment fields", map[string]string{
				"BK_APP_CODE":   " env-app\n",
				"BK_APP_SECRET": " env-secret\n",
				"BK_TOKEN":      " env-token\n",
				"BK_TICKET":     " env-ticket\n",
				"ACCESS_TOKEN":  " env-access\n",
			}, []string{"--access_token=flag-access"},
				credential.Credential{Type: credential.TypeAccessToken, AccessToken: "flag-access"}),
			Entry("explicit flag values retain their existing whitespace behavior", nil,
				[]string{"--access_token= flag-access\n"},
				credential.Credential{Type: credential.TypeAccessToken, AccessToken: " flag-access\n"}),
		)

		DescribeTable("rejects incomplete or conflicting environment credentials before saving",
			func(values map[string]string, flags []string) {
				for name, value := range values {
					GinkgoT().Setenv("BK_CLI_"+name, value)
				}
				root := newTestRoot()
				out, stderr, err := executeCmdWithStderr(
					root,
					append([]string{"auth", "login"}, flags...)...)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid_credentials"))
				Expect(credential.Exists(config.CredentialsPath("default"))).To(BeFalse())
				for _, value := range values {
					Expect(out + stderr + err.Error()).NotTo(ContainSubstring(value))
				}
			},
			Entry("access token conflicts with app code", map[string]string{
				"ACCESS_TOKEN": "env-access", "BK_APP_CODE": "env-app",
			}, nil),
			Entry("personal token conflicts with app code", map[string]string{
				"PERSONAL_TOKEN": "env-personal", "BK_APP_CODE": "env-app",
			}, nil),
			Entry("personal token conflicts with app secret", map[string]string{
				"PERSONAL_TOKEN": "env-personal", "BK_APP_SECRET": "env-secret",
			}, nil),
			Entry("personal token conflicts with user token", map[string]string{
				"PERSONAL_TOKEN": "env-personal", "BK_TOKEN": "env-token",
			}, nil),
			Entry("personal token conflicts with user ticket", map[string]string{
				"PERSONAL_TOKEN": "env-personal", "BK_TICKET": "env-ticket",
			}, nil),
			Entry("personal token conflicts with access token", map[string]string{
				"PERSONAL_TOKEN": "env-personal", "ACCESS_TOKEN": "env-access",
			}, nil),
			Entry("explicit app cannot fall back to personal token", map[string]string{
				"PERSONAL_TOKEN": "env-personal",
			}, []string{"--bk_app_code=flag-app", "--bk_app_secret=flag-secret"}),
			Entry("access token conflicts with app secret", map[string]string{
				"ACCESS_TOKEN": "env-access", "BK_APP_SECRET": "env-secret",
			}, nil),
			Entry("access token conflicts with token", map[string]string{
				"ACCESS_TOKEN": "env-access", "BK_TOKEN": "env-token",
			}, nil),
			Entry("access token conflicts with ticket", map[string]string{
				"ACCESS_TOKEN": "env-access", "BK_TICKET": "env-ticket",
			}, nil),
			Entry("both environment user modes", map[string]string{
				"BK_TOKEN": "env-token", "BK_TICKET": "env-ticket",
			}, nil),
			Entry("explicit app with ambiguous environment user modes", map[string]string{
				"BK_TOKEN": "env-token", "BK_TICKET": "env-ticket",
			}, []string{"--bk_app_code=flag-app", "--bk_app_secret=flag-secret"}),
			Entry("explicit app does not fall back to access token", map[string]string{
				"ACCESS_TOKEN": "env-access",
			}, []string{"--bk_app_code=flag-app", "--bk_app_secret=flag-secret"}),
			Entry("partial environment app identity", map[string]string{"BK_APP_CODE": "env-app"}, nil),
			Entry("environment user without app identity", map[string]string{"BK_TOKEN": "env-token"}, nil),
			Entry("environment app without a user", map[string]string{
				"BK_APP_CODE": "env-app", "BK_APP_SECRET": "env-secret",
			}, nil),
		)

		DescribeTable("offers environment remedies when the app user credential is missing",
			func(userKey string) {
				Expect((&config.Config{
					BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
					UserKey:      userKey,
				}).Save(config.ConfigPath("default"))).To(Succeed())
				GinkgoT().Setenv("BK_CLI_BK_APP_CODE", "env-app")
				GinkgoT().Setenv("BK_CLI_BK_APP_SECRET", "env-secret")
				out, stderr, err := executeCmdWithStderr(newTestRoot(), "auth", "login")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid_credentials"))
				for _, name := range []string{"BK_CLI_BK_TOKEN", "BK_CLI_BK_TICKET"} {
					Expect(err.Error()).To(ContainSubstring(name))
					Expect(stderr).To(ContainSubstring(name))
				}
				Expect(out + stderr + err.Error()).NotTo(ContainSubstring("env-secret"))
				Expect(credential.Exists(config.CredentialsPath("default"))).To(BeFalse())
			},
			Entry("token context", "bk_token"),
			Entry("ticket context", "bk_ticket"),
		)

		DescribeTable("rejects surrounding whitespace in selected environment credentials without overwriting",
			func(name string) {
				_, err := executeCmd(newTestRoot(), "auth", "login", "--access_token=saved-access")
				Expect(err).NotTo(HaveOccurred())
				before, err := os.ReadFile(config.CredentialsPath("default"))
				Expect(err).NotTo(HaveOccurred())
				if name != "BK_CLI_ACCESS_TOKEN" && name != "BK_CLI_PERSONAL_TOKEN" {
					GinkgoT().Setenv("BK_CLI_BK_APP_CODE", "env-app")
					GinkgoT().Setenv("BK_CLI_BK_APP_SECRET", "env-secret")
					if name == "BK_CLI_BK_TICKET" {
						GinkgoT().Setenv("BK_CLI_BK_TICKET", "env-ticket")
					} else {
						GinkgoT().Setenv("BK_CLI_BK_TOKEN", "env-token")
					}
				}
				for _, value := range []string{" env-sensitive", "env-sensitive ", "env-sensitive\n", "\tenv-sensitive", "\u00a0env-sensitive", " \t\r\n"} {
					GinkgoT().Setenv(name, value)
					out, stderr, err := executeCmdWithStderr(newTestRoot(), "auth", "login")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid_credentials"))
					Expect(err.Error()).To(ContainSubstring(name))
					Expect(stderr).To(ContainSubstring(name))
					Expect(out + stderr + err.Error()).NotTo(ContainSubstring("env-sensitive"))
					after, err := os.ReadFile(config.CredentialsPath("default"))
					Expect(err).NotTo(HaveOccurred())
					Expect(after).To(Equal(before))
				}
			},
			Entry("app code", "BK_CLI_BK_APP_CODE"),
			Entry("app secret", "BK_CLI_BK_APP_SECRET"),
			Entry("user token", "BK_CLI_BK_TOKEN"),
			Entry("user ticket", "BK_CLI_BK_TICKET"),
			Entry("access token", "BK_CLI_ACCESS_TOKEN"),
			Entry("personal token", "BK_CLI_PERSONAL_TOKEN"),
		)

		It("rejects each explicitly empty credential flag instead of using environment values", func() {
			for _, flag := range []string{"bk_app_code", "bk_app_secret", "bk_token", "bk_ticket", "access_token", "personal_token"} {
				GinkgoT().Setenv("BK_CLI_ACCESS_TOKEN", "env-access")
				_, _, err := executeCmdWithStderr(newTestRoot(), "auth", "login", "--"+flag+"=")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid_credentials"))
				Expect(err.Error()).To(ContainSubstring(flag))
				Expect(err.Error()).NotTo(ContainSubstring("env-access"))
				Expect(credential.Exists(config.CredentialsPath("default"))).To(BeFalse())
			}
		})

		DescribeTable(
			"saves environment credentials only to the specified context",
			func(name string, expected credential.Credential) {
				Expect(config.CreateContext("dev", &config.Config{
					BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
				})).To(Succeed())
				GinkgoT().Setenv(name, "env-value")
				_, err := executeCmd(newTestRoot(), "auth", "login", "--context", "dev")
				Expect(err).NotTo(HaveOccurred())
				Expect(credential.Exists(config.CredentialsPath("default"))).To(BeFalse())
				key, err := credential.DeriveKey()
				Expect(err).NotTo(HaveOccurred())
				cred, err := credential.LoadFromFile(config.CredentialsPath("dev"), key)
				Expect(err).NotTo(HaveOccurred())
				Expect(*cred).To(Equal(expected))
			},
			Entry("access token", "BK_CLI_ACCESS_TOKEN", credential.Credential{
				Type: credential.TypeAccessToken, AccessToken: "env-value",
			}),
			Entry("personal token", "BK_CLI_PERSONAL_TOKEN", credential.Credential{
				Type: credential.TypePersonalToken, PersonalToken: "env-value",
			}),
		)

		It("does not overwrite saved credentials when no login input is provided", func() {
			_, err := executeCmd(newTestRoot(), "auth", "login", "--access_token=saved-access")
			Expect(err).NotTo(HaveOccurred())
			before, err := os.ReadFile(config.CredentialsPath("default"))
			Expect(err).NotTo(HaveOccurred())
			_, err = executeCmd(newTestRoot(), "auth", "login")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not_implemented"))
			after, err := os.ReadFile(config.CredentialsPath("default"))
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(before))
		})

		It("keeps environment credential values out of help and flag defaults", func() {
			for _, name := range []string{
				"BK_APP_CODE", "BK_APP_SECRET", "BK_TOKEN", "BK_TICKET", "ACCESS_TOKEN", "PERSONAL_TOKEN",
			} {
				GinkgoT().Setenv("BK_CLI_"+name, "help-secret-"+name)
			}
			root := newTestRoot()
			out, stderr, err := executeCmdWithStderr(root, "auth", "login", "--help")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("BK_CLI_ACCESS_TOKEN"))
			Expect(out).To(ContainSubstring("BK_CLI_PERSONAL_TOKEN"))
			Expect(out + stderr).NotTo(ContainSubstring("help-secret-"))
			Expect(credential.Exists(config.CredentialsPath("default"))).To(BeFalse())
		})

		DescribeTable(
			"reads environment credentials at execution without modifying flag values",
			func(name, flag string) {
				root := newTestRoot()
				GinkgoT().Setenv(name, "first-env-value")
				_, err := executeCmd(root, "auth", "login")
				Expect(err).NotTo(HaveOccurred())
				login, _, err := root.Find([]string{"auth", "login"})
				Expect(err).NotTo(HaveOccurred())
				value, err := login.Flags().GetString(flag)
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(BeEmpty())
				GinkgoT().Setenv(name, "second-env-value")
				_, err = executeCmd(root, "auth", "login")
				Expect(err).NotTo(HaveOccurred())
				key, err := credential.DeriveKey()
				Expect(err).NotTo(HaveOccurred())
				cred, err := credential.LoadFromFile(config.CredentialsPath("default"), key)
				Expect(err).NotTo(HaveOccurred())
				if flag == "personal_token" {
					Expect(cred.Type).To(Equal(credential.TypePersonalToken))
					Expect(cred.PersonalToken).To(Equal("second-env-value"))
				} else {
					Expect(cred.Type).To(Equal(credential.TypeAccessToken))
					Expect(cred.AccessToken).To(Equal("second-env-value"))
				}
			},
			Entry("access token", "BK_CLI_ACCESS_TOKEN", "access_token"),
			Entry("personal token", "BK_CLI_PERSONAL_TOKEN", "personal_token"),
		)

		It(
			"rejects malformed or conflicting environment personal tokens without replacing saved credentials",
			func() {
				_, err := executeCmd(newTestRoot(), "auth", "login", "--access_token=saved-access")
				Expect(err).NotTo(HaveOccurred())
				before, err := os.ReadFile(config.CredentialsPath("default"))
				Expect(err).NotTo(HaveOccurred())
				for _, value := range []string{"Bearer env-sensitive", "env-sensitive value", "env-sensitive\r\nvalue", "env-sensitive:bad", "env-sensitive=bad", "env-sensitive\x7f", "秘密"} {
					GinkgoT().Setenv("BK_CLI_PERSONAL_TOKEN", value)
					out, stderr, err := executeCmdWithStderr(newTestRoot(), "auth", "login")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid_credentials"))
					Expect(out + stderr + err.Error()).NotTo(ContainSubstring(value))
					after, err := os.ReadFile(config.CredentialsPath("default"))
					Expect(err).NotTo(HaveOccurred())
					Expect(after).To(Equal(before))
				}
				GinkgoT().Setenv("BK_CLI_PERSONAL_TOKEN", "env-personal")
				for _, name := range []string{"BK_CLI_BK_APP_CODE", "BK_CLI_BK_APP_SECRET", "BK_CLI_BK_TOKEN", "BK_CLI_BK_TICKET", "BK_CLI_ACCESS_TOKEN"} {
					GinkgoT().Setenv(name, "env-sensitive")
					out, stderr, err := executeCmdWithStderr(newTestRoot(), "auth", "login")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid_credentials"))
					Expect(err.Error()).To(ContainSubstring("BK_CLI_PERSONAL_TOKEN"))
					Expect(out + stderr + err.Error()).NotTo(ContainSubstring("env-sensitive"))
					Expect(out + stderr + err.Error()).NotTo(ContainSubstring("env-personal"))
					after, err := os.ReadFile(config.CredentialsPath("default"))
					Expect(err).NotTo(HaveOccurred())
					Expect(after).To(Equal(before))
					GinkgoT().Setenv(name, "")
				}
			},
		)

		It("does not overwrite saved credentials on an environment conflict", func() {
			_, err := executeCmd(newTestRoot(), "auth", "login", "--access_token=saved-access")
			Expect(err).NotTo(HaveOccurred())
			before, err := os.ReadFile(config.CredentialsPath("default"))
			Expect(err).NotTo(HaveOccurred())
			GinkgoT().Setenv("BK_CLI_ACCESS_TOKEN", "env-access")
			GinkgoT().Setenv("BK_CLI_BK_TICKET", "env-ticket")
			_, _, err = executeCmdWithStderr(newTestRoot(), "auth", "login")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid_credentials"))
			after, err := os.ReadFile(config.CredentialsPath("default"))
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(before))
		})

		It("succeeds with bk_app_code + bk_app_secret + bk_token", func() {
			root := newTestRoot()
			out, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
				"--bk_token=mytoken",
			)
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())
			Expect(env["message"]).To(ContainSubstring("default"))

			// Verify credentials file was created
			credPath := config.CredentialsPath("default")
			Expect(credential.Exists(credPath)).To(BeTrue())
		})

		It("succeeds with access_token", func() {
			root := newTestRoot()
			out, err := executeCmd(root, "auth", "login",
				"--access_token=my_access_token",
			)
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			// Verify we can load and it's the right type
			credPath := config.CredentialsPath("default")
			key, err := credential.DeriveKey()
			Expect(err).NotTo(HaveOccurred())
			cred, err := credential.LoadFromFile(credPath, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(cred.Type).To(Equal(credential.TypeAccessToken))
			Expect(cred.AccessToken).To(Equal("my_access_token"))
		})

		It("returns JSON error when no flags provided (device flow not implemented)", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not_implemented"))
		})

		It("returns a context error when no context is initialized", func() {
			Expect(config.DeleteContext("default")).To(Succeed())
			Expect(os.Remove(filepath.Join(tmpDir, "current"))).To(Succeed())

			root := newTestRoot()
			_, stderr, err := executeCmdWithStderr(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
				"--bk_token=mytoken",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context init"))
			Expect(stderr).To(ContainSubstring(`--bk_api_url_tmpl=URL`))
		})

		It("fails with incomplete app credentials", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
			)
			Expect(err).To(HaveOccurred())
		})

		It("explains that app credential mode needs a user credential when user_key is bk_token", func() {
			root := newTestRoot()
			_, stderr, err := executeCmdWithStderr(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provide --bk_token or --bk_ticket"))
			Expect(err.Error()).To(ContainSubstring("current context defaults to --bk_token"))
			Expect(stderr).To(ContainSubstring(`--bk_app_code='APP_CODE'`))
			Expect(stderr).To(ContainSubstring(`--bk_app_secret='APP_SECRET'`))
			Expect(stderr).To(ContainSubstring(`--bk_token='VALUE'`))
		})

		It("explains that app credential mode needs a user credential when user_key is bk_ticket", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
				UserKey:      "bk_ticket",
			}
			Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())

			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provide --bk_token or --bk_ticket"))
			Expect(err.Error()).To(ContainSubstring("current context defaults to --bk_ticket"))
		})

		It("stores bk_ticket credentials when provided", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
				"--bk_ticket=myticket",
			)
			Expect(err).NotTo(HaveOccurred())

			credPath := config.CredentialsPath("default")
			key, err := credential.DeriveKey()
			Expect(err).NotTo(HaveOccurred())
			cred, err := credential.LoadFromFile(credPath, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(cred.BkTicket).To(Equal("myticket"))
			Expect(cred.BkToken).To(BeEmpty())
		})

		It("rejects tenant_id because tenant belongs to context config", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--access_token=my_access_token",
				"--tenant_id=tenant-a",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown flag"))
			Expect(err.Error()).To(ContainSubstring("tenant_id"))
		})
	})

	Describe("personal token", func() {
		It("stores encrypted credentials and supports status, check, logout and context isolation", func() {
			Expect(
				config.CreateContext(
					"dev",
					&config.Config{BkAPIURLTmpl: "https://example.com/{gateway_name}/"},
				),
			).To(
				Succeed(),
			)
			_, err := executeCmd(newTestRoot(), "auth", "login", "--access_token=legacy")
			Expect(err).NotTo(HaveOccurred())
			out, err := executeCmd(
				newTestRoot(),
				"auth",
				"login",
				"--context=dev",
				"--personal_token=bkp_test-personal",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).NotTo(ContainSubstring("bkp_test-personal"))
			key, err := credential.DeriveKey()
			Expect(err).NotTo(HaveOccurred())
			stored, err := os.ReadFile(config.CredentialsPath("dev"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(stored)).NotTo(ContainSubstring("bkp_test-personal"))
			cred, err := credential.LoadFromFile(config.CredentialsPath("dev"), key)
			Expect(err).NotTo(HaveOccurred())
			raw, err := cred.Marshal()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).To(ContainSubstring(`"personal_token":"bkp_test-personal"`))
			for _, action := range []string{"status", "check"} {
				out, err = executeCmd(newTestRoot(), "auth", action, "--context=dev")
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(ContainSubstring(`"credential_type": "personal_token"`))
				Expect(out).NotTo(ContainSubstring("bkp_test-personal"))
			}
			_, err = executeCmd(newTestRoot(), "auth", "logout", "--context=dev")
			Expect(err).NotTo(HaveOccurred())
			Expect(credential.Exists(config.CredentialsPath("dev"))).To(BeFalse())
			out, err = executeCmd(newTestRoot(), "auth", "check")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`"credential_type": "access_token"`))
		})

		It(
			"rejects malformed inputs and mixed flags without replacing stored credentials or leaking values",
			func() {
				_, err := executeCmd(newTestRoot(), "auth", "login", "--access_token=legacy")
				Expect(err).NotTo(HaveOccurred())
				before, err := os.ReadFile(config.CredentialsPath("default"))
				Expect(err).NotTo(HaveOccurred())
				inputs := [][]string{
					{"--personal_token="},
					{"--personal_token= "},
					{"--personal_token=secret\r\nvalue"},
					{"--personal_token=secret value"},
					{"--personal_token=Bearer secret"},
					{"--personal_token=secret:bad"},
					{"--personal_token=secret=bad"},
					{"--personal_token=secret\x7f"},
					{"--personal_token=秘密"},
				}
				for _, flag := range []string{"bk_app_code", "bk_app_secret", "bk_token", "bk_ticket", "access_token"} {
					inputs = append(inputs, []string{"--personal_token=secret", "--" + flag + "="})
					inputs = append(
						inputs,
						[]string{"--personal_token=secret", "--" + flag + "=legacy"},
					)
				}
				for _, input := range inputs {
					_, stderr, err := executeCmdWithStderr(
						newTestRoot(),
						append([]string{"auth", "login"}, input...)...)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid_credentials"))
					Expect(err.Error()).NotTo(ContainSubstring("secret value"))
					Expect(stderr).NotTo(ContainSubstring("secret value"))
					after, err := os.ReadFile(config.CredentialsPath("default"))
					Expect(err).NotTo(HaveOccurred())
					Expect(after).To(Equal(before))
				}
			},
		)
	})

	Describe("status", func() {
		It("shows credential info after login", func() {
			// First login
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp123",
				"--bk_app_secret=mysecret",
				"--bk_token=mytoken",
			)
			Expect(err).NotTo(HaveOccurred())

			// Then check status
			root2 := newTestRoot()
			out, err := executeCmd(root2, "auth", "status")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data, ok := env["data"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(data["context"]).To(Equal("default"))
			Expect(data["credential_type"]).To(Equal("app_user"))
			Expect(data["has_credentials"]).To(BeTrue())
			Expect(data["bk_app_code"]).To(Equal("my***23"))
			Expect(data["user_key"]).To(Equal("bk_token"))
		})

		It("returns has_credentials false when no credentials exist", func() {
			root := newTestRoot()
			out, err := executeCmd(root, "auth", "status")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data, ok := env["data"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(data["context"]).To(Equal("default"))
			Expect(data["has_credentials"]).To(BeFalse())
			Expect(data).NotTo(HaveKey("credential_type"))
		})
	})

	Describe("check", func() {
		It("succeeds when credentials exist", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--access_token=my_access_token",
			)
			Expect(err).NotTo(HaveOccurred())

			root2 := newTestRoot()
			out, err := executeCmd(root2, "auth", "check")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data, ok := env["data"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(data["context"]).To(Equal("default"))
			Expect(data["credential_type"]).To(Equal("access_token"))
			Expect(data["has_credentials"]).To(BeTrue())
		})

		It("returns error when no credentials exist", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "check")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no_credentials"))
		})
	})

	Describe("logout", func() {
		It("removes credentials after login", func() {
			// First login
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
				"--bk_token=mytoken",
			)
			Expect(err).NotTo(HaveOccurred())

			credPath := config.CredentialsPath("default")
			Expect(credential.Exists(credPath)).To(BeTrue())

			// Then logout
			root2 := newTestRoot()
			out, err := executeCmd(root2, "auth", "logout")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			Expect(credential.Exists(credPath)).To(BeFalse())
		})
	})
})
