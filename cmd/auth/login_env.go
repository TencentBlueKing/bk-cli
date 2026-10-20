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

package auth

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

// resolveLoginInput selects a mode from explicit flags before applying environment fallback.
// Keep flag defaults empty: environment credentials must never appear in help output.
func resolveLoginInput(cmd *cobra.Command) (credential.Credential, error) {
	var input credential.Credential
	hasFlags := false
	fields := []struct {
		name  string
		value *string
	}{
		{"bk_app_code", &input.BkAppCode},
		{"bk_app_secret", &input.BkAppSecret},
		{"bk_token", &input.BkToken},
		{"bk_ticket", &input.BkTicket},
		{"access_token", &input.AccessToken},
		{"personal_token", &input.PersonalToken},
	}
	for _, field := range fields {
		*field.value, _ = cmd.Flags().GetString(field.name)
		if cmd.Flags().Changed(field.name) {
			hasFlags = true
			if *field.value == "" {
				return input, output.UserError("invalid_credentials",
					fmt.Sprintf("--%s must not be empty when explicitly provided", field.name),
					"Omit the flag to use its environment fallback. See: bk-cli auth login --help")
			}
		}
	}

	// Explicit standalone modes ignore environment credentials for every other mode.
	if input.PersonalToken != "" || input.AccessToken != "" {
		return input, nil
	}
	if !hasFlags {
		input.AccessToken = os.Getenv("BK_CLI_ACCESS_TOKEN")
		input.PersonalToken = os.Getenv("BK_CLI_PERSONAL_TOKEN")
	}
	if input.BkAppCode == "" {
		input.BkAppCode = os.Getenv("BK_CLI_BK_APP_CODE")
	}
	if input.BkAppSecret == "" {
		input.BkAppSecret = os.Getenv("BK_CLI_BK_APP_SECRET")
	}
	// An explicit user flag fixes the user mode; ignore environment tickets for other modes.
	if input.BkToken == "" && input.BkTicket == "" {
		input.BkToken = os.Getenv("BK_CLI_BK_TOKEN")
		input.BkTicket = os.Getenv("BK_CLI_BK_TICKET")
		if input.BkToken != "" && input.BkTicket != "" {
			return input, output.UserError(
				"invalid_credentials",
				"BK_CLI_BK_TOKEN and BK_CLI_BK_TICKET select conflicting user credential modes",
				"Keep one user credential environment variable or select --bk_token or --bk_ticket explicitly.",
			)
		}
	}
	if input.PersonalToken != "" && (input.AccessToken != "" || input.BkAppCode != "" ||
		input.BkAppSecret != "" || input.BkToken != "" || input.BkTicket != "") {
		return input, output.UserError("invalid_credentials",
			"BK_CLI_PERSONAL_TOKEN conflicts with other credential environment variables",
			"Keep one credential mode in the environment or select a mode with explicit credential flags.")
	}
	if input.AccessToken != "" && (input.BkAppCode != "" || input.BkAppSecret != "" ||
		input.BkToken != "" || input.BkTicket != "") {
		return input, output.UserError("invalid_credentials",
			"BK_CLI_ACCESS_TOKEN conflicts with app credential environment variables",
			"Keep one credential mode in the environment or select a mode with explicit credential flags.")
	}
	for _, field := range fields {
		if !cmd.Flags().Changed(field.name) && strings.TrimSpace(*field.value) != *field.value {
			return input, output.UserError(
				"invalid_credentials",
				fmt.Sprintf(
					"BK_CLI_%s must not contain leading or trailing whitespace",
					strings.ToUpper(field.name),
				),
				"Remove surrounding whitespace from the environment value. See: bk-cli auth login --help",
			)
		}
	}
	return input, nil
}
