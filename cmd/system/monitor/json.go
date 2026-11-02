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

package monitor

import (
	"bytes"
	"errors"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
)

// decodeJSON consumes exactly one JSON value and preserves the original number literals.
func decodeJSON(raw []byte, target any) error {
	if !json.Valid(raw) {
		return errors.New("invalid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func jsonObject(raw []byte, target *map[string]any) error {
	if err := decodeJSON(raw, target); err != nil || *target == nil {
		return monitorapi.InputError("invalid_body", "--body 必须是 JSON 对象。")
	}
	return nil
}
