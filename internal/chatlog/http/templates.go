package http

import (
	"bytes"
	"html/template"
	"io"
	"sync"

	"github.com/rs/zerolog/log"
)

// 模板缓存
var (
	templateCache       = make(map[string]string)
	templateCacheLock   sync.RWMutex
	previewHTMLSnippet  string
	previewSnippetOnce  sync.Once
)

// loadTemplate 从嵌入的文件系统加载模板
func loadTemplate(name string) (string, error) {
	log.Debug().Str("template", name).Msg("loading template")
	templateCacheLock.RLock()
	if content, ok := templateCache[name]; ok {
		log.Debug().Str("template", name).Msg("template found in cache")
		templateCacheLock.RUnlock()
		return content, nil
	}
	templateCacheLock.RUnlock()

	// 读取模板文件
	data, err := EFS.ReadFile("static/templates/" + name)
	if err != nil {
		log.Debug().Err(err).Str("template", name).Msg("failed to read template file")
		return "", err
	}

	content := string(data)

	// 缓存模板内容
	templateCacheLock.Lock()
	templateCache[name] = content
	templateCacheLock.Unlock()

	log.Debug().Str("template", name).Msg("template loaded and cached")
	return content, nil
}

// getPreviewHTMLSnippet 获取完整的预览 HTML 片段
func getPreviewHTMLSnippet() string {
	base, err := loadTemplate("preview-base.html")
	if err != nil {
		// 如果加载失败，返回空字符串或者默认内容
		return ""
	}

	voice, err := loadTemplate("voice-transcribe.html")
	if err != nil {
		return base
	}

	return base + voice
}

// writeChatlogHTMLHeader 写入聊天记录 HTML 头部
func writeChatlogHTMLHeader(w io.Writer, title string) error {
	log.Debug().Str("title", title).Msg("writing chatlog HTML header")
	tmplContent, err := loadTemplate("chatlog-head.html")
	if err != nil {
		return err
	}

	tmpl, err := template.New("chatlog-head").Parse(tmplContent)
	if err != nil {
		log.Debug().Err(err).Msg("failed to parse chatlog-head template")
		return err
	}

	if err := tmpl.Execute(w, map[string]interface{}{
		"Title": title,
	}); err != nil {
		log.Debug().Err(err).Msg("failed to execute chatlog-head template")
		return err
	}
	return nil
}

// writeHTMLFooter 写入 HTML 页脚（包含预览组件）
func writeHTMLFooter(w io.Writer) error {
	log.Debug().Msg("writing HTML footer")
	snippet := getPreviewHTMLSnippet()
	if _, err := io.WriteString(w, snippet); err != nil {
		log.Debug().Err(err).Msg("failed to write preview snippet")
		return err
	}
	if _, err := io.WriteString(w, "</body></html>"); err != nil {
		log.Debug().Err(err).Msg("failed to write footer close tags")
		return err
	}
	return nil
}

// getPreviewSnippet 获取预览片段（懒加载，线程安全）
func getPreviewSnippet() string {
	previewSnippetOnce.Do(func() {
		previewHTMLSnippet = getPreviewHTMLSnippet()
	})
	return previewHTMLSnippet
}

// writeChatlogHTMLHeaderCompat 兼容旧版本的函数签名
func writeChatlogHTMLHeaderCompat(w io.Writer, title string) {
	log.Debug().Str("title", title).Msg("writing chatlog HTML header (compat)")
	var buf bytes.Buffer
	err := writeChatlogHTMLHeader(&buf, title)
	if err != nil {
		log.Debug().Err(err).Msg("failed to write chatlog header, using fallback")
		// 如果模板加载失败，使用硬编码的简单版本
		buf.WriteString("<html><head><meta charset=\"utf-8\"><title>")
		buf.WriteString(template.HTMLEscapeString(title))
		buf.WriteString("</title></head><body>")
	}
	w.Write(buf.Bytes())
}