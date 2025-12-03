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

func (s *Service) handleSearch(c *gin.Context) {
	log.Debug().Msg("handling search request")
	params := struct {
		Query     string `form:"q"`
		Talker    string `form:"talker"`
		Sender    string `form:"sender"`
		Time      string `form:"time"`
		Start     string `form:"start"`
		End       string `form:"end"`
		Limit     int    `form:"limit"`
		Offset    int    `form:"offset"`
		Format    string `form:"format"`
		SkipTotal bool   `form:"skip_total"`
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
		Query:     query,
		Talker:    talker,
		Sender:    strings.TrimSpace(params.Sender),
		Limit:     limit,
		Offset:    offset,
		SkipTotal: params.SkipTotal,
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
		log.Debug().Err(err).Msg("search messages failed")
		errors.Err(c, err)
		return
	}
	if resp == nil {
		resp = &model.SearchResponse{Hits: []*model.SearchHit{}, Limit: limit, Offset: offset}
	}

	log.Debug().Int("hits", len(resp.Hits)).Int("total", resp.Total).Msg("search completed")

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