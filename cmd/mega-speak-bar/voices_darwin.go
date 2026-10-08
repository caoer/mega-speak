//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/caoer/mega-speak/internal/mac"
	"github.com/caoer/mega-speak/internal/voices"
)

type voiceRequest struct {
	ID       string `json:"id"`
	Action   string `json:"action"`
	Voice    string `json:"voice"`
	Favorite bool   `json:"favorite"`
}
type voiceState struct {
	Current   string   `json:"current_voice"`
	Favorites []string `json:"favorites"`
	Error     string   `json:"error,omitempty"`
}
type voiceReply struct {
	ID    string      `json:"id"`
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	State *voiceState `json:"state,omitempty"`
}
type voicePicker struct {
	window        *mac.VoiceWindow
	cli, settings string
	library       voices.Library
	allowed       map[string]bool
	requests      chan voiceRequest
}

func newVoicePicker(cli, settings string) (*voicePicker, error) {
	library, err := voices.Load()
	if err != nil {
		return nil, err
	}
	p := &voicePicker{cli: cli, settings: settings, library: library, allowed: map[string]bool{}, requests: make(chan voiceRequest, 32)}
	for _, voice := range library.Voices {
		p.allowed[voice.ID] = true
	}
	p.window = mac.NewVoiceWindow()
	p.window.HTML("<html><body style='font:16px -apple-system;padding:40px'><h2>正在载入音色库…</h2><p>准备本地图标和试听文件。</p></body></html>")
	p.window.OnMessage = p.message
	go func() {
		directory, err := library.Prepare(filepath.Join(settings, "library"))
		if err != nil {
			log.Printf("voice library: %v", err)
			p.window.HTML("<html><body style='font:16px -apple-system;padding:40px'><h2>音色库无法载入</h2><p>" + html.EscapeString(err.Error()) + "</p><p>请检查本地目录权限后重新启动 Mega Speak。</p></body></html>")
			return
		}
		p.window.Load(filepath.Join(directory, "picker.html"), directory)
	}()
	go p.loop()
	return p, nil
}
func (p *voicePicker) message(raw string) {
	if len(raw) > 4096 {
		return
	}
	var request voiceRequest
	if json.Unmarshal([]byte(raw), &request) != nil || request.ID == "" || len(request.ID) > 100 {
		return
	}
	select {
	case p.requests <- request:
	default:
		p.reply(voiceReply{ID: request.ID, Error: "操作排队中，请稍后重试。"})
	}
}
func (p *voicePicker) reply(r voiceReply) {
	data, _ := json.Marshal(r)
	p.window.Eval("window.receiveReply(" + string(data) + ")")
}
func (p *voicePicker) readVoice(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.cli, args...)
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("无法读写音色配置，请检查 mega-speak 命令或配置文件：%w", err)
	}
	var state struct {
		Voice      string `json:"voice"`
		Overridden bool   `json:"overridden"`
	}
	if err = json.Unmarshal(b, &state); err != nil {
		return "", fmt.Errorf("mega-speak 返回了无法识别的音色配置：%w", err)
	}
	if state.Overridden {
		return state.Voice, fmt.Errorf("偏好已保存，但 MEGA_SPEAK_CONFIG 覆盖了选择；请移除该配置里的 voice 设置后重试。")
	}
	return state.Voice, nil
}
func (p *voicePicker) loop() {
	for request := range p.requests {
		r := voiceReply{ID: request.ID, OK: true, State: &voiceState{}}
		favorites, err := voices.ReadFavorites(filepath.Join(p.settings, "favorites.json"))
		if err != nil {
			r.OK = false
			r.Error = "无法读取收藏文件：" + err.Error()
			r.State.Error = r.Error
			p.reply(r)
			continue
		}
		if request.Action != "state" && !p.allowed[request.Voice] {
			err = fmt.Errorf("音色不在本地目录中。")
		} else {
			switch request.Action {
			case "state":
			case "choose":
				r.State.Current, err = p.readVoice("--set-voice", request.Voice)
			case "favorite":
				err = favorites.Set(request.Voice, request.Favorite)
			case "copy":
				if !mac.CopyText(request.Voice) {
					err = fmt.Errorf("无法写入剪贴板，请手动复制音色 ID。")
				}
			default:
				err = fmt.Errorf("未知操作。")
			}
		}
		if err != nil {
			r.OK = false
			r.Error = err.Error()
		}
		if r.State.Current == "" {
			current, readErr := p.readVoice("--voice-state")
			r.State.Current = current
			if readErr != nil {
				r.State.Error = readErr.Error()
				if r.Error == "" {
					r.Error = r.State.Error
				}
				r.OK = false
			}
		}
		r.State.Favorites = favorites.List()
		p.reply(r)
	}
}
