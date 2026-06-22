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

// Package api builds and executes BlueKing API requests.
package api

import (
	"fmt"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

// AuthPolicy describes the auth behavior required to build an API Gateway auth header.
type AuthPolicy interface {
	RequiresAuth() bool
	RequiresAppVerification() bool
	RequiresUserVerification() bool
}

// AuthRequirements describes the auth identity requirements needed to build
// an API Gateway auth header.
//
// A nil *AuthRequirements means the auth behavior was not explicitly configured
// (for example, `bk-cli api`), so the caller should keep the existing
// permissive/default behavior instead of treating it as "no auth required".
// Use &AuthRequirements{} to explicitly disable generated auth requirements.
type AuthRequirements struct {
	AppVerifiedRequired  bool
	UserVerifiedRequired bool
}

// RequiresAuth reports whether an action needs any generated auth header.
func (c *AuthRequirements) RequiresAuth() bool {
	if c == nil {
		return true
	}
	return c.RequiresAppVerification() || c.RequiresUserVerification()
}

// RequiresAppVerification reports whether app verification fields are required.
func (c *AuthRequirements) RequiresAppVerification() bool {
	if c == nil {
		return true
	}
	return c.AppVerifiedRequired
}

// RequiresUserVerification reports whether user identity fields are required.
func (c *AuthRequirements) RequiresUserVerification() bool {
	if c == nil {
		return true
	}
	return c.UserVerifiedRequired
}

// BuildAuthHeader constructs the X-Bkapi-Authorization header value
// from the given credential.
//
// for `bk-cli api`, the authConfig is nil
//
//	-- only provide what we have in the credential
//
// for `bk-cli system action`, the authConfig is not nil
//
//	-- provide what is required by the authConfig
//
// Mapping:
//
//	app_user (bk_token):  {"bk_app_code":"x","bk_app_secret":"y","bk_token":"z"}
//	app_user (bk_ticket): {"bk_app_code":"x","bk_app_secret":"y","bk_ticket":"z"}
//	app_user (app-only):  {"bk_app_code":"x","bk_app_secret":"y"}
//	access_token:         {"access_token":"z"}
func BuildAuthHeader(cred *credential.Credential, authConfig AuthPolicy) (string, error) {
	if authConfig != nil && !authConfig.RequiresAuth() {
		return "", nil
	}

	if authConfig != nil && authConfig.RequiresAuth() && cred == nil {
		return "", fmt.Errorf("credential is required for configured authentication")
	}

	if cred == nil {
		return "", nil
	}

	authMap := make(map[string]string)

	switch cred.Type {
	case credential.TypeAppUser:
		if authConfig == nil || authConfig.RequiresAppVerification() {
			if cred.BkAppCode == "" {
				return "", fmt.Errorf("bk_app_code is required for configured app verification")
			}
			if cred.BkAppSecret == "" {
				return "", fmt.Errorf("bk_app_secret is required for configured app verification")
			}
			authMap["bk_app_code"] = cred.BkAppCode
			authMap["bk_app_secret"] = cred.BkAppSecret
		}
		if authConfig == nil || authConfig.RequiresUserVerification() {
			if cred.BkToken == "" && cred.BkTicket == "" && authConfig != nil {
				return "", fmt.Errorf(
					"bk_token or bk_ticket is required for configured user verification",
				)
			}
			if cred.BkToken != "" {
				authMap["bk_token"] = cred.BkToken
			}
			if cred.BkTicket != "" {
				authMap["bk_ticket"] = cred.BkTicket
			}
		}
	case credential.TypeAccessToken:
		if cred.AccessToken == "" && authConfig != nil {
			return "", fmt.Errorf("access_token is required for configured authentication")
		}
		authMap["access_token"] = cred.AccessToken
	}

	data, err := json.Marshal(authMap)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
