package v4

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/internal/wechatdb/msgstore"
	"github.com/sjzar/chatlog/pkg/util"
)

// MessageDBInfo 存储消息数据库的信息
type MessageDBInfo struct {
	FilePath  string
	StartTime time.Time
	EndTime   time.Time
}

func (ds *DataSource) ListMessageStores(ctx context.Context) ([]*msgstore.Store, error) {
	_ = ctx

	ds.messageStoreMu.RLock()
	defer ds.messageStoreMu.RUnlock()

	stores := make([]*msgstore.Store, len(ds.messageStores))
	for i, store := range ds.messageStores {
		stores[i] = store.Clone()
	}
	return stores, nil
}

func (ds *DataSource) LocateMessageStore(msg *model.Message) (*msgstore.Store, error) {
	if msg == nil {
		return nil, errors.MessageStoreNotFound("nil message")
	}

	talker := strings.TrimSpace(msg.Talker)
	ds.messageStoreMu.RLock()
	defer ds.messageStoreMu.RUnlock()

	if talker != "" {
		hash := md5.Sum([]byte(talker))
		key := hex.EncodeToString(hash[:])
		if path, ok := ds.talkerDBMap[key]; ok {
			if store, exists := ds.messageStoreByPath[path]; exists {
				return store, nil
			}
		}
	}

	ts := msg.Time
	if !ts.IsZero() {
		for _, store := range ds.messageStores {
			if (ts.Equal(store.StartTime) || ts.After(store.StartTime)) && ts.Before(store.EndTime) {
				return store, nil
			}
		}
	}

	if talker == "" {
		talker = "unknown"
	}
	return nil, errors.MessageStoreNotFound(talker)
}

func (ds *DataSource) initMessageDbs() error {
	log.Debug().Msg("initializing message dbs")
	dbPaths, err := ds.dbm.GetDBPath(Message)
	if err != nil {
		if strings.Contains(err.Error(), "db file not found") {
			return nil
		}
		return err
	}

	// 处理每个数据库文件
	infos := make([]MessageDBInfo, 0)
	talkerDBMap := make(map[string]string)
	talkerSets := make(map[string]map[string]struct{})
	for _, filePath := range dbPaths {
		db, err := ds.dbm.OpenDB(filePath)
		if err != nil {
			log.Err(err).Msgf("获取数据库 %s 失败", filePath)
			continue
		}

		talkers := make(map[string]struct{})
		talkerSets[filePath] = talkers

		// 获取 Timestamp 表中的开始时间
		var startTime time.Time
		var timestamp int64

		row := db.QueryRow("SELECT timestamp FROM Timestamp LIMIT 1")
		if err := row.Scan(&timestamp); err != nil {
			log.Err(err).Msgf("获取数据库 %s 的时间戳失败", filePath)
			continue
		}
		startTime = time.Unix(timestamp, 0)

		rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%'")
		if err != nil {
			log.Debug().Err(err).Msgf("数据库 %s 查询 Msg 表失败", filePath)
		} else {
			for rows.Next() {
				var tableName string
				if err := rows.Scan(&tableName); err != nil {
					log.Debug().Err(err).Msgf("数据库 %s 扫描 Msg 表失败", filePath)
					continue
				}

				hash := strings.TrimPrefix(tableName, "Msg_")
				if hash == "" {
					continue
				}
				talkers[hash] = struct{}{}
				talkerDBMap[hash] = filePath
			}
			rows.Close()
		}

		// 保存数据库信息
		infos = append(infos, MessageDBInfo{
			FilePath:  filePath,
			StartTime: startTime,
		})
	}

	// 按照 StartTime 排序数据库文件
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].StartTime.Before(infos[j].StartTime)
	})

	// 设置结束时间
	for i := range infos {
		if i == len(infos)-1 {
			infos[i].EndTime = time.Now().Add(time.Hour)
		} else {
			infos[i].EndTime = infos[i+1].StartTime
		}
	}
	ds.messageStoreMu.RLock()
	currentCount := len(ds.messageStores)
	ds.messageStoreMu.RUnlock()

	if currentCount > 0 && len(infos) < currentCount {
		log.Warn().Msgf("message db count decreased from %d to %d, skip init", currentCount, len(infos))
		return nil
	}
	log.Debug().Int("count", len(infos)).Msg("found message dbs")

	stores := make([]*msgstore.Store, 0, len(infos))
	storeByPath := make(map[string]*msgstore.Store, len(infos))
	for _, info := range infos {
		filename := filepath.Base(info.FilePath)
		id := strings.TrimSuffix(filename, filepath.Ext(filename))
		var talkerMap map[string]struct{}
		if set := talkerSets[info.FilePath]; len(set) > 0 {
			talkerMap = make(map[string]struct{}, len(set))
			for hash := range set {
				talkerMap[hash] = struct{}{}
			}
		}
		store := &msgstore.Store{
			ID:        id,
			FilePath:  info.FilePath,
			FileName:  filename,
			IndexPath: filepath.Join(ds.path, "indexes", "messages", id+".fts.db"),
			StartTime: info.StartTime,
			EndTime:   info.EndTime,
			Talkers:   talkerMap,
		}
		stores = append(stores, store)
		storeByPath[info.FilePath] = store
	}

	ds.messageStoreMu.Lock()
	ds.messageStores = stores
	ds.messageStoreByPath = storeByPath
	ds.messageStoreMu.Unlock()
	ds.talkerDBMap = talkerDBMap
	return nil
}

func (ds *DataSource) GetMessages(ctx context.Context, startTime, endTime time.Time, talker string, sender string, keyword string, limit, offset int) ([]*model.Message, error) {
	log.Debug().Str("talker", talker).Time("start", startTime).Time("end", endTime).Msg("GetMessages request")
	if talker == "" {
		return nil, errors.ErrTalkerEmpty
	}

	// 解析talker参数，支持多个talker（以英文逗号分隔）
	talkers := util.Str2List(talker, ",")
	if len(talkers) == 0 {
		return nil, errors.ErrTalkerEmpty
	}

	// 预计算 talker hashes
	talkerHashes := make(map[string]string)
	for _, t := range talkers {
		hash := md5.Sum([]byte(t))
		talkerHashes[t] = hex.EncodeToString(hash[:])
	}

	// 查找相关的数据库文件
	ds.messageStoreMu.RLock()
	var targetStores []*msgstore.Store
	for _, store := range ds.messageStores {
		// 时间范围检查
		if store.EndTime.Before(startTime) || store.StartTime.After(endTime) {
			continue
		}
		// 检查是否包含请求的 talker
		hasTalker := false
		for _, t := range talkers {
			if _, ok := store.Talkers[talkerHashes[t]]; ok {
				hasTalker = true
				break
			}
		}
		if hasTalker {
			targetStores = append(targetStores, store)
		}
	}
	ds.messageStoreMu.RUnlock()

	log.Debug().Int("dbs", len(targetStores)).Msg("found dbs for time range and talkers")
	if len(targetStores) == 0 {
		return nil, errors.TimeRangeNotFound(startTime, endTime)
	}

	// 解析sender参数，支持多个发送者（以英文逗号分隔）
	senders := util.Str2List(sender, ",")

	// 预编译正则表达式（如果有keyword）
	var regex *regexp.Regexp
	if keyword != "" {
		var err error
		regex, err = regexp.Compile(keyword)
		if err != nil {
			return nil, errors.QueryFailed("invalid regex pattern", err)
		}
	}

	// 从每个相关数据库中查询消息，并在读取时进行过滤
	filteredMessages := []*model.Message{}

	for _, store := range targetStores {
		log.Debug().Str("db", store.FilePath).Msg("querying db")
		// 检查上下文是否已取消
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		db, err := ds.dbm.OpenDB(store.FilePath)
		if err != nil {
			log.Error().Msgf("数据库 %s 未打开", store.FilePath)
			continue
		}

		// 对每个talker进行查询
		for _, talkerItem := range talkers {
			talkerMd5 := talkerHashes[talkerItem]

			// 检查该数据库是否包含该 talker
			if _, ok := store.Talkers[talkerMd5]; !ok {
				continue
			}

			tableName := "Msg_" + talkerMd5

			// 构建查询条件
			conditions := []string{"create_time >= ? AND create_time <= ?"}
			args := []interface{}{startTime.Unix(), endTime.Unix()}

			// 将 sender 过滤下推到 SQL
			if len(senders) > 0 {
				placeholders := make([]string, len(senders))
				for i, s := range senders {
					placeholders[i] = "?"
					args = append(args, s)
				}
				conditions = append(conditions, fmt.Sprintf("n.user_name IN (%s)", strings.Join(placeholders, ",")))
			}

			log.Debug().Msgf("Table name: %s", tableName)
			log.Debug().Msgf("Start time: %d, End time: %d", startTime.Unix(), endTime.Unix())

			query := fmt.Sprintf(`
				SELECT m.sort_seq, m.server_id, m.local_type, n.user_name, m.create_time, m.message_content, m.packed_info_data, m.status
				FROM %s m
				LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
				WHERE %s 
				ORDER BY m.sort_seq ASC
			`, tableName, strings.Join(conditions, " AND "))

			// 执行查询
			rows, err := db.QueryContext(ctx, query, args...)
			if err != nil {
				log.Err(err).Msgf("从数据库 %s 查询消息失败", store.FilePath)
				continue
			}

			// 处理查询结果，在读取时进行过滤
			for rows.Next() {
				var msg model.MessageV4
				err := rows.Scan(
					&msg.SortSeq,
					&msg.ServerID,
					&msg.LocalType,
					&msg.UserName,
					&msg.CreateTime,
					&msg.MessageContent,
					&msg.PackedInfoData,
					&msg.Status,
				)
				if err != nil {
					rows.Close()
					return nil, errors.ScanRowFailed(err)
				}

				// 将消息转换为标准格式
				message := msg.Wrap(talkerItem)

				// 应用keyword过滤
				if regex != nil {
					plainText := message.PlainTextContent()
					if !regex.MatchString(plainText) {
						continue // 不匹配keyword，跳过此消息
					}
				}

				// 通过所有过滤条件，保留此消息
				filteredMessages = append(filteredMessages, message)

				// 检查是否已经满足分页处理数量
				if limit > 0 && len(filteredMessages) >= offset+limit {
					// 已经获取了足够的消息，可以提前返回
					rows.Close()

					log.Debug().Int("count", len(filteredMessages)).Msg("GetMessages result")

					// 对所有消息按时间排序
					sort.Slice(filteredMessages, func(i, j int) bool {
						return filteredMessages[i].Seq < filteredMessages[j].Seq
					})

					// 处理分页
					if offset >= len(filteredMessages) {
						return []*model.Message{}, nil
					}
					end := offset + limit
					if end > len(filteredMessages) {
						end = len(filteredMessages)
					}
					log.Debug().Int("count", len(filteredMessages[offset:end])).Msg("GetMessages result (early exit)")
					return filteredMessages[offset:end], nil
				}
			}
			rows.Close()
		}
	}

	// 对所有消息按时间排序
	sort.Slice(filteredMessages, func(i, j int) bool {
		return filteredMessages[i].Seq < filteredMessages[j].Seq
	})

	// 处理分页
	if limit > 0 {
		if offset >= len(filteredMessages) {
			return []*model.Message{}, nil
		}
		end := offset + limit
		if end > len(filteredMessages) {
			end = len(filteredMessages)
		}
		return filteredMessages[offset:end], nil
	}

	return filteredMessages, nil
}

func (ds *DataSource) IterateMessages(ctx context.Context, talkers []string, handler func(*model.Message) error) error {
	log.Debug().Int("talkers", len(talkers)).Msg("IterateMessages request")
	if handler == nil {
		return errors.InvalidArg("handler")
	}

	if len(talkers) == 0 {
		var err error
		talkers, err = ds.ListTalkers(ctx)
		if err != nil {
			return err
		}
	}
	if len(talkers) == 0 {
		return nil
	}

	talkerHashes := make(map[string]string, len(talkers))
	for _, talker := range talkers {
		hash := md5.Sum([]byte(talker))
		talkerHashes[talker] = hex.EncodeToString(hash[:])
	}

	ds.messageStoreMu.RLock()
	stores := make([]*msgstore.Store, len(ds.messageStores))
	copy(stores, ds.messageStores)
	ds.messageStoreMu.RUnlock()

	for _, store := range stores {
		log.Debug().Str("db", store.FilePath).Msg("iterating db")
		if err := ctx.Err(); err != nil {
			return err
		}

		// Check if any talker is in this store
		hasTalker := false
		for _, talker := range talkers {
			if _, ok := store.Talkers[talkerHashes[talker]]; ok {
				hasTalker = true
				break
			}
		}
		if !hasTalker {
			continue
		}

		db, err := ds.dbm.OpenDB(store.FilePath)
		if err != nil {
			continue
		}

		for _, talker := range talkers {
			if err := ctx.Err(); err != nil {
				return err
			}

			talkerMd5 := talkerHashes[talker]
			if _, ok := store.Talkers[talkerMd5]; !ok {
				continue
			}

			tableName := "Msg_" + talkerMd5

			query := fmt.Sprintf(`
				SELECT m.sort_seq, m.server_id, m.local_type, n.user_name,
				       m.create_time, m.message_content, m.packed_info_data, m.status
				FROM %s AS m
				LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
				ORDER BY m.sort_seq ASC
			`, tableName)

			rows, err := db.QueryContext(ctx, query)
			if err != nil {
				if strings.Contains(err.Error(), "no such table") {
					continue
				}
				return errors.QueryFailed("iterate messages", err)
			}

			for rows.Next() {
				if err := ctx.Err(); err != nil {
					rows.Close()
					return err
				}
				var msg model.MessageV4
				var messageContent []byte
				if scanErr := rows.Scan(
					&msg.SortSeq,
					&msg.ServerID,
					&msg.LocalType,
					&msg.UserName,
					&msg.CreateTime,
					&messageContent,
					&msg.PackedInfoData,
					&msg.Status,
				); scanErr != nil {
					rows.Close()
					return errors.ScanRowFailed(scanErr)
				}
				msg.MessageContent = messageContent
				message := msg.Wrap(talker)
				if err := handler(message); err != nil {
					rows.Close()
					return err
				}
			}
			if err := rows.Err(); err != nil {
				log.Debug().Err(err).Msg("iterate messages failed")
			}
			rows.Close()
		}
	}
	return nil
}