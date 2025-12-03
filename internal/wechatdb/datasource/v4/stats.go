package v4

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzar/chatlog/internal/model"
)

// GlobalMessageStats 聚合统计（Windows/Darwin v4）
func (ds *DataSource) GlobalMessageStats(ctx context.Context) (*model.GlobalMessageStats, error) {
	stats := &model.GlobalMessageStats{ByType: make(map[string]int64)}
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return stats, nil
	}
	for _, db := range dbs {
		// 列举所有 Msg_ 前缀表
		trows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%'`)
		if err != nil {
			continue
		}
		var tables []string
		for trows.Next() {
			var name string
			if err := trows.Scan(&name); err == nil {
				tables = append(tables, name)
			}
		}
		trows.Close()
		for _, tbl := range tables {
			// total/sent/min/max
			q := fmt.Sprintf(`SELECT COUNT(*) AS total,
				SUM(CASE WHEN status=2 THEN 1 ELSE 0 END) AS sent,
				MIN(create_time) AS minct,
				MAX(create_time) AS maxct FROM %s`, tbl)
			row := db.QueryRowContext(ctx, q)
			var total, sent, minct, maxct int64
			if err := row.Scan(&total, &sent, &minct, &maxct); err == nil {
				stats.Total += total
				stats.Sent += sent
				stats.Received += (total - sent)
				if stats.EarliestUnix == 0 || (minct > 0 && minct < stats.EarliestUnix) {
					stats.EarliestUnix = minct
				}
				if maxct > stats.LatestUnix {
					stats.LatestUnix = maxct
				}
			}
			// by type (细分 49)
			// 先统计除 49 之外类型
			q2 := fmt.Sprintf(`SELECT local_type, COUNT(*) FROM %s WHERE local_type != 49 GROUP BY local_type`, tbl)
			rows, err := db.QueryContext(ctx, q2)
			if err == nil {
				for rows.Next() {
					var t int64
					var cnt int64
					if err := rows.Scan(&t, &cnt); err == nil {
						label := mapV4TypeToLabel(t)
						if label != "" {
							stats.ByType[label] += cnt
						}
					}
				}
				rows.Close()
			}
			// 针对 49 类型再做细分：简单解析 message_content 判断是文件、链接或通用 XML
			q49 := fmt.Sprintf(`SELECT message_content FROM %s WHERE local_type = 49`, tbl)
			orows, err := db.QueryContext(ctx, q49)
			if err == nil {
				for orows.Next() {
					var mc []byte
					if err := orows.Scan(&mc); err == nil {
						content := string(mc)
						// 可能压缩，简单特征判断（保持轻量；深度解压需额外性能，可后续拓展）
						lc := strings.ToLower(content)
						if strings.Contains(lc, "<appmsg") {
							if strings.Contains(lc, "<type>") && strings.Contains(lc, "</type>") {
								// 简单提取 type 数字
								i1 := strings.Index(lc, "<type>")
								i2 := strings.Index(lc[i1+6:], "</type>")
								if i1 >= 0 && i2 > 0 {
									val := lc[i1+6 : i1+6+i2]
									// 常见：6=文件, 5/33=链接(网页), 3=音乐, 4=视频, 其他归类为 XML
									if strings.TrimSpace(val) == "6" {
										stats.ByType["文件消息"]++
										continue
									}
									if strings.TrimSpace(val) == "5" || strings.TrimSpace(val) == "33" {
										stats.ByType["链接消息"]++
										continue
									}
								}
							}
							// 兜底：若包含 url 或 http(s) 关键词也认为链接
							if strings.Contains(lc, "http://") || strings.Contains(lc, "https://") {
								stats.ByType["链接消息"]++
								continue
							}
							// 再兜底为 XML消息
							stats.ByType["XML消息"]++
						}
					}
				}
				orows.Close()
			}
		}
	}
	return stats, nil
}

// GroupMessageCounts 统计群聊消息数量（v4）：通过 chat_room.username 计算 md5 映射到 Msg_ 表
func (ds *DataSource) GroupMessageCounts(ctx context.Context) (map[string]int64, error) {
	result := make(map[string]int64)
	cdb, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return result, nil
	}
	// 获取所有群聊用户名
	urows, err := cdb.QueryContext(ctx, `SELECT username FROM chat_room`)
	if err != nil {
		return result, nil
	}
	var rooms []string
	for urows.Next() {
		var u string
		if err := urows.Scan(&u); err == nil {
			rooms = append(rooms, u)
		}
	}
	urows.Close()
	if len(rooms) == 0 {
		return result, nil
	}
	// 遍历消息库
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return result, nil
	}
	for _, db := range dbs {
		for _, uname := range rooms {
			md5sum := md5.Sum([]byte(uname))
			tbl := "Msg_" + hex.EncodeToString(md5sum[:])
			// 检查表是否存在
			var name string
			err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
			if err != nil {
				continue
			}
			// 计数
			q := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, tbl)
			var cnt int64
			if err := db.QueryRowContext(ctx, q).Scan(&cnt); err == nil {
				result[uname] += cnt
			}
		}
	}
	return result, nil
}

// MonthlyTrend 返回每月 sent/received（按 create_time 聚合）
func (ds *DataSource) MonthlyTrend(ctx context.Context, months int) ([]model.MonthlyTrend, error) {
	agg := make(map[string][2]int64)
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return []model.MonthlyTrend{}, nil
	}
	for _, db := range dbs {
		trows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%'`)
		if err != nil {
			continue
		}
		var tables []string
		for trows.Next() {
			var name string
			if err := trows.Scan(&name); err == nil {
				tables = append(tables, name)
			}
		}
		trows.Close()
		for _, tbl := range tables {
			q := fmt.Sprintf(`SELECT strftime('%%Y-%%m', datetime(create_time, 'unixepoch')) AS ym,
				SUM(CASE WHEN status=2 THEN 1 ELSE 0 END) AS sent,
				SUM(CASE WHEN status!=2 THEN 1 ELSE 0 END) AS recv
				FROM %s GROUP BY ym ORDER BY ym`, tbl)
			rows, err := db.QueryContext(ctx, q)
			if err != nil {
				continue
			}
			for rows.Next() {
				var ym string
				var sent, recv int64
				if err := rows.Scan(&ym, &sent, &recv); err == nil {
					cur := agg[ym]
					cur[0] += sent
					cur[1] += recv
					agg[ym] = cur
				}
			}
			rows.Close()
		}
	}
	// 排序输出
	keys := make([]string, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	trends := make([]model.MonthlyTrend, 0, len(keys))
	for _, k := range keys {
		v := agg[k]
		trends = append(trends, model.MonthlyTrend{Date: k, Sent: v[0], Received: v[1]})
	}
	return trends, nil
}

// Heatmap 小时x星期（wday: 0=Sunday..6）
func (ds *DataSource) Heatmap(ctx context.Context) ([24][7]int64, error) {
	var grid [24][7]int64
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return grid, nil
	}
	for _, db := range dbs {
		trows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%'`)
		if err != nil {
			continue
		}
		var tables []string
		for trows.Next() {
			var name string
			if err := trows.Scan(&name); err == nil {
				tables = append(tables, name)
			}
		}
		trows.Close()
		for _, tbl := range tables {
			q := fmt.Sprintf(`SELECT CAST(strftime('%%H', datetime(create_time,'unixepoch')) AS INTEGER) AS h,
				CAST(strftime('%%w', datetime(create_time,'unixepoch')) AS INTEGER) AS d,
				COUNT(*) FROM %s GROUP BY h,d`, tbl)
			rows, err := db.QueryContext(ctx, q)
			if err != nil {
				continue
			}
			for rows.Next() {
				var h, d int
				var cnt int64
				if err := rows.Scan(&h, &d, &cnt); err == nil {
					if h >= 0 && h < 24 && d >= 0 && d < 7 {
						grid[h][d] += cnt
					}
				}
			}
			rows.Close()
		}
	}
	return grid, nil
}

// IntimacyBase 统计按联系人（非群聊）聚合的亲密度基础数据（v4）
func (ds *DataSource) IntimacyBase(ctx context.Context) (map[string]*model.IntimacyBase, error) {
	result := make(map[string]*model.IntimacyBase)

	// 构建 md5->username 映射（来自 contact 表）
	md5ToUser := make(map[string]string)
	if cdb, err := ds.dbm.GetDB(Contact); err == nil && cdb != nil {
		rows, err := cdb.QueryContext(ctx, `SELECT username FROM contact`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var uname string
				if rows.Scan(&uname) == nil && uname != "" {
					sum := md5.Sum([]byte(uname))
					md5ToUser[hex.EncodeToString(sum[:])] = uname
				}
			}
		}
	}

	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return result, nil
	}

	// 列出所有 Msg_% 表并求全局最大时间
	var maxCT int64
	type tbl struct {
		db   *sql.DB
		name string
	}
	var tables []tbl
	for _, db := range dbs {
		rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%'`)
		if err == nil {
			for rows.Next() {
				var name string
				if rows.Scan(&name) == nil {
					tables = append(tables, tbl{db: db, name: name})
				}
			}
			rows.Close()
		}
	}
	for _, t := range tables {
		row := t.db.QueryRowContext(ctx, `SELECT MAX(create_time) FROM `+t.name)
		var v sql.NullInt64
		if row.Scan(&v) == nil && v.Valid && v.Int64 > maxCT {
			maxCT = v.Int64
		}
	}
	if maxCT == 0 {
		return result, nil
	}
	since90 := maxCT - 90*86400
	since7 := maxCT - 7*86400

	// 按表（即按 talker）聚合
	for _, t := range tables {
		// 从表名提取 md5 并映射成 username
		if !strings.HasPrefix(t.name, "Msg_") {
			continue
		}
		md5hex := strings.TrimPrefix(t.name, "Msg_")
		talker := md5ToUser[md5hex]
		if talker == "" {
			continue
		}
		if strings.HasSuffix(talker, "@chatroom") {
			continue
		}

		// total, sent, min, max
		row := t.db.QueryRowContext(ctx, `SELECT COUNT(*), SUM(CASE WHEN status=2 THEN 1 ELSE 0 END), MIN(create_time), MAX(create_time) FROM `+t.name)
		var total, sent, minct, maxct sql.NullInt64
		if row.Scan(&total, &sent, &minct, &maxct) == nil {
			base := result[talker]
			if base == nil {
				base = &model.IntimacyBase{UserName: talker}
				result[talker] = base
			}
			if total.Valid {
				base.MsgCount += total.Int64
				base.ReceivedCount += (total.Int64 - func() int64 {
					if sent.Valid {
						return sent.Int64
					}
					return 0
				}())
			}
			if sent.Valid {
				base.SentCount += sent.Int64
			}
			if minct.Valid {
				if base.MinCreateUnix == 0 || minct.Int64 < base.MinCreateUnix {
					base.MinCreateUnix = minct.Int64
				}
			}
			if maxct.Valid {
				if maxct.Int64 > base.MaxCreateUnix {
					base.MaxCreateUnix = maxct.Int64
				}
			}
		}

		// 活跃天数
		row2 := t.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT date(datetime(create_time,'unixepoch'))) FROM `+t.name)
		var days sql.NullInt64
		if row2.Scan(&days) == nil && days.Valid {
			base := result[talker]
			if base == nil {
				base = &model.IntimacyBase{UserName: talker}
				result[talker] = base
			}
			base.MessagingDays += days.Int64
		}

		// 最近90天消息数
		row3 := t.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+t.name+` WHERE create_time>=?`, since90)
		var c90 sql.NullInt64
		if row3.Scan(&c90) == nil && c90.Valid {
			base := result[talker]
			if base == nil {
				base = &model.IntimacyBase{UserName: talker}
				result[talker] = base
			}
			base.Last90DaysMsg += c90.Int64
		}

		// 过去7天发送
		row4 := t.db.QueryRowContext(ctx, `SELECT SUM(CASE WHEN status=2 THEN 1 ELSE 0 END) FROM `+t.name+` WHERE create_time>=?`, since7)
		var s7 sql.NullInt64
		if row4.Scan(&s7) == nil && s7.Valid {
			base := result[talker]
			if base == nil {
				base = &model.IntimacyBase{UserName: talker}
				result[talker] = base
			}
			base.Past7DaysSentMsg += s7.Int64
		}
	}

	return result, nil
}

func mapV4TypeToLabel(t int64) string {
	// 依据文档统一的消息类型映射
	switch t {
	case 1:
		return "文本消息"
	case 3:
		return "图片消息"
	case 34:
		return "语音消息"
	case 37:
		return "好友验证消息"
	case 42:
		return "好友推荐消息"
	case 47:
		return "聊天表情"
	case 48:
		return "位置消息"
	case 49:
		return "XML消息"
	case 50:
		return "音视频通话"
	case 51:
		return "手机端操作消息"
	case 10000:
		return "系统通知"
	case 10002:
		return "撤回消息"
	default:
		return ""
	}
}

// GroupTodayMessageCounts 统计群聊今日消息数（v4）：通过 chat_room.username 计算 md5 映射到 Msg_ 表，create_time >= 今日零点
func (ds *DataSource) GroupTodayMessageCounts(ctx context.Context) (map[string]int64, error) {
	result := make(map[string]int64)
	cdb, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return result, nil
	}
	// 获取所有群聊用户名
	urows, err := cdb.QueryContext(ctx, `SELECT username FROM chat_room`)
	if err != nil {
		return result, nil
	}
	var rooms []string
	for urows.Next() {
		var u string
		if err := urows.Scan(&u); err == nil {
			rooms = append(rooms, u)
		}
	}
	urows.Close()
	if len(rooms) == 0 {
		return result, nil
	}
	// 今日零点
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	since := today.Unix()
	// 遍历消息库
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return result, nil
	}
	for _, db := range dbs {
		for _, uname := range rooms {
			md5sum := md5.Sum([]byte(uname))
			tbl := "Msg_" + hex.EncodeToString(md5sum[:])
			// 检查表是否存在
			var name string
			err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
			if err != nil {
				continue
			}
			// 计数
			q := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE create_time >= ?`, tbl)
			var cnt int64
			if err := db.QueryRowContext(ctx, q, since).Scan(&cnt); err == nil {
				result[uname] += cnt
			}
		}
	}
	return result, nil
}

// GroupTodayHourly 统计群聊今日按小时消息数（v4）
func (ds *DataSource) GroupTodayHourly(ctx context.Context) (map[string][24]int64, error) {
	result := make(map[string][24]int64)
	cdb, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return result, nil
	}
	urows, err := cdb.QueryContext(ctx, `SELECT username FROM chat_room`)
	if err != nil {
		return result, nil
	}
	var rooms []string
	for urows.Next() {
		var u string
		if urows.Scan(&u) == nil {
			rooms = append(rooms, u)
		}
	}
	urows.Close()
	if len(rooms) == 0 {
		return result, nil
	}
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return result, nil
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	end := start + 86400
	for _, db := range dbs {
		for _, uname := range rooms {
			md5sum := md5.Sum([]byte(uname))
			tbl := "Msg_" + hex.EncodeToString(md5sum[:])
			var name string
			if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name); err != nil {
				continue
			}
			q := fmt.Sprintf(`SELECT CAST(strftime('%%H', datetime(create_time,'unixepoch')) AS INTEGER) AS h, COUNT(*) FROM %s WHERE create_time >= ? AND create_time < ? GROUP BY h`, tbl)
			rows, err := db.QueryContext(ctx, q, start, end)
			if err != nil {
				continue
			}
			for rows.Next() {
				var hour int
				var cnt int64
				if rows.Scan(&hour, &cnt) == nil {
					if hour >= 0 && hour < 24 {
						bucket := result[uname]
						bucket[hour] += cnt
						result[uname] = bucket
					}
				}
			}
			rows.Close()
		}
	}
	return result, nil
}

// GroupWeekMessageCount 统计本周(周一00:00起至当前)所有群聊消息总数
// 复用 GroupMessageCounts + 时间过滤会很重，这里直接遍历相关 Msg_ 表做时间范围聚合
func (ds *DataSource) GroupWeekMessageCount(ctx context.Context) (int64, error) {
	var total int64
	cdb, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return 0, nil
	}
	// 群列表
	urows, err := cdb.QueryContext(ctx, `SELECT username FROM chat_room`)
	if err != nil {
		return 0, nil
	}
	var rooms []string
	for urows.Next() {
		var u string
		if urows.Scan(&u) == nil {
			rooms = append(rooms, u)
		}
	}
	urows.Close()
	if len(rooms) == 0 {
		return 0, nil
	}
	now := time.Now()
	// 计算周一 00:00
	wday := int(now.Weekday()) // Sunday=0
	// 以周一为起点，若是周日(0)则回退6天
	offset := wday - 1
	if wday == 0 {
		offset = -6
	}
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -offset)
	since := monday.Unix()
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return 0, nil
	}
	for _, db := range dbs {
		for _, uname := range rooms {
			md5sum := md5.Sum([]byte(uname))
			tbl := "Msg_" + hex.EncodeToString(md5sum[:])
			var name string
			if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name); err != nil {
				continue
			}
			q := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE create_time >= ?`, tbl)
			var cnt int64
			if err := db.QueryRowContext(ctx, q, since).Scan(&cnt); err == nil {
				total += cnt
			}
		}
	}
	return total, nil
}

// GlobalTodayHourly 返回今日(本地时区)每小时全部消息量（v4）
func (ds *DataSource) GlobalTodayHourly(ctx context.Context) ([24]int64, error) {
	var hours [24]int64
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return hours, nil
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	end := start + 86400
	for _, db := range dbs {
		trows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%'`)
		if err != nil {
			continue
		}
		var tables []string
		for trows.Next() {
			var name string
			if trows.Scan(&name) == nil {
				tables = append(tables, name)
			}
		}
		trows.Close()
		for _, tbl := range tables {
			q := fmt.Sprintf(`SELECT CAST(strftime('%%H', datetime(create_time,'unixepoch')) AS INTEGER) AS h, COUNT(*) FROM %s WHERE create_time >= ? AND create_time < ? GROUP BY h`, tbl)
			rows, err := db.QueryContext(ctx, q, start, end)
			if err != nil {
				continue
			}
			for rows.Next() {
				var h int
				var cnt int64
				if rows.Scan(&h, &cnt) == nil {
					if h >= 0 && h < 24 {
						hours[h] += cnt
					}
				}
			}
			rows.Close()
		}
	}
	return hours, nil
}

// GroupMessageTypeStats 统计群聊消息类型分布（v4）
func (ds *DataSource) GroupMessageTypeStats(ctx context.Context) (map[string]int64, error) {
	result := make(map[string]int64)
	cdb, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return result, nil
	}
	urows, err := cdb.QueryContext(ctx, `SELECT username FROM chat_room`)
	if err != nil {
		return result, nil
	}
	var rooms []string
	for urows.Next() {
		var u string
		if urows.Scan(&u) == nil {
			rooms = append(rooms, u)
		}
	}
	urows.Close()
	if len(rooms) == 0 {
		return result, nil
	}
	dbs, err := ds.dbm.GetDBs(Message)
	if err != nil {
		return result, nil
	}
	for _, db := range dbs {
		for _, uname := range rooms {
			md5sum := md5.Sum([]byte(uname))
			tbl := "Msg_" + hex.EncodeToString(md5sum[:])
			// 先统计非49
			q := fmt.Sprintf(`SELECT local_type, COUNT(*) FROM %s WHERE local_type != 49 GROUP BY local_type`, tbl)
			rows, err := db.QueryContext(ctx, q)
			if err == nil {
				for rows.Next() {
					var t int64
					var cnt int64
					if rows.Scan(&t, &cnt) == nil {
						label := mapV4TypeToLabel(t)
						if label != "" {
							result[label] += cnt
						}
					}
				}
				rows.Close()
			}
			// 处理49
			q49 := fmt.Sprintf(`SELECT message_content FROM %s WHERE local_type=49`, tbl)
			orows, err := db.QueryContext(ctx, q49)
			if err == nil {
				for orows.Next() {
					var mc []byte
					if err := orows.Scan(&mc); err == nil {
						lc := strings.ToLower(string(mc))
						if strings.Contains(lc, "<appmsg") {
							if strings.Contains(lc, "<type>") && strings.Contains(lc, "</type>") {
								i1 := strings.Index(lc, "<type>")
								i2 := strings.Index(lc[i1+6:], "</type>")
								if i1 >= 0 && i2 > 0 {
									val := strings.TrimSpace(lc[i1+6 : i1+6+i2])
									if val == "6" {
										result["文件消息"]++
										continue
									}
									if val == "5" || val == "33" {
										result["链接消息"]++
										continue
									}
								}
							}
						}
						if strings.Contains(lc, "http://") || strings.Contains(lc, "https://") {
							result["链接消息"]++
							continue
						}
						result["XML消息"]++
					}
				}
				orows.Close()
			}
		}
	}
	return result, nil
}