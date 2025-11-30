package chatlog

import (
	stdlog "log"
	"os"
	"path/filepath"
	"time"

	"github.com/sjzar/chatlog/internal/chatlog/conf"
	"github.com/sjzar/chatlog/pkg/util"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	Debug   bool
	Console bool
)

func initLog(cmd *cobra.Command, args []string) {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	if Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	stdlog.SetOutput(os.Stderr)
}

func getLogDir() string {
	// 0. 获取配置文件
	cfg, _, err := conf.LoadTUIConfig("")

	if err != nil {
		// 1. 如果配置文件不存在(或加载失败)， 则默认从 os.Executable() 获取同路径
		exePath, exeErr := os.Executable()
		if exeErr == nil {
			return filepath.Dir(exePath)
		}
		return "."
	}

	// 尝试获取当前账号的 WorkDir
	if cfg.LastAccount != "" {
		history := cfg.ParseHistory()
		if pc, ok := history[cfg.LastAccount]; ok {
			if pc.WorkDir != "" {
				// 3. 如果配置文件里workDir不为空，则使用 workDir
				return pc.WorkDir
			}
		}
	}

	// 2. 如果配置文件存在，但 workDir 配置为空， 则默认在配置文件同路径
	return cfg.ConfigDir
}

func initTuiLog(cmd *cobra.Command, args []string) {
	logDir := getLogDir()

	util.PrepareDir(logDir)
	logPath := filepath.Join(logDir, "chatlog.log")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, os.ModePerm)
	if err != nil {
		// 如果log文件打开失败，则转为 initLog方法 （stdlog ）
		initLog(cmd, args)
		log.Warn().Err(err).Str("path", logPath).Msg("failed to open log file, fallback to stderr")
		return
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: logFile, NoColor: true, TimeFormat: time.RFC3339})
	logrus.SetOutput(logFile)
	stdlog.SetOutput(logFile)

	if Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}