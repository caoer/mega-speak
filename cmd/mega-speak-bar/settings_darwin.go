//go:build darwin

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/caoer/mega-speak/internal/mac"
	"os/exec"
	"strings"
	"time"
)

//go:embed settings.html
var settingsHTML string

func newSettingsWindow(cli string) *mac.VoiceWindow {
	w := mac.NewWebWindow("Mega Speak · 设置", 720, 700)
	requests := make(chan struct {
		ID       string          `json:"id"`
		Action   string          `json:"action"`
		Settings json.RawMessage `json:"settings"`
		Key      string          `json:"key"`
	}, 16)
	reply := func(id string, state json.RawMessage, err error) {
		data := map[string]any{"id": id, "ok": err == nil, "state": state}
		if err != nil {
			data["error"] = err.Error()
		}
		b, _ := json.Marshal(data)
		w.Eval("window.receiveSettingsReply(" + string(b) + ")")
	}
	w.OnMessage = func(raw string) {
		if len(raw) > 8192 {
			return
		}
		var r struct {
			ID       string          `json:"id"`
			Action   string          `json:"action"`
			Settings json.RawMessage `json:"settings"`
			Key      string          `json:"key"`
		}
		if json.Unmarshal([]byte(raw), &r) != nil || r.ID == "" || len(r.ID) > 100 {
			return
		}
		select {
		case requests <- r:
		default:
			reply(r.ID, nil, fmt.Errorf("操作排队中，请稍后重试。"))
		}
	}
	go func() {
		for r := range requests {
			args := []string{"--settings-state"}
			stdin := ""
			switch r.Action {
			case "state":
			case "save":
				args = []string{"--set-settings", string(r.Settings)}
			case "key-state":
				args = []string{"--key-state"}
			case "set-key":
				// the key travels on stdin, never in argv where ps shows it
				args, stdin = []string{"--set-key"}, r.Key
			default:
				reply(r.ID, nil, fmt.Errorf("未知设置操作。"))
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			cmd := exec.CommandContext(ctx, cli, args...)
			cmd.Stdin = strings.NewReader(stdin)
			b, err := cmd.Output()
			cancel()
			if err != nil {
				if e, ok := err.(*exec.ExitError); ok {
					err = fmt.Errorf("无法保存或读取设置：%s", strings.TrimSpace(string(e.Stderr)))
				} else {
					err = fmt.Errorf("无法运行 mega-speak：%w", err)
				}
				reply(r.ID, nil, err)
				continue
			}
			var state struct {
				Overridden []string `json:"overridden"`
			}
			if err = json.Unmarshal(b, &state); err == nil && len(state.Overridden) > 0 {
				err = fmt.Errorf("偏好已保存，但 MEGA_SPEAK_CONFIG 覆盖了 %s，请调整该配置后重试。", strings.Join(state.Overridden, ", "))
			}
			reply(r.ID, json.RawMessage(b), err)
		}
	}()
	w.HTML(settingsHTML)
	return w
}
