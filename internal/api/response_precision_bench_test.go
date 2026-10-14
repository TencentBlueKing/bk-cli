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

package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/TencentBlueKing/bk-cli/internal/api"
)

func BenchmarkParseResponse(b *testing.B) {
	for _, kind := range []string{"integer_rows", "decimal_series", "mixed_logs"} {
		rows := make([]any, 10000)
		for i := range rows {
			switch kind {
			case "integer_rows":
				rows[i] = map[string]any{"id": i, "time": 1700000000 + i, "count": i % 100}
			case "decimal_series":
				rows[i] = []any{1700000000 + i, float64(i)*0.1 + 0.1234567}
			case "mixed_logs":
				rows[i] = map[string]any{
					"id":       i,
					"message":  "synthetic log event with no sensitive data",
					"service":  "api",
					"duration": float64(i)*0.1 + 0.1234567,
				}
			}
		}
		raw, err := json.Marshal(map[string]any{"list": rows})
		if err != nil {
			b.Fatal(err)
		}
		b.Run(kind, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := api.ParseResponse(&http.Response{Body: io.NopCloser(bytes.NewReader(raw))})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
