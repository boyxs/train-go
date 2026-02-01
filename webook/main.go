package main

import (
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"strings"
	"time"
)

func main() {
	db := initDB()
	server := initServer()
	// handler
	initUserHandler(db, server)

	err := server.Run(":8089")
	if err != nil {
		panic(err)
	}
}

func initUserHandler(db *gorm.DB, server *gin.Engine) {
	userDAO := dao.NewUserDAO(db)
	userRepository := repository.NewUserRepository(userDAO)
	userService := service.NewUserService(userRepository)
	//u := &web.UserHandler{}
	u := web.NewUserHandler(userService)
	u.RegisterRoutes(server)
}

func initDB() *gorm.DB {
	db, err := gorm.Open(mysql.Open("root:13520@tcp(localhost:3306)/webook"), &gorm.Config{})
	if err != nil {
		// 数据库都连接不上，就不要启动服务了
		panic("failed to connect database")
	}
	err = dao.InitTable(db)
	if err != nil {
		panic(err)
	}
	return db
}

func initServer() *gin.Engine {
	server := gin.Default()
	//server.Use(func(ctx *gin.Context) {}) // 自定义中间件
	server.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:3000"},
		//AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Content-Length", "Authorization"},
		AllowCredentials: true,
		AllowOriginFunc: func(origin string) bool {
			if strings.HasPrefix(origin, "http://localhost") {
				return true
			}
			return strings.Contains(origin, "https://github.com")
		},
		MaxAge: 12 * time.Hour,
	}))
	// session
	useSession(server)
	return server
}

func useSession(server *gin.Engine) {
	loginMiddleware := middleware.NewLoginMiddlewareBuilder().
		IgnorePaths("/user/register", "/user/login").
		Build()
	//store := cookie.NewStore([]byte("k6CswdUm75WKcbM68UQUuxVsHSpTCwgK"))
	store, err := redis.NewStore(16,
		"tcp",
		"localhost:6379",
		"",
		"",
		[]byte("k6CswdUm75WKcbM68UQUuxVsHSpTCwgK"),
	)
	if err != nil {
		panic(err)
	}
	server.Use(sessions.Sessions("ssid", store), loginMiddleware)
}
