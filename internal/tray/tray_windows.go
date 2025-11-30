//go:build windows

package tray

import (
	_ "embed"
	"sync"

	"github.com/getlantern/systray"
	"github.com/rs/zerolog/log"
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

//go:embed icon.ico
var iconData []byte

func trayIcon() ([]byte, error) {
	return iconData, nil
}

// Start launches the Windows notification area icon.
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
	if data, err := trayIcon(); err != nil {
		log.Warn().Err(err).Msg("failed to load tray icon from icon.ico")
	} else if len(data) > 0 {
		systray.SetIcon(data)
	} else {
		log.Warn().Msg("tray icon icon.ico is empty")
	}

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
