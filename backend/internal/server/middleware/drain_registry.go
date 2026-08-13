package middleware

import "github.com/gin-gonic/gin"

const longLivedConnectionRegistryKey = "sub2api:long_lived_connection_registry"

// LongLivedConnectionRegistry tracks upgraded connections that net/http no
// longer owns after hijacking. closeFn must force the connection closed without
// waiting for a peer response so the shutdown deadline remains bounded.
type LongLivedConnectionRegistry interface {
	RegisterLongLivedConnection(closeFn func() error) func()
}

// SetLongLivedConnectionRegistry makes the process drain registry available to
// handlers without coupling the handler package back to the server package.
func SetLongLivedConnectionRegistry(c *gin.Context, registry LongLivedConnectionRegistry) {
	if c == nil || registry == nil {
		return
	}
	c.Set(longLivedConnectionRegistryKey, registry)
}

// RegisterLongLivedConnection registers an upgraded connection when the
// lifecycle middleware is installed. Tests and standalone handlers without the
// middleware receive a no-op release function.
func RegisterLongLivedConnection(c *gin.Context, closeFn func() error) func() {
	if c == nil {
		return func() {}
	}
	value, ok := c.Get(longLivedConnectionRegistryKey)
	if !ok {
		return func() {}
	}
	registry, ok := value.(LongLivedConnectionRegistry)
	if !ok || registry == nil {
		return func() {}
	}
	return registry.RegisterLongLivedConnection(closeFn)
}
