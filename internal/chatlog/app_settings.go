package chatlog

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rivo/tview"
	"github.com/sjzar/chatlog/internal/chatlog/conf"
	"github.com/sjzar/chatlog/internal/ui/form"
	"github.com/sjzar/chatlog/internal/ui/menu"
)

type settingsKey string

const (
	settingKeySpeechProvider  settingsKey = "speech_provider"
	settingKeyLocalServiceURL settingsKey = "local_service_url"
	settingKeyHTTPAddr        settingsKey = "http_addr"
	settingKeyToggleListen    settingsKey = "toggle_listen"
	settingKeyWorkDir         settingsKey = "work_dir"
	settingKeyDataDir         settingsKey = "data_dir"
	settingKeyDataKey         settingsKey = "data_key"
	settingKeyImgKey          settingsKey = "img_key"
	settingKeyOpenAIAPIKey    settingsKey = "openai_api_key"
	settingKeyOpenAIBaseURL   settingsKey = "openai_base_url"
	settingKeyOpenAIProxy     settingsKey = "openai_proxy"
	settingKeyOpenAITimeout   settingsKey = "openai_timeout"
	settingKeyWhisperModel    settingsKey = "whisper_model"
	settingKeyWhisperThreads  settingsKey = "whisper_threads"
)

func (a *App) initSettingsTab() {
	a.settingsMenu = menu.NewSubMenu("设置")
	if a.settingsMenu == nil {
		return
	}
	a.settingsMenu.SetCancelFunc(nil)

	a.settingsItems = []*menu.Item{
		a.newSettingsItem(1, "设置语音服务提供商", settingKeySpeechProvider, a.settingSpeechProvider),
		a.newSettingsItem(2, "设置 Whisper.cpp 模型路径", settingKeyWhisperModel, a.settingWhisperModelPath),
		a.newSettingsItem(3, "设置 Whisper.cpp 线程数", settingKeyWhisperThreads, a.settingWhisperThreads),
		a.newSettingsItem(4, "设置本地语音服务地址", settingKeyLocalServiceURL, a.settingLocalServiceURL),
		a.newSettingsItem(5, "设置 HTTP 服务地址", settingKeyHTTPAddr, a.settingHTTPPort),
		a.newSettingsItem(6, "切换局域网监听", settingKeyToggleListen, a.toggleListen),
		a.newSettingsItem(7, "设置工作目录", settingKeyWorkDir, a.settingWorkDir),
		a.newSettingsItem(8, "设置数据目录", settingKeyDataDir, a.settingDataDir),
		a.newSettingsItem(9, "设置数据密钥", settingKeyDataKey, a.settingDataKey),
		a.newSettingsItem(10, "设置图片密钥", settingKeyImgKey, a.settingImgKey),
		a.newSettingsItem(11, "设置 OpenAI API Key", settingKeyOpenAIAPIKey, a.settingOpenAIAPIKey),
		a.newSettingsItem(12, "设置 OpenAI Base URL", settingKeyOpenAIBaseURL, a.settingOpenAIBaseURL),
		a.newSettingsItem(13, "设置 OpenAI 代理", settingKeyOpenAIProxy, a.settingOpenAIProxy),
		a.newSettingsItem(14, "设置 OpenAI 请求超时", settingKeyOpenAITimeout, a.settingOpenAITimeout),
	}

	a.settingsMenu.SetItems(a.settingsItems)
	a.refreshSettingsMenu()
}

func (a *App) newSettingsItem(index int, name string, key settingsKey, action func()) *menu.Item {
	item := &menu.Item{
		Index: index,
		Name:  name,
		Selected: func(*menu.Item) {
			if action != nil {
				action()
			}
		},
	}
	if a.settingsItemMap == nil {
		a.settingsItemMap = make(map[settingsKey]*menu.Item)
	}
	a.settingsItemMap[key] = item
	return item
}

func (a *App) refreshSettingsMenu() {
	if a.settingsMenu == nil || len(a.settingsItems) == 0 {
		return
	}

	speechCfg := a.ctx.GetSpeech()

	providerLabel := "OpenAI 官方服务"
	isWebService := false
	if speechCfg != nil {
		switch strings.ToLower(strings.TrimSpace(speechCfg.Provider)) {
		case "webservice", "local", "docker", "http", "whisper-asr":
			providerLabel = "本地 Docker Whisper"
			isWebService = true
		case "openai", "":
			providerLabel = "OpenAI 官方服务"
		case "whispercpp":
			providerLabel = "Whisper.cpp 本地模型"
		default:
			providerLabel = speechCfg.Provider
		}
	}

	if item := a.settingsItemMap[settingKeySpeechProvider]; item != nil {
		item.Description = fmt.Sprintf("当前提供商: %s", providerLabel)
	}

	if item := a.settingsItemMap[settingKeyWhisperModel]; item != nil {
		current := "未设置"
		if speechCfg != nil {
			trimmed := strings.TrimSpace(speechCfg.Model)
			if trimmed != "" {
				current = trimmed
			}
			if strings.ToLower(strings.TrimSpace(speechCfg.Provider)) != "whispercpp" {
				current = current + " (当前提供商未启用)"
			}
		}
		item.Description = fmt.Sprintf("当前模型路径: %s", current)
	}

	if item := a.settingsItemMap[settingKeyWhisperThreads]; item != nil {
		threadsLabel := "默认"
		if speechCfg != nil && speechCfg.Threads > 0 {
			threadsLabel = strconv.Itoa(speechCfg.Threads)
		}
		if speechCfg != nil && strings.ToLower(strings.TrimSpace(speechCfg.Provider)) != "whispercpp" {
			threadsLabel = threadsLabel + " (当前提供商未启用)"
		}
		item.Description = fmt.Sprintf("当前线程数: %s", threadsLabel)
	}

	if item := a.settingsItemMap[settingKeyLocalServiceURL]; item != nil {
		fallback := "http://127.0.0.1:9000"
		current := fallback
		if speechCfg != nil {
			current = formatPathWithFallback(speechCfg.ServiceURL, fallback)
		}
		suffix := ""
		if speechCfg != nil && !isWebService {
			suffix = " (备用)"
		}
		item.Description = fmt.Sprintf("当前服务地址: %s%s", current, suffix)
	}

	if item := a.settingsItemMap[settingKeyHTTPAddr]; item != nil {
		current := formatPathWithFallback(a.ctx.GetHTTPAddr(), "127.0.0.1:5030")
		item.Description = fmt.Sprintf("当前监听地址: %s", current)
	}

	if item := a.settingsItemMap[settingKeyToggleListen]; item != nil {
		host := a.ctx.GetHTTPAddr()
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if strings.TrimSpace(host) == "" {
			host = "127.0.0.1"
		}
		item.Description = fmt.Sprintf("当前监听主机: %s", host)
	}

	if item := a.settingsItemMap[settingKeyWorkDir]; item != nil {
		item.Description = fmt.Sprintf("当前工作目录: %s", formatPathWithFallback(a.ctx.WorkDir, "未设置"))
	}

	if item := a.settingsItemMap[settingKeyDataDir]; item != nil {
		item.Description = fmt.Sprintf("当前数据目录: %s", formatPathWithFallback(a.ctx.DataDir, "未设置"))
	}

	if item := a.settingsItemMap[settingKeyDataKey]; item != nil {
		item.Description = fmt.Sprintf("当前数据密钥: %s", formatSecretSummary(a.ctx.DataKey))
	}

	if item := a.settingsItemMap[settingKeyImgKey]; item != nil {
		item.Description = fmt.Sprintf("当前图片密钥: %s", formatSecretSummary(a.ctx.ImgKey))
	}

	if item := a.settingsItemMap[settingKeyOpenAIAPIKey]; item != nil {
		openAIKey := "未设置"
		if speechCfg != nil {
			openAIKey = formatSecretSummary(speechCfg.APIKey)
		}
		item.Description = fmt.Sprintf("当前 API Key: %s", openAIKey)
	}

	if item := a.settingsItemMap[settingKeyOpenAIBaseURL]; item != nil {
		baseURL := "未设置"
		if speechCfg != nil {
			baseURL = formatPathWithFallback(speechCfg.BaseURL, "未设置")
		}
		item.Description = fmt.Sprintf("当前 Base URL: %s", baseURL)
	}

	if item := a.settingsItemMap[settingKeyOpenAIProxy]; item != nil {
		proxy := "未设置"
		if speechCfg != nil {
			proxy = formatPathWithFallback(speechCfg.Proxy, "未设置")
		}
		item.Description = fmt.Sprintf("当前代理: %s", proxy)
	}

	if item := a.settingsItemMap[settingKeyOpenAITimeout]; item != nil {
		timeoutValue := 0
		if speechCfg != nil {
			timeoutValue = speechCfg.RequestTimeoutSeconds
		}
		item.Description = fmt.Sprintf("当前请求超时: %s", formatTimeoutSummary(timeoutValue))
	}

	a.settingsMenu.SetItems(a.settingsItems)
}

func (a *App) updateSpeechConfig(mutator func(*conf.SpeechConfig)) error {
	current := a.ctx.GetSpeech()
	cfg := conf.SpeechConfig{Enabled: true}
	if current != nil {
		cfg = *current
	} else {
		cfg.Provider = "openai"
	}

	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}

	if mutator != nil {
		mutator(&cfg)
	}

	cfg.Normalize()
	return a.m.SaveSpeechConfig(&cfg)
}

func (a *App) settingSpeechProvider() {
	buttons := []string{"OpenAI 官方服务", "本地 Docker Whisper", "Whisper.cpp 本地模型", "取消"}
	a.showModal("选择语音服务提供商", buttons, func(buttonIndex int, buttonLabel string) {
		a.mainPages.RemovePage("modal")

		var (
			provider string
			message  string
		)

		switch buttonLabel {
		case "OpenAI 官方服务":
			provider = "openai"
			message = "语音服务已切换到 OpenAI 官方服务"
		case "本地 Docker Whisper":
			provider = "webservice"
			message = "语音服务已切换到本地 Docker Whisper"
		case "Whisper.cpp 本地模型":
			provider = "whispercpp"
			message = "语音服务已切换到 Whisper.cpp 本地模型"
		default:
			return
		}

		if err := a.updateSpeechConfig(func(cfg *conf.SpeechConfig) {
			cfg.Provider = provider
			if provider == "webservice" && strings.TrimSpace(cfg.ServiceURL) == "" {
				cfg.ServiceURL = "http://127.0.0.1:9000"
			}
		}); err != nil {
			a.showError(err)
			return
		}

		a.refreshSettingsMenu()
		if message != "" {
			a.showInfo(message)
		}
	})
}

func (a *App) settingWhisperModelPath() {
	formView := form.NewForm("设置 Whisper.cpp 模型路径")

	speech := a.ctx.GetSpeech()
	currentValue := ""
	if speech != nil {
		currentValue = strings.TrimSpace(speech.Model)
	}
	tempValue := currentValue

	formView.AddInputField("模型文件路径", tempValue, 0, nil, func(text string) {
		tempValue = text
	})

	formView.AddButton("保存", func() {
		trimmed := strings.TrimSpace(tempValue)
		if trimmed != "" {
			trimmed = filepath.Clean(trimmed)
		}

		if err := a.updateSpeechConfig(func(cfg *conf.SpeechConfig) {
			cfg.Model = trimmed
		}); err != nil {
			a.showError(err)
			return
		}
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("Whisper.cpp 模型路径已更新")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

func (a *App) settingWhisperThreads() {
	formView := form.NewForm("设置 Whisper.cpp 线程数")

	speech := a.ctx.GetSpeech()
	currentValue := ""
	if speech != nil && speech.Threads > 0 {
		currentValue = strconv.Itoa(speech.Threads)
	}
	tempValue := currentValue

	acceptNumeric := func(text string, lastChar rune) bool {
		if lastChar == 0 {
			return true
		}
		return lastChar >= '0' && lastChar <= '9'
	}

	formView.AddInputField("线程数", tempValue, 0, acceptNumeric, func(text string) {
		tempValue = text
	})

	formView.AddButton("保存", func() {
		trimmed := strings.TrimSpace(tempValue)
		threads := 0
		if trimmed != "" {
			v, err := strconv.Atoi(trimmed)
			if err != nil || v < 0 {
				a.showError(fmt.Errorf("请输入合法的非负整数"))
				return
			}
			threads = v
		}

		if err := a.updateSpeechConfig(func(cfg *conf.SpeechConfig) {
			cfg.Threads = threads
		}); err != nil {
			a.showError(err)
			return
		}
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("Whisper.cpp 线程数已更新")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

func (a *App) settingLocalServiceURL() {
	formView := form.NewForm("设置本地语音服务地址")

	speech := a.ctx.GetSpeech()
	currentValue := "http://127.0.0.1:9000"
	if speech != nil {
		currentValue = formatPathWithFallback(speech.ServiceURL, currentValue)
	}

	tempValue := currentValue

	formView.AddInputField("服务地址", tempValue, 0, nil, func(text string) {
		tempValue = text
	})

	formView.AddButton("保存", func() {
		if err := a.updateSpeechConfig(func(cfg *conf.SpeechConfig) {
			cfg.ServiceURL = tempValue
		}); err != nil {
			a.showError(err)
			return
		}
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("本地语音服务地址已更新")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

func (a *App) settingOpenAIAPIKey() {
	formView := form.NewForm("设置 OpenAI API Key")
	speech := a.ctx.GetSpeech()
	currentValue := ""
	if speech != nil {
		currentValue = speech.APIKey
	}
	tempValue := currentValue

	formView.AddInputField("API Key", tempValue, 0, nil, func(text string) {
		tempValue = text
	})

	formView.AddButton("保存", func() {
		if err := a.updateSpeechConfig(func(cfg *conf.SpeechConfig) {
			cfg.APIKey = tempValue
		}); err != nil {
			a.showError(err)
			return
		}
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("OpenAI API Key 已更新")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

func (a *App) settingOpenAIBaseURL() {
	formView := form.NewForm("设置 OpenAI Base URL")
	speech := a.ctx.GetSpeech()
	currentValue := ""
	if speech != nil {
		currentValue = speech.BaseURL
	}
	tempValue := currentValue

	formView.AddInputField("Base URL", tempValue, 0, nil, func(text string) {
		tempValue = text
	})

	formView.AddButton("保存", func() {
		if err := a.updateSpeechConfig(func(cfg *conf.SpeechConfig) {
			cfg.BaseURL = tempValue
		}); err != nil {
			a.showError(err)
			return
		}
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("OpenAI Base URL 已更新")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

func (a *App) settingOpenAIProxy() {
	formView := form.NewForm("设置 OpenAI 代理")
	speech := a.ctx.GetSpeech()
	currentValue := ""
	if speech != nil {
		currentValue = speech.Proxy
	}
	tempValue := currentValue

	formView.AddInputField("代理地址", tempValue, 0, nil, func(text string) {
		tempValue = text
	})

	formView.AddButton("保存", func() {
		if err := a.updateSpeechConfig(func(cfg *conf.SpeechConfig) {
			cfg.Proxy = tempValue
		}); err != nil {
			a.showError(err)
			return
		}
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("OpenAI 代理已更新")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

func (a *App) settingOpenAITimeout() {
	formView := form.NewForm("设置 OpenAI 请求超时")
	speech := a.ctx.GetSpeech()
	currentValue := ""
	if speech != nil && speech.RequestTimeoutSeconds > 0 {
		currentValue = strconv.Itoa(speech.RequestTimeoutSeconds)
	}
	tempValue := currentValue

	acceptNumeric := func(text string, lastChar rune) bool {
		if lastChar == 0 {
			return true
		}
		return lastChar >= '0' && lastChar <= '9'
	}

	formView.AddInputField("超时(秒)", tempValue, 0, acceptNumeric, func(text string) {
		tempValue = text
	})

	formView.AddButton("保存", func() {
		trimmed := strings.TrimSpace(tempValue)
		seconds := 0
		if trimmed != "" {
			v, err := strconv.Atoi(trimmed)
			if err != nil {
				a.showError(fmt.Errorf("请输入合法的非负整数"))
				return
			}
			seconds = v
		}

		if err := a.updateSpeechConfig(func(cfg *conf.SpeechConfig) {
			cfg.RequestTimeoutSeconds = seconds
		}); err != nil {
			a.showError(err)
			return
		}
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("OpenAI 请求超时已更新")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// settingHTTPPort 设置 HTTP 端口
func (a *App) settingHTTPPort() {
	// 使用我们的自定义表单组件
	formView := form.NewForm("设置 HTTP 地址")

	// 临时存储用户输入的值
	tempHTTPAddr := a.ctx.HTTPAddr

	// 添加输入字段 - 不再直接设置HTTP地址，而是更新临时变量
	formView.AddInputField("地址", tempHTTPAddr, 0, nil, func(text string) {
		tempHTTPAddr = text // 只更新临时变量
	})

	// 添加按钮 - 点击保存时才设置HTTP地址
	formView.AddButton("保存", func() {
		a.m.SetHTTPAddr(tempHTTPAddr) // 在这里设置HTTP地址
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("HTTP 地址已设置为 " + a.ctx.HTTPAddr)
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// toggleListen 在 127.0.0.1 与 0.0.0.0 之间切换监听主机，保持端口不变
func (a *App) toggleListen() {
	// 计算新的地址
	cur := a.ctx.GetHTTPAddr()
	host, port, err := net.SplitHostPort(cur)
	if err != nil || port == "" {
		// 回退到默认端口
		host = "127.0.0.1"
		port = "5030"
	}
	h := strings.TrimSpace(host)
	var newHost string
	if h == "0.0.0.0" || h == "::" || h == "[::]" || h == "" {
		newHost = "127.0.0.1"
	} else {
		newHost = "0.0.0.0"
	}
	newAddr := net.JoinHostPort(newHost, port)

	// 若服务正在运行，则重启服务以应用新监听
	if a.ctx.HTTPEnabled {
		modal := tview.NewModal().SetText("正在切换监听地址...")
		a.mainPages.AddPage("modal", modal, true, true)
		a.SetFocus(modal)
		go func() {
			// 停止服务
			stopErr := a.m.StopService()
			if stopErr == nil {
				// 设置新地址
				_ = a.m.SetHTTPAddr(newAddr)
				// 启动服务
				startErr := a.m.StartService()
				a.QueueUpdateDraw(func() {
					a.mainPages.RemovePage("modal")
					if startErr != nil {
						a.showError(fmt.Errorf("切换失败: %v", startErr))
					} else {
						a.refreshSettingsMenu()
						a.showInfo("已切换监听地址为 " + newAddr)
					}
				})
				return
			}
			// 停止失败时直接报错
			a.QueueUpdateDraw(func() {
				a.mainPages.RemovePage("modal")
				a.showError(fmt.Errorf("切换失败: %v", stopErr))
			})
		}()
		return
	}

	// 服务未运行，仅更新配置
	_ = a.m.SetHTTPAddr(newAddr)
	a.refreshSettingsMenu()
	a.showInfo("已切换监听地址为 " + newAddr)
}

// settingWorkDir 设置工作目录
func (a *App) settingWorkDir() {
	// 使用我们的自定义表单组件
	formView := form.NewForm("设置工作目录")

	// 临时存储用户输入的值
	tempWorkDir := a.ctx.WorkDir

	// 添加输入字段 - 不再直接设置工作目录，而是更新临时变量
	formView.AddInputField("工作目录", tempWorkDir, 0, nil, func(text string) {
		tempWorkDir = text // 只更新临时变量
	})

	// 添加按钮 - 点击保存时才设置工作目录
	formView.AddButton("保存", func() {
		a.ctx.SetWorkDir(tempWorkDir) // 在这里设置工作目录
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("工作目录已设置为 " + a.ctx.WorkDir)
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// settingDataKey 设置数据密钥
func (a *App) settingDataKey() {
	// 使用我们的自定义表单组件
	formView := form.NewForm("设置数据密钥")

	// 临时存储用户输入的值
	tempDataKey := a.ctx.DataKey

	// 添加输入字段 - 不直接设置数据密钥，而是更新临时变量
	formView.AddInputField("数据密钥", tempDataKey, 0, nil, func(text string) {
		tempDataKey = text // 只更新临时变量
	})

	// 添加按钮 - 点击保存时才设置数据密钥
	formView.AddButton("保存", func() {
		a.ctx.SetDataKey(tempDataKey)
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("数据密钥已设置")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// settingImgKey 设置图片密钥 (ImgKey)
func (a *App) settingImgKey() {
	formView := form.NewForm("设置图片密钥")

	tempImgKey := a.ctx.ImgKey

	formView.AddInputField("图片密钥", tempImgKey, 0, nil, func(text string) {
		tempImgKey = text
	})

	formView.AddButton("保存", func() {
		a.ctx.SetImgKey(tempImgKey)
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("图片密钥已设置")
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// settingDataDir 设置数据目录
func (a *App) settingDataDir() {
	// 使用我们的自定义表单组件
	formView := form.NewForm("设置数据目录")

	// 临时存储用户输入的值
	tempDataDir := a.ctx.DataDir

	// 添加输入字段 - 不直接设置数据目录，而是更新临时变量
	formView.AddInputField("数据目录", tempDataDir, 0, nil, func(text string) {
		tempDataDir = text // 只更新临时变量
	})

	// 添加按钮 - 点击保存时才设置数据目录
	formView.AddButton("保存", func() {
		a.ctx.SetDataDir(tempDataDir)
		a.mainPages.RemovePage("submenu2")
		a.refreshSettingsMenu()
		a.showInfo("数据目录已设置为 " + a.ctx.DataDir)
	})

	formView.AddButton("取消", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}