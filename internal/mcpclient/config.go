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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// Server is the selected, structurally validated server. Headers remain literal
// until ResolveHeaders is called, so static help requires no credentials.
type Server struct {
	Name       string
	ConfigFile string
	URL        string
	Headers    map[string]string
}

// LoadServer selects exactly one file, validates its complete structure, and
// returns the exact named server without resolving credentials or reading context.
func LoadServer(path, name string) (*Server, error) {
	if path == "" {
		path = filepath.Join(config.BaseDirectory(), "mcp.json")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, configError()
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, &Error{"mcp_config_not_found", "MCP configuration file not found"}
	}
	if err != nil {
		return nil, configError()
	}
	defer func() { _ = file.Close() }()
	raw, err := decodeJSON(file)
	if err != nil {
		return nil, configError()
	}
	document, ok := raw.(map[string]any)
	if !ok || len(document) != 1 {
		return nil, configError()
	}
	servers, ok := document["mcpServers"].(map[string]any)
	if !ok {
		return nil, configError()
	}
	var selected *Server
	for alias, value := range servers {
		server, err := parseServer(alias, value)
		if err != nil {
			return nil, err
		}
		if alias == name {
			selected = server
		}
	}
	if selected == nil {
		return nil, &Error{"mcp_server_not_found", "Named MCP server not found in selected file"}
	}
	selected.ConfigFile = path
	return selected, nil
}

func parseServer(name string, raw any) (*Server, error) {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		return nil, configError()
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, configError()
	}
	for key := range object {
		if key != "type" && key != "url" && key != "headers" {
			return nil, configError()
		}
	}
	transport, ok := object["type"].(string)
	if !ok || transport != "http" {
		return nil, configError()
	}
	endpoint, ok := object["url"].(string)
	if !ok || !validEndpoint(endpoint) {
		return nil, configError()
	}
	server := &Server{Name: name, URL: endpoint, Headers: make(map[string]string)}
	if rawHeaders, exists := object["headers"]; exists {
		headers, ok := rawHeaders.(map[string]any)
		if !ok {
			return nil, configError()
		}
		for key, value := range headers {
			text, ok := value.(string)
			if !ok {
				return nil, configError()
			}
			server.Headers[key] = text
		}
	}
	if err := validateHeaders(server.Headers); err != nil {
		return nil, err
	}
	return server, nil
}

func validEndpoint(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Hostname() != "" && parsed.User == nil && !strings.Contains(endpoint, "#") && parsed.Opaque == ""
}

func validateHeaders(headers map[string]string) error {
	seen := make(map[string]bool)
	for name, value := range headers {
		lower := strings.ToLower(name)
		if seen[lower] || managedHeader(lower) || validate.ValidateHeaderName(name) != nil ||
			validate.ValidateHeaderValue(value) != nil {
			return configError()
		}
		seen[lower] = true
	}
	return nil
}

func managedHeader(name string) bool {
	if strings.HasPrefix(name, "mcp-") {
		return true
	}
	switch name {
	case "host", "content-length", "content-type", "accept", "connection", "keep-alive",
		"proxy-authenticate", "proxy-authorization", "proxy-connection", "te", "trailer",
		"transfer-encoding", "upgrade", "last-event-id":
		return true
	}
	return false
}

// MaskedHeaders exposes names only. Every configured value uses the same mask.
func (s Server) MaskedHeaders() map[string]string {
	headers := make(map[string]string, len(s.Headers))
	for name := range s.Headers {
		headers[name] = "***"
	}
	return headers
}

// ResolveHeaders expands only explicit references in this server's headers and
// validates authentication. It never loads context or fallback credentials.
func (s Server) ResolveHeaders() (http.Header, error) {
	if err := validateHeaders(s.Headers); err != nil {
		return nil, err
	}
	headers := make(http.Header)
	for name, value := range s.Headers {
		expanded, err := expandHeader(value)
		if err != nil {
			return nil, err
		}
		if validate.ValidateHeaderValue(expanded) != nil {
			return nil, configError()
		}
		headers.Set(name, expanded)
	}
	if err := validateAuth(headers); err != nil {
		return nil, err
	}
	return headers, nil
}
