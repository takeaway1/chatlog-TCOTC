package v4

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
)

func (ds *DataSource) ListTalkers(ctx context.Context) ([]string, error) {
	log.Debug().Msg("ListTalkers request")
	talkerSet := make(map[string]struct{})
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := talkerSet[name]; ok {
			return
		}
		talkerSet[name] = struct{}{}
	}

	db, err := ds.dbm.GetDB(Contact)
	if err != nil {
		log.Debug().Err(err).Msg("query contact usernames failed")
	} else if db != nil {
		rows, err := db.QueryContext(ctx, `SELECT username FROM contact WHERE IFNULL(username,'') <> ''`)
		if err == nil {
			for rows.Next() {
				if err := ctx.Err(); err != nil {
					rows.Close()
					return nil, err
				}
				var username string
				if scanErr := rows.Scan(&username); scanErr == nil {
					add(username)
				} else {
					log.Debug().Err(scanErr).Msg("scan contact username failed")
				}
			}
			if err := rows.Err(); err != nil {
				log.Debug().Err(err).Msg("iterate contact usernames failed")
			}
			rows.Close()
		} else {
			log.Debug().Err(err).Msg("query contact usernames failed")
		}

		roomRows, err := db.QueryContext(ctx, `SELECT username FROM chat_room WHERE IFNULL(username,'') <> ''`)
		if err == nil {
			for roomRows.Next() {
				if err := ctx.Err(); err != nil {
					roomRows.Close()
					return nil, err
				}
				var username string
				if scanErr := roomRows.Scan(&username); scanErr == nil {
					add(username)
				} else {
					log.Debug().Err(scanErr).Msg("scan chat_room username failed")
				}
			}
			if err := roomRows.Err(); err != nil {
				log.Debug().Err(err).Msg("iterate chat_room usernames failed")
			}
			roomRows.Close()
		} else {
			log.Debug().Err(err).Msg("query chat_room usernames failed")
		}
	}

	db, err = ds.dbm.GetDB(Session)
	if err != nil {
		log.Debug().Err(err).Msg("query session usernames failed")
	} else if db != nil {
		rows, err := db.QueryContext(ctx, `SELECT username FROM SessionTable WHERE IFNULL(username,'') <> ''`)
		if err == nil {
			for rows.Next() {
				if err := ctx.Err(); err != nil {
					rows.Close()
					return nil, err
				}
				var username string
				if scanErr := rows.Scan(&username); scanErr == nil {
					add(username)
				} else {
					log.Debug().Err(scanErr).Msg("scan session username failed")
				}
			}
			if err := rows.Err(); err != nil {
				log.Debug().Err(err).Msg("iterate session usernames failed")
			}
			rows.Close()
		} else {
			log.Debug().Err(err).Msg("query session usernames failed")
		}
	}

	talkers := make([]string, 0, len(talkerSet))
	for username := range talkerSet {
		talkers = append(talkers, username)
	}
	sort.Strings(talkers)
	log.Debug().Int("count", len(talkers)).Msg("ListTalkers result")
	return talkers, nil
}

func (ds *DataSource) GetContacts(ctx context.Context, key string, limit, offset int) ([]*model.Contact, error) {
	var query string
	var args []interface{}

	if key != "" {
		// 按照关键字查询
		// When searching by key, allow chatrooms to be found (they might be in contact table)
		query = `SELECT username, local_type, flag, alias, remark, nick_name, big_head_url, small_head_url, head_img_md5
				FROM contact
				WHERE username = ? OR alias = ? OR remark = ? OR nick_name = ?`
		args = []interface{}{key, key, key, key}
	} else {
		// 查询所有联系人（排除群聊，避免混淆）
		query = `SELECT username, local_type, flag, alias, remark, nick_name, big_head_url, small_head_url, head_img_md5 FROM contact`
	}

	// 添加排序、分页
	query += ` ORDER BY username`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}

	// 执行查询
	db, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.QueryFailed(query, err)
	}
	defer rows.Close()

	contacts := []*model.Contact{}
	for rows.Next() {
		var contactV4 model.ContactV4
		err := rows.Scan(
			&contactV4.UserName,
			&contactV4.LocalType,
			&contactV4.Flag,
			&contactV4.Alias,
			&contactV4.Remark,
			&contactV4.NickName,
			&contactV4.BigHeadUrl,
			&contactV4.SmallHeadUrl,
			&contactV4.HeadImgMd5,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		contacts = append(contacts, contactV4.Wrap())
	}

	return contacts, nil
}

func (ds *DataSource) GetPinnedUserNames(ctx context.Context) ([]string, error) {
	// Bit 11 (2048) 表示置顶
	query := `SELECT username FROM contact WHERE (flag & 2048) != 0`

	db, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, errors.QueryFailed(query, err)
	}
	defer rows.Close()

	userNames := []string{}
	for rows.Next() {
		var userName string
		if err := rows.Scan(&userName); err != nil {
			return nil, errors.ScanRowFailed(err)
		}
		userNames = append(userNames, userName)
	}

	return userNames, nil
}

// 群聊
func (ds *DataSource) GetChatRooms(ctx context.Context, key string, limit, offset int) ([]*model.ChatRoom, error) {
	var query string
	var args []interface{}

	// 执行查询
	db, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return nil, err
	}

	if key != "" {
		// 按照关键字查询
		query = `SELECT username, owner, ext_buffer FROM chat_room WHERE username = ?`
		args = []interface{}{key}

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, errors.QueryFailed(query, err)
		}
		defer rows.Close()

		chatRooms := []*model.ChatRoom{}
		for rows.Next() {
			var chatRoomV4 model.ChatRoomV4
			err := rows.Scan(
				&chatRoomV4.UserName,
				&chatRoomV4.Owner,
				&chatRoomV4.ExtBuffer,
			)

			if err != nil {
				return nil, errors.ScanRowFailed(err)
			}

			chatRooms = append(chatRooms, chatRoomV4.Wrap())
		}

		// 如果没有找到群聊，尝试通过联系人查找
		if len(chatRooms) == 0 {
			contacts, err := ds.GetContacts(ctx, key, 1, 0)
			if err == nil && len(contacts) > 0 && strings.HasSuffix(contacts[0].UserName, "@chatroom") {
				// 再次尝试通过用户名查找群聊
				rows, err := db.QueryContext(ctx,
					`SELECT username, owner, ext_buffer FROM chat_room WHERE username = ?`,
					contacts[0].UserName)

				if err != nil {
					return nil, errors.QueryFailed(query, err)
				}
				defer rows.Close()

				for rows.Next() {
					var chatRoomV4 model.ChatRoomV4
					err := rows.Scan(
						&chatRoomV4.UserName,
						&chatRoomV4.Owner,
						&chatRoomV4.ExtBuffer,
					)

					if err != nil {
						return nil, errors.ScanRowFailed(err)
					}

					chatRooms = append(chatRooms, chatRoomV4.Wrap())
				}

				// 如果群聊记录不存在，但联系人记录存在，创建一个模拟的群聊对象
				if len(chatRooms) == 0 {
					chatRooms = append(chatRooms, &model.ChatRoom{
						Name:             contacts[0].UserName,
						Users:            make([]model.ChatRoomUser, 0),
						User2DisplayName: make(map[string]string),
					})
				}
			}
		}

		return chatRooms, nil
	} else {
		// 查询所有群聊
		query = `SELECT username, owner, ext_buffer FROM chat_room`

		// 添加排序、分页
		query += ` ORDER BY username`
		if limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", limit)
			if offset > 0 {
				query += fmt.Sprintf(" OFFSET %d", offset)
			}
		}

		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return nil, errors.QueryFailed(query, err)
		}
		defer rows.Close()

		chatRooms := []*model.ChatRoom{}
		for rows.Next() {
			var chatRoomV4 model.ChatRoomV4
			err := rows.Scan(
				&chatRoomV4.UserName,
				&chatRoomV4.Owner,
				&chatRoomV4.ExtBuffer,
			)

			if err != nil {
				return nil, errors.ScanRowFailed(err)
			}

			chatRooms = append(chatRooms, chatRoomV4.Wrap())
		}

		return chatRooms, nil
	}
}