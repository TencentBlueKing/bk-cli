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
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

const (
	gatewayName = "bk-aidev"
	uploadPath  = "/openapi/aidev/private/v1/upload/"
)

func newUploadFileCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		stage   string
		file    string
		module  string
		spaceID string
		headers []string
	)

	cmd := &cobra.Command{
		Use:   "upload_file",
		Short: "上传 Skill 或知识文件制品，取得供后续 upsert 或导入使用的文件引用。适用于普通文件上传；返回上传成功不等于资源已同步。",
		Args:  cobra.NoArgs,
		Long: `上传 Skill 或知识文件制品，取得供后续 upsert 或导入使用的文件引用。适用于普通文件上传；返回上传成功不等于资源已同步。

该接口使用 multipart/form-data。必须传入 --file 和 --space_id，--module 默认 skill。

文件必须存在且为可读的常规文件。上传时流式读取文件；--dry-run 校验文件并显示 size（字节），不读取文件内容。`,
		Example: "  bk-cli aidev upload_file --file ./artifact.zip --space_id demo-space\n" +
			"  bk-cli aidev upload_file --file ./knowledge.zip --space_id demo-space --module knowledge",
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
	fh, err := os.Open(filePath)
	if err != nil {
		return output.UserError("request_error", err.Error(), "Check --file path")
	}
	defer func() { _ = fh.Close() }()
	info, err := fh.Stat()
	if err != nil {
		return output.UserError("request_error", err.Error(), "Check --file path")
	}
	if !info.Mode().IsRegular() {
		return output.UserError("request_error", "--file must be a regular file", "Check --file path")
	}

	var body io.Reader = bytes.NewReader(nil)
	contentType := "multipart/form-data; boundary={...}"
	var contentLength int64
	if runtime == nil || !runtime.DryRun {
		body, contentType, contentLength, err = buildUploadBody(fh, info.Size(), module, spaceID)
		if err != nil {
			return err
		}
	}

	return systemcmd.ExecuteRequest(cmd, runtime, "upload_file", syslib.RequestSpec{
		GatewayName:   gatewayName,
		Method:        "POST",
		Path:          uploadPath,
		BodyReader:    body,
		ContentType:   contentType,
		ContentLength: contentLength,
		DryRunBody: map[string]any{
			"file":     filepath.Base(filePath),
			"size":     info.Size(),
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

// buildUploadBody buffers only multipart metadata; the caller owns the file
// and keeps it open until shared request execution returns.
func buildUploadBody(fh *os.File, fileSize int64, module, spaceID string) (io.Reader, string, int64, error) {
	metadata := &bytes.Buffer{}
	writer := multipart.NewWriter(metadata)

	if _, err := writer.CreateFormFile("file", filepath.Base(fh.Name())); err != nil {
		return nil, "", 0, output.SystemError("request_build_failed", err.Error(), "")
	}
	fileHeaderSize := metadata.Len()
	if err := writer.WriteField("module", module); err != nil {
		return nil, "", 0, output.SystemError("request_build_failed", err.Error(), "")
	}
	if err := writer.WriteField("space_id", spaceID); err != nil {
		return nil, "", 0, output.SystemError("request_build_failed", err.Error(), "")
	}
	if err := writer.Close(); err != nil {
		return nil, "", 0, output.SystemError("request_build_failed", err.Error(), "")
	}

	// Insert the file stream between its part header and the remaining fields.
	data := metadata.Bytes()
	body := io.MultiReader(bytes.NewReader(data[:fileHeaderSize]), fh, bytes.NewReader(data[fileHeaderSize:]))
	return body, writer.FormDataContentType(), int64(len(data)) + fileSize, nil
}
