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
)

func (s *Service) handleDiary(c *gin.Context) {
	log.Debug().Msg("handling diary request")
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