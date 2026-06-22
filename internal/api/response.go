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

package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

// BuildResponseEnvelope creates an output.Envelope from an HTTP response.
func BuildResponseEnvelope(resp *http.Response) (*output.Envelope, error) {
	data, err := ParseResponse(resp)
	if err != nil {
		return nil, err
	}

	headers := ExtractGatewayHeaders(resp)

	return output.APIResponse(resp.StatusCode, headers, data), nil
}

// BuildDryRunEnvelope creates a dry-run output showing what would be sent.
func BuildDryRunEnvelope(req *Request) *output.Envelope {
	dryReq := &output.DryRunRequest{
		Method: req.Method,
		URL:    req.URL,
	}

	// Collect all headers (redact auth)
	headers := make(map[string]string)
	headers["User-Agent"] = userAgent
	if req.BodyJSON != "" {
		headers["Content-Type"] = "application/json"
	}
	if req.AuthHeader != "" {
		headers["X-Bkapi-Authorization"] = "{...redacted...}"
	}
	if req.TenantID != "" {
		headers["X-Bk-Tenant-Id"] = req.TenantID
	}
	for key, value := range req.Headers {
		for existingKey := range headers {
			if strings.EqualFold(existingKey, key) {
				delete(headers, existingKey)
			}
		}
		if strings.EqualFold(key, "X-Bkapi-Authorization") {
			headers[key] = "{...redacted...}"
			continue
		}
		headers[key] = value
	}
	if len(headers) > 0 {
		dryReq.Headers = headers
	}

	// Params
	if req.ParamsJSON != "" {
		dryReq.Params = parseDryRunJSON(req.ParamsJSON)
	}

	// Body
	if req.BodyJSON != "" {
		dryReq.Body = parseDryRunJSON(req.BodyJSON)
	}

	return output.DryRun(dryReq)
}

func parseDryRunJSON(raw string) any {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()

	var value any
	if err := dec.Decode(&value); err != nil {
		return raw
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return raw
	}
	return value
}
