// REPL 公共函数
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/vanadiry/serein/core/store"
)

var cachedCfg *store.Config

func loadCfg(home string) (store.Config, error) {
	if cachedCfg != nil {
		return *cachedCfg, nil
	}
	cfg, err := store.LoadConfig(home)
	if err != nil {
		return cfg, err
	}
	cachedCfg = &cfg
	return cfg, nil
}

func apiGetRaw(home, path string) ([]byte, error) {
	resp, err := http.Get(apiAddr(home) + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func printJSON(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var pretty any
	if err := json.Unmarshal(body, &pretty); err != nil {
		fmt.Println(string(body))
		return nil
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	fmt.Println(string(out))
	return nil
}
