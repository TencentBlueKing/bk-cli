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

package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	maxArchiveBytes    int64 = 256 << 20
	maxExecutableBytes int64 = 256 << 20
	maxArchiveEntries        = 4096
)

// httpsOnlyRedirectPolicy rejects redirect chains that leave HTTPS.
func httpsOnlyRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != "https" {
		return errors.New("redirect to non-https url rejected")
	}
	return nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       5 * time.Minute,
		CheckRedirect: httpsOnlyRedirectPolicy,
	}
}

// downloadToFile downloads url into dst without sending any BlueKing credentials.
func downloadToFile(ctx context.Context, client *http.Client, url, dst string, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return SystemError(CodeDownloadFailed, fmt.Sprintf("invalid download url: %v", err), "")
	}
	if req.URL.Scheme != "https" {
		return SystemError(CodeDownloadFailed, "download url must use https", "")
	}
	resp, err := client.Do(req)
	if err != nil {
		return SystemError(
			CodeDownloadFailed,
			fmt.Sprintf("download failed: %v", err),
			"Check network access, or use --from-file",
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return SystemError(
			CodeDownloadFailed,
			fmt.Sprintf("download returned HTTP %d", resp.StatusCode),
			"Check network access, or use --from-file",
		)
	}
	// #nosec G304 -- dst is inside the plugin staging directory.
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	n, copyErr := io.Copy(f, io.LimitReader(resp.Body, limit+1))
	closeErr := f.Close()
	if copyErr != nil {
		return SystemError(CodeDownloadFailed, fmt.Sprintf("download interrupted: %v", copyErr), "")
	}
	if closeErr != nil {
		return SystemError(CodeIOError, closeErr.Error(), "")
	}
	if n > limit {
		return SystemError(CodeDownloadFailed, "download exceeds size limit", "")
	}
	return nil
}
