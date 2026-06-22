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

package cmdb

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newListBizHostsAllCmd(deps systemcmd.BuildDeps) *cobra.Command {
	const defaultLimit = 500

	var (
		bizID     int
		setID     int
		moduleID  int
		pageLimit int
		stage     string
		headers   []string
	)

	cmd := &cobra.Command{
		Use:   "list_biz_hosts_all",
		Short: "List all hosts in a business by paging through CMDB results",
		Example: strings.Join([]string{
			"  bk-cli cmdb list_biz_hosts_all --bk_biz_id 2",
			"  bk-cli cmdb list_biz_hosts_all --bk_biz_id 2 --bk_set_id 10 --page_limit 200",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
				return err
			}
			if err := systemcmd.ValidatePositiveIntFlagIfChanged(
				"bk_set_id",
				setID,
				cmd.Flags().Changed("bk_set_id"),
			); err != nil {
				return err
			}
			if err := systemcmd.ValidatePositiveIntFlagIfChanged(
				"bk_module_id",
				moduleID,
				cmd.Flags().Changed("bk_module_id"),
			); err != nil {
				return err
			}
			if err := systemcmd.ValidatePositiveIntFlag("page_limit", pageLimit); err != nil {
				return err
			}

			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			initialBody, err := buildListBizHostsAllBody(
				0,
				pageLimit,
				setID,
				cmd.Flags().Changed("bk_set_id"),
				moduleID,
				cmd.Flags().Changed("bk_module_id"),
			)
			if err != nil {
				return err
			}

			if runtime.DryRun {
				result, err := syslib.ExecuteRequest(runtime, syslib.RequestSpec{
					GatewayName: gatewayName,
					Method:      "POST",
					Path:        fmt.Sprintf("/api/v3/open/hosts/app/%d/list_hosts", bizID),
					BodyJSON:    initialBody,
					Headers:     headers,
					Stage:       stage,
					AuthConfig: &syslib.AuthConfig{
						AppVerifiedRequired:        true,
						UserVerifiedRequired:       true,
						ResourcePermissionRequired: true,
					},
				})
				if err != nil {
					return err
				}
				if err := systemcmd.EnsureEnvelope("list_biz_hosts_all", result.Envelope); err != nil {
					return err
				}
				result.Envelope.Data = map[string]any{
					"pagination": map[string]any{
						"aggregates_all_pages": true,
						"page_limit":           pageLimit,
					},
				}
				return result.Envelope.WriteJSON(cmd.OutOrStdout())
			}

			const maxPages = 1000

			allHosts := make([]any, 0)
			seenHostIDs := make(map[int64]struct{})
			lastReportedCount := 0
			start := 0
			var lastEnvelope *output.Envelope
			completed := false

			for range maxPages {
				bodyJSON, err := buildListBizHostsAllBody(
					start,
					pageLimit,
					setID,
					cmd.Flags().Changed("bk_set_id"),
					moduleID,
					cmd.Flags().Changed("bk_module_id"),
				)
				if err != nil {
					return err
				}

				result, err := syslib.ExecuteRequest(runtime, syslib.RequestSpec{
					GatewayName: gatewayName,
					Method:      "POST",
					Path:        fmt.Sprintf("/api/v3/open/hosts/app/%d/list_hosts", bizID),
					BodyJSON:    bodyJSON,
					Headers:     headers,
					Stage:       stage,
					AuthConfig: &syslib.AuthConfig{
						AppVerifiedRequired:        true,
						UserVerifiedRequired:       true,
						ResourcePermissionRequired: true,
					},
				})
				if err != nil {
					return err
				}
				if err := systemcmd.EnsureEnvelope("list_biz_hosts_all", result.Envelope); err != nil {
					return err
				}

				count, hosts, err := parsePagedInfoResponse("list_biz_hosts_all", result.Envelope.Data)
				if err != nil {
					return err
				}
				lastEnvelope = result.Envelope
				lastReportedCount = count

				if len(hosts) == 0 {
					completed = true
					break
				}

				var addedThisPage int
				allHosts, addedThisPage, err = appendUniqueHosts(
					"list_biz_hosts_all",
					allHosts,
					seenHostIDs,
					hosts,
				)
				if err != nil {
					return err
				}
				if addedThisPage == 0 {
					return output.SystemError(
						"pagination_inconsistent",
						fmt.Sprintf(
							"list_biz_hosts_all received a page with no new hosts at start %d; "+
								"pagination may be unstable while data is changing",
							start,
						),
						"",
					)
				}
				if len(hosts) < pageLimit {
					completed = true
					break
				}

				start += pageLimit
			}

			if !completed {
				return output.SystemError(
					"pagination_limit",
					fmt.Sprintf(
						"list_biz_hosts_all exceeded maximum page iterations (%d); collected %d unique hosts; last reported count was %d",
						maxPages,
						len(allHosts),
						lastReportedCount,
					),
					"",
				)
			}

			if lastEnvelope == nil {
				return output.SystemError(
					"empty_response",
					"list_biz_hosts_all received an empty response envelope",
					"",
				)
			}

			env := output.APIResponse(lastEnvelope.Status, lastEnvelope.Headers, map[string]any{
				"count": len(allHosts),
				"info":  allHosts,
			})
			return env.WriteJSON(cmd.OutOrStdout())
		},
	}

	cmd.Flags().IntVar(&bizID, "bk_biz_id", 0, "Business ID")
	cmd.Flags().IntVar(&setID, "bk_set_id", 0, "Optional set filter")
	cmd.Flags().IntVar(&moduleID, "bk_module_id", 0, "Optional module filter")
	cmd.Flags().IntVar(&pageLimit, "page_limit", defaultLimit, "Page size for each CMDB request")
	systemcmd.AddCommonRequestFlagsWithoutBody(cmd, &stage, &headers)

	return cmd
}

func buildListBizHostsAllBody(
	start, pageLimit int,
	setID int,
	setIDProvided bool,
	moduleID int,
	moduleIDProvided bool,
) (string, error) {
	if err := systemcmd.ValidateNonNegativeIntFlag("start", start); err != nil {
		return "", err
	}
	if err := systemcmd.ValidatePositiveIntFlag("page_limit", pageLimit); err != nil {
		return "", err
	}
	if err := systemcmd.ValidatePositiveIntFlagIfChanged("bk_set_id", setID, setIDProvided); err != nil {
		return "", err
	}
	if err := systemcmd.ValidatePositiveIntFlagIfChanged("bk_module_id", moduleID, moduleIDProvided); err != nil {
		return "", err
	}

	payload := map[string]any{
		"fields": []string{
			"bk_host_id",
			"bk_host_innerip",
			"bk_cloud_id",
			"bk_host_name",
			"operator",
			"bk_bak_operator",
		},
		"page": map[string]int{
			"start": start,
			"limit": pageLimit,
		},
	}
	if setIDProvided {
		payload["set_cond"] = []map[string]any{
			{
				"field":    "bk_set_id",
				"operator": "$eq",
				"value":    setID,
			},
		}
	}
	if moduleIDProvided {
		payload["module_cond"] = []map[string]any{
			{
				"field":    "bk_module_id",
				"operator": "$eq",
				"value":    moduleID,
			},
		}
	}

	return systemcmd.MarshalJSON(payload)
}

func parsePagedInfoResponse(actionName string, data any) (int, []any, error) {
	payload, ok := data.(map[string]any)
	if !ok {
		return 0, nil, output.SystemError(
			"response_error",
			actionName+" received an unexpected response shape",
			"",
		)
	}

	count, ok := payload["count"].(float64)
	if !ok {
		return 0, nil, output.SystemError(
			"response_error",
			actionName+" response is missing numeric count",
			"",
		)
	}

	info, ok := payload["info"].([]any)
	if !ok {
		return 0, nil, output.SystemError(
			"response_error",
			actionName+" response is missing info list",
			"",
		)
	}

	return int(count), info, nil
}

func appendUniqueHosts(
	actionName string,
	allHosts []any,
	seenHostIDs map[int64]struct{},
	hosts []any,
) ([]any, int, error) {
	addedThisPage := 0
	for _, host := range hosts {
		hostID, err := parseHostID(actionName, host)
		if err != nil {
			return nil, 0, err
		}
		if _, ok := seenHostIDs[hostID]; ok {
			continue
		}

		seenHostIDs[hostID] = struct{}{}
		allHosts = append(allHosts, host)
		addedThisPage++
	}

	return allHosts, addedThisPage, nil
}

func parseHostID(actionName string, host any) (int64, error) {
	hostPayload, ok := host.(map[string]any)
	if !ok {
		return 0, output.SystemError(
			"response_error",
			actionName+" response contains a host entry with unexpected shape",
			"",
		)
	}

	rawHostID, ok := hostPayload["bk_host_id"]
	if !ok {
		return 0, output.SystemError(
			"response_error",
			actionName+" response contains a host entry without bk_host_id",
			"",
		)
	}

	switch hostID := rawHostID.(type) {
	case int:
		return int64(hostID), nil
	case int32:
		return int64(hostID), nil
	case int64:
		return hostID, nil
	case float32:
		asInt := int64(hostID)
		if hostID != float32(asInt) {
			break
		}
		return asInt, nil
	case float64:
		asInt := int64(hostID)
		if hostID != float64(asInt) {
			break
		}
		return asInt, nil
	}

	return 0, output.SystemError(
		"response_error",
		actionName+" response contains a host entry with non-numeric bk_host_id",
		"",
	)
}
