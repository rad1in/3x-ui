package controller

import (
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// speedtestDownload streams ?bytes= of incompressible data so a master can time
// the link; the panel gzips responses, which would shrink a constant payload.
func (a *ServerController) speedtestDownload(c *gin.Context) {
	n, err := strconv.ParseInt(c.Query("bytes"), 10, 64)
	if err != nil || n < 1 || n > service.SpeedTestMaxBytes {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(rand.Uint32())
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Length", strconv.FormatInt(n, 10))
	c.Status(http.StatusOK)
	_, _ = io.CopyN(c.Writer, rand.NewChaCha8(seed), n)
}

// speedtestUpload discards the body, up to SpeedTestMaxBytes, so a master can
// time the opposite direction.
func (a *ServerController) speedtestUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.SpeedTestMaxBytes)
	if _, err := io.Copy(io.Discard, c.Request.Body); err != nil {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
		return
	}
	jsonMsg(c, "", nil)
}
