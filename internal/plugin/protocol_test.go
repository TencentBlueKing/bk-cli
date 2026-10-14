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

package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

func envValue(env []string, key string) (string, int) {
	value, count := "", 0
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value, count = v, count+1
		}
	}
	return value, count
}

var _ = Describe("ProjectCredential", func() {
	It("projects app_user with a ticket including app credentials", func() {
		p, err := ProjectCredential(&credential.Credential{
			Type:      credential.TypeAppUser,
			BkAppCode: "app-s", BkAppSecret: "secret-s", BkTicket: "ticket-s",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(
			*p,
		).To(
			Equal(
				AuthPayload{
					Type:        "app_user",
					BkAppCode:   "app-s",
					BkAppSecret: "secret-s",
					BkTicket:    "ticket-s",
				},
			),
		)
	})
	It("projects app_user with a token", func() {
		p, err := ProjectCredential(&credential.Credential{
			Type:      credential.TypeAppUser,
			BkAppCode: "a", BkAppSecret: "s", BkToken: "token-s",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(p.BkToken).To(Equal("token-s"))
		Expect(p.BkTicket).To(BeEmpty())
	})
	It("projects access_token without app fields", func() {
		p, err := ProjectCredential(
			&credential.Credential{Type: credential.TypeAccessToken, AccessToken: "at-s"},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(*p).To(Equal(AuthPayload{Type: "access_token", AccessToken: "at-s"}))
	})
	It("returns nil for nil credentials", func() {
		Expect(ProjectCredential(nil)).To(BeNil())
	})
	DescribeTable(
		"rejects ambiguous or incomplete credentials without echoing values",
		func(c credential.Credential) {
			_, err := ProjectCredential(&c)
			expectPluginError(err, CodeCredentialError)
			Expect(err.Error()).NotTo(ContainSubstring("sentinel"))
			var pErr *Error
			Expect(errors.As(err, &pErr)).To(BeTrue())
			Expect(pErr.Hint).NotTo(ContainSubstring("sentinel"))
		},
		Entry(
			"token and ticket",
			credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "a",
				BkAppSecret: "s",
				BkToken:     "sentinel1",
				BkTicket:    "sentinel2",
			},
		),
		Entry(
			"no user key",
			credential.Credential{Type: credential.TypeAppUser, BkAppCode: "a", BkAppSecret: "sentinel"},
		),
		Entry(
			"missing secret",
			credential.Credential{Type: credential.TypeAppUser, BkAppCode: "a", BkTicket: "sentinel"},
		),
		Entry(
			"app_user with access token",
			credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "a",
				BkAppSecret: "s",
				BkTicket:    "t",
				AccessToken: "sentinel",
			},
		),
		Entry(
			"access_token with user key",
			credential.Credential{Type: credential.TypeAccessToken, AccessToken: "sentinel", BkToken: "t"},
		),
		Entry("empty access token", credential.Credential{Type: credential.TypeAccessToken}),
		Entry("unknown type", credential.Credential{Type: "sentinel", AccessToken: "other"}),
		Entry("NUL byte", credential.Credential{Type: credential.TypeAccessToken, AccessToken: "sentinel\x00"}),
	)
})

var _ = Describe("LoadSession", func() {
	var key []byte
	BeforeEach(func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
		var err error
		key, err = credential.DeriveKey()
		Expect(err).NotTo(HaveOccurred())
		Expect(
			config.CreateContext(
				"alpha",
				&config.Config{
					BkAPIURLTmpl: "https://alpha.example/api/{gateway_name}/",
					TenantID:     "t-alpha",
				},
			),
		).To(
			Succeed(),
		)
		Expect(
			config.CreateContext(
				"beta",
				&config.Config{BkAPIURLTmpl: "https://beta.example/api/{gateway_name}/"},
			),
		).To(
			Succeed(),
		)
		Expect(config.SetActiveContext("alpha")).To(Succeed())
		Expect(
			credential.Save(
				config.CredentialsPath("alpha"),
				&credential.Credential{Type: credential.TypeAccessToken, AccessToken: "alpha-token"},
				key,
			),
		).To(
			Succeed(),
		)
		Expect(
			credential.Save(
				config.CredentialsPath("beta"),
				&credential.Credential{
					Type:        credential.TypeAppUser,
					BkAppCode:   "a",
					BkAppSecret: "s",
					BkTicket:    "beta-ticket",
				},
				key,
			),
		).To(
			Succeed(),
		)
	})

	It("uses the active context and shares credentials", func() {
		s, err := LoadSession("", "shared")
		Expect(err).NotTo(HaveOccurred())
		Expect(
			*s.Context,
		).To(
			Equal(
				ContextInfo{
					Name:         "alpha",
					BkAPIURLTmpl: "https://alpha.example/api/{gateway_name}/",
					TenantID:     "t-alpha",
				},
			),
		)
		Expect(s.Auth.AccessToken).To(Equal("alpha-token"))
	})
	It("prefers the explicit context and does not change the active one", func() {
		s, err := LoadSession("beta", "shared")
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Context.Name).To(Equal("beta"))
		Expect(s.Auth.BkTicket).To(Equal("beta-ticket"))
		Expect(config.ActiveContextName()).To(Equal("alpha"))
	})
	It("provides context but never reads credentials for auth none", func() {
		Expect(os.WriteFile(config.CredentialsPath("alpha"), []byte("corrupted"), 0o600)).To(Succeed())
		s, err := LoadSession("", "none")
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Context.Name).To(Equal("alpha"))
		Expect(s.Auth).To(BeNil())
	})
	It("returns null auth when the context is not logged in", func() {
		Expect(os.Remove(config.CredentialsPath("alpha"))).To(Succeed())
		s, err := LoadSession("", "shared")
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Context).NotTo(BeNil())
		Expect(s.Auth).To(BeNil())
	})
	It("returns an empty session when no context exists", func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
		s, err := LoadSession("", "shared")
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(Equal(Session{}))
		_, err = os.Stat(filepath.Join(os.Getenv("BK_CLI_CONFIG_DIR"), "current"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("fails for an explicit missing context", func() {
		_, err := LoadSession("missing", "none")
		expectPluginError(err, CodeContextError)
	})
	It("fails instead of picking another context when no active or default context exists", func() {
		Expect(os.Remove(filepath.Join(config.BaseDirectory(), "current"))).To(Succeed())
		s, err := LoadSession("", "shared")
		expectPluginError(err, CodeContextError)
		Expect(s).To(Equal(Session{}))
		Expect(err.Error()).NotTo(ContainSubstring("alpha-token"))
	})
	It("uses the default context when no active context exists", func() {
		Expect(os.Remove(filepath.Join(config.BaseDirectory(), "current"))).To(Succeed())
		Expect(
			config.CreateContext(
				"default",
				&config.Config{BkAPIURLTmpl: "https://default.example/api/{gateway_name}/"},
			),
		).To(Succeed())
		s, err := LoadSession("", "shared")
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Context.Name).To(Equal("default"))
		Expect(s.Auth).To(BeNil())
	})
	It("fails for corrupted credentials instead of degrading", func() {
		Expect(os.WriteFile(config.CredentialsPath("alpha"), []byte("corrupted"), 0o600)).To(Succeed())
		_, err := LoadSession("", "shared")
		expectPluginError(err, CodeCredentialError)
	})
	It("fails when the credentials path is not reachable", func() {
		blocker := filepath.Join(config.BaseDirectory(), "blocker-file")
		Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())
		credPath := config.CredentialsPath("alpha")
		Expect(os.Remove(credPath)).To(Succeed())
		Expect(os.Symlink(filepath.Join(blocker, "credentials.enc"), credPath)).To(Succeed())
		_, err := LoadSession("", "shared")
		expectPluginError(err, CodeCredentialError)
		Expect(err.Error()).NotTo(ContainSubstring("alpha-token"))
	})
})

var _ = Describe("BuildEnv", func() {
	It("replaces inherited protocol variables and appends exactly one of each", func() {
		s := Session{
			Context: &ContextInfo{Name: "alpha", BkAPIURLTmpl: "https://a/{gateway_name}/"},
			Auth:    &AuthPayload{Type: "access_token", AccessToken: "at"},
		}
		env, err := BuildEnv(
			[]string{"PATH=/usr/bin", "BK_CLI_PLUGIN_AUTH=old-secret", "BK_CLI_PLUGIN_FUTURE=x", "HOME=/h"},
			s,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(env).To(ContainElements("PATH=/usr/bin", "HOME=/h"))
		Expect(strings.Join(env, "\n")).NotTo(ContainSubstring("old-secret"))
		Expect(strings.Join(env, "\n")).NotTo(ContainSubstring("BK_CLI_PLUGIN_FUTURE"))
		v, n := envValue(env, EnvProtocol)
		Expect(n).To(Equal(1))
		Expect(v).To(Equal("1"))
		v, _ = envValue(env, EnvContext)
		Expect(v).To(MatchJSON(`{"name":"alpha","bk_api_url_tmpl":"https://a/{gateway_name}/"}`))
		v, _ = envValue(env, EnvAuth)
		Expect(v).To(MatchJSON(`{"type":"access_token","access_token":"at"}`))
	})
	It("writes null for missing context and auth", func() {
		env, err := BuildEnv(nil, Session{})
		Expect(err).NotTo(HaveOccurred())
		v, n := envValue(env, EnvContext)
		Expect(v).To(Equal("null"))
		Expect(n).To(Equal(1))
		v, n = envValue(env, EnvAuth)
		Expect(v).To(Equal("null"))
		Expect(n).To(Equal(1))
	})
	It("does not modify the bk-cli process environment", func() {
		GinkgoT().Setenv(EnvAuth, "parent")
		_, err := BuildEnv(os.Environ(), Session{})
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Getenv(EnvAuth)).To(Equal("parent"))
	})
})
