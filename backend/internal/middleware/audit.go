package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditBoundary logs only request metadata. Domain services persist immutable
// before/after evidence atomically with each successful write transaction.
func AuditBoundary(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		actor, _ := ActorFromContext(c)
		logger.Info("write request completed",
			"request_id", c.GetString("request_id"), "actor", actor.Username, "role", actor.Role,
			"method", c.Request.Method, "path", c.FullPath(), "status", c.Writer.Status(),
			"duration_ms", time.Since(started).Milliseconds(),
		)
		c.Next()
	}
}
