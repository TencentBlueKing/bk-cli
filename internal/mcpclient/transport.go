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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Fixed remote limits apply while reading, before SDK decoding or aggregation.
const (
	MaxResponseBytes  = 16 << 20
	MaxCatalogBytes   = 32 << 20
	MaxDiscoveryPages = 100
)

type wireState struct {
	mu          sync.Mutex
	failure     *Error
	result      map[string]any
	rpcError    map[string]any
	resultBytes int
	known       bool
	reached     bool
	calls       int
	headers     map[string]string
}

func remoteError(code string) *Error {
	messages := map[string]string{
		"mcp_auth_required":          "MCP authentication is missing or rejected; check the selected server headers",
		"mcp_permission_denied":      "MCP permission denied; check server, tool and business permissions",
		"mcp_connection_error":       "MCP connection failed; check the configured endpoint and TLS settings",
		"mcp_timeout":                "MCP operation timed out or was cancelled",
		"mcp_http_error":             "The MCP endpoint returned an unsuccessful HTTP status",
		"mcp_rpc_error":              "The MCP server returned a JSON-RPC error",
		"mcp_protocol_error":         "Invalid MCP response or protocol negotiation",
		"mcp_discovery_incomplete":   "MCP tool discovery did not complete",
		"mcp_response_too_large":     "MCP response or tool catalog exceeds the fixed size limit",
		"mcp_tool_error":             "The MCP tool reported an error",
		"mcp_capability_unsupported": "MCP interaction or asynchronous execution is not supported",
	}
	return &Error{code, messages[code]}
}

func (s *wireState) fail(code string) *Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure == nil || code == "mcp_response_too_large" {
		s.failure = remoteError(code)
	}
	return s.failure
}

func (s *wireState) begin() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = nil
	s.result = nil
	s.rpcError = nil
	s.resultBytes = 0
	s.known = false
}

// observe retains lossless JSON before the SDK converts generic metadata to
// float64. It does not implement protocol negotiation, routing or retries.
func (s *wireState) observe(data []byte, method string, id json.RawMessage) (bool, error) {
	raw, err := decodeJSON(bytes.NewReader(data))
	if err != nil {
		return true, s.fail("mcp_protocol_error")
	}
	object, ok := raw.(map[string]any)
	if !ok || object["jsonrpc"] != "2.0" {
		return true, s.fail("mcp_protocol_error")
	}
	if incoming, ok := object["method"].(string); ok {
		if incoming == "sampling/createMessage" || incoming == "elicitation/create" ||
			strings.HasPrefix(incoming, "tasks/") {
			s.mu.Lock()
			s.result = map[string]any{"unsupported_request": object}
			s.known = true
			s.mu.Unlock()
			return true, s.fail("mcp_capability_unsupported")
		}
		return false, nil
	}
	var identity struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(data, &identity) != nil || !bytes.Equal(bytes.TrimSpace(identity.ID), bytes.TrimSpace(id)) {
		return true, s.fail("mcp_protocol_error")
	}
	if method != "tools/list" && method != "tools/call" {
		return true, nil
	}
	if _, exists := object["error"]; exists {
		if !validRPCError(object) {
			return true, s.fail("mcp_protocol_error")
		}
		s.mu.Lock()
		s.known = true
		detail, _ := object["error"].(map[string]any) // checked by validRPCError above
		s.rpcError = map[string]any{"code": detail["code"], "message": detail["message"]}
		s.mu.Unlock()
		return true, s.fail("mcp_rpc_error")
	}
	result, ok := object["result"].(map[string]any)
	if !ok {
		return true, s.fail("mcp_protocol_error")
	}
	s.mu.Lock()
	s.result = result
	s.resultBytes = len(data)
	s.mu.Unlock()
	return true, nil
}

func validRPCError(object map[string]any) bool {
	if object["jsonrpc"] != "2.0" {
		return false
	}
	if _, exists := object["result"]; exists {
		return false
	}
	detail, ok := object["error"].(map[string]any)
	if !ok {
		return false
	}
	code, ok := detail["code"].(json.Number)
	if !ok {
		return false
	}
	if _, err := strconv.ParseInt(string(code), 10, 64); err != nil {
		return false
	}
	_, ok = detail["message"].(string)
	return ok
}

type guardedTransport struct {
	base    *http.Transport
	ctx     context.Context
	headers http.Header
	state   *wireState
}

// RoundTrip applies one target's headers and enforces no-replay and read limits.
func (t *guardedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	state := t.state
	var rpc struct {
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
	}
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, state.fail("mcp_protocol_error")
		}
		err = json.NewDecoder(body).Decode(&rpc)
		_ = body.Close()
		if err != nil {
			return nil, state.fail("mcp_protocol_error")
		}
	}
	// Cleanup, notifications and replies have private diagnostics so they
	// cannot overwrite the current operation's headers or failure.
	if rpc.Method != "server/discover" && rpc.Method != "initialize" &&
		rpc.Method != "tools/list" && rpc.Method != "tools/call" {
		state = &wireState{}
	}
	if t.ctx.Err() != nil {
		return nil, state.fail("mcp_timeout")
	}
	state.mu.Lock()
	failure := state.failure
	state.mu.Unlock()
	// A failed operation must not prevent bounded SDK session cleanup.
	if failure != nil && request.Method != http.MethodDelete {
		return nil, failure
	}
	if rpc.Method == "tools/call" {
		state.mu.Lock()
		state.calls++
		calls := state.calls
		state.mu.Unlock()
		if calls > 1 {
			return nil, state.fail("mcp_protocol_error")
		}
	}
	requestContext, cancel := context.WithCancel(request.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	release := func() { stop(); cancel() }
	cloned := request.Clone(requestContext)
	for key, values := range t.headers {
		cloned.Header[key] = append([]string(nil), values...)
	}
	// Do not give net/http a replay function for mutating MCP POST requests.
	cloned.GetBody = nil
	response, err := t.base.RoundTrip(cloned)
	if err != nil {
		release()
		if request.Context().Err() != nil || t.ctx.Err() != nil {
			return nil, state.fail("mcp_timeout")
		}
		return nil, state.fail("mcp_connection_error")
	}
	state.mu.Lock()
	state.reached = response.StatusCode >= 200 && response.StatusCode < 300
	// SDK cleanup may run before the caller renders the failed operation.
	if request.Method != http.MethodDelete {
		state.headers = maskedResponseHeaders(response.Header, t.headers)
	}
	state.mu.Unlock()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer release()
		return state.handleHTTPFailure(response, rpc.Method, rpc.ID)
	}
	sse := strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
	response.Body = &observedBody{
		source:  response.Body,
		state:   state,
		method:  rpc.Method,
		id:      rpc.ID,
		sse:     sse,
		release: release,
	}
	return response, nil
}

// handleHTTPFailure bounds error bodies and preserves RPC errors without changing
// authentication, redirect or SDK discovery-fallback precedence.
func (s *wireState) handleHTTPFailure(
	response *http.Response,
	method string,
	id json.RawMessage,
) (*http.Response, error) {
	data, readErr := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	_ = response.Body.Close()
	if len(data) > MaxResponseBytes {
		return nil, s.fail("mcp_response_too_large")
	}
	if readErr != nil {
		return nil, s.fail("mcp_connection_error")
	}
	// The SDK owns the specified server/discover -> initialize compatibility fallback.
	if method == "server/discover" &&
		(response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusNotFound ||
			response.StatusCode == http.StatusMethodNotAllowed) {
		response.Body = io.NopCloser(bytes.NewReader(data))
		return response, nil
	}
	code := "mcp_http_error"
	switch response.StatusCode {
	case http.StatusUnauthorized:
		code = "mcp_auth_required"
	case http.StatusForbidden:
		code = "mcp_permission_denied"
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		code = "mcp_connection_error"
	}
	if code == "mcp_http_error" && (method == "tools/list" || method == "tools/call") {
		// HTTP status errors may still contain a valid JSON-RPC error. Keep
		// HTTP auth/permission/redirect classifications authoritative.
		raw, decodeErr := decodeJSON(bytes.NewReader(data))
		object, ok := raw.(map[string]any)
		if decodeErr == nil && ok && validRPCError(object) {
			_, rpcErr := s.observe(data, method, id)
			return nil, rpcErr
		}
	}
	return nil, s.fail(code)
}

func maskedResponseHeaders(headers, configured http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		value := strings.Join(values, ", ")
		if configured.Get(key) != "" || strings.EqualFold(key, "Authorization") ||
			strings.EqualFold(key, "X-Bkapi-Authorization") ||
			strings.EqualFold(key, "Set-Cookie") || strings.EqualFold(key, "Mcp-Session-Id") {
			value = "***"
		} else {
			for name := range configured {
				if strings.EqualFold(name, key) {
					value = "***"
					break
				}
			}
		}
		result[key] = value
	}
	return result
}

// observedBody mirrors frames solely to preserve raw data. The SDK remains the
// SSE consumer. Both an individual event and JSON/error response are bounded.
type observedBody struct {
	source     io.ReadCloser
	state      *wireState
	method     string
	id         json.RawMessage
	sse        bool
	data       []byte
	line       []byte
	eventBytes int
	eventName  string
	terminal   bool
	afterCR    bool
	release    func()
}

// Close releases the response stream and its operation cancellation hook.
func (b *observedBody) Close() error {
	if b.release != nil {
		b.release()
	}
	return b.source.Close()
}

func (b *observedBody) Read(p []byte) (int, error) {
	if b.sse && b.terminal {
		return 0, io.EOF
	}
	n, err := b.source.Read(p)
	if b.sse {
		for _, char := range p[:n] {
			if observeErr := b.observeByte(char); observeErr != nil {
				return 0, observeErr
			}
		}
		if errors.Is(err, io.EOF) && !b.terminal {
			if b.afterCR || len(b.line) > 0 {
				b.afterCR = false
				if observeErr := b.observeLine(); observeErr != nil {
					return 0, observeErr
				}
			}
			if observeErr := b.dispatchEvent(); observeErr != nil {
				return 0, observeErr
			}
		}
	} else {
		if len(b.data)+n > MaxResponseBytes {
			return 0, b.state.fail("mcp_response_too_large")
		}
		b.data = append(b.data, p[:n]...)
		if errors.Is(err, io.EOF) && len(bytes.TrimSpace(b.data)) > 0 {
			terminal, observeErr := b.state.observe(b.data, b.method, b.id)
			if observeErr != nil {
				return 0, observeErr
			}
			if !terminal {
				return 0, b.state.fail("mcp_protocol_error")
			}
		}
	}
	return n, err
}

func (b *observedBody) observeByte(char byte) error {
	// Delay a CR terminator until the next byte so a CRLF pair is counted
	// together before dispatching/resetting the event budget.
	if b.terminal {
		return nil
	}
	if b.afterCR {
		b.afterCR = false
		if char != '\n' {
			if err := b.observeLine(); err != nil || b.terminal {
				return err
			}
		}
	}
	b.eventBytes++
	if b.eventBytes > MaxResponseBytes {
		return b.state.fail("mcp_response_too_large")
	}
	if char == '\r' {
		b.afterCR = true
		return nil
	}
	if char == '\n' {
		return b.observeLine()
	}
	b.line = append(b.line, char)
	return nil
}

func (b *observedBody) observeLine() error {
	line := b.line
	b.line = nil
	if len(line) == 0 {
		return b.dispatchEvent()
	}
	field, value, _ := bytes.Cut(line, []byte{':'})
	value = bytes.TrimPrefix(value, []byte{' '})
	switch string(field) {
	case "data":
		b.data = append(b.data, value...)
		b.data = append(b.data, '\n')
	case "event":
		b.eventName = string(value)
	}
	return nil
}

func (b *observedBody) dispatchEvent() error {
	if b.terminal {
		return nil
	}
	data := bytes.TrimSuffix(b.data, []byte{'\n'})
	if len(data) > 0 && (b.eventName == "" || b.eventName == "message") {
		terminal, err := b.state.observe(data, b.method, b.id)
		b.terminal = terminal
		if err != nil {
			return err
		}
	}
	b.data = nil
	b.eventBytes = 0
	b.eventName = ""
	return nil
}
