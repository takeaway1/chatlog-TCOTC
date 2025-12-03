package v4

import (
	"context"
	"fmt"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
)

func (ds *DataSource) GetSessions(ctx context.Context, key string, limit, offset int) ([]*model.Session, error) {
	var query string
	var args []interface{}

	if key != "" {
		// 按照关键字查询
		query = `SELECT username, summary, last_timestamp, last_msg_sender, last_sender_display_name 
				FROM SessionTable 
				WHERE username = ? OR last_sender_display_name = ?
				ORDER BY sort_timestamp DESC`
		args = []interface{}{key, key}
	} else {
		// 查询所有会话
		query = `SELECT username, summary, last_timestamp, last_msg_sender, last_sender_display_name 
				FROM SessionTable 
				ORDER BY sort_timestamp DESC`
	}

	// 添加分页
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}

	// 执行查询
	db, err := ds.dbm.GetDB(Session)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.QueryFailed(query, err)
	}
	defer rows.Close()

	sessions := []*model.Session{}
	for rows.Next() {
		var sessionV4 model.SessionV4
		err := rows.Scan(
			&sessionV4.Username,
			&sessionV4.Summary,
			&sessionV4.LastTimestamp,
			&sessionV4.LastMsgSender,
			&sessionV4.LastSenderDisplayName,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		sessions = append(sessions, sessionV4.Wrap())
	}

	return sessions, nil
}