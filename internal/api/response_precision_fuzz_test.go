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
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/TencentBlueKing/bk-cli/internal/api"
)

// The oracle uses exact rationals independently of the production decimal-span
// comparison. Invalid input must stay text; every decoded number must emit the
// same numeric value, including in arrays, maps and escaped numeric strings.
func FuzzResponsePrecision(f *testing.F) {
	for _, raw := range []string{`42`, `0.1`, `9007199254740992`, `9007199254740993`, `1e1000`, `1e-1000000`, `0e-1000000`, `{"id":01}`, `{"id":1}{"id":2}`, `{"s":"\\\"9","n":[0.0012300e2,9007199254740993]}`, `[true,null,"1",1.00E-1,-0.0]`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			t.Skip()
		}
		actual, err := api.ParseResponse(&http.Response{Body: io.NopCloser(strings.NewReader(raw))})
		if err != nil {
			t.Fatal(err)
		}
		if raw == "" {
			if actual != nil {
				t.Fatal("empty response was not nil")
			}
			return
		}
		if !json.Valid([]byte(raw)) {
			if actual != raw {
				t.Fatalf("malformed JSON decoded: %q", raw)
			}
			return
		}
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var expected any
		if err := decoder.Decode(&expected); err != nil {
			t.Fatal(err)
		}
		if err := checkResponseValue(expected, actual); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	})
}

func checkResponseValue(expected, actual any) error {
	switch value := expected.(type) {
	case map[string]any:
		obj, ok := actual.(map[string]any)
		if !ok || len(obj) != len(value) {
			return fmt.Errorf("object shape changed")
		}
		for key, item := range value {
			if err := checkResponseValue(item, obj[key]); err != nil {
				return err
			}
		}
	case []any:
		list, ok := actual.([]any)
		if !ok || len(list) != len(value) {
			return fmt.Errorf("array shape changed")
		}
		for i, item := range value {
			if err := checkResponseValue(item, list[i]); err != nil {
				return err
			}
		}
	case json.Number:
		if number, ok := actual.(json.Number); ok {
			if number != value {
				return fmt.Errorf("number spelling changed")
			}
			return nil
		}
		number, ok := actual.(float64)
		if !ok {
			return fmt.Errorf("number became %T", actual)
		}
		// Zero with an enormous exponent is still zero; don't allocate a huge
		// denominator in the independent oracle for that special case.
		mantissa, _, _ := strings.Cut(strings.ToLower(value.String()), "e")
		if !strings.ContainsAny(mantissa, "123456789") {
			if number != 0 {
				return fmt.Errorf("zero changed")
			}
			return nil
		}
		original, ok := new(big.Rat).SetString(value.String())
		if !ok {
			return fmt.Errorf("invalid oracle number")
		}
		emitted, ok := new(big.Rat).SetString(strconv.FormatFloat(number, 'g', -1, 64))
		if !ok || original.Cmp(emitted) != 0 {
			return fmt.Errorf("number rounded: %s -> %v", value, actual)
		}
	default:
		want, _ := json.Marshal(expected)
		got, _ := json.Marshal(actual)
		if !bytes.Equal(want, got) {
			return fmt.Errorf("value changed")
		}
	}
	return nil
}
