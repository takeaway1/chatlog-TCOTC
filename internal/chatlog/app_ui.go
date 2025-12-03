package chatlog

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

func (a *App) switchTab(step int) {
	index := (a.activeTab + step) % a.tabCount
	if index < 0 {
		index = a.tabCount - 1
	}
	a.activeTab = index
	a.tabPages.SwitchToPage(fmt.Sprint(a.activeTab))
	switch a.activeTab {
	case 0:
		a.SetFocus(a.menu)
	case 1:
		a.SetFocus(a.help)
	case 2:
		if a.settingsMenu != nil {
			a.SetFocus(a.settingsMenu)
		}
	}
}

func (a *App) focusSettingsTab() {
	if a.settingsMenu == nil {
		return
	}
	a.activeTab = 2
	a.tabPages.SwitchToPage("2")
	a.refreshSettingsMenu()
	a.SetFocus(a.settingsMenu)
}

func (a *App) showModal(text string, buttons []string, doneFunc func(buttonIndex int, buttonLabel string)) {
	modal := tview.NewModal().
		SetText(text).
		AddButtons(buttons).
		SetDoneFunc(doneFunc)

	a.mainPages.AddPage("modal", modal, true, true)
	a.SetFocus(modal)
}

// showError 显示错误对话框
func (a *App) showError(err error) {
	a.showModal(err.Error(), []string{"OK"}, func(buttonIndex int, buttonLabel string) {
		a.mainPages.RemovePage("modal")
	})
}

// showInfo 显示信息对话框
func (a *App) showInfo(text string) {
	a.showModal(text, []string{"OK"}, func(buttonIndex int, buttonLabel string) {
		a.mainPages.RemovePage("modal")
	})
}

func formatPathWithFallback(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func formatSecretSummary(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "未设置"
	}
	if len(trimmed) <= 6 {
		return "已设置"
	}
	return fmt.Sprintf("已设置(长度 %d)", len(trimmed))
}

func formatTimeoutSummary(seconds int) string {
	if seconds <= 0 {
		return "默认"
	}
	return fmt.Sprintf("%d 秒", seconds)
}