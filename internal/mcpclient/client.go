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
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client owns one CLI operation's SDK session, deadlines and raw result capture.
type Client struct {
	ctx       context.Context
	endpoint  string
	http      *http.Client
	transport *guardedTransport
	state     *wireState
	session   *mcp.ClientSession
}

// NewClient creates an isolated client. Headers must already be resolved and
// validated; no context credentials, OAuth handlers or retry middleware are used.
func NewClient(ctx context.Context, endpoint string, headers http.Header, insecure bool) *Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	base = base.Clone()
	base.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: insecure,
	} // #nosec G402 -- explicit CLI flag.
	state := &wireState{}
	transport := &guardedTransport{base: base, ctx: ctx, headers: headers.Clone(), state: state}
	return &Client{ctx: ctx, endpoint: endpoint, state: state, transport: transport, http: &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Connect performs SDK-owned negotiation without connecting any other server.
func (c *Client) Connect() error {
	sdk := mcp.NewClient(&mcp.Implementation{Name: "bk-cli", Version: "1"}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{}, MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	session, err := sdk.Connect(c.ctx, &mcp.StreamableClientTransport{
		Endpoint:             c.endpoint,
		HTTPClient:           c.http,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
		MaxEventSize:         MaxResponseBytes,
	}, nil)
	if err != nil {
		return c.classify(err)
	}
	c.session = session
	return nil
}

// Close releases the logical session and idle connections. Cleanup is bounded
// by the operation context at the HTTP boundary, including SDK DELETE requests.
func (c *Client) Close() {
	if c.session != nil {
		_ = c.session.Close()
	}
	c.transport.base.CloseIdleConnections()
}

// Connected reports whether any target HTTP response was received.
func (c *Client) Connected() bool {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return c.state.reached
}

// ProtocolVersion returns the SDK-negotiated version after successful Connect.
func (c *Client) ProtocolVersion() string {
	if c.session == nil {
		return ""
	}
	return c.session.InitializeResult().ProtocolVersion
}

// Headers returns the last operation response headers, excluding session cleanup,
// with configured/known secrets masked.
func (c *Client) Headers() map[string]string {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return maps.Clone(c.state.headers)
}

// RPCError returns the server's validated JSON-RPC code and message, if present.
// Like tool results, this remote payload is kept separate from safe diagnostics.
func (c *Client) RPCError() map[string]any {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return maps.Clone(c.state.rpcError)
}

// CallAttempted reports whether a tools/call entered the HTTP transport.
func (c *Client) CallAttempted() bool {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return c.state.calls > 0
}

// ResultKnown distinguishes explicit protocol/tool outcomes from lost responses.
func (c *Client) ResultKnown() bool {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return c.state.known
}

func (c *Client) classify(err error) error {
	if c.ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return remoteError("mcp_timeout")
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.failure != nil {
		return c.state.failure
	}
	return remoteError("mcp_protocol_error")
}

func (c *Client) result() (map[string]any, int) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return c.state.result, c.state.resultBytes
}

// Discover consumes every page, rejects conflicts/cycles, and returns the exact
// losslessly decoded descriptors, including SDK-supported optional metadata.
func (c *Client) Discover() ([]map[string]any, error) {
	tools := make([]map[string]any, 0)
	names := make(map[string]bool)
	cursors := make(map[string]bool)
	cursor := ""
	total := 0
	for range MaxDiscoveryPages {
		c.state.begin()
		_, err := c.session.ListTools(c.ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, c.classify(err)
		}
		raw, size := c.result()
		total += size
		if total > MaxCatalogBytes {
			return nil, remoteError("mcp_response_too_large")
		}
		list, ok := raw["tools"].([]any)
		if !ok {
			return nil, remoteError("mcp_protocol_error")
		}
		for _, entry := range list {
			tool, ok := entry.(map[string]any)
			if !ok {
				return nil, remoteError("mcp_protocol_error")
			}
			name, ok := tool["name"].(string)
			if !ok || name == "" || names[name] {
				return nil, remoteError("mcp_protocol_error")
			}
			if _, ok := tool["inputSchema"].(map[string]any); !ok {
				return nil, remoteError("mcp_protocol_error")
			}
			names[name] = true
			tools = append(tools, tool)
		}
		next, exists := raw["nextCursor"]
		if !exists || next == nil || next == "" {
			return tools, nil
		}
		cursor, ok = next.(string)
		if !ok {
			return nil, remoteError("mcp_protocol_error")
		}
		if cursors[cursor] {
			return nil, remoteError("mcp_discovery_incomplete")
		}
		cursors[cursor] = true
	}
	return nil, remoteError("mcp_discovery_incomplete")
}

// Call sends at most one tools/call. A non-nil result is preserved for explicit
// tool errors or unsupported asynchronous/input-required outcomes.
func (c *Client) Call(name string, arguments map[string]any) (map[string]any, error) {
	c.state.begin()
	_, err := c.session.CallTool(c.ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	result, _ := c.result()
	if err != nil {
		return result, c.classify(err)
	}
	if result == nil {
		return nil, remoteError("mcp_protocol_error")
	}
	if unsupportedResult(result) {
		c.markKnown()
		return result, remoteError("mcp_capability_unsupported")
	}
	if _, ok := result["content"].([]any); !ok {
		return nil, remoteError("mcp_protocol_error")
	}
	if value, exists := result["isError"]; exists {
		failed, ok := value.(bool)
		if !ok {
			return nil, remoteError("mcp_protocol_error")
		}
		if failed {
			c.markKnown()
			return result, remoteError("mcp_tool_error")
		}
	}
	c.markKnown()
	return result, nil
}

func (c *Client) markKnown() { c.state.mu.Lock(); c.state.known = true; c.state.mu.Unlock() }

func unsupportedResult(result map[string]any) bool {
	if kind, ok := result["resultType"].(string); ok && kind != "" && kind != "complete" {
		return true
	}
	for _, key := range []string{"task", "taskId", "inputRequests", "unsupported_request"} {
		if _, ok := result[key]; ok {
			return true
		}
	}
	return false
}
