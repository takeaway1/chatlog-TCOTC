package http

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/pkg/util"
)

// grouped 聊天记录分组结构
type grouped struct {
	Talker     string           `json:"talker"`
	TalkerName string           `json:"talkerName,omitempty"`
	Messages   []*model.Message `json:"messages"`
}

// handleSearch 处理搜索请求
func (s *Service) handleSearch(c *gin.Context) {
	params := struct {
		Query  string `form:"q"`
		Talker string `form:"talker"`
		Sender string `form:"sender"`
		Time   string `form:"time"`
		Start  string `form:"start"`
		End    string `form:"end"`
		Limit  int    `form:"limit"`
		Offset int    `form:"offset"`
		Format string `form:"format"`
	}{}

	if err := c.BindQuery(&params); err != nil {
		errors.Err(c, err)
		return
	}

	query := strings.TrimSpace(params.Query)
	talker := strings.TrimSpace(params.Talker)

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	req := &model.SearchRequest{
		Query:  query,
		Talker: talker,
		Sender: strings.TrimSpace(params.Sender),
		Limit:  limit,
		Offset: offset,
	}

	// 解析时间范围
	if params.Time != "" {
		start, end, ok := util.TimeRangeOf(params.Time)
		if !ok {
			errors.Err(c, errors.InvalidArg("time"))
			return
		}
		req.Start = start
		req.End = end
	} else {
		if params.Start != "" && params.End != "" {
			start, end, ok := util.TimeRangeOf(params.Start + "~" + params.End)
			if !ok {
				errors.Err(c, errors.InvalidArg("time"))
				return
			}
			req.Start = start
			req.End = end
		} else if params.Start != "" {
			start, end, ok := util.TimeRangeOf(params.Start)
			if !ok {
				errors.Err(c, errors.InvalidArg("start"))
				return
			}
			req.Start = start
			req.End = end
		} else if params.End != "" {
			start, end, ok := util.TimeRangeOf(params.End)
			if !ok {
				errors.Err(c, errors.InvalidArg("end"))
				return
			}
			req.Start = start
			req.End = end
		}
	}

	if !req.Start.IsZero() && !req.End.IsZero() && req.End.Before(req.Start) {
		req.Start, req.End = req.End, req.Start
	}

	resp, err := s.db.SearchMessages(req)
	if err != nil {
		errors.Err(c, err)
		return
	}
	if resp == nil {
		resp = &model.SearchResponse{Hits: []*model.SearchHit{}, Limit: limit, Offset: offset}
	}

	resp.Query = req.Query
	resp.Talker = req.Talker
	resp.Sender = req.Sender
	resp.Start = req.Start
	resp.End = req.End
	resp.Limit = limit
	resp.Offset = offset

	format := strings.ToLower(strings.TrimSpace(params.Format))
	if format == "" {
		format = "json"
	}

	switch format {
	case "html":
		s.renderSearchHTML(c, resp)
	case "text", "plain":
		s.renderSearchText(c, resp)
	case "csv":
		s.renderSearchCSV(c, resp)
	default:
		c.JSON(http.StatusOK, resp)
	}
}

// renderSearchHTML 渲染搜索结果为 HTML
func (s *Service) renderSearchHTML(c *gin.Context, resp *model.SearchResponse) {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writeChatlogHTMLHeaderCompat(c.Writer, "Search Result")
	c.Writer.WriteString("<h1>搜索结果</h1>")
	c.Writer.WriteString("<div class=\"search-meta\">")
	
	if resp.Query != "" {
		c.Writer.WriteString("<p class=\"meta\"><strong>关键词：</strong>" + template.HTMLEscapeString(resp.Query) + "</p>")
	}
	
	talkerLabel := "全部会话"
	if resp.Talker != "" {
		talkerLabel = template.HTMLEscapeString(resp.Talker)
	}
	c.Writer.WriteString("<p class=\"meta\"><strong>会话：</strong>" + talkerLabel + "</p>")
	
	if resp.Sender != "" {
		c.Writer.WriteString("<p class=\"meta\"><strong>发送者：</strong>" + template.HTMLEscapeString(resp.Sender) + "</p>")
	}
	
	timeLabel := "不限"
	if !resp.Start.IsZero() && !resp.End.IsZero() {
		timeLabel = resp.Start.Format("2006-01-02 15:04:05") + " ~ " + resp.End.Format("2006-01-02 15:04:05")
	} else if !resp.Start.IsZero() {
		timeLabel = ">= " + resp.Start.Format("2006-01-02 15:04:05")
	} else if !resp.End.IsZero() {
		timeLabel = "<= " + resp.End.Format("2006-01-02 15:04:05")
	}
	c.Writer.WriteString("<p class=\"meta\"><strong>时间范围：</strong>" + template.HTMLEscapeString(timeLabel) + "</p>")
	c.Writer.WriteString(fmt.Sprintf("<p class=\"meta\"><strong>命中条数：</strong>%d（本页 %d 条）</p>", resp.Total, len(resp.Hits)))
	c.Writer.WriteString("</div>")

	if len(resp.Hits) == 0 {
		c.Writer.WriteString("<div class=\"empty\">暂无搜索结果</div>")
	} else {
		for idx, hit := range resp.Hits {
			if hit == nil || hit.Message == nil {
				continue
			}
			msg := hit.Message
			msg.SetContent("host", c.Request.Host)
			
			talkerDisplay := msg.Talker
			if msg.TalkerName != "" {
				talkerDisplay = fmt.Sprintf("%s (%s)", msg.TalkerName, msg.Talker)
			}
			
			senderDisplay := msg.Sender
			if msg.IsSelf {
				senderDisplay = "我"
			}
			if msg.SenderName != "" {
				senderDisplay = fmt.Sprintf("%s(%s)", msg.SenderName, msg.Sender)
			}
			
			avatarURL := template.HTMLEscapeString(s.composeAvatarURL(msg.Sender) + "?size=big")
			talkerText := template.HTMLEscapeString(talkerDisplay)
			senderText := template.HTMLEscapeString(senderDisplay)
			timeText := template.HTMLEscapeString(msg.Time.Format("2006-01-02 15:04:05"))
			
			c.Writer.WriteString("<div class=\"msg\"><div class=\"msg-row\"><img class=\"avatar\" src=\"" + avatarURL + "\" loading=\"lazy\" alt=\"avatar\" onerror=\"this.style.visibility='hidden'\"/><div class=\"msg-content\">")
			c.Writer.WriteString("<div class=\"meta\"><span class=\"talker\">#" + fmt.Sprintf("%d", idx+1) + " · " + talkerText + "</span><span class=\"sender\">" + senderText + "</span><span class=\"time\">" + timeText + "</span>")
			if hit.Score > 0 {
				c.Writer.WriteString("<span class=\"score\">score: " + fmt.Sprintf("%.4f", hit.Score) + "</span>")
			}
			c.Writer.WriteString("</div>")
			c.Writer.WriteString("<pre>" + messageHTMLPlaceholder(msg) + "</pre>")
			c.Writer.WriteString("</div></div></div>")
		}
	}
	
	c.Writer.WriteString(getPreviewSnippet())
	c.Writer.WriteString("</body></html>")
}

// renderSearchText 渲染搜索结果为纯文本
func (s *Service) renderSearchText(c *gin.Context, resp *model.SearchResponse) {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	
	fmt.Fprintf(c.Writer, "关键词: %s\n", resp.Query)
	
	talkerLabel := resp.Talker
	if talkerLabel == "" {
		talkerLabel = "全部会话"
	}
	fmt.Fprintf(c.Writer, "会话: %s\n", talkerLabel)
	
	if resp.Sender != "" {
		fmt.Fprintf(c.Writer, "发送者: %s\n", resp.Sender)
	}
	
	switch {
	case !resp.Start.IsZero() && !resp.End.IsZero():
		fmt.Fprintf(c.Writer, "时间: %s ~ %s\n", resp.Start.Format("2006-01-02 15:04:05"), resp.End.Format("2006-01-02 15:04:05"))
	case !resp.Start.IsZero():
		fmt.Fprintf(c.Writer, "时间: >= %s\n", resp.Start.Format("2006-01-02 15:04:05"))
	case !resp.End.IsZero():
		fmt.Fprintf(c.Writer, "时间: <= %s\n", resp.End.Format("2006-01-02 15:04:05"))
	default:
		fmt.Fprintln(c.Writer, "时间: 不限")
	}
	
	fmt.Fprintf(c.Writer, "总命中: %d, 本页: %d\n", resp.Total, len(resp.Hits))
	fmt.Fprintln(c.Writer, strings.Repeat("-", 60))
	
	for idx, hit := range resp.Hits {
		if hit == nil || hit.Message == nil {
			continue
		}
		msg := hit.Message
		msg.SetContent("host", c.Request.Host)
		
		title := msg.Talker
		if msg.TalkerName != "" {
			title = fmt.Sprintf("%s (%s)", msg.TalkerName, msg.Talker)
		}
		
		sender := msg.Sender
		if msg.IsSelf {
			sender = "我"
		}
		if msg.SenderName != "" {
			sender = fmt.Sprintf("%s(%s)", msg.SenderName, msg.Sender)
		}
		
		fmt.Fprintf(c.Writer, "[%d] %s @ %s\n", idx+1, msg.Time.Format("2006-01-02 15:04:05"), title)
		fmt.Fprintf(c.Writer, "发送者: %s\n", sender)
		fmt.Fprintf(c.Writer, "%s\n", msg.PlainTextContent())
		
		if snippet := strings.TrimSpace(hit.Snippet); snippet != "" {
			fmt.Fprintf(c.Writer, "Snippet: %s\n", snippet)
		}
		fmt.Fprintln(c.Writer, strings.Repeat("-", 60))
	}
}

// renderSearchCSV 渲染搜索结果为 CSV
func (s *Service) renderSearchCSV(c *gin.Context, resp *model.SearchResponse) {
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=search_%s.csv", time.Now().Format("20060102_150405")))
	
	csvWriter := csv.NewWriter(c.Writer)
	csvWriter.Write([]string{"Seq", "Time", "Talker", "TalkerName", "Sender", "SenderName", "Content", "Snippet"})
	
	for _, hit := range resp.Hits {
		if hit == nil || hit.Message == nil {
			continue
		}
		msg := hit.Message
		msg.SetContent("host", c.Request.Host)
		csvWriter.Write([]string{
			fmt.Sprintf("%d", msg.Seq),
			msg.Time.Format("2006-01-02 15:04:05"),
			msg.Talker,
			msg.TalkerName,
			msg.Sender,
			msg.SenderName,
			msg.PlainTextContent(),
			strings.ReplaceAll(hit.Snippet, "\n", " "),
		})
	}
	csvWriter.Flush()
}

// handleChatlog 处理聊天记录请求
func (s *Service) handleChatlog(c *gin.Context) {
	q := struct {
		Time    string `form:"time"`
		Talker  string `form:"talker"`
		Sender  string `form:"sender"`
		Keyword string `form:"keyword"`
		Limit   int    `form:"limit"`
		Offset  int    `form:"offset"`
		Format  string `form:"format"`
	}{}

	if err := c.BindQuery(&q); err != nil {
		errors.Err(c, err)
		return
	}

	start, end, ok := util.TimeRangeOf(q.Time)
	if !ok {
		errors.Err(c, errors.InvalidArg("time"))
		return
	}
	
	if q.Limit < 0 {
		q.Limit = 0
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	format := strings.ToLower(strings.TrimSpace(q.Format))
	if format == "" {
		format = "json"
	}

	// 未指定 talker: 分组输出
	if q.Talker == "" {
		s.handleChatlogGrouped(c, start, end, q.Sender, q.Keyword, format)
		return
	}

	// 指定 talker: 单会话消息
	s.handleChatlogSingle(c, start, end, q.Talker, q.Sender, q.Keyword, q.Limit, q.Offset, format)
}

// handleChatlogGrouped 处理分组聊天记录
func (s *Service) handleChatlogGrouped(c *gin.Context, start, end time.Time, sender, keyword, format string) {
	sessionsResp, err := s.db.GetSessions("", 0, 0)
	if err != nil {
		errors.Err(c, err)
		return
	}
	
	groups := make([]*grouped, 0)
	for _, sess := range sessionsResp.Items {
		msgs, err := s.db.GetMessages(start, end, sess.UserName, sender, keyword, 0, 0)
		if err != nil || len(msgs) == 0 {
			continue
		}
		groups = append(groups, &grouped{Talker: sess.UserName, TalkerName: sess.NickName, Messages: msgs})
	}
	
	switch format {
	case "html":
		s.renderChatlogGroupedHTML(c, groups, start, end)
	case "csv":
		s.renderChatlogGroupedCSV(c, groups, start, end)
	case "text", "plain":
		s.renderChatlogGroupedText(c, groups)
	default:
		c.JSON(http.StatusOK, groups)
	}
}

// renderChatlogGroupedHTML 渲染分组聊天记录为 HTML
func (s *Service) renderChatlogGroupedHTML(c *gin.Context, groups []*grouped, start, end time.Time) {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writeChatlogHTMLHeaderCompat(c.Writer, "Chatlog")
	c.Writer.WriteString(fmt.Sprintf("<h2>All Messages %s ~ %s</h2>", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05")))
	
	for _, g := range groups {
		title := g.Talker
		if g.TalkerName != "" {
			title = fmt.Sprintf("%s (%s)", g.TalkerName, g.Talker)
		}
		c.Writer.WriteString("<details open><summary>" + template.HTMLEscapeString(title) + fmt.Sprintf(" - %d 条消息</summary>", len(g.Messages)))
		
		for _, m := range g.Messages {
			m.SetContent("host", c.Request.Host)
			senderDisplay := m.Sender
			if m.IsSelf {
				senderDisplay = "我"
			}
			if m.SenderName != "" {
				senderDisplay = template.HTMLEscapeString(m.SenderName) + "(" + template.HTMLEscapeString(senderDisplay) + ")"
			} else {
				senderDisplay = template.HTMLEscapeString(senderDisplay)
			}
			
			aurl := template.HTMLEscapeString(s.composeAvatarURL(m.Sender) + "?size=big")
			timeText := template.HTMLEscapeString(m.Time.Format("2006-01-02 15:04:05"))
			c.Writer.WriteString("<div class=\"msg\"><div class=\"msg-row\"><img class=\"avatar\" src=\"" + aurl + "\" loading=\"lazy\" alt=\"avatar\" onerror=\"this.style.visibility='hidden'\"/><div class=\"msg-content\"><div class=\"meta\"><span class=\"sender\">" + senderDisplay + "</span><span class=\"time\">" + timeText + "</span></div><pre>" + messageHTMLPlaceholder(m) + "</pre></div></div></div>")
		}
		c.Writer.WriteString("</details>")
	}
	
	c.Writer.WriteString(getPreviewSnippet())
	c.Writer.WriteString("</body></html>")
}

// renderChatlogGroupedCSV 渲染分组聊天记录为 CSV
func (s *Service) renderChatlogGroupedCSV(c *gin.Context, groups []*grouped, start, end time.Time) {
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=all_%s_%s.csv", start.Format("2006-01-02"), end.Format("2006-01-02")))
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	csvWriter := csv.NewWriter(c.Writer)
	csvWriter.Write([]string{"Talker", "TalkerName", "Time", "SenderName", "Sender", "Content"})
	
	for _, g := range groups {
		for _, m := range g.Messages {
			csvWriter.Write([]string{g.Talker, g.TalkerName, m.Time.Format("2006-01-02 15:04:05"), m.SenderName, m.Sender, m.PlainTextContent()})
		}
	}
	csvWriter.Flush()
}

// renderChatlogGroupedText 渲染分组聊天记录为文本
func (s *Service) renderChatlogGroupedText(c *gin.Context, groups []*grouped) {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	for _, g := range groups {
		header := g.Talker
		if g.TalkerName != "" {
			header = fmt.Sprintf("%s (%s)", g.TalkerName, g.Talker)
		}
		c.Writer.WriteString(header + "\n")
		
		for _, m := range g.Messages {
			sender := m.Sender
			if m.IsSelf {
				sender = "我"
			}
			if m.SenderName != "" {
				sender = m.SenderName + "(" + sender + ")"
			}
			c.Writer.WriteString(m.Time.Format("2006-01-02 15:04:05") + " " + sender + " " + m.PlainTextContent() + "\n")
		}
		c.Writer.WriteString("-----------------------------\n")
	}
}

// handleChatlogSingle 处理单会话聊天记录
func (s *Service) handleChatlogSingle(c *gin.Context, start, end time.Time, talker, sender, keyword string, limit, offset int, format string) {
	messages, err := s.db.GetMessages(start, end, talker, sender, keyword, limit, offset)
	if err != nil {
		errors.Err(c, err)
		return
	}
	
	switch format {
	case "html":
		s.renderChatlogSingleHTML(c, messages, start, end, talker)
	case "csv":
		s.renderChatlogSingleCSV(c, messages, start, end, talker)
	case "json":
		c.JSON(http.StatusOK, messages)
	default:
		s.renderChatlogSingleText(c, messages, start, end, talker)
	}
}

// renderChatlogSingleHTML 渲染单会话聊天记录为 HTML
func (s *Service) renderChatlogSingleHTML(c *gin.Context, messages []*model.Message, start, end time.Time, talker string) {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writeChatlogHTMLHeaderCompat(c.Writer, "Chatlog")
	c.Writer.WriteString(fmt.Sprintf("<h2>Messages %s ~ %s (%s)</h2>", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"), template.HTMLEscapeString(talker)))
	
	for _, m := range messages {
		m.SetContent("host", c.Request.Host)
		c.Writer.WriteString("<div class=\"msg\"><div class=\"msg-row\">")
		aurl := template.HTMLEscapeString(s.composeAvatarURL(m.Sender) + "?size=big")
		c.Writer.WriteString("<img class=\"avatar\" src=\"" + aurl + "\" loading=\"lazy\" alt=\"avatar\" onerror=\"this.style.visibility='hidden'\"/>")
		c.Writer.WriteString("<div class=\"msg-content\"><div class=\"meta\"><span class=\"sender\">")
		
		if m.SenderName != "" {
			c.Writer.WriteString(template.HTMLEscapeString(m.SenderName) + "(")
		}
		c.Writer.WriteString(template.HTMLEscapeString(m.Sender))
		if m.SenderName != "" {
			c.Writer.WriteString(")")
		}
		
		timeText := template.HTMLEscapeString(m.Time.Format("2006-01-02 15:04:05"))
		c.Writer.WriteString("</span><span class=\"time\">" + timeText + "</span></div><pre>")
		c.Writer.WriteString(messageHTMLPlaceholder(m))
		c.Writer.WriteString("</pre></div></div></div>")
	}
	
	c.Writer.WriteString(getPreviewSnippet())
	c.Writer.WriteString("</body></html>")
}

// renderChatlogSingleCSV 渲染单会话聊天记录为 CSV
func (s *Service) renderChatlogSingleCSV(c *gin.Context, messages []*model.Message, start, end time.Time, talker string) {
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s_%s.csv", talker, start.Format("2006-01-02"), end.Format("2006-01-02")))
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	csvWriter := csv.NewWriter(c.Writer)
	csvWriter.Write([]string{"Time", "SenderName", "Sender", "TalkerName", "Talker", "Content"})
	
	for _, m := range messages {
		csvWriter.Write(m.CSV(c.Request.Host))
	}
	csvWriter.Flush()
}

// renderChatlogSingleText 渲染单会话聊天记录为文本
func (s *Service) renderChatlogSingleText(c *gin.Context, messages []*model.Message, start, end time.Time, talker string) {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	for _, m := range messages {
		c.Writer.WriteString(m.PlainText(strings.Contains(talker, ","), util.PerfectTimeFormat(start, end), c.Request.Host) + "\n")
	}
}

// handleContacts 处理联系人查询
func (s *Service) handleContacts(c *gin.Context) {
	q := struct {
		Keyword string `form:"keyword"`
		Limit   int    `form:"limit"`
		Offset  int    `form:"offset"`
		Format  string `form:"format"`
	}{}

	if err := c.BindQuery(&q); err != nil {
		errors.Err(c, err)
		return
	}
	
	q.Keyword = strings.TrimSpace(q.Keyword)

	list, err := s.db.GetContacts(q.Keyword, q.Limit, q.Offset)
	if err != nil {
		errors.Err(c, err)
		return
	}

	format := strings.ToLower(strings.TrimSpace(q.Format))
	if format == "" {
		format = "json"
	}
	
	switch format {
	case "html":
		s.renderContactsHTML(c, list.Items)
	case "json":
		for _, item := range list.Items {
			item.AvatarURL = s.composeAvatarURL(item.UserName)
		}
		c.JSON(http.StatusOK, list)
	default:
		s.renderContactsCSV(c, list.Items, format)
	}
}

// renderContactsHTML 渲染联系人列表为 HTML
func (s *Service) renderContactsHTML(c *gin.Context, items []*model.Contact) {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Write([]byte(`<style>
  .contacts{font-family:Arial,Helvetica,sans-serif;font-size:14px;}
  .c-item{display:flex;align-items:center;gap:10px;border:1px solid #ddd;border-radius:6px;padding:6px 8px;margin:6px 0;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.04);} 
  .c-avatar{width:36px;height:36px;border-radius:50%;object-fit:cover;background:#f2f2f2;border:1px solid #eee}
  .c-name{font-weight:600;color:#2c3e50}
  .c-sub{color:#666;font-size:12px}
</style><div class="contacts">`))
	
	for _, contact := range items {
		uname := template.HTMLEscapeString(contact.UserName)
		nick := template.HTMLEscapeString(contact.NickName)
		remark := template.HTMLEscapeString(contact.Remark)
		alias := template.HTMLEscapeString(contact.Alias)
		aurl := template.HTMLEscapeString(s.composeAvatarURL(contact.UserName))
		
		c.Writer.WriteString(`<div class="c-item">`)
		c.Writer.WriteString(`<img class="c-avatar" src="` + aurl + `" loading="lazy" onerror="this.style.visibility='hidden'"/>`)
		c.Writer.WriteString(`<div>`)
		c.Writer.WriteString(`<div class="c-name">` + nick + `</div>`)
		c.Writer.WriteString(`<div class="c-sub">` + uname)
		if remark != "" {
			c.Writer.WriteString(` · ` + remark)
		}
		if alias != "" {
			c.Writer.WriteString(` · alias:` + alias)
		}
		c.Writer.WriteString(`</div></div></div>`)
	}
	c.Writer.WriteString(`</div>`)
}

// renderContactsCSV 渲染联系人列表为 CSV
func (s *Service) renderContactsCSV(c *gin.Context, items []*model.Contact, format string) {
	if format == "csv" {
		c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	} else {
		c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	c.Writer.WriteString("UserName,Alias,Remark,NickName,AvatarURL\n")
	for _, contact := range items {
		avatarURL := s.composeAvatarURL(contact.UserName)
		c.Writer.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s\n", contact.UserName, contact.Alias, contact.Remark, contact.NickName, avatarURL))
	}
	c.Writer.Flush()
}

// composeAvatarURL 构建头像 URL
func (s *Service) composeAvatarURL(username string) string {
	if username == "" {
		return ""
	}
	return "/avatar/" + username
}

// handleAvatar 处理头像请求
func (s *Service) handleAvatar(c *gin.Context) {
	username := c.Param("username")
	size := c.Query("size")
	
	avatar, err := s.db.GetAvatar(username, size)
	if err != nil {
		errors.Err(c, err)
		return
	}
	if avatar == nil {
		errors.Err(c, errors.ErrAvatarNotFound)
		return
	}
	
	if avatar.URL != "" {
		c.Redirect(http.StatusFound, avatar.URL)
		return
	}
	
	ct := avatar.ContentType
	if ct == "" {
		ct = "image/jpeg"
	}
	c.Data(http.StatusOK, ct, avatar.Data)
}

// handleChatRooms 处理聊天室查询
func (s *Service) handleChatRooms(c *gin.Context) {
	q := struct {
		Keyword string `form:"keyword"`
		Limit   int    `form:"limit"`
		Offset  int    `form:"offset"`
		Format  string `form:"format"`
	}{}

	if err := c.BindQuery(&q); err != nil {
		errors.Err(c, err)
		return
	}

	list, err := s.db.GetChatRooms(q.Keyword, q.Limit, q.Offset)
	if err != nil {
		errors.Err(c, err)
		return
	}

	format := strings.ToLower(strings.TrimSpace(q.Format))
	if format == "" {
		format = "json"
	}
	
	switch format {
	case "json":
		c.JSON(http.StatusOK, list)
	case "csv":
		s.renderChatRoomsCSV(c, list.Items)
	default:
		c.JSON(http.StatusOK, list)
	}
}

// renderChatRoomsCSV 渲染聊天室列表为 CSV
func (s *Service) renderChatRoomsCSV(c *gin.Context, items []*model.ChatRoom) {
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	csvWriter := csv.NewWriter(c.Writer)
	csvWriter.Write([]string{"Name", "NickName", "MemberCount"})
	
	for _, room := range items {
		csvWriter.Write([]string{room.Name, room.NickName, fmt.Sprintf("%d", len(room.Users))})
	}
	csvWriter.Flush()
}

// handleSessions 处理会话查询
func (s *Service) handleSessions(c *gin.Context) {
	q := struct {
		Keyword string `form:"keyword"`
		Limit   int    `form:"limit"`
		Offset  int    `form:"offset"`
		Format  string `form:"format"`
	}{}

	if err := c.BindQuery(&q); err != nil {
		errors.Err(c, err)
		return
	}

	list, err := s.db.GetSessions(q.Keyword, q.Limit, q.Offset)
	if err != nil {
		errors.Err(c, err)
		return
	}

	format := strings.ToLower(strings.TrimSpace(q.Format))
	if format == "" {
		format = "json"
	}
	
	switch format {
	case "json":
		c.JSON(http.StatusOK, list)
	case "csv":
		s.renderSessionsCSV(c, list.Items)
	default:
		c.JSON(http.StatusOK, list)
	}
}

// renderSessionsCSV 渲染会话列表为 CSV
func (s *Service) renderSessionsCSV(c *gin.Context, items []*model.Session) {
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	csvWriter := csv.NewWriter(c.Writer)
	csvWriter.Write([]string{"UserName", "NickName", "NOrder", "NTime"})
	
	for _, session := range items {
		csvWriter.Write([]string{
			session.UserName,
			session.NickName,
			fmt.Sprintf("%d", session.NOrder),
			session.NTime.Format("2006-01-02 15:04:05"),
		})
	}
	csvWriter.Flush()
}

// handleDiary 处理日记查询（返回指定日期内"我"参与的消息，按 talker 分组）
func (s *Service) handleDiary(c *gin.Context) {
	q := struct {
		Date   string `form:"date"`
		Talker string `form:"talker"`
		Format string `form:"format"`
	}{}
	
	if err := c.BindQuery(&q); err != nil {
		errors.Err(c, err)
		return
	}

	dateStr := strings.TrimSpace(q.Date)
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	parsed, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		errors.Err(c, errors.InvalidArg("date"))
		return
	}
	
	start := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, parsed.Location())
	end := start.Add(24*time.Hour - time.Nanosecond)

	startDisplay := start.Format("2006-01-02 15:04:05")
	endDisplay := end.Format("2006-01-02 15:04:05")
	heading := fmt.Sprintf("%s 的聊天日记（%s ~ %s）", start.Format("2006-01-02"), startDisplay, endDisplay)

	sessionsResp, err := s.db.GetSessions(q.Talker, 0, 0)
	if err != nil {
		errors.Err(c, err)
		return
	}

	groups := make([]*grouped, 0)

	for _, sess := range sessionsResp.Items {
		msgs, err := s.db.GetMessages(start, end, sess.UserName, "", "", 0, 0)
		if err != nil || len(msgs) == 0 {
			continue
		}
		
		hasSelf := false
		for _, m := range msgs {
			if m.IsSelf {
				hasSelf = true
				break
			}
		}
		if !hasSelf {
			continue
		}
		
		groups = append(groups, &grouped{Talker: sess.UserName, TalkerName: sess.NickName, Messages: msgs})
	}

	format := strings.ToLower(strings.TrimSpace(q.Format))
	if format == "" {
		format = "json"
	}
	
	switch format {
	case "html":
		s.renderDiaryHTML(c, groups, heading)
	case "json":
		c.JSON(http.StatusOK, groups)
	case "csv":
		s.renderDiaryCSV(c, groups)
	default:
		s.renderDiaryText(c, groups)
	}
}

// renderDiaryHTML 渲染日记为 HTML
func (s *Service) renderDiaryHTML(c *gin.Context, groups []*grouped, heading string) {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteString(`<html><head><meta charset="utf-8"><title>Diary</title><style>body{font-family:Arial,Helvetica,sans-serif;font-size:14px;}details{margin:8px 0;padding:6px 8px;border:1px solid #ddd;border-radius:6px;background:#fafafa;}summary{cursor:pointer;font-weight:600;} .msg{margin:4px 0;padding:4px 6px;border-left:3px solid #2ecc71;background:#fff;} .msg-row{display:flex;gap:8px;align-items:flex-start;} .avatar{width:28px;height:28px;border-radius:6px;object-fit:cover;background:#f2f2f2;border:1px solid #eee;flex:0 0 28px} .msg-content{flex:1;min-width:0} .meta{color:#666;font-size:12px;margin-bottom:2px;} pre{white-space:pre-wrap;word-break:break-word;margin:0;} .sender{color:#27ae60;} .time{color:#16a085;margin-left:6px;} a.media{color:#2c3e50;text-decoration:none;} a.media:hover{text-decoration:underline;}</style></head><body>`)
	c.Writer.WriteString(fmt.Sprintf("<h2>%s</h2>", template.HTMLEscapeString(heading)))
	
	for _, g := range groups {
		title := g.Talker
		if g.TalkerName != "" {
			title = fmt.Sprintf("%s (%s)", g.TalkerName, g.Talker)
		}
		c.Writer.WriteString("<details open><summary>" + template.HTMLEscapeString(title) + fmt.Sprintf(" - %d 条消息</summary>", len(g.Messages)))
		
		for _, m := range g.Messages {
			m.SetContent("host", c.Request.Host)
			senderDisplay := m.Sender
			if m.IsSelf {
				senderDisplay = "我"
			}
			if m.SenderName != "" {
				senderDisplay = template.HTMLEscapeString(m.SenderName) + "(" + template.HTMLEscapeString(senderDisplay) + ")"
			} else {
				senderDisplay = template.HTMLEscapeString(senderDisplay)
			}
			
			aurl := template.HTMLEscapeString(s.composeAvatarURL(m.Sender) + "?size=big")
			c.Writer.WriteString("<div class=\"msg\"><div class=\"msg-row\"><img class=\"avatar\" src=\"" + aurl + "\" loading=\"lazy\" alt=\"avatar\" onerror=\"this.style.visibility='hidden'\"/><div class=\"msg-content\"><div class=\"meta\"><span class=\"sender\">" + senderDisplay + "</span><span class=\"time\">" + m.Time.Format("2006-01-02 15:04:05") + "</span></div><pre>" + messageHTMLPlaceholder(m) + "</pre></div></div></div>")
		}
		c.Writer.WriteString("</details>")
	}
	
	c.Writer.WriteString(getPreviewSnippet())
	c.Writer.WriteString("</body></html>")
}

// renderDiaryCSV 渲染日记为 CSV
func (s *Service) renderDiaryCSV(c *gin.Context, groups []*grouped) {
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	writer := csv.NewWriter(c.Writer)
	writer.Write([]string{"Talker", "TalkerName", "Time", "SenderName", "Sender", "Content"})
	
	for _, g := range groups {
		for _, m := range g.Messages {
			writer.Write([]string{m.Talker, m.TalkerName, m.Time.Format("2006-01-02 15:04:05"), m.SenderName, m.Sender, m.PlainTextContent()})
		}
	}
	writer.Flush()
}

// renderDiaryText 渲染日记为文本
func (s *Service) renderDiaryText(c *gin.Context, groups []*grouped) {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()
	
	for _, g := range groups {
		if g.TalkerName != "" {
			c.Writer.WriteString(fmt.Sprintf("%s (%s)\n", g.TalkerName, g.Talker))
		} else {
			c.Writer.WriteString(g.Talker + "\n")
		}
		
		for _, m := range g.Messages {
			senderDisplay := m.Sender
			if m.IsSelf {
				senderDisplay = "我"
			}
			if m.SenderName != "" {
				senderDisplay = m.SenderName + "(" + senderDisplay + ")"
			}
			
			c.Writer.WriteString(m.Time.Format("2006-01-02 15:04:05"))
			c.Writer.WriteString(" ")
			c.Writer.WriteString(senderDisplay)
			c.Writer.WriteString(" ")
			c.Writer.WriteString(m.PlainTextContent())
			c.Writer.WriteString("\n")
		}
		c.Writer.WriteString("-----------------------------\n")
	}
}