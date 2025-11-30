//go:build darwin

package tray

import (
	"sync"

	"fyne.io/systray"
)

type controller struct {
	stopOnce sync.Once
	stopped  chan struct{}
}

func newController() *controller {
	return &controller{stopped: make(chan struct{})}
}

func (c *controller) Stop() {
	c.stopOnce.Do(func() {
		systray.Quit()
	})
	<-c.stopped
}

// Start launches the system tray icon.
func Start(opts Options) (Controller, error) {
	ctrl := newController()
	ready := make(chan struct{})

	go systray.Run(func() {
		setupTray(opts, ctrl)
		close(ready)
	}, func() {
		close(ctrl.stopped)
	})

	<-ready
	return ctrl, nil
}

func setupTray(opts Options, ctrl *controller) {
	// macOS 菜单栏图标通常使用模板图像（Template Image），即只有 alpha 通道的黑白图像
	// 这里我们暂时不设置图标，只设置标题，或者后续可以添加专门的 macOS 图标
	// systray.SetIcon(data)

	// 设置标题，这在 macOS 上很常见
	systray.SetTitle("Chatlog")

	tip := opts.Tooltip
	if tip == "" {
		tip = "Chatlog"
	}
	systray.SetTooltip(tip)

	openItem := systray.AddMenuItem("Open Chatlog", "Open Chatlog web interface")
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("Exit Chatlog", "Quit Chatlog")

	go func() {
		for {
			select {
			case <-openItem.ClickedCh:
				if opts.OnOpen != nil {
					opts.OnOpen()
				}
			case <-quitItem.ClickedCh:
				if opts.OnQuit != nil {
					opts.OnQuit()
				}
				ctrl.Stop()
				return
			case <-ctrl.stopped:
				return
			}
		}
	}()
}