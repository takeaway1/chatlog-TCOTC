package database

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/chatlog/conf"
	"github.com/sjzar/chatlog/internal/chatlog/webhook"
	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/internal/wechatdb"
)

// VFSMode constants for database access mode
const (
	VFSModeAuto     = "auto"     // Automatically choose based on conditions
	VFSModeEnabled  = "enabled"  // Force VFS mode (direct encrypted reading)
	VFSModeDisabled = "disabled" // Force traditional copy mode
)

const (
	StateInit = iota
	StateDecrypting
	StateReady
	StateError
)

type Service struct {
	State         int
	StateMsg      string
	conf          Config
	db            *wechatdb.DB
	webhook       *webhook.Service
	webhookCancel context.CancelFunc
}

type Config interface {
	GetWorkDir() string
	GetDataDir() string
	GetDataKey() string
	GetPlatform() string
	GetVersion() int
	GetWebhook() *conf.Webhook
	GetVFSMode() string // Returns VFSModeAuto, VFSModeEnabled, or VFSModeDisabled
}

func NewService(conf Config) *Service {
	return &Service{
		conf:    conf,
		webhook: webhook.New(conf),
	}
}

func (s *Service) Start() error {
	// Determine whether to use VFS mode
	var opts []wechatdb.Option
	path := s.conf.GetWorkDir()

	vfsMode := s.conf.GetVFSMode()
	dataKey := s.conf.GetDataKey()
	dataDir := s.conf.GetDataDir()

	useVFS := false
	switch vfsMode {
	case VFSModeEnabled:
		// Force VFS mode if data key and data dir are available
		if dataKey != "" && dataDir != "" {
			useVFS = true
			path = dataDir
			log.Info().Msg("database: VFS mode enabled (forced)")
		} else {
			log.Warn().Msg("database: VFS mode requested but dataKey or dataDir missing, falling back to copy mode")
		}
	case VFSModeDisabled:
		// Explicitly disabled, use copy mode
		log.Debug().Msg("database: VFS mode disabled, using copy mode")
	case VFSModeAuto, "":
		// Auto mode: use VFS if we have the key and data dir, otherwise use work dir
		// For now, default to copy mode for backward compatibility
		log.Debug().Msg("database: VFS mode auto, using copy mode for compatibility")
	}

	if useVFS {
		opts = append(opts, wechatdb.WithVFS(dataKey))
	}

	db, err := wechatdb.New(path, s.conf.GetPlatform(), s.conf.GetVersion(), opts...)
	if err != nil {
		return err
	}
	s.SetReady()
	s.db = db
	s.initWebhook()
	return nil
}

func (s *Service) Stop() error {
	if s.db != nil {
		s.db.Close()
	}
	s.SetInit()
	s.db = nil
	if s.webhookCancel != nil {
		s.webhookCancel()
		s.webhookCancel = nil
	}
	return nil
}

func (s *Service) SetInit() {
	s.State = StateInit
}

func (s *Service) SetDecrypting() {
	s.State = StateDecrypting
}

func (s *Service) SetReady() {
	s.State = StateReady
}

func (s *Service) SetError(msg string) {
	s.State = StateError
	s.StateMsg = msg
}

func (s *Service) GetDB() *wechatdb.DB {
	return s.db
}

// GetWorkDir exposes the underlying work directory where decrypted DB files are stored.
// This is useful for higher layers (HTTP) to compute DB sizes for summary statistics.
func (s *Service) GetWorkDir() string {
	if s == nil || s.conf == nil {
		return ""
	}
	return s.conf.GetWorkDir()
}

func (s *Service) GetMessages(start, end time.Time, talker string, sender string, keyword string, limit, offset int) ([]*model.Message, error) {
	return s.db.GetMessages(start, end, talker, sender, keyword, limit, offset)
}

func (s *Service) SearchMessages(req *model.SearchRequest) (*model.SearchResponse, error) {
	if s.db == nil {
		return nil, errors.InvalidArg("search before db ready")
	}
	return s.db.SearchMessages(req)
}

func (s *Service) GetContacts(key string, limit, offset int) (*wechatdb.GetContactsResp, error) {
	return s.db.GetContacts(key, limit, offset)
}

func (s *Service) GetChatRooms(key string, limit, offset int) (*wechatdb.GetChatRoomsResp, error) {
	return s.db.GetChatRooms(key, limit, offset)
}

// GetSession retrieves session information
func (s *Service) GetSessions(key string, limit, offset int) (*wechatdb.GetSessionsResp, error) {
	return s.db.GetSessions(key, limit, offset)
}

func (s *Service) GetMedia(_type string, key string) (*model.Media, error) {
	return s.db.GetMedia(_type, key)
}

func (s *Service) GetAvatar(username string, size string) (*model.Avatar, error) {
	return s.db.GetAvatar(username, size)
}

func (s *Service) initWebhook() error {
	if s.webhook == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.webhookCancel = cancel
	hooks := s.webhook.GetHooks(ctx, s.db)
	for _, hook := range hooks {
		log.Info().Msgf("set callback %#v", hook)
		if err := s.db.SetCallback(hook.Group(), hook.Callback); err != nil {
			log.Error().Err(err).Msgf("set callback %#v failed", hook)
			return err
		}
	}
	return nil
}

// Close closes the database connection
func (s *Service) Close() {
	// Add cleanup code if needed
	s.db.Close()
	if s.webhookCancel != nil {
		s.webhookCancel()
		s.webhookCancel = nil
	}
}
