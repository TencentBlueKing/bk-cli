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
	"bytes"
	"encoding/json"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

type metadataFailWriter struct{}

func (metadataFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

var _ = Describe("final envelope metadata", func() {
	It("projects metadata and preserves large integer literals", func() {
		cmd := &cobra.Command{}
		AddJQFlag(cmd)
		Expect(cmd.Flags().Set("jq", `{source:.source,id:.data.id + 1}`)).To(Succeed())
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		env := SuccessData(map[string]any{"id": json.Number("9007199254740993")})
		Expect(WriteCommandEnvelopeWithMetadata(cmd, env, map[string]any{"source": "sample"})).To(Succeed())
		Expect(stdout.String()).To(Equal("{\"id\":9007199254740994,\"source\":\"sample\"}\n"))
	})
	It("keeps the original complete failed envelope rather than applying jq", func() {
		cmd := &cobra.Command{}
		AddJQFlag(cmd)
		Expect(cmd.Flags().Set("jq", "empty")).To(Succeed())
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		Expect(
			WriteCommandEnvelopeWithMetadata(
				cmd,
				Err("denied", "denied", ""),
				map[string]any{"source": "sample"},
			),
		).To(
			Succeed(),
		)
		Expect(stdout.String()).To(ContainSubstring(`"ok": false`))
		Expect(stdout.String()).To(ContainSubstring(`"source": "sample"`))
	})
	It("rejects metadata that could overwrite the success or error contract", func() {
		Expect(
			WriteCommandEnvelopeWithMetadata(&cobra.Command{}, Success("ok"), map[string]any{"ok": false}),
		).To(
			MatchError(ContainSubstring("cannot replace")),
		)
	})
	It("rejects non-JSON metadata without writing a partial envelope", func() {
		var out bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&out)
		Expect(
			WriteCommandEnvelopeWithMetadata(cmd, Success("ok"), map[string]any{"source": func() {}}),
		).To(
			HaveOccurred(),
		)
		Expect(out.Len()).To(BeZero())
		Expect(WriteCommandEnvelopeWithMetadata(cmd, SuccessData(func() {}), nil)).To(HaveOccurred())
	})
	It("propagates output failures", func() {
		cmd := &cobra.Command{}
		cmd.SetOut(metadataFailWriter{})
		Expect(
			WriteCommandEnvelopeWithMetadata(cmd, Success("ok"), map[string]any{"source": "sample"}),
		).To(
			MatchError(io.ErrClosedPipe),
		)
	})
})
