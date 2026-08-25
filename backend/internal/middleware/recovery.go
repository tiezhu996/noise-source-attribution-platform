package middleware

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
)

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("panic recovered", "request_id", c.GetString("request_id"), "path", c.Request.URL.Path, "panic", fmt.Sprint(recovered))
		c.Abort()
	})
}
