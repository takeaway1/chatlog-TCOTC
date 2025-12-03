package dbm

import (
	"database/sql"
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
)

type DBManager struct {
	path    string
	id      string
	fm      *filemonitor.FileMonitor
	fgs     map[string]*filemonitor.FileGroup
	dbs     map[string]*sql.DB
	dbPaths map[string][]string
	mutex   sync.RWMutex
}

func NewDBManager(path string) *DBManager {
	log.Debug().Str("path", path).Msg("dbm: creating new DBManager")
	return &DBManager{
		path:    path,
		id:      filepath.Base(path),
		fm:      filemonitor.NewFileMonitor(),
		fgs:     make(map[string]*filemonitor.FileGroup),
		dbs:     make(map[string]*sql.DB),
		dbPaths: make(map[string][]string),
	}
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
	log.Debug().Str("path", path).Msg("dbm: OpenDB request")
	d.mutex.RLock()
	db, ok := d.dbs[path]
	d.mutex.RUnlock()
	if ok {
		log.Debug().Str("path", path).Msg("dbm: cache hit for db connection")
		return db, nil
	}
	log.Debug().Str("path", path).Msg("dbm: cache miss for db connection, opening new")
	var err error
	tempPath := path
	if runtime.GOOS == "windows" {
		tempPath, err = filecopy.GetTempCopy(d.id, path)
		if err != nil {
			log.Err(err).Msgf("获取临时拷贝文件 %s 失败", path)
			return nil, err
		}
		log.Debug().Str("original", path).Str("temp", tempPath).Msg("dbm: using temp copy")
	}
	db, err = sql.Open("sqlite3", tempPath)
	if err != nil {
		log.Err(err).Msgf("连接数据库 %s 失败", path)
		return nil, err
	}
	d.mutex.Lock()
	d.dbs[path] = db
	d.mutex.Unlock()
	log.Debug().Str("path", path).Msg("dbm: db opened successfully")
	return db, nil
}

func (d *DBManager) Callback(event fsnotify.Event) error {
	log.Debug().Str("event", event.String()).Msg("dbm: file event callback")
	if !event.Op.Has(fsnotify.Create) {
		log.Debug().Str("op", event.Op.String()).Msg("dbm: ignoring non-create event")
		return nil
	}

	d.mutex.Lock()
	db, ok := d.dbs[event.Name]
	if ok {
		log.Debug().Str("file", event.Name).Msg("dbm: closing stale db connection")
		delete(d.dbs, event.Name)
		go func(db *sql.DB) {
			time.Sleep(time.Second * 5)
			db.Close()
			log.Debug().Msg("dbm: stale db connection closed")
		}(db)
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
	for _, db := range d.dbs {
		db.Close()
	}
	return d.fm.Stop()
}
