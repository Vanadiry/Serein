package checker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type openvsxResp struct {
	Version string `json:"version"`
	Files   struct {
		Download string `json:"download"`
	} `json:"files"`
}

func CheckOpenVSX(extID string, client *http.Client) (PlatformResult, error) {
	parts := strings.SplitN(extID, ".", 2)
	if len(parts) != 2 {
		return PlatformResult{}, fmt.Errorf("openvsx: 无效的扩展 ID %q，应为 publisher.extension 格式", extID)
	}

	apiURL := fmt.Sprintf("https://open-vsx.org/api/%s/%s/latest", parts[0], parts[1])
	data, err := doRequest(client, apiURL, "", nil)
	if err != nil {
		return PlatformResult{}, fmt.Errorf("openvsx: %w", err)
	}

	var resp openvsxResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return PlatformResult{}, fmt.Errorf("openvsx: %w", err)
	}
	if resp.Version == "" {
		return PlatformResult{}, fmt.Errorf("openvsx: 未找到扩展 %s", extID)
	}

	ver := stripVersionAffixes(resp.Version)

	return PlatformResult{
		LatestVersion: ver,
		URL:           resp.Files.Download,
	}, nil
}
