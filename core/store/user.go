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

	"github.com/vanadiry/serein/core/events"
	"github.com/vanadiry/serein/core/log"
)

// UserData 用户追踪数据，按应用、平台、版本号三层组织
type UserData map[string]map[string]string

const userDataFile = "software.json"

// LoadUserData 读取“已确认安装到哪个版本”
// 无论是否出错都返回一个可用的非 nil map。这份数据一旦被整份丢弃，所有应用会同时
// 显示成“有更新”，用户以为全部过期，实际上只是文件读不出来
// 整份无法解析时把原文件改名留档以便手动找回；个别键坏了则跳过那些键、保留其余
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

	info, _ := f.Stat()
	if info.Size() == 0 {
		f.Close()
		return ud, nil // 空文件视为"还没确认过任何版本"
	}

	var top map[string]json.RawMessage
	decodeErr := json.NewDecoder(f).Decode(&top)
	// Windows 不允许 rename 打开的文件，句柄不关留档必然失败
	f.Close()
	if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		err := decodeErr
		backup, qerr := quarantineUserData(path)
		if qerr != nil {
			reportBroken(fmt.Sprintf("已确认版本数据无法解析，且无法留档：%v", err))
			return ud, fmt.Errorf("software.json 不合法，且无法留档：%w；留档失败：%v", err, qerr)
		}
		// 原文件已移走，下次读不会再走到这里，所以这条提示天然只报一次
		reportBroken(fmt.Sprintf("已确认版本数据无法解析，已留档为 %s，原文件可从该路径取回", backup))
		return ud, fmt.Errorf("software.json 不合法，已留档为 %s：%w", backup, err)
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
		log.LogfWarn("[user] software.json 有 %d 个条目格式不对，已忽略：%s",
			len(bad), strings.Join(bad, ", "))
		return ud, fmt.Errorf("software.json 有 %d 个条目格式不对，已忽略：%s",
			len(bad), strings.Join(bad, ", "))
	}
	return ud, nil
}

// reportBroken 报告已确认版本数据整体不可用
// 走事件总线而非某个接口的响应，并非任何一次用户操作造成的，是后端自己的数据文件坏了
// 走响应的话只有恰好读到它的那个请求能通知到，而且各接口都要各自携带
// 用常驻事件是因为这份数据一坏，所有应用会同时显示成“有更新”，用户不看到就会以为全部软件过期了
func reportBroken(msg string) {
	log.LogfError("[user] %s", msg)
	events.EmitSticky("error", "[user]", msg)
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

// SaveUserData 写入已确认版本号。数据在调用前已在内存里拼好，直接覆盖写
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
