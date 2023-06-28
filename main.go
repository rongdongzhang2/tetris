package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"net/http"
	"strconv"
	"tetris/middlewares"
	"tetris/room"
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

// 定义一个任何类型的键值对，方便转换为json
type JSON map[string]any

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
		// 用户注册
		api.POST("/login", login)

		// 同步棋盘数据
		api.POST("/sync_game", syncGame)

		// 创建房间
		api.POST("/create_room", createRoom)
		// 加入房间
		api.POST("/join_room", joinRoom)
		// 开始游戏
		api.POST("/start_game", startGame)
		// 获取游戏棋盘数据
		api.POST("/get_game_board", getGameBoard)

	}
	// ws连接
	r.GET("/ws", ws)

	// 监听80端口
	r.Run("0.0.0.0:80")

	go room.CheckRoomActive()
}

// 获取游戏棋盘
func getGameBoard(c *gin.Context) {
	// 获取房间号和index
	roomId := c.PostForm("room_id")
	index, _ := strconv.Atoi(c.PostForm("index"))
	if roomId == "" || index == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "获取游戏棋盘失败",
		})
		return
	}

	RandArr, err := room.GetRandArr(roomId, index)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 返回随机数组
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": JSON{"randArr": RandArr},
	})
}

// 开始游戏
func startGame(c *gin.Context) {
	// 获取房间号
	roomId := c.PostForm("room_id")
	if roomId == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "开始游戏失败，请输入房间id",
		})
		return
	}

	err := room.StartGame(roomId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	Room, _ := room.GetRoom(roomId)

	jsonData := JSON{
		"command": "start_game",
		"data": JSON{
			"randArr": Room.RandArr,
			"timeout": Room.Timeout,
		},
	}

	val, _ := json.Marshal(jsonData)

	// 循环房间内的用户，发送开始游戏的消息
	for _, v := range Room.UserList {
		// 获取连接
		conn := wsconnect[v]
		if conn == nil {
			continue
		}
		// 推送游戏开始指令
		conn.WriteMessage(websocket.TextMessage, val)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "开始游戏成功",
	})
}

// 加入房间
func joinRoom(c *gin.Context) {
	// 加入房间应该判断游戏状态，如果正在游戏中，不可以加入房间
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

	if roomId == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "加入房间失败，请输入房间id",
		})
		return
	}

	err = room.JoinRoom(roomId, token)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 给房间内的其他用户发送提醒，有人加入
	Room, _ := room.GetRoom(roomId)

	// 发送的消息
	message, _ := json.Marshal(JSON{
		"command": "join_room",
		"data": JSON{
			"token": token,
		},
	})

	// 循环room.UserList
	for _, v := range Room.UserList {
		if v == token {
			continue
		}

		// 获取连接
		conn := wsconnect[v]
		if conn == nil {
			continue
		}

		// 发送消息
		conn.WriteMessage(websocket.TextMessage, message)
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

	// 判断房间是否已经存在
	isExist := room.IsRoomExist(token)
	if isExist {
		c.JSON(200, gin.H{
			"code": 1,
			"data": gin.H{"room_id": token},
			"msg":  "success",
		})
		return
	}

	// 创建房间
	roomId := room.CreateRoom(token, token)
	// 把当前用户加入到房间中
	_ = room.JoinRoom(roomId, token)

	c.JSON(200, gin.H{
		"code": 1,
		"data": gin.H{"room_id": roomId},
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
	var tokenJson map[string]interface{}

	c.ShouldBindJSON(&tokenJson)
	if val, ok := tokenJson["token"]; ok && val != "" {
		token = tokenJson["token"].(string)
		return token, nil
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

func syncGame(c *gin.Context) {
	// 获取post的json数据
	var requestBody map[string]interface{}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		fmt.Println(err)
		c.JSON(400, gin.H{"error": "Invalid JSON payload"})
		return
	}

	// 获取房间号
	roomId := requestBody["room_id"].(string)

	// 获取游戏棋盘数据
	gameData := requestBody["board"]

	// 获取用户的token
	token, err := getToken(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	message, _ := json.Marshal(JSON{
		"command": "sync_game",
		"data": JSON{
			"board": gameData,
			"token": token,
		},
	})

	Room, _ := room.GetRoom(roomId)

	// 循环room.UserList
	for _, v := range Room.UserList {
		if v == token {
			continue
		}
		conn := wsconnect[v]
		if conn == nil {
			continue
		}

		// 发送消息
		conn.WriteMessage(websocket.TextMessage, message)
	}

	c.JSON(200, gin.H{
		"code": 1,
		"data": "OK",
	})

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
