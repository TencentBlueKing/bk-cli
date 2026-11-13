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

package mcpclient

import (
	"net/http"
	"regexp"
	"strings"
)

func validateAuth(headers http.Header) error {
	_, hasAuthorization := headers["Authorization"]
	_, hasBlueKing := headers[http.CanonicalHeaderKey("X-Bkapi-Authorization")]
	if hasAuthorization && hasBlueKing {
		return &Error{"mcp_auth_conflict", "Authorization and X-Bkapi-Authorization cannot be combined"}
	}
	if hasAuthorization {
		value := headers.Get("Authorization")
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return invalidAuth("Authorization must be non-empty; Bearer requires one valid token")
		}
		if strings.EqualFold(fields[0], "Bearer") && (len(fields) != 2 || !validBearer(fields[1])) {
			return invalidAuth("Authorization must be non-empty; Bearer requires one valid token")
		}
	}
	if hasBlueKing {
		return validateBlueKingAuth(headers.Get("X-Bkapi-Authorization"))
	}
	return nil
}

var bearerToken = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

func validBearer(value string) bool { return bearerToken.MatchString(value) }

func validateBlueKingAuth(value string) error {
	raw, err := decodeJSON(strings.NewReader(value))
	if err != nil {
		return invalidBlueKingAuth()
	}
	fields, ok := raw.(map[string]any)
	if !ok {
		return invalidBlueKingAuth()
	}
	auth := make(map[string]string)
	for key, value := range fields {
		switch key {
		case "access_token", "bk_app_code", "bk_app_secret", "bk_token", "bk_ticket":
		default:
			return invalidBlueKingAuth()
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return invalidBlueKingAuth()
		}
		auth[key] = text
	}
	if auth["access_token"] != "" {
		if len(auth) != 1 {
			return invalidBlueKingAuth()
		}
		return nil
	}
	if len(auth) != 3 || auth["bk_app_code"] == "" || auth["bk_app_secret"] == "" ||
		(auth["bk_token"] == "") == (auth["bk_ticket"] == "") {
		return invalidBlueKingAuth()
	}
	return nil
}

func invalidAuth(message string) error {
	return &Error{"invalid_mcp_auth", message}
}

func invalidBlueKingAuth() error {
	return invalidAuth(
		"X-Bkapi-Authorization requires exactly access_token, or bk_app_code + bk_app_secret + " +
			"one of bk_ticket/bk_token; all values must be non-empty strings",
	)
}
