// Package decoy answers requests that hit no panel or subscription route, or
// arrive under a foreign Host, the way an ordinary web server would.
package decoy

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ModeOff      = ""
	ModeNginx    = "nginx"
	ModeRedirect = "redirect"
	ModeHTML     = "html"
)

// Site is the decoy the admin chose: Target is used by redirect, HTML by html.
type Site struct {
	Mode   string
	Target string
	HTML   string
}

// ValidTarget reports whether target is an absolute http(s) URL a redirect may point at.
func ValidTarget(target string) bool {
	u, err := url.Parse(strings.TrimSpace(target))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Handler serves the decoy, or is nil when it is off or cannot work.
func (s Site) Handler() gin.HandlerFunc {
	switch s.Mode {
	case ModeNginx:
		return serveNginx
	case ModeRedirect:
		target := strings.TrimSpace(s.Target)
		if !ValidTarget(target) {
			return nil
		}
		return func(c *gin.Context) {
			bare(c)
			c.Redirect(http.StatusFound, target)
			c.Abort()
		}
	case ModeHTML:
		if strings.TrimSpace(s.HTML) == "" {
			return nil
		}
		body := []byte(s.HTML)
		return func(c *gin.Context) {
			bare(c)
			c.Data(http.StatusOK, "text/html; charset=utf-8", body)
			c.Abort()
		}
	default:
		return nil
	}
}

// bare drops the headers the panel's middleware set (CSP with a nonce, ...), which
// no plain web server sends; the gzip writer's own ones must stay with the body.
func bare(c *gin.Context) {
	h := c.Writer.Header()
	for k := range h {
		if k != "Content-Encoding" && k != "Vary" {
			delete(h, k)
		}
	}
	h.Set("Server", "nginx")
}

const nginxWelcome = `<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
html { color-scheme: light dark; }
body { width: 35em; margin: 0 auto;
font-family: Tahoma, Verdana, Arial, sans-serif; }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
`

func nginxError(text string) []byte {
	return []byte("<html>\r\n<head><title>" + text + "</title></head>\r\n<body>\r\n<center><h1>" + text +
		"</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n")
}

// serveNginx answers like a fresh nginx install: the welcome page at the root,
// 404 elsewhere, and 405 for methods a static site does not take.
func serveNginx(c *gin.Context) {
	bare(c)
	status, body := http.StatusNotFound, nginxError("404 Not Found")
	switch {
	case c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead:
		status, body = http.StatusMethodNotAllowed, nginxError("405 Not Allowed")
	case c.Request.URL.Path == "/" || c.Request.URL.Path == "/index.html":
		status, body = http.StatusOK, []byte(nginxWelcome)
	}
	if c.Request.Method == http.MethodHead {
		c.Status(status)
		c.Header("Content-Type", "text/html")
		c.Abort()
		return
	}
	c.Data(status, "text/html", body)
	c.Abort()
}
