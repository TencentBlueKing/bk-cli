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

// Package credential manages stored authentication credentials and encryption.
package credential

import (
	"fmt"

	json "github.com/goccy/go-json"
)

// CredentialType identifies the credential mode.
type CredentialType string

const (
	TypeAppUser     CredentialType = "app_user"
	TypeAccessToken CredentialType = "access_token"
)

// Credential represents stored authentication data.
type Credential struct {
	Type        CredentialType `json:"type"`
	BkAppCode   string         `json:"bk_app_code,omitempty"`
	BkAppSecret string         `json:"bk_app_secret,omitempty"`
	BkToken     string         `json:"bk_token,omitempty"`
	BkTicket    string         `json:"bk_ticket,omitempty"`
	AccessToken string         `json:"access_token,omitempty"`
}

// Validate checks that the credential has all required fields.
func (c *Credential) Validate() error {
	switch c.Type {
	case TypeAppUser:
		if c.BkAppCode == "" {
			return fmt.Errorf("bk_app_code is required for app_user credential")
		}
		if c.BkAppSecret == "" {
			return fmt.Errorf("bk_app_secret is required for app_user credential")
		}
		if c.BkToken == "" && c.BkTicket == "" {
			return fmt.Errorf("bk_token or bk_ticket is required for app_user credential")
		}
	case TypeAccessToken:
		if c.AccessToken == "" {
			return fmt.Errorf("access_token is required for access_token credential")
		}
	default:
		return fmt.Errorf("unknown credential type: %q", c.Type)
	}
	return nil
}

// Marshal serializes the credential to JSON bytes.
func (c *Credential) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

// Unmarshal deserializes a credential from JSON bytes.
func Unmarshal(data []byte) (*Credential, error) {
	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("failed to parse credential: %w", err)
	}
	return &cred, nil
}

// MaskedAppCode returns the bk_app_code with middle chars replaced by ***.
func (c *Credential) MaskedAppCode() string {
	if len(c.BkAppCode) <= 4 {
		return c.BkAppCode
	}
	return c.BkAppCode[:2] + "***" + c.BkAppCode[len(c.BkAppCode)-2:]
}

// UserKeyType returns which user key is set: "bk_token", "bk_ticket", or "".
func (c *Credential) UserKeyType() string {
	if c.BkToken != "" {
		return "bk_token"
	}
	if c.BkTicket != "" {
		return "bk_ticket"
	}
	return ""
}
