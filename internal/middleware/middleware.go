// Package middleware 暴露 Gin 中间件，用于 HTTP 服务器：
// 请求日志、CORS、JWT 认证、管理员守卫和权限检查。
package middleware

// Context keys for values produced by the auth middleware.
const (
	CtxUserID       = "ctx_user_id"
	CtxUserRole     = "ctx_user_role"
	CtxUserTier     = "ctx_user_tier"
	CtxTokenPurpose = "ctx_token_purpose"
	CtxTokenMediaID = "ctx_token_media_id"

	// AccessTokenCookieName carries the web access token for browser-managed
	// resource requests such as <img>, which cannot attach Authorization.
	AccessTokenCookieName = "msgo_access_token"
	AccessTokenCookiePath = "/api"
)
