package dbm

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/pkg/filecopy"
	"github.com/sjzar/chatlog/pkg/filemonitor"
	"github.com/sjzar/chatlog/pkg/wechatvfs"
)

// dbEntry holds a database connection along with its associated temp file path (if any).
type dbEntry struct {
	db       *sql.DB
	tempPath string // The temp file path used on Windows, empty on other platforms
}

// Config holds configuration options for DBManager
type Config struct {
	// UseVFS enables direct reading of encrypted databases using VFS
	UseVFS bool
	// DataKey is the hex-encoded decryption key (required when UseVFS is true)
	DataKey string
	// Platform is the platform type (windows, darwin)
	Platform string
	// Version is the WeChat version (3, 4)
	Version int
}

type DBManager struct {
	path    string
	id      string
	fm      *filemonitor.FileMonitor
	fgs     map[string]*filemonitor.FileGroup
	dbs     map[string]*dbEntry
	dbPaths map[string][]string
	mutex   sync.RWMutex

	// VFS support
	useVFS   bool
	dataKey  string
	platform string
	version  int
	vfsInit  sync.Once
	vfsErr   error
}

func NewDBManager(path string, opts ...Option) *DBManager {
	log.Debug().Str("path", path).Msg("dbm: creating new DBManager")
	d := &DBManager{
		path:    path,
		id:      filepath.Base(path),
		fm:      filemonitor.NewFileMonitor(),
		fgs:     make(map[string]*filemonitor.FileGroup),
		dbs:     make(map[string]*dbEntry),
		dbPaths: make(map[string][]string),
	}

	// Apply options
	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Option is a functional option for DBManager
type Option func(*DBManager)

// WithVFS enables VFS mode for direct encrypted database reading
func WithVFS(dataKey string, platform string, version int) Option {
	return func(d *DBManager) {
		if dataKey != "" {
			d.useVFS = true
			d.dataKey = dataKey
			d.platform = platform
			d.version = version
			log.Debug().Str("platform", platform).Int("version", version).Msg("dbm: VFS mode enabled")
		}
	}
}

// WithConfig applies a Config to the DBManager
func WithConfig(cfg *Config) Option {
	return func(d *DBManager) {
		if cfg != nil && cfg.UseVFS && cfg.DataKey != "" {
			d.useVFS = true
			d.dataKey = cfg.DataKey
			d.platform = cfg.Platform
			d.version = cfg.Version
			log.Debug().Str("platform", cfg.Platform).Int("version", cfg.Version).Msg("dbm: VFS mode enabled via config")
		}
	}
}

// initVFS initializes the VFS subsystem (called once)
func (d *DBManager) initVFS() error {
	d.vfsInit.Do(func() {
		if !d.useVFS {
			return
		}
		log.Debug().Msg("dbm: initializing VFS")
		d.vfsErr = wechatvfs.RegisterVFS()
		if d.vfsErr != nil {
			log.Err(d.vfsErr).Msg("dbm: failed to register VFS")
		} else {
			log.Info().Msg("dbm: VFS registered successfully")
		}
	})
	return d.vfsErr
}

func (d *DBManager) AddGroup(g *Group) error {
	log.Debug().Str("group", g.Name).Str("pattern", g.Pattern).Msg("dbm: adding group")
	fg, err := filemonitor.NewFileGroup(g.Name, d.path, g.Pattern, g.BlackList)
	if err != nil {
		log.Debug().Err(err).Msg("dbm: failed to create file group")
		return err
	}
	fg.AddCallback(d.Callback)
	d.fm.AddGroup(fg)
	d.mutex.Lock()
	d.fgs[g.Name] = fg
	d.mutex.Unlock()
	log.Debug().Str("group", g.Name).Msg("dbm: group added successfully")
	return nil
}

func (d *DBManager) AddCallback(group string, callback func(event fsnotify.Event) error) error {
	log.Debug().Str("group", group).Msg("dbm: adding callback")
	d.mutex.RLock()
	fg, ok := d.fgs[group]
	d.mutex.RUnlock()
	if !ok {
		log.Debug().Str("group", group).Msg("dbm: group not found for callback")
		return errors.FileGroupNotFound(group)
	}
	fg.AddCallback(callback)
	log.Debug().Str("group", group).Msg("dbm: callback added successfully")
	return nil
}

func (d *DBManager) GetDB(name string) (*sql.DB, error) {
	log.Debug().Str("name", name).Msg("dbm: GetDB request")
	dbPaths, err := d.GetDBPath(name)
	if err != nil {
		log.Debug().Err(err).Str("name", name).Msg("dbm: GetDBPath failed")
		return nil, err
	}
	log.Debug().Str("path", dbPaths[0]).Msg("dbm: opening first db path")
	return d.OpenDB(dbPaths[0])
}

func (d *DBManager) GetDBs(name string) ([]*sql.DB, error) {
	log.Debug().Str("name", name).Msg("dbm: GetDBs request")
	dbPaths, err := d.GetDBPath(name)
	if err != nil {
		log.Debug().Err(err).Str("name", name).Msg("dbm: GetDBPath failed")
		return nil, err
	}
	log.Debug().Int("count", len(dbPaths)).Msg("dbm: found db paths")
	dbs := make([]*sql.DB, 0)
	for _, file := range dbPaths {
		db, err := d.OpenDB(file)
		if err != nil {
			log.Debug().Err(err).Str("file", file).Msg("dbm: OpenDB failed")
			return nil, err
		}
		dbs = append(dbs, db)
	}
	return dbs, nil
}

func (d *DBManager) GetDBPath(name string) ([]string, error) {
	log.Debug().Str("name", name).Msg("dbm: GetDBPath request")
	d.mutex.RLock()
	dbPaths, ok := d.dbPaths[name]
	d.mutex.RUnlock()
	if !ok {
		log.Debug().Str("name", name).Msg("dbm: cache miss for db paths")
		d.mutex.RLock()
		fg, ok := d.fgs[name]
		d.mutex.RUnlock()
		if !ok {
			log.Debug().Str("name", name).Msg("dbm: group not found")
			return nil, errors.FileGroupNotFound(name)
		}
		list, err := fg.List()
		if err != nil {
			log.Debug().Err(err).Str("pattern", fg.PatternStr).Msg("dbm: list files failed")
			return nil, errors.DBFileNotFound(d.path, fg.PatternStr, err)
		}
		log.Debug().Int("count", len(list)).Msg("dbm: found files")
		if len(list) == 0 {
			return nil, errors.DBFileNotFound(d.path, fg.PatternStr, nil)
		}
		dbPaths = list
		d.mutex.Lock()
		d.dbPaths[name] = dbPaths
		d.mutex.Unlock()
	} else {
		log.Debug().Str("name", name).Int("count", len(dbPaths)).Msg("dbm: cache hit for db paths")
	}
	return dbPaths, nil
}

func (d *DBManager) OpenDB(path string) (*sql.DB, error) {
	log.Debug().Str("path", path).Bool("useVFS", d.useVFS).Msg("dbm: OpenDB request")
	d.mutex.RLock()
	entry, ok := d.dbs[path]
	d.mutex.RUnlock()
	if ok {
		log.Debug().Str("path", path).Msg("dbm: cache hit for db connection")
		return entry.db, nil
	}
	log.Debug().Str("path", path).Msg("dbm: cache miss for db connection, opening new")

	// Choose opening method based on VFS mode
	if d.useVFS {
		return d.openDBWithVFS(path)
	}
	return d.openDBWithCopy(path)
}

// openDBWithVFS opens a database using the WeChat VFS for direct encrypted reading
func (d *DBManager) openDBWithVFS(path string) (*sql.DB, error) {
	// Initialize VFS on first use
	if err := d.initVFS(); err != nil {
		log.Err(err).Msg("dbm: VFS initialization failed, falling back to copy mode")
		return d.openDBWithCopy(path)
	}

	// Register the key for this database path with platform and version
	wechatvfs.RegisterKeyWithParams(path, d.dataKey, d.platform, d.version)

	// Open database using VFS
	// Format: file:路径?vfs=wechat&mode=ro
	dsn := fmt.Sprintf("file:%s?vfs=wechat&mode=ro", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		log.Err(err).Str("path", path).Msg("dbm: VFS open failed, falling back to copy mode")
		wechatvfs.UnregisterKey(path)
		return d.openDBWithCopy(path)
	}

	// Test the connection to verify decryption works
	if err := db.Ping(); err != nil {
		log.Err(err).Str("path", path).Msg("dbm: VFS connection test failed, falling back to copy mode")
		db.Close()
		wechatvfs.UnregisterKey(path)
		return d.openDBWithCopy(path)
	}

	entry := &dbEntry{
		db:       db,
		tempPath: "", // No temp file in VFS mode
	}
	d.mutex.Lock()
	d.dbs[path] = entry
	d.mutex.Unlock()
	log.Debug().Str("path", path).Msg("dbm: db opened successfully via VFS")
	return db, nil
}

// openDBWithCopy opens a database using the traditional copy method
func (d *DBManager) openDBWithCopy(path string) (*sql.DB, error) {
	var err error
	tempPath := path
	if runtime.GOOS == "windows" {
		tempPath, err = filecopy.GetTempCopy(path)
		if err != nil {
			log.Err(err).Msgf("获取临时拷贝文件 %s 失败", path)
			return nil, err
		}
		log.Debug().Str("original", path).Str("temp", tempPath).Msg("dbm: using temp copy")
	}
	db, err := sql.Open("sqlite3", tempPath)
	if err != nil {
		log.Err(err).Msgf("连接数据库 %s 失败", path)
		return nil, err
	}
	entry := &dbEntry{
		db:       db,
		tempPath: tempPath,
	}
	d.mutex.Lock()
	d.dbs[path] = entry
	d.mutex.Unlock()
	log.Debug().Str("path", path).Msg("dbm: db opened successfully via copy")
	return db, nil
}

func (d *DBManager) Callback(event fsnotify.Event) error {
	log.Debug().Str("event", event.String()).Msg("dbm: file event callback")
	if !event.Op.Has(fsnotify.Create) {
		log.Debug().Str("op", event.Op.String()).Msg("dbm: ignoring non-create event")
		return nil
	}

	d.mutex.Lock()
	entry, ok := d.dbs[event.Name]
	if ok {
		log.Debug().Str("file", event.Name).Msg("dbm: closing stale db connection")
		delete(d.dbs, event.Name)
		go func(entry *dbEntry, path string, useVFS bool) {
			time.Sleep(time.Second * 5)
			entry.db.Close()
			log.Debug().Msg("dbm: stale db connection closed")
			// Cleanup based on mode
			if useVFS {
				wechatvfs.UnregisterKey(path)
			} else if entry.tempPath != "" && entry.tempPath != path {
				filecopy.NotifyFileReleased(entry.tempPath)
			}
		}(entry, event.Name, d.useVFS)
	} else {
		log.Debug().Str("file", event.Name).Msg("dbm: no stale db connection found")
	}
	d.mutex.Unlock()

	return nil
}

func (d *DBManager) Start() error {
	log.Debug().Msg("dbm: starting file monitor")
	return d.fm.Start()
}

func (d *DBManager) Stop() error {
	log.Debug().Msg("dbm: stopping file monitor")
	return d.fm.Stop()
}

func (d *DBManager) Close() error {
	log.Debug().Msg("dbm: closing DBManager")
	for path, entry := range d.dbs {
		entry.db.Close()
		// Cleanup based on mode
		if d.useVFS {
			wechatvfs.UnregisterKey(path)
		} else if entry.tempPath != "" {
			filecopy.NotifyFileReleased(entry.tempPath)
		}
	}
	return d.fm.Stop()
}

// IsVFSMode returns whether the DBManager is using VFS mode
func (d *DBManager) IsVFSMode() bool {
	return d.useVFS
}