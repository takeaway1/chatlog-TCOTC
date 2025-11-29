package tray

// Options controls how the tray icon behaves.
type Options struct {
	Tooltip string
	OnOpen  func()
	OnQuit  func()
}