//go:build darwin

package tray

import (
	"sync"

	"github.com/getlantern/systray"
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