package main

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"net/http"
	"time"
)

var rdb *redis.Client

var ctx = context.Background()

var upgrade = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// 从get中获取token
		token := r.URL.Query().Get("token")
		if token == "1234" {
			return true
		}

		return false
	},
}

func main() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 没有密码，默认值
		DB:       0,  // 默认DB 0
	})
	// 判断是否连接成功
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(err)
	}

	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.GET("/ws", func(c *gin.Context) {
		// 升级成 websocket 连接
		ws, err := upgrade.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			// 处理错误
			fmt.Println(err)
			return
		}
		// 完成时关闭连接释放资源
		defer ws.Close()
		go func() {
			// 监听连接“完成”事件，其实也可以说丢失事件
			<-c.Done()
			// 这里也可以做用户在线/下线功能
			fmt.Println("ws lost connection")
		}()
		for {
			// 读取客户端发送过来的消息，如果没发就会一直阻塞住
			mt, message, err := ws.ReadMessage()
			if err != nil {
				fmt.Println("read error")
				fmt.Println(err)
				break
			}
			if string(message) == "ping" {
				message = []byte("pong")
			}
			err = ws.WriteMessage(mt, message)
			if err != nil {
				fmt.Println(err)
				break
			}
		}
	})

	// 用户注册
	r.POST("/register", register)

	r.Run() // listen and serve on 0.0.0.0:8080
}

// 注册
func register(c *gin.Context) {
	// 获取用户的唯一标识
	uniqueid := c.PostForm("uniqueid")
	// 如果为空，直接返回失败
	if uniqueid == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "uniqueid is empty",
		})
		return
	}

	// 判断是否在Redis sorted set中
	ctx := context.Background()
	_, err := rdb.ZScore(ctx, "uniqueid", uniqueid).Result()
	// 如果存在，返回失败
	if err == nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "uniqueid is exist",
		})
		return
	}

	// 如果不存在，直接添加
	if err == redis.Nil {
		rdb.ZAdd(ctx, "uniqueid", redis.Z{
			Score:  float64(time.Now().Unix()),
			Member: uniqueid,
		})
		c.JSON(http.StatusOK, gin.H{
			"code": 1,
			"msg":  "register success",
		})
		return
	}
}
