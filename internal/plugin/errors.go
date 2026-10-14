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

// Package plugin implements the catalog-backed third-party CLI plugin runtime.
package plugin

import "fmt"

// ExitHostFailure is the exit code used when bk-cli fails before starting a plugin process.
const ExitHostFailure = 125

const (
	// CodeUnknown indicates the requested plugin is not in the catalog.
	CodeUnknown = "plugin_unknown"
	// CodeVersionNotAllowed indicates the requested version is not allowed.
	CodeVersionNotAllowed = "plugin_version_not_allowed"
	// CodePlatformUnsupported indicates the requested platform is not supported.
	CodePlatformUnsupported = "plugin_platform_unsupported"
	// CodeNotInstalled indicates the plugin is not installed.
	CodeNotInstalled = "plugin_not_installed"
	// CodeUnsupportedHostFlag indicates an unsupported host flag was passed.
	CodeUnsupportedHostFlag = "plugin_unsupported_host_flag"
	// CodeContextError indicates a context resolution error.
	CodeContextError = "plugin_context_error"
	// CodeDigestMismatch indicates a digest verification failure.
	CodeDigestMismatch = "plugin_digest_mismatch"
	// CodeCredentialError indicates a credential handling error.
	CodeCredentialError = "plugin_credential_error"
	// CodeLaunchFailed indicates the plugin process failed to launch.
	CodeLaunchFailed = "plugin_launch_failed"
	// CodeCatalogInvalid indicates the catalog is invalid.
	CodeCatalogInvalid = "plugin_catalog_invalid"
	// CodeStateInvalid indicates the plugin state is invalid.
	CodeStateInvalid = "plugin_state_invalid"
	// CodeDownloadFailed indicates a download failure.
	CodeDownloadFailed = "plugin_download_failed"
	// CodeArchiveInvalid indicates an archive extraction or format error.
	CodeArchiveInvalid = "plugin_archive_invalid"
	// CodeIOError indicates an I/O error.
	CodeIOError = "plugin_io_error"
	// CodeBusy indicates the plugin manager is busy.
	CodeBusy = "plugin_busy"
)

// Error is a machine-readable plugin error. Message and Hint must never contain credential values.
type Error struct {
	Code     string
	Message  string
	Hint     string
	ExitCode int
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// UserError returns an input or state error that the user can fix.
func UserError(code, message, hint string) *Error {
	return &Error{Code: code, Message: message, Hint: hint, ExitCode: 1}
}

// SystemError returns an environment, I/O, or integrity error.
func SystemError(code, message, hint string) *Error {
	return &Error{Code: code, Message: message, Hint: hint, ExitCode: 2}
}
