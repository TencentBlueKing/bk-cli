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

// Package output defines the JSON envelopes written by the CLI.
package output

import (
	"io"
	"os"

	json "github.com/goccy/go-json"
)

// Envelope is the standard JSON response wrapper.
type Envelope struct {
	OK      bool              `json:"ok"`
	Status  int               `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Data    any               `json:"data,omitempty"`
	Message string            `json:"message,omitempty"`
	DryRun  bool              `json:"dry_run,omitempty"`
	Request *DryRunRequest    `json:"request,omitempty"`
	Error   *ErrorDetail      `json:"error,omitempty"`
}

// DryRunRequest represents the request that would be made.
type DryRunRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Params  any               `json:"params,omitempty"`
	Body    any               `json:"body,omitempty"`
}

// ErrorDetail contains machine-readable error information.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Success creates a success envelope with a message.
func Success(message string) *Envelope {
	return &Envelope{OK: true, Message: message}
}

// SuccessData creates a success envelope with data.
func SuccessData(data any) *Envelope {
	return &Envelope{OK: true, Data: data}
}

// APIResponse creates an API response envelope.
func APIResponse(status int, headers map[string]string, data any) *Envelope {
	return &Envelope{
		OK:      status >= 200 && status < 300,
		Status:  status,
		Headers: headers,
		Data:    data,
	}
}

// DryRun creates a dry-run envelope.
func DryRun(req *DryRunRequest) *Envelope {
	return &Envelope{OK: true, DryRun: true, Request: req}
}

// Err creates an error envelope.
func Err(code, message, hint string) *Envelope {
	return &Envelope{
		OK:    false,
		Error: &ErrorDetail{Code: code, Message: message, Hint: hint},
	}
}

// Print writes the envelope to stdout as JSON.
func (e *Envelope) Print() error {
	return e.WriteJSON(os.Stdout)
}

// PrintErr writes the envelope to stderr as JSON.
func (e *Envelope) PrintErr() error {
	return e.WriteJSON(os.Stderr)
}

// WriteJSON writes the envelope as formatted JSON to the given writer.
func (e *Envelope) WriteJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(e)
}
