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

package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// WriteCommandEnvelopeWithMetadata adds command-local provenance to the complete
// final document before applying the existing jq/error/output contract. Core fields
// cannot be replaced, and ordinary envelope writers keep their original behavior.
func WriteCommandEnvelopeWithMetadata(cmd *cobra.Command, env *Envelope, fields map[string]any) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	document := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	for key, value := range fields {
		switch key {
		case "ok", "status", "headers", "data", "message", "dry_run", "request", "error":
			return fmt.Errorf("metadata cannot replace envelope field %q", key)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		document[key] = encoded
	}
	return writeCommandEnvelope(cmd, env, document)
}

func writeFinalDocument(writer io.Writer, document any) error {
	if env, ok := document.(*Envelope); ok {
		return env.WriteJSON(writer)
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
