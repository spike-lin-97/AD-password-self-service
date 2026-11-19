// Package response 统一 JSON 响应格式。
package response

import "github.com/gin-gonic/gin"

// JsonRes 通用返回。code=0 成功，非 0 失败。
type JsonRes struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Hint    string      `json:"hint,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// OK 成功响应
func OK(c *gin.Context, httpCode int, message string, data interface{}) {
	c.JSON(httpCode, JsonRes{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

// Fail 失败响应
func Fail(c *gin.Context, httpCode int, hint, message string) {
	c.JSON(httpCode, JsonRes{
		Code:    -1,
		Message: message,
		Hint:    hint,
	})
}

// Abort 失败并中止后续处理（用于中间件）
func Abort(c *gin.Context, httpCode int, hint, message string) {
	c.AbortWithStatusJSON(httpCode, JsonRes{
		Code:    -1,
		Message: message,
		Hint:    hint,
	})
}
