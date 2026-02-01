package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func main() {
	// 创建带默认中间件（日志与恢复）的 Gin 路由器
	server := gin.Default()

	// 定义简单的 GET 路由
	server.GET("/ping", func(c *gin.Context) {
		// 返回 JSON 响应
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	// 路由参数
	server.GET("/user/:name", func(ctx *gin.Context) {
		name := ctx.Param("name")
		result := strings.ToUpper(name[:1]) + name[1:]
		ctx.String(http.StatusOK, "Hello %s", result)
	})
	// 此 handler 将匹配 /user/john/ 和 /user/john/send
	// 如果没有其他路由匹配 /user/john，它将重定向到 /user/john/
	server.GET("/user/:name/*action", func(c *gin.Context) {
		name := c.Param("name")
		action := c.Param("action")
		message := name + " is " + action
		c.String(http.StatusOK, message)
	})

	// 查询参数
	// 示例 URL：/welcome?firstname=Jane&lastname=Doe
	server.GET("/welcome", func(ctx *gin.Context) {
		firstname := ctx.DefaultQuery("firstname", "Guest")
		lastname := ctx.Query("lastname") // c.Request.URL.Query().Get("lastname") 的一种快捷方式
		ctx.String(http.StatusOK, "Hello %s %s", firstname, lastname)
	})

	// Query 和 post form（表单参数）
	/**
	POST /post?id=1234&page=1 HTTP/1.1
	Content-Type: application/x-www-form-urlencoded
	name=manu&message=this_is_great
	*/
	server.POST("/post", func(ctx *gin.Context) {
		id := ctx.Query("id")
		page := ctx.DefaultQuery("page", "0")
		name := ctx.PostForm("name")
		message := ctx.PostForm("message")
		fmt.Printf("id: %s; page: %s; name: %s; message: %s", id, page, name, message)
	})

	// body 参数
	type UserRequest struct {
		Name string `json:"name"` // tag 对应 json key
		Age  int    `json:"age"`
	}
	server.POST("/user", func(ctx *gin.Context) {
		var req UserRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"name": req.Name, "age": req.Age})
	})

	// 默认端口 8080 启动服务器
	// 监听 0.0.0.0:8080（Windows 下为 localhost:8080）
	err := server.Run(":8080")
	if err != nil {
		return
	}
}
