package chatlog

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rs/zerolog/log"
	"github.com/sjzar/chatlog/internal/chatlog/ctx"
	"github.com/sjzar/chatlog/internal/ui/footer"
	"github.com/sjzar/chatlog/internal/ui/help"
	"github.com/sjzar/chatlog/internal/ui/infobar"
	"github.com/sjzar/chatlog/internal/ui/menu"
	"github.com/sjzar/chatlog/pkg/util"
)

const (
	RefreshInterval = 1000 * time.Millisecond
)

type App struct {
	*tview.Application

	ctx         *ctx.Context
	m           *Manager
	stopRefresh chan struct{}

	// page
	mainPages *tview.Pages
	infoBar   *infobar.InfoBar
	tabPages  *tview.Pages
	footer    *footer.Footer

	// tab
	menu            *menu.Menu
	help            *help.Help
	settingsMenu    *menu.SubMenu
	settingsItems   []*menu.Item
	settingsItemMap map[settingsKey]*menu.Item
	activeTab       int
	tabCount        int
}

func NewApp(ctx *ctx.Context, m *Manager) *App {
	app := &App{
		ctx:             ctx,
		m:               m,
		Application:     tview.NewApplication(),
		mainPages:       tview.NewPages(),
		infoBar:         infobar.New(),
		tabPages:        tview.NewPages(),
		footer:          footer.New(),
		menu:            menu.New("主菜单"),
		help:            help.New(),
		settingsItemMap: make(map[settingsKey]*menu.Item),
	}

	app.initMenu()
	app.initSettingsTab()

	app.updateMenuItemsState()

	return app
}

func (a *App) Run() error {
	log.Debug().Msg("App Run started")

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.infoBar, infobar.InfoBarViewHeight, 0, false).
		AddItem(a.tabPages, 0, 1, true).
		AddItem(a.footer, 1, 1, false)

	a.mainPages.AddPage("main", flex, true, true)

	a.tabPages.
		AddPage("0", a.menu, true, true).
		AddPage("1", a.help, true, false)
	if a.settingsMenu != nil {
		a.tabPages.AddPage("2", a.settingsMenu, true, false)
		a.tabCount = 3
	} else {
		a.tabCount = 2
	}

	a.SetInputCapture(a.inputCapture)

	go a.refresh()

	if err := a.SetRoot(a.mainPages, true).EnableMouse(false).Run(); err != nil {
		return err
	}

	return nil
}

func (a *App) Stop() {
	if a.stopRefresh != nil {
		close(a.stopRefresh)
		a.stopRefresh = nil
	}
	a.Application.Stop()
}

func (a *App) refresh() {
	log.Debug().Msg("App refresh called")
	tick := time.NewTicker(RefreshInterval)
	defer tick.Stop()

	for {
		select {
		case <-a.stopRefresh:
			return
		case <-tick.C:
			if a.ctx.AutoDecrypt || a.ctx.HTTPEnabled {
				a.m.RefreshSession()
			}
			a.infoBar.UpdateAccount(a.ctx.Account)
			a.infoBar.UpdateBasicInfo(a.ctx.PID, a.ctx.FullVersion, a.ctx.ExePath)
			a.infoBar.UpdateStatus(a.ctx.Status)
			a.infoBar.UpdateDataKey(a.ctx.DataKey)
			a.infoBar.UpdateImageKey(a.ctx.ImgKey)
			a.infoBar.UpdatePlatform(a.ctx.Platform)
			a.infoBar.UpdateDataUsageDir(a.ctx.DataUsage, a.ctx.DataDir)
			a.infoBar.UpdateWorkUsageDir(a.ctx.WorkUsage, a.ctx.WorkDir)
			if a.ctx.LastSession.Unix() > 1000000000 {
				a.infoBar.UpdateSession(a.ctx.LastSession.Format("2006-01-02 15:04:05"))
			}
			if a.ctx.HTTPEnabled {
				addr := a.ctx.HTTPAddr
				h, _, err := net.SplitHostPort(addr)
				if err != nil { // Fallback if malformed
					a.infoBar.UpdateHTTPServer(fmt.Sprintf("[green][已启动][white] [%s]", addr))
				} else {
					h = strings.TrimSpace(h)
					if h == "0.0.0.0" || h == "::" || h == "[::]" || h == "" {
						lan := util.ComposeLANURL(addr)
						a.infoBar.UpdateHTTPServer(fmt.Sprintf("[green][已启动][white] [%s]", lan))
					} else {
						a.infoBar.UpdateHTTPServer(fmt.Sprintf("[green][已启动][white] [%s]", addr))
					}
				}
			} else {
				a.infoBar.UpdateHTTPServer("[未启动]")
			}
			if a.ctx.AutoDecrypt {
				a.infoBar.UpdateAutoDecrypt("[green][已开启][white]")
			} else {
				a.infoBar.UpdateAutoDecrypt("[未开启]")
			}

			a.Draw()
		}
	}
}

func (a *App) inputCapture(event *tcell.EventKey) *tcell.EventKey {

	// 如果当前页面不是主页面，ESC 键返回主页面
	if a.mainPages.HasPage("submenu") && event.Key() == tcell.KeyEscape {
		a.mainPages.RemovePage("submenu")
		a.mainPages.SwitchToPage("main")
		return nil
	}

	if a.tabPages.HasFocus() {
		switch event.Key() {
		case tcell.KeyLeft:
			a.switchTab(-1)
			return nil
		case tcell.KeyRight:
			a.switchTab(1)
			return nil
		}
	}

	switch event.Key() {
	case tcell.KeyCtrlC:
		a.Stop()
	}

	return event
}