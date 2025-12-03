package v4

import (
	"sync"

	"github.com/fsnotify/fsnotify"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/wechatdb/datasource/dbm"
	"github.com/sjzar/chatlog/internal/wechatdb/msgstore"
)

const (
	Message = "message"
	Contact = "contact"
	Session = "session"
	Media   = "media"
	Voice   = "voice"
)

var Groups = []*dbm.Group{
	{
		Name:      Message,
		Pattern:   `^message_([0-9]?[0-9])?\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Contact,
		Pattern:   `^contact\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Session,
		Pattern:   `session\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Media,
		Pattern:   `^hardlink\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Voice,
		Pattern:   `^media_([0-9]?[0-9])?\.db$`,
		BlackList: []string{},
	},
	{
		Name:      "headimg",
		Pattern:   `^head_image\.db$`,
		BlackList: []string{},
	},
}

type DataSource struct {
	path string
	dbm  *dbm.DBManager

	talkerDBMap        map[string]string
	messageStores      []*msgstore.Store
	messageStoreByPath map[string]*msgstore.Store
	messageStoreMu     sync.RWMutex
}

func New(path string) (*DataSource, error) {
	log.Debug().Str("path", path).Msg("initializing v4 datasource")

	ds := &DataSource{
		path:               path,
		dbm:                dbm.NewDBManager(path),
		talkerDBMap:        make(map[string]string),
		messageStores:      make([]*msgstore.Store, 0),
		messageStoreByPath: make(map[string]*msgstore.Store),
	}

	for _, g := range Groups {
		ds.dbm.AddGroup(g)
	}

	if err := ds.dbm.Start(); err != nil {
		return nil, err
	}

	if err := ds.initMessageDbs(); err != nil {
		return nil, errors.DBInitFailed(err)
	}
	log.Debug().Msg("v4 datasource initialized")

	ds.dbm.AddCallback(Message, func(event fsnotify.Event) error {
		if !event.Op.Has(fsnotify.Create) {
			return nil
		}
		if err := ds.initMessageDbs(); err != nil {
			log.Err(err).Msgf("Failed to reinitialize message DBs: %s", event.Name)
		}
		return nil
	})

	return ds, nil
}

func (ds *DataSource) SetCallback(group string, callback func(event fsnotify.Event) error) error {
	if group == "chatroom" {
		group = Contact
	}
	return ds.dbm.AddCallback(group, callback)
}

func (ds *DataSource) Close() error {
	return ds.dbm.Close()
}