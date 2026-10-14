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

package aidev

import (
	"bytes"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	"github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

const (
	gatewayName = "bk-aidev"
	uploadPath  = "/openapi/aidev/private/v1/upload/"
)

func newCreatePrivateUploadCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		stage   string
		file    string
		module  string
		spaceID string
		headers []string
	)

	cmd := &cobra.Command{
		Use:   "create_private_upload",
		Short: "统一上传 skill / knowledge 制品，返回路径、元数据和 expires_at；默认 48 小时过期，由公共任务清理",
		Args:  cobra.NoArgs,
		Long: `统一上传 skill / knowledge 制品，返回路径、元数据和 expires_at；默认 48 小时过期，由公共任务清理。

该接口使用 multipart/form-data。必须传入 --file 和 --space_id，--module 默认 skill。`,
		Example: "  bk-cli aidev create_private_upload --file ./artifact.zip --space_id demo-space\n" +
			"  bk-cli aidev create_private_upload --file ./knowledge.zip --space_id demo-space --module knowledge",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := systemcmd.ValidateNonEmptyStringFlag("file", file); err != nil {
				return err
			}
			if err := systemcmd.ValidateNonEmptyStringFlag("space_id", spaceID); err != nil {
				return err
			}
			if err := systemcmd.ValidateNonEmptyStringFlag("module", module); err != nil {
				return err
			}

			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			return executeUpload(cmd, runtime, file, module, spaceID, stage, headers)
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "[Required] Local file path to upload")
	cmd.Flags().StringVar(&spaceID, "space_id", "", "[Required] Space ID")
	cmd.Flags().StringVar(&module, "module", "skill", "[Optional] Module type, commonly skill or knowledge")
	systemcmd.AddCommonRequestFlagsWithoutBody(cmd, &stage, &headers)

	return cmd
}

func executeUpload(
	cmd *cobra.Command,
	runtime *syslib.Runtime,
	filePath string,
	module string,
	spaceID string,
	stage string,
	headers []string,
) error {
	body, contentType, err := uploadBodyForRuntime(runtime, filePath, module, spaceID)
	if err != nil {
		return err
	}

	return systemcmd.ExecuteRequest(cmd, runtime, "create_private_upload", syslib.RequestSpec{
		GatewayName: gatewayName,
		Method:      "POST",
		Path:        uploadPath,
		BodyReader:  body,
		ContentType: contentType,
		DryRunBody: map[string]any{
			"file":     filepath.Base(filePath),
			"module":   module,
			"space_id": spaceID,
		},
		Headers: headers,
		Stage:   stage,
		AuthConfig: &syslib.AuthConfig{
			AppVerifiedRequired:        true,
			UserVerifiedRequired:       true,
			ResourcePermissionRequired: true,
		},
	}, nil)
}

func uploadBodyForRuntime(runtime *syslib.Runtime, filePath, module, spaceID string) (*bytes.Buffer, string, error) {
	if runtime != nil && runtime.DryRun {
		return bytes.NewBuffer(nil), "multipart/form-data; boundary={...}", nil
	}

	return buildUploadBody(filePath, module, spaceID)
}

func buildUploadBody(filePath, module, spaceID string) (*bytes.Buffer, string, error) {
	fh, err := os.Open(filePath)
	if err != nil {
		return nil, "", output.UserError("request_error", err.Error(), "Check --file path")
	}
	defer func() { _ = fh.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, "", output.SystemError("request_build_failed", err.Error(), "")
	}
	if _, err := io.Copy(part, fh); err != nil {
		return nil, "", output.UserError("request_error", err.Error(), "Check --file path")
	}
	if err := writer.WriteField("module", module); err != nil {
		return nil, "", output.SystemError("request_build_failed", err.Error(), "")
	}
	if err := writer.WriteField("space_id", spaceID); err != nil {
		return nil, "", output.SystemError("request_build_failed", err.Error(), "")
	}
	if err := writer.Close(); err != nil {
		return nil, "", output.SystemError("request_build_failed", err.Error(), "")
	}

	return body, writer.FormDataContentType(), nil
}
