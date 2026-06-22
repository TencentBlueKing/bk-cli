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

package gse

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("buildAgentListBody", func() {
	It("rejects more than 1000 agent IDs", func() {
		ids := make([]string, 1001)
		for i := range ids {
			ids[i] = fmt.Sprintf("020000000000000000000000%08d", i)
		}

		_, err := buildAgentListBody("", strings.Join(ids, ","))
		Expect(err).To(MatchError(ContainSubstring("agent_id_list cannot contain more than 1000 entries")))
	})
})
