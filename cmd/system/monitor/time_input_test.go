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
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("explicit monitor time inputs", func() {
	var shanghai *time.Location
	var reference time.Time
	BeforeEach(func() {
		var err error
		shanghai, err = time.LoadLocation(defaultTimeZone)
		Expect(err).NotTo(HaveOccurred())
		reference = time.Date(2026, 10, 14, 12, 30, 0, 0, shanghai)
		now = func() time.Time { return reference }
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
		GinkgoT().Setenv("BK_CLI_MONITOR_PROFILE_FILE", "")
		Expect(systemtest.SetupTestContext("https://api.example.com")).To(Succeed())
	})
	AfterEach(func() { now = time.Now })

	DescribeTable("normalizes equivalent calendar/RFC3339 inputs to the same instant", func(raw string) {
		got, err := parseTimeValue(raw, "s", shanghai, reference)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Unix()).To(Equal(time.Date(2026, 10, 13, 14, 0, 0, 0, shanghai).Unix()))
	},
		Entry("local minutes", "2026-10-13 14:00"),
		Entry("local seconds", "2026-10-13 14:00:00"),
		Entry("ISO local", "2026-10-13T14:00:00"),
		Entry("explicit offset", "2026-10-13T14:00:00+08:00"),
		Entry("UTC", "2026-10-13T06:00:00Z"),
		Entry("yesterday clock", "yesterday 14:00"),
		Entry("Chinese yesterday clock", "昨天 14:00:00"),
	)

	It("uses the specified zone for local times, but not for offset times or epochs", func() {
		local, err := parseTimeValue("2026-10-13 14:00:00", "s", time.UTC, reference)
		Expect(err).NotTo(HaveOccurred())
		zoned, err := parseTimeValue("2026-10-13T14:00:00+08:00", "s", time.UTC, reference)
		Expect(err).NotTo(HaveOccurred())
		Expect(local.Sub(zoned)).To(Equal(8 * time.Hour))
		unix, err := parseTimeValue(strconv.FormatInt(zoned.Unix(), 10), "s", time.UTC, reference)
		Expect(err).NotTo(HaveOccurred())
		Expect(unix.Equal(zoned)).To(BeTrue())
	})

	It("supports calendar-day endpoints and samples the runtime clock once", func() {
		calls := 0
		now = func() time.Time { calls++; return reference.Add(time.Duration(calls-1) * time.Hour) }
		cmd := toolCommand("search_logs", true)
		setFlags(cmd, "bk_biz_id", "200200", "index_set_id", "101001", "start_time", "今天", "end_time", "现在")
		args := dryRunArgs(cmd)
		Expect(calls).To(Equal(1))
		Expect(
			args["start_time"],
		).To(
			Equal(strconv.FormatInt(time.Date(2026, 10, 14, 0, 0, 0, 0, shanghai).Unix(), 10)),
		)
		Expect(args["end_time"]).To(Equal(strconv.FormatInt(reference.Unix(), 10)))
	})

	It("converts readable dates and explicit s/ms suffixes to the tool's native unit", func() {
		stamp := time.Date(2026, 10, 13, 14, 0, 0, 0, shanghai)
		for _, input := range []string{"2026-10-13 14:00:00", strconv.FormatInt(stamp.Unix(), 10) + "s", strconv.FormatInt(stamp.UnixMilli(), 10) + "ms"} {
			cmd := toolCommand("execute_sql_query", true)
			setFlags(
				cmd,
				"bk_biz_id",
				"200200",
				"table_id",
				"example.table",
				"sql",
				"select 1",
				"start_time",
				input,
				"end_time",
				"2026-10-13 15:00:00",
			)
			args := dryRunArgs(cmd)
			Expect(args["start_time"]).To(Equal(strconv.FormatInt(stamp.UnixMilli(), 10)))
		}
		value, err := parseTimeValue("1000", "ms", shanghai, reference)
		Expect(err).NotTo(HaveOccurred())
		Expect(value.Unix()).To(Equal(int64(1))) // No digit heuristic.
	})

	It("sends integer Profiling times for --since as required by its schema", func() {
		cmd := toolCommand("get_profile_type", true)
		setFlags(cmd, "bk_biz_id", "200200", "app_name", "app", "service_name", "worker", "since", "1h")
		args := dryRunArgs(cmd)
		Expect(args["start_time"]).To(BeNumerically("==", reference.Add(-time.Hour).Unix()))
		Expect(args["start_time"]).NotTo(BeAssignableToTypeOf(""))
	})

	It("normalizes body time fields and retains the named-flag override precedence", func() {
		cmd := toolCommand("search_logs", true)
		setFlags(
			cmd,
			"body",
			`{"bk_biz_id":"200200","index_set_id":101001,"start_time":"yesterday 10:00","end_time":"yesterday 12:00"}`,
			"end_time",
			"yesterday 11:00",
		)
		args := dryRunArgs(cmd)
		Expect(
			args["end_time"],
		).To(
			Equal(strconv.FormatInt(time.Date(2026, 10, 13, 11, 0, 0, 0, shanghai).Unix(), 10)),
		)
	})

	DescribeTable(
		"rejects ambiguous or invalid inputs before sending a request",
		func(start, end, zone, code string) {
			cmd := toolCommand("search_logs", true)
			setFlags(
				cmd,
				"bk_biz_id",
				"200200",
				"index_set_id",
				"101001",
				"start_time",
				start,
				"end_time",
				end,
			)
			if zone != "" {
				setFlags(cmd, "timezone", zone)
			}
			_, err := runProfileCommand(cmd)
			Expect(err.(*output.CLIError).Code).To(Equal(code))
		},
		Entry("reverse range", "today 11:00", "today 10:00", "", "invalid_time_range"),
		Entry("equal range", "today", "today", "", "invalid_time_range"),
		Entry("bad date", "2026-02-30", "2026-03-01", "", "invalid_time"),
		Entry("vague phrase", "昨天下午", "now", "", "invalid_time"),
		Entry("clock without date", "10:00", "now", "", "invalid_time"),
		Entry("unknown timezone", "today", "now", "Asia/NotAZone", "invalid_timezone"),
		Entry("machine Local zone", "today", "now", "Local", "invalid_timezone"),
		Entry(
			"millisecond precision into seconds",
			"2026-10-13T10:00:00.123+08:00",
			"now",
			"",
			"invalid_time_precision",
		),
		Entry("overflow", "9223372036854775807", "now", "", "invalid_time"),
		Entry("log range too long", "2026-10-11", "2026-10-13", "", "invalid_time_range"),
		Entry(
			"DST missing hour",
			"2025-03-09 02:30:00",
			"2025-03-09 04:00:00",
			"America/New_York",
			"nonexistent_local_time",
		),
		Entry(
			"DST repeated hour",
			"2025-11-02 01:30:00",
			"2025-11-02 03:00:00",
			"America/New_York",
			"ambiguous_local_time",
		),
	)

	It("allows explicit offsets to disambiguate a repeated local hour", func() {
		zone, err := time.LoadLocation("America/New_York")
		Expect(err).NotTo(HaveOccurred())
		a, err := parseTimeValue("2025-11-02T01:30:00-04:00", "s", zone, reference)
		Expect(err).NotTo(HaveOccurred())
		b, err := parseTimeValue("2025-11-02T01:30:00-05:00", "s", zone, reference)
		Expect(err).NotTo(HaveOccurred())
		Expect(b.Sub(a)).To(Equal(time.Hour))
	})

	It("allows exact milliseconds but never silently truncates finer precision", func() {
		value, err := parseTimeValue("2026-10-13 14:00:00.1200", "ms", shanghai, reference)
		Expect(err).NotTo(HaveOccurred())
		Expect(value.Nanosecond()).To(Equal(120000000))
		_, err = parseTimeValue("2026-10-13T14:00:00.1234+08:00", "ms", shanghai, reference)
		Expect(err).To(MatchError(ContainSubstring("invalid_time_precision")))
	})

	It("rejects --since with body endpoints and too-small relative windows", func() {
		cmd := toolCommand("search_logs", true)
		setFlags(
			cmd,
			"body",
			`{"bk_biz_id":"200200","index_set_id":101001,"start_time":"yesterday","end_time":"today"}`,
			"since",
			"1h",
		)
		_, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("conflicting_time"))
		cmd = toolCommand("search_logs", true)
		setFlags(cmd, "bk_biz_id", "200200", "index_set_id", "101001", "since", "0.1s")
		_, err = runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("invalid_since"))
	})
})
