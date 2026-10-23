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
	"sort"
	"strings"
	"unicode"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// Server is a structurally validated configuration entry. Header values are final
// literal values; ResolveHeaders validates authentication before remote use.
type Server struct {
	Name       string
	ConfigFile string
	URL        string
	Headers    map[string]string
}

// Config contains the selected file and all structurally validated servers.
// Loading configuration never resolves credentials or connects to a server.
type Config struct {
	File    string
	Servers []*Server
}

// LoadServer selects an exact server name from one validated configuration file.
func LoadServer(path, name string) (*Server, error) {
	loaded, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	for _, server := range loaded.Servers {
		if server.Name == name {
			return server, nil
		}
	}
	return nil, &Error{"mcp_server_not_found", "Named MCP server not found in selected file"}
}

// LoadConfig loads exactly one file and validates every server's structure.
// Servers are sorted by name for stable local listings; headers remain literal.
func LoadConfig(path string) (*Config, error) {
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
	loaded := &Config{File: path, Servers: make([]*Server, 0, len(servers))}
	for alias, value := range servers {
		server, err := parseServer(alias, value)
		if err != nil {
			return nil, err
		}
		server.ConfigFile = path
		loaded.Servers = append(loaded.Servers, server)
	}
	sort.Slice(loaded.Servers, func(i, j int) bool { return loaded.Servers[i].Name < loaded.Servers[j].Name })
	return loaded, nil
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

// ResolveHeaders validates authentication and copies configured values verbatim.
// It never expands environment variables or loads context/fallback credentials.
func (s Server) ResolveHeaders() (http.Header, error) {
	if err := validateHeaders(s.Headers); err != nil {
		return nil, err
	}
	headers := make(http.Header)
	for name, value := range s.Headers {
		headers.Set(name, value)
	}
	if err := validateAuth(headers); err != nil {
		return nil, err
	}
	return headers, nil
}
