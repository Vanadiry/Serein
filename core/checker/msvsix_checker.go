package checker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const vsixAPI = "https://marketplace.visualstudio.com/_apis/public/gallery/extensionquery"

type vsixBody struct {
	Filters []vsixFilter `json:"filters"`
	Flags   int          `json:"flags"`
}

type vsixFilter struct {
	Criteria   []vsixCriteria `json:"criteria"`
	PageNumber int            `json:"pageNumber"`
	PageSize   int            `json:"pageSize"`
}

type vsixCriteria struct {
	FilterType int    `json:"filterType"`
	Value      string `json:"value"`
}

type vsixResp struct {
	Results []struct {
		Extensions []struct {
			Versions []struct {
				Version string `json:"version"`
			} `json:"versions"`
		} `json:"extensions"`
	} `json:"results"`
}

func CheckMSVSIX(extID string, client *http.Client) (PlatformResult, error) {
	parts := strings.SplitN(extID, ".", 2)
	if len(parts) != 2 {
		return PlatformResult{}, fmt.Errorf("msvsix: 无效的扩展 ID %q，应为 publisher.extension 格式", extID)
	}

	body := vsixBody{
		Filters: []vsixFilter{{
			Criteria:   []vsixCriteria{{FilterType: 7, Value: extID}},
			PageNumber: 1,
			PageSize:   1,
		}},
		Flags: 914,
	}
	bodyJSON, _ := json.Marshal(body)

	headers := map[string]string{
		"Content-Type":       "application/json",
		"Accept":             "application/json;api-version=3.0-preview.1",
		"X-Market-Client-Id": "VSCode",
	}
	data, err := doPostRequest(client, vsixAPI, "VSCode", headers, bodyJSON)
	if err != nil {
		return PlatformResult{}, fmt.Errorf("msvsix: %w", err)
	}

	var resp vsixResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return PlatformResult{}, fmt.Errorf("msvsix: %w", err)
	}
	if len(resp.Results) == 0 || len(resp.Results[0].Extensions) == 0 || len(resp.Results[0].Extensions[0].Versions) == 0 {
		return PlatformResult{}, fmt.Errorf("msvsix: 未找到扩展 %s", extID)
	}

	ver := resp.Results[0].Extensions[0].Versions[0].Version
	ver = stripVersionAffixes(ver)
	dl := fmt.Sprintf("https://marketplace.visualstudio.com/_apis/public/gallery/publishers/%s/vsextensions/%s/%s/vspackage", parts[0], parts[1], ver)

	return PlatformResult{
		LatestVersion: ver,
		URL:           dl,
	}, nil
}
