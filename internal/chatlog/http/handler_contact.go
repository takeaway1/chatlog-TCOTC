package http

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
)

func (s *Service) handleContacts(c *gin.Context) {
	log.Debug().Msg("handling contacts request")
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
		// for _, item := range list.Items {
		// 	item.AvatarURL = s.composeAvatarURL(item.UserName)
		// }
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
	log.Debug().Str("username", username).Str("size", size).Msg("handling avatar request")

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
	log.Debug().Msg("handling chatrooms request")
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
	log.Debug().Msg("handling sessions request")
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