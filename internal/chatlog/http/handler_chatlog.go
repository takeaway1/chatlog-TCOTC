package http

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

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

func (s *Service) handleChatlog(c *gin.Context) {
	log.Debug().Msg("handling chatlog request")
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
	log.Debug().Time("start", start).Time("end", end).Msg("handling chatlog grouped")
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
	log.Debug().Str("talker", talker).Time("start", start).Time("end", end).Msg("handling chatlog single")
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