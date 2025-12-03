package http

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/model"
)

// Dashboard 数据结构定义
type DBStats struct {
	DbSizeMB  float64 `json:"db_size_mb"`
	DirSizeMB float64 `json:"dir_size_mb"`
}

type MsgStats struct {
	TotalMsgs      int64 `json:"total_msgs"`
	SentMsgs       int64 `json:"sent_msgs"`
	ReceivedMsgs   int64 `json:"received_msgs"`
	UniqueMsgTypes int   `json:"unique_msg_types"`
}

type OverviewGroup struct {
	ChatRoomName string `json:"ChatRoomName"`
	NickName     string `json:"NickName"`
	MemberCount  int    `json:"member_count"`
	MessageCount int64  `json:"message_count"`
}

type Timeline struct {
	Earliest int64 `json:"earliest_msg_time"`
	Latest   int64 `json:"latest_msg_time"`
	Duration int   `json:"duration_days"`
}

type Migration struct {
	ID        int    `json:"id"`
	File      string `json:"file"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Overview struct {
	User       string           `json:"user"`
	DBStats    DBStats          `json:"dbStats"`
	MsgStats   MsgStats         `json:"msgStats"`
	MsgTypes   map[string]int64 `json:"msgTypes"`
	Groups     []OverviewGroup  `json:"groups"`
	Timeline   Timeline         `json:"timeline"`
	Migrations []Migration      `json:"migrations"`
}

type GroupOverview struct {
	TotalGroups    int    `json:"total_groups"`
	ActiveGroups   int    `json:"active_groups"`
	TodayMessages  int    `json:"today_messages"`
	WeeklyAvg      int    `json:"weekly_avg"`
	MostActiveHour string `json:"most_active_hour"`
}

type ContentAnalysis struct {
	Text   int64 `json:"text_messages"`
	Images int64 `json:"images"`
	Voice  int64 `json:"voice_messages"`
	Files  int64 `json:"files"`
	Links  int64 `json:"links"`
	Others int64 `json:"others"`
}

type GroupListItem struct {
	Name     string `json:"name"`
	Members  int    `json:"members"`
	Messages int64  `json:"messages"`
	Active   bool   `json:"active"`
}

type GroupAnalysis struct {
	Title           string          `json:"title"`
	Overview        GroupOverview   `json:"overview"`
	ContentAnalysis ContentAnalysis `json:"content_analysis"`
	GroupList       []GroupListItem `json:"group_list"`
}

type ContentTypeStats struct {
	Count      int64    `json:"count"`
	Percentage float64  `json:"percentage"`
	SizeMB     *float64 `json:"size_mb,omitempty"`
	Trend      *string  `json:"trend,omitempty"`
}

type SourceChannel struct {
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

type ProcessingStatus struct {
	Processed  int `json:"processed"`
	Processing int `json:"processing"`
	Pending    int `json:"pending"`
}

type QualityMetrics struct {
	DataIntegrity          float64 `json:"data_integrity"`
	ClassificationAccuracy float64 `json:"classification_accuracy"`
	DuplicateRate          float64 `json:"duplicate_rate"`
	ErrorRate              float64 `json:"error_rate"`
}

type DataTypeAnalysis struct {
	Title            string                      `json:"title"`
	ContentTypes     map[string]ContentTypeStats `json:"content_types"`
	SourceChannels   map[string]SourceChannel    `json:"source_channels"`
	ProcessingStatus ProcessingStatus            `json:"processing_status"`
	QualityMetrics   QualityMetrics              `json:"quality_metrics"`
	PieGradient      string                      `json:"pieGradient,omitempty"`
}

type VisualizationDefaults struct {
	SelectedGroupIndex int `json:"selectedGroupIndex"`
}

type RelationshipNode struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Messages int64  `json:"messages"`
	Avatar   string `json:"avatar,omitempty"`
}

type RelationshipNetwork struct {
	Nodes []RelationshipNode `json:"nodes"`
}

type Visualization struct {
	Defaults            VisualizationDefaults `json:"defaults"`
	GroupAnalysis       GroupAnalysis         `json:"groupAnalysis"`
	DataTypeAnalysis    DataTypeAnalysis      `json:"dataTypeAnalysis"`
	RelationshipNetwork RelationshipNetwork   `json:"relationshipNetwork"`
}

type Dashboard struct {
	Overview      Overview      `json:"overview"`
	Visualization Visualization `json:"visualization"`
}

type groupAggregate struct {
	id       string
	nickName string
	members  int
	messages int64
	active   bool
}

// handleDashboard 处理 Dashboard 数据请求
func (s *Service) handleDashboard(c *gin.Context) {
	log.Debug().Msg("handling dashboard request")

	// Check cache
	s.dashboardCache.mu.RLock()
	if s.dashboardCache.data != nil && time.Now().Before(s.dashboardCache.expiry) && c.Query("refresh") != "1" {
		log.Debug().Msg("returning cached dashboard data")
		c.JSON(http.StatusOK, s.dashboardCache.data)
		s.dashboardCache.mu.RUnlock()
		return
	}
	s.dashboardCache.mu.RUnlock()

	resp, err := s.buildDashboardData()
	if err != nil {
		log.Debug().Err(err).Msg("failed to build dashboard data")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build dashboard", "detail": err.Error()})
		return
	}

	// Update cache
	s.dashboardCache.mu.Lock()
	s.dashboardCache.data = resp
	s.dashboardCache.expiry = time.Now().Add(5 * time.Minute)
	s.dashboardCache.mu.Unlock()

	// 持久化 dashboard
	s.saveDashboard(resp)

	// 处理下载请求
	if c.Query("download") == "1" {
		b, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal failed", "detail": err.Error()})
			return
		}
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", "attachment; filename=dashboard.json")
		c.Data(http.StatusOK, "application/json", b)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// buildDashboardData 构建 Dashboard 数据
func (s *Service) buildDashboardData() (*Dashboard, error) {
	log.Debug().Msg("building dashboard data")

	var (
		gstats                    *model.GlobalMessageStats
		groupCounts               map[string]int64
		dbSizeBytes, dirSizeBytes int64
		currentUser               string
		accountID                 string
		err                       error
	)

	var wg sync.WaitGroup
	wg.Add(4)

	// 1. 获取基础统计数据
	go func() {
		defer wg.Done()
		var e error
		gstats, e = s.db.GetDB().GlobalMessageStats()
		if e != nil {
			err = fmt.Errorf("global stats failed: %w", e)
		}
	}()

	// 2. 获取群组消息计数
	go func() {
		defer wg.Done()
		groupCounts, _ = s.db.GetDB().GroupMessageCounts()
	}()

	// 3. 计算文件和目录大小
	go func() {
		defer wg.Done()
		dbSizeBytes, dirSizeBytes = s.calculateStorageSizes()
	}()

	// 4. 获取当前用户信息
	go func() {
		defer wg.Done()
		currentUser, accountID = s.extractCurrentUser()
	}()

	wg.Wait()

	if err != nil {
		return nil, err
	}

	dbSize := roundMB(dbSizeBytes)
	dirSize := roundMB(dirSizeBytes)

	// 构建消息类型统计
	msgTypes := s.buildMessageTypeStats(gstats)

	// 构建群组信息
	groupAggs, activeGroups := s.buildGroupAggregates(groupCounts)

	// 构建 Overview 数据
	overview := s.buildOverview(currentUser, dbSize, dirSize, gstats, msgTypes, groupAggs)

	// 构建 Visualization 数据
	visualization := s.buildVisualization(gstats, msgTypes, groupAggs, activeGroups, groupCounts, accountID)

	return &Dashboard{
		Overview:      overview,
		Visualization: visualization,
	}, nil
}

// calculateStorageSizes 计算存储大小
func (s *Service) calculateStorageSizes() (dbSize, dirSize int64) {
	dataDir := s.conf.GetDataDir()
	workDir := dataDir
	if s.db != nil {
		if wd := s.db.GetWorkDir(); wd != "" {
			workDir = wd
		}
	}
	dirSizeBytes := safeDirSize(dataDir)
	dbSizeBytes := estimateDBSize(workDir)
	log.Debug().Int64("dbSizeBytes", dbSizeBytes).Int64("dirSizeBytes", dirSizeBytes).Msg("calculated storage sizes")
	return dbSizeBytes, dirSizeBytes
}

// extractCurrentUser 提取当前用户信息
func (s *Service) extractCurrentUser() (currentUser, accountID string) {
	log.Debug().Msg("extracting current user")
	dataDir := s.conf.GetDataDir()
	workDir := dataDir
	if s.db != nil {
		if wd := s.db.GetWorkDir(); wd != "" {
			workDir = wd
		}
	}

	extractWxid := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "" {
			return ""
		}
		parts := strings.Split(filepath.Clean(p), string(filepath.Separator))
		for _, seg := range parts {
			if strings.HasPrefix(strings.ToLower(seg), "wxid_") {
				return seg
			}
		}
		return filepath.Base(filepath.Clean(p))
	}

	if workDir != "" {
		accountID = extractWxid(workDir)
	}
	if accountID == "" {
		accountID = extractWxid(dataDir)
	}

	if accountID != "" && accountID != "." && accountID != string(filepath.Separator) {
		lookupID := accountID
		low := strings.ToLower(lookupID)
		if strings.HasPrefix(low, "wxid_") {
			rest := lookupID[len("wxid_"):]
			if idx := strings.Index(rest, "_"); idx >= 0 {
				lookupID = lookupID[:len("wxid_")+idx]
			}
		}
		if clist, err := s.db.GetContacts(lookupID, 0, 0); err == nil && clist != nil && len(clist.Items) > 0 {
			for _, it := range clist.Items {
				if it != nil && it.UserName == lookupID {
					if strings.TrimSpace(it.NickName) != "" {
						currentUser = it.NickName
					}
					break
				}
			}
			if currentUser == "" && clist.Items[0] != nil && clist.Items[0].UserName == lookupID {
				currentUser = clist.Items[0].NickName
			}
		}
		if strings.TrimSpace(currentUser) == "" {
			currentUser = accountID
		}
	}

	log.Debug().Str("user", currentUser).Str("accountID", accountID).Msg("extracted current user")
	return
}

// buildMessageTypeStats 构建消息类型统计
func (s *Service) buildMessageTypeStats(gstats *model.GlobalMessageStats) map[string]int64 {
	log.Debug().Int64("total", gstats.Total).Msg("building message type stats")
	msgTypes := map[string]int64{
		"文本消息":    0,
		"图片消息":    0,
		"语音消息":    0,
		"好友验证消息":  0,
		"好友推荐消息":  0,
		"聊天表情":    0,
		"位置消息":    0,
		"XML消息":   0,
		"文件消息":    0,
		"链接消息":    0,
		"音视频通话":   0,
		"手机端操作消息": 0,
		"系统通知":    0,
		"撤回消息":    0,
	}

	for k, v := range gstats.ByType {
		if _, ok := msgTypes[k]; ok {
			msgTypes[k] += v
		}
	}

	return msgTypes
}

// buildGroupAggregates 构建群组聚合数据
func (s *Service) buildGroupAggregates(groupCounts map[string]int64) ([]groupAggregate, int) {
	log.Debug().Int("count", len(groupCounts)).Msg("building group aggregates")
	groupAggs := make([]groupAggregate, 0)
	activeGroups := 0

	rooms, err := s.db.GetChatRooms("", 0, 0)
	if err != nil {
		return groupAggs, activeGroups
	}

	for _, r := range rooms.Items {
		if strings.TrimSpace(r.NickName) == "" {
			continue
		}
		mc := groupCounts[r.Name]
		active := mc > 0
		if active {
			activeGroups++
		}
		groupAggs = append(groupAggs, groupAggregate{
			id:       r.Name,
			nickName: r.NickName,
			members:  len(r.Users),
			messages: mc,
			active:   active,
		})
	}

	sort.Slice(groupAggs, func(i, j int) bool {
		if groupAggs[i].messages == groupAggs[j].messages {
			return groupAggs[i].nickName < groupAggs[j].nickName
		}
		return groupAggs[i].messages > groupAggs[j].messages
	})

	return groupAggs, activeGroups
}

// buildOverview 构建 Overview 数据
func (s *Service) buildOverview(currentUser string, dbSize, dirSize float64, gstats *model.GlobalMessageStats, msgTypes map[string]int64, groupAggs []groupAggregate) Overview {
	// 构建群组列表
	overviewGroups := make([]OverviewGroup, 0, len(groupAggs))
	for _, g := range groupAggs {
		overviewGroups = append(overviewGroups, OverviewGroup{
			ChatRoomName: g.id,
			NickName:     g.nickName,
			MemberCount:  g.members,
			MessageCount: g.messages,
		})
	}

	// 计算时间轴
	durationDays := 0
	if gstats.EarliestUnix > 0 && gstats.LatestUnix >= gstats.EarliestUnix {
		span := gstats.LatestUnix - gstats.EarliestUnix
		if span < 0 {
			span = 0
		}
		durationDays = int(math.Round(float64(span) / 86400.0))
	}

	// 计算唯一消息类型数
	uniqueTypes := 0
	for _, v := range msgTypes {
		if v > 0 {
			uniqueTypes++
		}
	}

	return Overview{
		User:       currentUser,
		DBStats:    DBStats{DbSizeMB: dbSize, DirSizeMB: dirSize},
		MsgStats:   MsgStats{TotalMsgs: gstats.Total, SentMsgs: gstats.Sent, ReceivedMsgs: gstats.Received, UniqueMsgTypes: uniqueTypes},
		MsgTypes:   msgTypes,
		Groups:     overviewGroups,
		Timeline:   Timeline{Earliest: gstats.EarliestUnix, Latest: gstats.LatestUnix, Duration: durationDays},
		Migrations: []Migration{},
	}
}

// buildVisualization 构建 Visualization 数据
func (s *Service) buildVisualization(gstats *model.GlobalMessageStats, msgTypes map[string]int64, groupAggs []groupAggregate, activeGroups int, groupCounts map[string]int64, accountID string) Visualization {
	// 构建群组列表
	groupList := make([]GroupListItem, 0, len(groupAggs))
	for _, g := range groupAggs {
		groupList = append(groupList, GroupListItem{
			Name:     g.nickName,
			Members:  g.members,
			Messages: g.messages,
			Active:   g.active,
		})
	}

	// 计算今日消息和最活跃时段
	todayMessages, mostActiveHour := s.calculateTodayStats()

	// 计算本周平均
	weeklyAvg := s.calculateWeeklyAverage()

	// 计算内容类型百分比
	contentTypes := s.buildContentTypeStats(msgTypes, gstats.Total)

	// 计算来源渠道
	sourceChannels := s.buildSourceChannels(gstats.Total, groupCounts)

	// 构建关系网络
	relationshipNodes := s.buildRelationshipNetwork(accountID)

	// 计算其他消息数
	others := gstats.Total - (msgTypes["文本消息"] + msgTypes["图片消息"] + msgTypes["语音消息"] + msgTypes["文件消息"] + msgTypes["链接消息"])
	if others < 0 {
		others = 0
	}

	defaultSelectedIndex := 0
	if len(groupList) == 0 {
		defaultSelectedIndex = -1
	}

	processingStatus := ProcessingStatus{}
	if gstats.Total > 0 {
		processingStatus.Processed = 100
	}

	return Visualization{
		Defaults: VisualizationDefaults{SelectedGroupIndex: defaultSelectedIndex},
		GroupAnalysis: GroupAnalysis{
			Title: "群聊分析",
			Overview: GroupOverview{
				TotalGroups:    len(groupAggs),
				ActiveGroups:   activeGroups,
				TodayMessages:  int(todayMessages),
				WeeklyAvg:      weeklyAvg,
				MostActiveHour: mostActiveHour,
			},
			ContentAnalysis: ContentAnalysis{
				Text:   msgTypes["文本消息"],
				Images: msgTypes["图片消息"],
				Voice:  msgTypes["语音消息"],
				Files:  msgTypes["文件消息"],
				Links:  msgTypes["链接消息"],
				Others: others,
			},
			GroupList: groupList,
		},
		DataTypeAnalysis: DataTypeAnalysis{
			Title:            "数据类型统计",
			ContentTypes:     contentTypes,
			SourceChannels:   sourceChannels,
			ProcessingStatus: processingStatus,
			QualityMetrics:   QualityMetrics{},
			PieGradient:      "#3b82f6 0deg 180deg, #10b981 180deg 270deg, #f59e0b 270deg 315deg, #ef4444 315deg 360deg",
		},
		RelationshipNetwork: RelationshipNetwork{Nodes: relationshipNodes},
	}
}

// calculateTodayStats 计算今日统计
func (s *Service) calculateTodayStats() (todayMessages int64, mostActiveHour string) {
	perHourTotal := make([]int64, 24)
	if s.db != nil && s.db.GetDB() != nil {
		if hours, err := s.db.GetDB().GlobalTodayHourly(); err == nil {
			for i := 0; i < 24; i++ {
				perHourTotal[i] = hours[i]
			}
		}
		if todayCounts, err := s.db.GetDB().GroupTodayMessageCounts(); err == nil {
			for _, v := range todayCounts {
				todayMessages += v
			}
		}
	}

	maxHour := 0
	for h := 1; h < 24; h++ {
		if perHourTotal[h] > perHourTotal[maxHour] {
			maxHour = h
		}
	}
	mostActiveHour = fmt.Sprintf("%02d:00-%02d:00", maxHour, (maxHour+1)%24)

	return
}

// calculateWeeklyAverage 计算本周平均
func (s *Service) calculateWeeklyAverage() int {
	weeklyAvg := 0
	if s.db != nil && s.db.GetDB() != nil {
		if weekTotal, err := s.db.GetDB().GroupWeekMessageCount(); err == nil && weekTotal > 0 {
			now := time.Now()
			wday := int(now.Weekday())
			passed := 0
			if wday == 0 {
				passed = 7
			} else {
				passed = wday
			}
			if passed <= 0 {
				passed = 1
			}
			avg := float64(weekTotal) / float64(passed)
			weeklyAvg = int(math.Round(avg))
		}
	}
	return weeklyAvg
}

// buildContentTypeStats 构建内容类型统计
func (s *Service) buildContentTypeStats(msgTypes map[string]int64, totalMsgs int64) map[string]ContentTypeStats {
	ctKeys := []string{
		"XML消息", "位置消息", "图片消息", "好友推荐消息", "好友验证消息", "手机端操作消息",
		"撤回消息", "文件消息", "文本消息", "系统通知", "聊天表情", "语音消息", "链接消息", "音视频通话",
	}

	var sumCT int64
	maxKey := ""
	var maxCnt int64
	for _, k := range ctKeys {
		sumCT += msgTypes[k]
		if msgTypes[k] > maxCnt {
			maxCnt = msgTypes[k]
			maxKey = k
		}
	}

	round2 := func(f float64) float64 { return math.Round(f*100) / 100 }
	pctCT := func(n int64) float64 {
		if sumCT == 0 {
			return 0
		}
		return round2(float64(n) * 100.0 / float64(sumCT))
	}

	// 计算每类百分比
	ctPerc := make(map[string]float64, len(ctKeys))
	sumPerc := 0.0
	for _, k := range ctKeys {
		p := pctCT(msgTypes[k])
		ctPerc[k] = p
		sumPerc += p
	}

	// 差额校正到 100%
	if diff := round2(100.0 - sumPerc); diff != 0 && maxKey != "" {
		ctPerc[maxKey] = round2(ctPerc[maxKey] + diff)
	}

	floatPtr := func(v float64) *float64 { return &v }
	stringPtr := func(v string) *string { return &v }

	return map[string]ContentTypeStats{
		"文本消息":    {Count: msgTypes["文本消息"], Percentage: ctPerc["文本消息"]},
		"图片消息":    {Count: msgTypes["图片消息"], Percentage: ctPerc["图片消息"]},
		"语音消息":    {Count: msgTypes["语音消息"], Percentage: ctPerc["语音消息"]},
		"文件消息":    {Count: msgTypes["文件消息"], Percentage: ctPerc["文件消息"]},
		"链接消息":    {Count: msgTypes["链接消息"], Percentage: ctPerc["链接消息"], SizeMB: floatPtr(0), Trend: stringPtr("")},
		"XML消息":   {Count: msgTypes["XML消息"], Percentage: ctPerc["XML消息"]},
		"好友验证消息":  {Count: msgTypes["好友验证消息"], Percentage: ctPerc["好友验证消息"]},
		"好友推荐消息":  {Count: msgTypes["好友推荐消息"], Percentage: ctPerc["好友推荐消息"]},
		"聊天表情":    {Count: msgTypes["聊天表情"], Percentage: ctPerc["聊天表情"]},
		"位置消息":    {Count: msgTypes["位置消息"], Percentage: ctPerc["位置消息"]},
		"音视频通话":   {Count: msgTypes["音视频通话"], Percentage: ctPerc["音视频通话"]},
		"手机端操作消息": {Count: msgTypes["手机端操作消息"], Percentage: ctPerc["手机端操作消息"]},
		"系统通知":    {Count: msgTypes["系统通知"], Percentage: ctPerc["系统通知"]},
		"撤回消息":    {Count: msgTypes["撤回消息"], Percentage: ctPerc["撤回消息"]},
	}
}

// buildSourceChannels 构建来源渠道统计
func (s *Service) buildSourceChannels(totalMsgs int64, groupCounts map[string]int64) map[string]SourceChannel {
	var groupTotal int64
	for _, v := range groupCounts {
		groupTotal += v
	}
	privateTotal := totalMsgs - groupTotal

	pct := func(n int64) float64 {
		if totalMsgs == 0 {
			return 0
		}
		return math.Round((float64(n) * 10000.0 / float64(totalMsgs))) / 100.0
	}

	return map[string]SourceChannel{
		"私聊数据": {Count: privateTotal, Percentage: pct(privateTotal)},
		"群聊数据": {Count: groupTotal, Percentage: pct(groupTotal)},
	}
}

// buildRelationshipNetwork 构建关系网络
func (s *Service) buildRelationshipNetwork(accountID string) []RelationshipNode {
	relationshipNodes := make([]RelationshipNode, 0)

	if s.db == nil || s.db.GetDB() == nil {
		return relationshipNodes
	}

	ibase, err := s.db.GetDB().IntimacyBase()
	if err != nil || len(ibase) == 0 {
		return relationshipNodes
	}

	skipIDs := map[string]struct{}{
		"filehelper":    {},
		"weixin":        {},
		"notifymessage": {},
		"fmessage":      {},
	}

	contactMap := map[string]*model.Contact{}
	if clist, err := s.db.GetContacts("", 0, 0); err == nil && clist != nil && len(clist.Items) > 0 {
		for _, ct := range clist.Items {
			if ct != nil {
				contactMap[ct.UserName] = ct
			}
		}
	}

	type pair struct {
		k string
		v *model.IntimacyBase
	}
	arr := make([]pair, 0, len(ibase))
	for k, v := range ibase {
		arr = append(arr, pair{k, v})
	}

	sort.Slice(arr, func(i, j int) bool {
		ai, aj := arr[i].v, arr[j].v
		if ai.Last90DaysMsg != aj.Last90DaysMsg {
			return ai.Last90DaysMsg > aj.Last90DaysMsg
		}
		if ai.MsgCount != aj.MsgCount {
			return ai.MsgCount > aj.MsgCount
		}
		return ai.Past7DaysSentMsg > aj.Past7DaysSentMsg
	})

	maxN := 24
	if len(arr) < maxN {
		maxN = len(arr)
	}

	added := 0
	for idx := 0; idx < len(arr) && added < maxN; idx++ {
		k := arr[idx].k
		v := arr[idx].v
		if accountID != "" && k == accountID {
			continue
		}
		if _, skip := skipIDs[k]; skip {
			continue
		}

		ct := contactMap[k]
		display := k
		if ct != nil {
			if strings.TrimSpace(ct.Remark) != "" {
				display = ct.Remark
			} else if strings.TrimSpace(ct.NickName) != "" {
				display = ct.NickName
			}
		}

		relationshipNodes = append(relationshipNodes, RelationshipNode{
			Name:     display,
			Type:     "contact",
			Messages: v.MsgCount,
			Avatar:   s.composeAvatarURL(k),
		})
		added++
	}

	return relationshipNodes
}

// saveDashboard 保存 Dashboard 数据到文件
func (s *Service) saveDashboard(resp *Dashboard) {
	log.Debug().Msg("saving dashboard data")
	baseDir := ""
	if s.db != nil {
		if wd := strings.TrimSpace(s.db.GetWorkDir()); wd != "" {
			baseDir = wd
		}
	}
	if baseDir == "" {
		if dir := strings.TrimSpace(s.conf.GetDataDir()); dir != "" {
			baseDir = dir
		}
	}
	if baseDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			baseDir = cwd
		}
	}

	if baseDir != "" {
		if err := os.MkdirAll(baseDir, 0o755); err == nil {
			if b, err := json.Marshal(resp); err == nil {
				path := filepath.Join(baseDir, "dashboard.json")
				if err := os.WriteFile(path, b, 0o644); err != nil {
					log.Debug().Err(err).Str("path", path).Msg("failed to save dashboard file")
				} else {
					log.Debug().Str("path", path).Msg("dashboard saved")
				}
			}
		}
	}
}

// roundMB 将字节数转换为 MB 并保留两位小数
func roundMB(bytes int64) float64 {
	if bytes <= 0 {
		return 0
	}
	mb := float64(bytes) / (1024.0 * 1024.0)
	return float64(int(mb*100+0.5)) / 100.0
}

// safeDirSize 计算目录大小，出错时返回 0
func safeDirSize(path string) int64 {
	var total int64
	if path == "" {
		return 0
	}
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// estimateDBSize 估算数据库文件大小
func estimateDBSize(workDir string) int64 {
	if workDir == "" {
		return 0
	}
	var total int64
	_ = filepath.Walk(workDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		name := strings.ToLower(info.Name())
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".sqlite") || 
			strings.HasSuffix(name, ".sqlite3") || strings.HasSuffix(name, ".db-wal") || 
			strings.HasSuffix(name, ".db-shm") {
			total += info.Size()
		}
		return nil
	})
	return total
}