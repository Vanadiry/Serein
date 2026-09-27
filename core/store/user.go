package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vanadiry/serein/core/log"
)

// UserData 用户追踪数据：app_id → 平台 → 版本号
type UserData map[string]map[string]string

const userDataFile = "software.json"

// LoadUserData 读取「已确认安装到哪个版本」。
//
// 无论是否出错都返回一个可用的非 nil map。这份数据一旦被整份丢弃，所有应用会同时
// 显示成「有更新」——用户以为全部过期，实际上只是文件读不出来。宁可显示旧版本号，
// 也不能凭空造出几百个假更新。
//
// 无法解析时把原文件改名留档（而不是删掉或就地覆写），好让人手动找回；只是个别键
// 坏了则跳过那些键、保留其余，不让一个手滑的条目清空全部记录。
func LoadUserData(home string) (UserData, error) {
	path := filepath.Join(home, "user", userDataFile)
	ud := make(UserData)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ud, nil
		}
		return ud, fmt.Errorf("read user data: %w", err)
	}
	defer f.Close()

	info, _ := f.Stat()
	if info.Size() == 0 {
		return ud, nil // 空文件 → 视为「还没确认过任何版本」
	}

	var top map[string]json.RawMessage
	if err := json.NewDecoder(f).Decode(&top); err != nil && !errors.Is(err, io.EOF) {
		backup, qerr := quarantineUserData(path)
		if qerr != nil {
			return ud, fmt.Errorf("decode user data: %w；留档失败: %v", err, qerr)
		}
		log.LogfError("[user] software.json 解析失败，已留档为 %s，原有已确认版本不再生效", backup)
		return ud, fmt.Errorf("decode user data: %w（已留档为 %s）", err, backup)
	}

	var bad []string
	for appID, rawPlatforms := range top {
		var platforms map[string]json.RawMessage
		if err := json.Unmarshal(rawPlatforms, &platforms); err != nil {
			bad = append(bad, appID)
			continue
		}
		versions := make(map[string]string, len(platforms))
		for osName, rawVersion := range platforms {
			var v string
			if err := json.Unmarshal(rawVersion, &v); err != nil {
				bad = append(bad, appID+"/"+osName)
				continue
			}
			versions[osName] = v
		}
		if len(versions) > 0 {
			ud[appID] = versions
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return ud, fmt.Errorf("software.json 有 %d 个条目格式不对，已跳过：%s",
			len(bad), strings.Join(bad, ", "))
	}
	return ud, nil
}

// quarantineUserData 把解析不了的 software.json 改名留档，返回留档后的文件名
func quarantineUserData(path string) (string, error) {
	base := time.Now().Format("20060102-150405")
	for i := 0; i <= 100; i++ {
		suffix := base
		if i > 0 {
			suffix = fmt.Sprintf("%s-%d", base, i)
		}
		dst := path + ".corrupt-" + suffix
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			if err := os.Rename(path, dst); err != nil {
				return "", err
			}
			return filepath.Base(dst), nil
		}
	}
	return "", fmt.Errorf("留档文件名连续冲突")
}

// SaveUserData 写入已确认版本号。数据在调用前已在内存里拼好，直接覆盖写。
func SaveUserData(home string, ud UserData) error {
	path := filepath.Join(home, "user", userDataFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(ud); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
