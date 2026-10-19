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
	"fmt"
	"os"
	"runtime"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

const maxCredentialValueBytes = 16 << 10

// EnvProtocol is the environment variable name for the plugin protocol version.
const EnvProtocol = "BK_CLI_PLUGIN_PROTOCOL"

// EnvContext is the environment variable name for the plugin context JSON payload.
const EnvContext = "BK_CLI_PLUGIN_CONTEXT"

// EnvAuth is the environment variable name for the plugin auth JSON payload.
const EnvAuth = "BK_CLI_PLUGIN_AUTH"

// ProtocolVersion is the supported plugin protocol version.
const ProtocolVersion = "1"

// ContextInfo is the non-secret context environment passed to plugin processes.
type ContextInfo struct {
	Name         string `json:"name"`
	BkAPIURLTmpl string `json:"bk_api_url_tmpl"`
	TenantID     string `json:"tenant_id,omitempty"`
}

// AuthPayload is the protocol v1 credential projection passed to plugin processes.
type AuthPayload struct {
	Type        string `json:"type"`
	BkAppCode   string `json:"bk_app_code,omitempty"`
	BkAppSecret string `json:"bk_app_secret,omitempty"`
	BkToken     string `json:"bk_token,omitempty"`
	BkTicket    string `json:"bk_ticket,omitempty"`
	AccessToken string `json:"access_token,omitempty"`
}

// Session holds the resolved context and optional auth payload for a plugin run.
type Session struct {
	Context *ContextInfo
	Auth    *AuthPayload
}

func credentialError(message string) *Error {
	return SystemError(CodeCredentialError, message, "Run: bk-cli auth login")
}

func checkValues(values ...string) error {
	for _, v := range values {
		if len(v) > maxCredentialValueBytes || strings.ContainsRune(v, 0) {
			return credentialError("stored credential contains an invalid value")
		}
	}
	return nil
}

// ProjectCredential converts a stored credential into the protocol v1 payload.
// It is defined independently of the internal Credential layout.
func ProjectCredential(c *credential.Credential) (*AuthPayload, error) {
	if c == nil {
		return nil, nil
	}
	switch c.Type {
	case credential.TypeAppUser:
		if c.BkAppCode == "" || c.BkAppSecret == "" {
			return nil, credentialError("app_user credential is missing bk_app_code or bk_app_secret")
		}
		if (c.BkToken == "") == (c.BkTicket == "") || c.AccessToken != "" {
			return nil, credentialError(
				"app_user credential must contain exactly one of bk_token or bk_ticket",
			)
		}
		if err := checkValues(c.BkAppCode, c.BkAppSecret, c.BkToken, c.BkTicket); err != nil {
			return nil, err
		}
		return &AuthPayload{
			Type: "app_user", BkAppCode: c.BkAppCode, BkAppSecret: c.BkAppSecret,
			BkToken: c.BkToken, BkTicket: c.BkTicket,
		}, nil
	case credential.TypeAccessToken:
		if c.AccessToken == "" || c.BkToken != "" || c.BkTicket != "" || c.BkAppCode != "" ||
			c.BkAppSecret != "" {
			return nil, credentialError("access_token credential must contain only access_token")
		}
		if err := checkValues(c.AccessToken); err != nil {
			return nil, err
		}
		return &AuthPayload{Type: "access_token", AccessToken: c.AccessToken}, nil
	default:
		return nil, credentialError("credential type is not supported by plugin protocol v1")
	}
}

// LoadSession resolves the context environment and, for auth "shared", the credential payload.
// It never creates files. The context is --context, the active context, or "default";
// when none of these applies but other contexts exist, it fails instead of picking one.
func LoadSession(contextOverride, auth string) (Session, error) {
	name, cfg, err := config.ResolveContextReadOnly(contextOverride)
	if err != nil {
		return Session{}, UserError(CodeContextError, err.Error(), "Run: bk-cli context list")
	}
	if cfg == nil {
		return Session{}, nil
	}
	s := Session{Context: &ContextInfo{
		Name:         name,
		BkAPIURLTmpl: config.NormalizeURLTemplate(cfg.BkAPIURLTmpl),
		TenantID:     cfg.TenantID,
	}}
	if auth != "shared" {
		return s, nil
	}
	path := config.CredentialsPath(name)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return Session{}, credentialError(fmt.Sprintf("cannot access credentials of context %q", name))
	}
	key, err := credential.DeriveKey()
	if err != nil {
		return Session{}, credentialError("cannot derive the credential key")
	}
	c, err := credential.LoadFromFile(path, key)
	if err != nil {
		return Session{}, credentialError(fmt.Sprintf("cannot read credentials of context %q", name))
	}
	payload, err := ProjectCredential(c)
	if err != nil {
		return Session{}, err
	}
	s.Auth = payload
	return s, nil
}

func isProtocolKey(key string) bool {
	if runtime.GOOS == "windows" {
		key = strings.ToUpper(key)
	}
	return strings.HasPrefix(key, "BK_CLI_PLUGIN_")
}

// BuildEnv copies base without any inherited BK_CLI_PLUGIN_* entries and appends protocol v1 values.
func BuildEnv(base []string, s Session) ([]string, error) {
	ctxJSON, err := json.Marshal(s.Context)
	if err != nil {
		return nil, SystemError(CodeLaunchFailed, "cannot encode plugin context", "")
	}
	authJSON, err := json.Marshal(s.Auth)
	if err != nil {
		return nil, credentialError("cannot encode plugin credentials")
	}
	env := make([]string, 0, len(base)+3)
	for _, kv := range base {
		key, _, _ := strings.Cut(kv, "=")
		if key != "" && isProtocolKey(key) {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		EnvProtocol+"="+ProtocolVersion,
		EnvContext+"="+string(ctxJSON),
		EnvAuth+"="+string(authJSON),
	), nil
}
