//go:build windows

package tray

import (
	_ "embed"
	"sync"

	"github.com/getlantern/systray"
	"github.com/rs/zerolog/log"
)

var (
	iconInit sync.Once
	iconData []byte
	iconErr  error
)


func trayIcon() ([]byte, error) {
	return iconData, nil
}

// Run starts the system tray. This function blocks until Stop is called or the Quit menu item is clicked.
func Run(opts Options) {
	systray.Run(func() {
		setupTray(opts)
	}, nil)
}

// Stop stops the system tray.
func Stop() {
	systray.Quit()
}

func setupTray(opts Options) {
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
				systray.Quit()
				return
			}
		}
	}()
}
