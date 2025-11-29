//go:build !windows

package tray

import (
	"os"
	"os/signal"
	"syscall"
)

var stopChan = make(chan struct{})

func Run(opts Options) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case <-c:
	case <-stopChan:
	}

	if opts.OnQuit != nil {
		opts.OnQuit()
	}
}

func Stop() {
	select {
	case <-stopChan:
	default:
		close(stopChan)
	}
}