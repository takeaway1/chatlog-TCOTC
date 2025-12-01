package http

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"html/template"
	"net/http"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/pkg/util"
	"github.com/sjzar/chatlog/pkg/util/dat2img"
	"github.com/sjzar/chatlog/pkg/util/silk"
)

// handleMedia 处理媒体文件请求
func (s *Service) handleMedia(c *gin.Context, _type string) {
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		errors.Err(c, errors.InvalidArg(key))
		return
	}

	keys := util.Str2List(key, ",")
	if len(keys) == 0 {
		errors.Err(c, errors.InvalidArg(key))
		return
	}

	var _err error
	for _, k := range keys {
		if strings.Contains(k, "/") {
			if absolutePath, err := s.findPath(_type, k); err == nil {
				c.Redirect(http.StatusFound, "/data/"+absolutePath)
				return
			}
		}
		
		media, err := s.db.GetMedia(_type, k)
		if err != nil {
			_err = err
			continue
		}
		
		if c.Query("info") != "" {
			c.JSON(http.StatusOK, media)
			return
		}
		
		if media.Type == "voice" && c.Query("transcribe") != "" {
			s.handleVoiceTranscription(c, k, media)
			return
		}
		
		switch media.Type {
		case "voice":
			s.HandleVoice(c, media.Data)
			return
		default:
			c.Redirect(http.StatusFound, "/data/"+media.Path)
			return
		}
	}

	if _err != nil {
		errors.Err(c, _err)
		return
	}
}

// handleVoiceTranscription 处理语音转写请求
func (s *Service) handleVoiceTranscription(c *gin.Context, key string, media *model.Media) {
	if s.speechTranscriber == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "speech transcription not enabled"})
		return
	}

	if len(media.Data) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "voice data unavailable"})
		return
	}

	ctx := c.Request.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
	}
	if cancel != nil {
		defer cancel()
	}

	opts := s.speechOptions
	if lang := strings.TrimSpace(c.Query("lang")); lang != "" {
		opts.Language = lang
		opts.LanguageSet = true
	}
	if translate := strings.TrimSpace(c.Query("translate")); translate != "" {
		switch strings.ToLower(translate) {
		case "1", "true", "yes", "on":
			opts.Translate = true
			opts.TranslateSet = true
		case "0", "false", "no", "off":
			opts.Translate = false
			opts.TranslateSet = true
		}
	}

	res, err := s.speechTranscriber.TranscribeSilk(ctx, media.Data, opts)
	if err != nil {
		if ctx.Err() != nil {
			c.JSON(http.StatusRequestTimeout, gin.H{"error": "transcription cancelled"})
			return
		}
		log.Error().Err(err).Str("media_key", key).Msg("voice transcription failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "transcription failed"})
		return
	}
	
	if res == nil {
		c.JSON(http.StatusOK, gin.H{"key": key, "text": "", "language": opts.Language, "duration": 0})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":      key,
		"text":     res.Text,
		"language": res.Language,
		"duration": res.Duration.Seconds(),
		"segments": res.Segments,
	})
}

// findPath 查找媒体文件路径
func (s *Service) findPath(_type string, key string) (string, error) {
	absolutePath := filepath.Join(s.conf.GetDataDir(), key)
	if _, err := os.Stat(absolutePath); err == nil {
		return key, nil
	}
	
	switch _type {
	case "image":
		for _, suffix := range []string{"_h.dat", ".dat", "_t.dat"} {
			if _, err := os.Stat(absolutePath + suffix); err == nil {
				return key + suffix, nil
			}
		}
	case "video":
		for _, suffix := range []string{".mp4", "_thumb.jpg"} {
			if _, err := os.Stat(absolutePath + suffix); err == nil {
				return key + suffix, nil
			}
		}
	}
	
	return "", errors.ErrMediaNotFound
}

// handleMediaData 处理媒体数据请求
func (s *Service) handleMediaData(c *gin.Context) {
	relativePath := filepath.Clean(c.Param("path"))
	absolutePath := filepath.Join(s.conf.GetDataDir(), relativePath)

	if _, err := os.Stat(absolutePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "File not found",
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(absolutePath))
	switch {
	case ext == ".dat":
		s.HandleDatFile(c, absolutePath)
	default:
		c.File(absolutePath)
	}
}

// HandleDatFile 处理 DAT 文件
func (s *Service) HandleDatFile(c *gin.Context, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		errors.Err(c, err)
		return
	}
	
	out, ext, err := dat2img.Dat2Image(b)
	if err != nil {
		c.File(path)
		return
	}

	switch ext {
	case "jpg", "jpeg":
		c.Data(http.StatusOK, "image/jpeg", out)
	case "png":
		c.Data(http.StatusOK, "image/png", out)
	case "gif":
		c.Data(http.StatusOK, "image/gif", out)
	case "bmp":
		c.Data(http.StatusOK, "image/bmp", out)
	case "mp4":
		c.Data(http.StatusOK, "video/mp4", out)
	default:
		c.Data(http.StatusOK, "image/jpg", out)
	}
}

// HandleVoice 处理语音文件
func (s *Service) HandleVoice(c *gin.Context, data []byte) {
	out, err := silk.Silk2MP3(data)
	if err != nil {
		c.Data(http.StatusOK, "audio/silk", data)
		return
	}
	c.Data(http.StatusOK, "audio/mp3", out)
}

// 消息占位符处理
var (
	placeholderPattern = regexp.MustCompile(`!?\[([^\]]+)\]\((https?://[^)]+)\)`)
)

// messageHTMLPlaceholder 将消息内容中的占位符转换为 HTML 链接
func messageHTMLPlaceholder(m *model.Message) string {
	content := m.PlainTextContent()
	return placeholderPattern.ReplaceAllStringFunc(content, func(s string) string {
		matches := placeholderPattern.FindStringSubmatch(s)
		if len(matches) != 3 {
			return template.HTMLEscapeString(s)
		}
		
		fullLabel := matches[1]
		url := matches[2]
		left := fullLabel
		rest := ""
		if p := strings.Index(fullLabel, "|"); p >= 0 {
			left = fullLabel[:p]
			rest = fullLabel[p+1:]
		}
		
		className := "media"
		if left == "动画表情" || left == "GIF表情" || strings.Contains(left, "表情") {
			className = "media anim"
		}
		if left == "语音" {
			className = "media voice-link"
		}
		
		var anchorText string
		if left == "链接" {
			escapedFull := template.HTMLEscapeString(fullLabel)
			escapedFull = strings.ReplaceAll(escapedFull, "\r", "")
			escapedFull = strings.ReplaceAll(escapedFull, "\n", "<br/>")
			anchorText = "[" + escapedFull + "]"
		} else if left == "文件" && rest != "" {
			anchorText = "[文件]" + template.HTMLEscapeString(rest)
		} else {
			anchorText = "[" + template.HTMLEscapeString(left) + "]"
		}
		
		escapedURL := template.HTMLEscapeString(url)
		anchor := `<a class="` + className + `" href="` + escapedURL + `" target="_blank">` + anchorText + `</a>`
		
		if left == "语音" {
			return `<span class="voice-entry">` + anchor + `<button type="button" class="voice-transcribe-btn">转文字</button><span class="voice-transcribe-result" aria-live="polite"></span></span>`
		}
		
		return anchor
	})
}