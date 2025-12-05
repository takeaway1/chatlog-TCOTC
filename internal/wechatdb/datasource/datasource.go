package datasource

import (
	"context"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/internal/wechatdb/datasource/darwinv3"
	v4 "github.com/sjzar/chatlog/internal/wechatdb/datasource/v4"
	"github.com/sjzar/chatlog/internal/wechatdb/datasource/windowsv3"
	"github.com/sjzar/chatlog/internal/wechatdb/msgstore"
)

type DataSource interface {
	msgstore.Provider

	// 消息
	GetMessages(ctx context.Context, startTime, endTime time.Time, talker string, sender string, keyword string, limit, offset int) ([]*model.Message, error)
	GetDatasetFingerprint(ctx context.Context) (string, error)

	// 联系人
	GetContacts(ctx context.Context, key string, limit, offset int) ([]*model.Contact, error)

	// 获取置顶的用户名列表
	GetPinnedUserNames(ctx context.Context) ([]string, error)

	// 群聊
	GetChatRooms(ctx context.Context, key string, limit, offset int) ([]*model.ChatRoom, error)

	// 最近会话
	GetSessions(ctx context.Context, key string, limit, offset int) ([]*model.Session, error)

	// 媒体
	GetMedia(ctx context.Context, _type string, key string) (*model.Media, error)

	// 头像
	GetAvatar(ctx context.Context, username string, size string) (*model.Avatar, error)

	// 统计聚合（避免逐条扫描）：
	// 全局消息统计：总数、发送/接收、最早/最晚、按(Type,SubType)计数
	GlobalMessageStats(ctx context.Context) (*model.GlobalMessageStats, error)
	// 群聊消息计数：返回 talker(群名) -> count
	GroupMessageCounts(ctx context.Context) (map[string]int64, error)
	// 群聊消息类型分布：返回 typeLabel -> count（只统计群消息）
	GroupMessageTypeStats(ctx context.Context) (map[string]int64, error)
	// 群聊今日消息计数：返回 talker(群名) -> today_count
	GroupTodayMessageCounts(ctx context.Context) (map[string]int64, error)
	// 群聊今日按小时计数：返回 talker(群名) -> [24]hour_counts
	GroupTodayHourly(ctx context.Context) (map[string][24]int64, error)
	// 本周(从周一00:00起)群聊消息总数（所有群合计）
	GroupWeekMessageCount(ctx context.Context) (int64, error)
	// 月度趋势（YYYY-MM）：sent/received
	MonthlyTrend(ctx context.Context, months int) ([]model.MonthlyTrend, error)
	// 热力图（小时x星期）：返回 [24][7] 计数（wday: 0=Sunday .. 6=Saturday）
	Heatmap(ctx context.Context) ([24][7]int64, error)
	// 今日按小时聚合（00:00 起），返回 [24] 计数
	GlobalTodayHourly(ctx context.Context) ([24]int64, error)

	// 亲密度基础统计（按联系人/会话聚合）
	IntimacyBase(ctx context.Context) (map[string]*model.IntimacyBase, error)

	// 设置回调函数
	SetCallback(group string, callback func(event fsnotify.Event) error) error

	Close() error
}

// Config holds configuration for creating a DataSource
type Config struct {
	// UseVFS enables direct reading of encrypted databases using VFS
	UseVFS bool
	// DataKey is the hex-encoded decryption key (required when UseVFS is true)
	DataKey string
}

// Option is a functional option for creating a DataSource
type Option func(*Config)

// WithVFS enables VFS mode for direct encrypted database reading
func WithVFS(dataKey string) Option {
	return func(c *Config) {
		if dataKey != "" {
			c.UseVFS = true
			c.DataKey = dataKey
		}
	}
}

func New(path string, platform string, version int, opts ...Option) (DataSource, error) {
	// Apply options
	cfg := &Config{}
	for _, opt := range opts {
		opt(cfg)
	}

	switch {
	case platform == "windows" && version == 3:
		if cfg.UseVFS {
			return windowsv3.New(path, windowsv3.WithVFS(cfg.DataKey, platform, version))
		}
		return windowsv3.New(path)
	case platform == "windows" && version == 4:
		if cfg.UseVFS {
			return v4.New(path, v4.WithVFS(cfg.DataKey, platform, version))
		}
		return v4.New(path)
	case platform == "darwin" && version == 3:
		if cfg.UseVFS {
			return darwinv3.New(path, darwinv3.WithVFS(cfg.DataKey, platform, version))
		}
		return darwinv3.New(path)
	case platform == "darwin" && version == 4:
		if cfg.UseVFS {
			return v4.New(path, v4.WithVFS(cfg.DataKey, platform, version))
		}
		return v4.New(path)
	default:
		return nil, errors.PlatformUnsupported(platform, version)
	}
}