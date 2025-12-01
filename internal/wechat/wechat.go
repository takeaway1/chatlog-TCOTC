package wechat

import (
	"context"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/wechat/decrypt"
	"github.com/sjzar/chatlog/internal/wechat/key"
	"github.com/sjzar/chatlog/internal/wechat/model"
)

// Account 表示一个微信账号
type Account struct {
	Name        string
	Platform    string
	Version     int
	FullVersion string
	DataDir     string
	Key         string
	ImgKey      string
	PID         uint32
	ExePath     string
	Status      string
}

// NewAccount 创建新的账号对象
func NewAccount(proc *model.Process) *Account {
	return &Account{
		Name:        proc.AccountName,
		Platform:    proc.Platform,
		Version:     proc.Version,
		FullVersion: proc.FullVersion,
		DataDir:     proc.DataDir,
		PID:         proc.PID,
		ExePath:     proc.ExePath,
		Status:      proc.Status,
	}
}

// RefreshStatus 刷新账号的进程状态
func (a *Account) RefreshStatus() error {
	log.Debug().Str("account", a.Name).Msg("refreshing account status")
	// 查找所有微信进程
	Load()

	process, err := GetProcess(a.Name)
	if err != nil {
		log.Debug().Err(err).Str("account", a.Name).Msg("process not found during refresh")
		a.Status = model.StatusOffline
		return nil
	}

	if process.AccountName == a.Name {
		// 更新进程信息
		a.PID = process.PID
		a.ExePath = process.ExePath
		a.Platform = process.Platform
		a.Version = process.Version
		a.FullVersion = process.FullVersion
		a.Status = process.Status
		a.DataDir = process.DataDir
	}

	return nil
}

// GetKey 获取账号的密钥
func (a *Account) GetKey(ctx context.Context) (string, string, error) {
	log.Debug().Str("account", a.Name).Msg("getting account key")
	// 如果已经有密钥，直接返回
	if a.Key != "" && (a.ImgKey != "" || a.Version == 3) {
		log.Debug().Str("account", a.Name).Msg("returning cached key")
		return a.Key, a.ImgKey, nil
	}

	// 刷新进程状态
	if err := a.RefreshStatus(); err != nil {
		log.Debug().Err(err).Str("account", a.Name).Msg("failed to refresh status")
		return "", "", errors.RefreshProcessStatusFailed(err)
	}

	// 检查账号状态
	if a.Status != model.StatusOnline {
		log.Debug().Str("account", a.Name).Str("status", a.Status).Msg("account not online")
		return "", "", errors.WeChatAccountNotOnline(a.Name)
	}

	// 创建密钥提取器 - 使用新的接口，传入平台和版本信息
	extractor, err := key.NewExtractor(a.Platform, a.Version)
	if err != nil {
		log.Debug().Err(err).Str("platform", a.Platform).Int("version", a.Version).Msg("failed to create extractor")
		return "", "", err
	}

	process, err := GetProcess(a.Name)
	if err != nil {
		log.Debug().Err(err).Str("account", a.Name).Msg("failed to get process")
		return "", "", err
	}

	validator, err := decrypt.NewValidator(process.Platform, process.Version, process.DataDir)
	if err != nil {
		log.Debug().Err(err).Str("platform", process.Platform).Msg("failed to create validator")
		return "", "", err
	}

	extractor.SetValidate(validator)

	// 提取密钥
	dataKey, imgKey, err := extractor.Extract(ctx, process)
	if err != nil {
		log.Debug().Err(err).Str("account", a.Name).Msg("failed to extract key")
		return "", "", err
	}
	log.Debug().Str("account", a.Name).Msg("key extracted successfully")

	if dataKey != "" {
		a.Key = dataKey
	}

	if imgKey != "" {
		a.ImgKey = imgKey
	}

	return dataKey, imgKey, nil
}

// DecryptDatabase 解密数据库
func (a *Account) DecryptDatabase(ctx context.Context, dbPath, outputPath string) error {
	log.Debug().Str("account", a.Name).Str("dbPath", dbPath).Str("outputPath", outputPath).Msg("decrypting database")
	// 获取密钥
	hexKey, _, err := a.GetKey(ctx)
	if err != nil {
		log.Debug().Err(err).Str("account", a.Name).Msg("failed to get key for decryption")
		return err
	}

	// 创建解密器 - 传入平台信息和版本
	decryptor, err := decrypt.NewDecryptor(a.Platform, a.Version)
	if err != nil {
		log.Debug().Err(err).Msg("failed to create decryptor")
		return err
	}

	// 创建输出文件
	output, err := os.Create(outputPath)
	if err != nil {
		log.Debug().Err(err).Str("path", outputPath).Msg("failed to create output file")
		return err
	}
	defer output.Close()

	// 解密数据库
	if err := decryptor.Decrypt(ctx, dbPath, hexKey, output); err != nil {
		log.Debug().Err(err).Msg("failed to decrypt database")
		return err
	}
	log.Debug().Msg("database decrypted successfully")
	return nil
}
