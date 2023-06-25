package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"net/http"
	"strings"
	"tetris/middlewares"
)

// 保存全局的Redis
var rdb *redis.Client
var ctx = context.Background()

// 创建一个map，保存websocket的连接
var wsconnect = make(map[string]*websocket.Conn)

var upgrade = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// 从get中获取token
		token := r.URL.Query().Get("token")
		if token == "" {
			return false
		}

		// TODO 判断是否存在该token

		return true
	},
}

type jsonToken struct {
	token string `json:"token"`
}

func connectRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 没有密码，默认值
		DB:       0,  // 默认DB 0
	})
	// 判断是否连接成功
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Println("Redis 连接失败，不能启动")
		panic(err)
	}
}

func main() {
	// 连接Redis
	connectRedis()
	r := gin.Default()

	r.Use(middlewares.Cors())

	r.StaticFS("/web", http.Dir("./web"))

	api := r.Group("/api")
	{
		//
		api.GET("/ping", ping)
		// 同步棋盘数据
		api.GET("/sync", sync)

		// 用户注册
		api.POST("/login", login)

		// 创建房间
		api.POST("/create_room", createRoom)
		// 加入房间
		api.POST("/join_room", joinRoom)

	}
	// ws连接
	r.GET("/ws", ws)

	r.Run() // listen and serve on 0.0.0.0:8080
}

// 加入房间
func joinRoom(c *gin.Context) {
	token, err := getToken(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 获取房间id
	roomId := c.PostForm("room_id")

	fmt.Println(roomId)
	if roomId == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "加入房间失败，请输入房间id",
		})
		return
	}

	result, err := rdb.HGet(ctx, "tetris_room", roomId).Result()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 解析result为数组
	arr := strings.Split(result, ",")
	arr = append(arr, token)

	// 对arr进行去重
	// 创建一个map用于存储切片中的元素，初始为空结构体
	seen := make(map[string]struct{})

	// 创建一个新的切片用于存储去重后的元素
	var uniqueSlice []string

	// 遍历原始切片
	for _, element := range arr {
		// 如果元素不在map中，则表示未重复，将其添加到去重切片和map中
		if _, ok := seen[element]; !ok {
			seen[element] = struct{}{}
			uniqueSlice = append(uniqueSlice, element)
		}
	}

	// 拼接result和token
	value := strings.Join(uniqueSlice, ",")

	err = rdb.HSet(ctx, "tetris_room", roomId, value).Err()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"code": 1,
		"data": gin.H{"room_id": roomId, "token": token},
		"msg":  "success",
	})
}

// 创建房间
func createRoom(c *gin.Context) {
	token, err := getToken(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 直接用token做房间号来创建
	err = rdb.HSet(ctx, "tetris_room", token, token).Err()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"code": 1,
		"data": gin.H{"room_id": token},
		"msg":  "success",
	})

}

func getToken(c *gin.Context) (token string, err error) {
	// 获取用户的cookie
	token, _ = c.Cookie("token")
	if token != "" {
		return token, nil
	}

	token = c.Query("token")
	if token != "" {
		return token, nil
	}

	token = c.PostForm("token")
	if token != "" {
		return token, nil
	}
	var tokenJson jsonToken

	c.BindJSON(&tokenJson)
	if tokenJson.token != "" {
		token = tokenJson.token
	}

	return "", errors.New("用户没有token，没有登录")
}

// 登录
func login(c *gin.Context) {
	// 获取用户的cookie
	token, _ := c.Cookie("token")
	// 如果uuid不为空，直接返回登录成功，并且返回uuid
	if token == "" {
		// 创建一个uuid，并且写入到cookie中
		token = uuid.New().String()
	}

	c.SetCookie("token", token, 86400, "*", "*", false, true)

	c.JSON(200, gin.H{
		"code": 1,
		"data": gin.H{"token": token},
		"msg":  "success",
	})

}

func ping(c *gin.Context) {
	// 如果存在，则发送消息到1234通道
	ws, ok := wsconnect["1234"]
	if ok {
		message := []byte("Hello")
		ws.WriteMessage(1, message)
	}

	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func sync(c *gin.Context) {
	// 获取房间id
	roomId := c.PostForm("room_id")
	if roomId == "" {
		c.JSON(200, gin.H{
			"code": 0,
			"msg":  "请输入房间id",
		})
	}

	// 通过roomId找到对手的token

}

func ws(c *gin.Context) {
	// 升级成 websocket 连接
	ws, err := upgrade.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// 处理错误
		fmt.Println(err)
		return
	}
	// 获取连接的token
	token := c.Request.URL.Query().Get("token")
	// 保存连接
	wsconnect[token] = ws

	// 完成时关闭连接释放资源
	defer ws.Close()
	go func() {
		// 监听连接“完成”事件，其实也可以说丢失事件
		<-c.Done()
		// 删除元素
		delete(wsconnect, token)

		// 这里也可以做用户在线/下线功能
		fmt.Println("ws lost connection")
	}()

	for {
		// 读取客户端发送过来的消息，如果没发就会一直阻塞住R
		mt, message, err := ws.ReadMessage()
		fmt.Println(mt)
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
}
