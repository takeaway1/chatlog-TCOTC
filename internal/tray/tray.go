package tray

// Options controls how the tray icon behaves.
type Options struct {
	Tooltip string
	OnOpen  func()
	OnQuit  func()
}

// Run starts the system tray.
func Run(opts Options) {
	run(opts)
}

// Stop stops the system tray.
func Stop() {
	stop()
}