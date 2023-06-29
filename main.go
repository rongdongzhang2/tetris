package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"net/http"
	"strconv"
	"tetris/middlewares"
	"tetris/room"
	"tetris/user"
)

var upgrade = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// 从get中获取token
		token := r.URL.Query().Get("token")
		if token == "" {
			return false
		}

		if !user.IsUserExist(token) {
			return false
		}

		return true
	},
}

// 定义一个任何类型的键值对，方便转换为json
type JSON map[string]any

func main() {
	r := gin.Default()

	r.Use(middlewares.Cors())

	r.StaticFS("/web", http.Dir("./web"))

	api := r.Group("/api")
	{
		// 用户注册
		api.POST("/login", login)
		// 创建房间
		api.POST("/create_room", createRoom)
		// 加入房间
		api.POST("/join_room", joinRoom)
		// 开始游戏
		api.POST("/start_game", startGame)
		// 同步棋盘数据
		api.POST("/sync_game", syncGame)
		// 获取游戏棋盘数据
		api.POST("/get_game_board", getGameBoard)
		// 获取房间信息
		api.GET("/get_room_info", getRoomInfo)

	}
	// ws连接
	r.GET("/ws", ws)

	// 监听80端口
	r.Run("0.0.0.0:80")

	go room.CheckRoomActive()
}

// 获取房间信息
func getRoomInfo(c *gin.Context) {
	User := getUser(c)
	if User == nil {
		return
	}

	// 获取房间id
	roomId := User.GetRoomId()
	if roomId == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "获取房间信息失败",
		})
		return
	}

	// 获取房间信息
	Room, err := room.GetRoom(roomId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": Room.ToJson(),
	})
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

	Room, err := room.GetRoom(roomId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 获取游戏棋盘
	RandArr, err := Room.GetRandArr(index)
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

	Room, err := room.GetRoom(roomId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	if err = Room.StartGame(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	jsonData := JSON{
		"command": "start_game",
		"data": JSON{
			"randArr": Room.RandArr,
			"timeout": Room.Timeout, // 毫秒
		},
	}

	val, _ := json.Marshal(jsonData)
	Room.SendMessage(val, nil)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "开始游戏成功",
	})
}

// 加入房间
func joinRoom(c *gin.Context) {
	// 加入房间应该判断游戏状态，如果正在游戏中，不可以加入房间
	User := getUser(c)
	if User == nil {
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

	Room, err := room.GetRoom(roomId)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	err = Room.JoinRoom(User)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 发送的消息
	message, _ := json.Marshal(JSON{
		"command": "join_room",
		"data": JSON{
			"token": User.UserId,
		},
	})

	// 排除当前用户
	Room.SendMessage(message, []*user.User{User})

	c.JSON(200, gin.H{
		"code": 1,
		"data": gin.H{"room_id": roomId, "token": User.UserId},
		"msg":  "success",
	})
}

// 创建房间
func createRoom(c *gin.Context) {
	// get user
	User := getUser(c)
	if User == nil {
		return
	}

	// 使用用户id来创建房间
	Room, _ := room.GetRoom(User.UserId)
	if Room == nil {
		// 创建房间
		Room = room.CreateRoom(User.UserId, User)
	}

	c.JSON(200, gin.H{
		"code": 1,
		"data": gin.H{"room_id": Room.RoomId},
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

// get User
func getUser(c *gin.Context) *user.User {
	token, err := getToken(c)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return nil
	}

	User, err := user.GetUser(token)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return nil
	}

	return User
}

// 登录
func login(c *gin.Context) {
	// 获取用户token
	token, err := getToken(c)

	if err != nil {
		// 创建用户
		token = uuid.New().String()
	}

	// 创建用户
	User := user.CreateUser(token)

	c.SetCookie("token", token, 86400, "*", "*", false, true)

	c.JSON(200, gin.H{
		"code": 1,
		"data": User.ToJson(),
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

	// 将 requestBody["board"] 转换为 [][]string 赋值给 gameData
	board := requestBody["board"].([]interface{})

	// 分数
	score := int(requestBody["score"].(float64))

	// index
	index := int(requestBody["index"].(float64))

	// 获取用户的token
	User := getUser(c)
	if User == nil {
		return
	}

	// 设置用户信息
	User.Score = score
	// Board
	User.Board = board
	User.Index = index

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

	User, err := user.GetUser(token)
	if err != nil {
		fmt.Println(err)
		ws.Close()
		return
	}

	// 设置用户 ws
	User.SetWS(ws)

	// 完成时关闭连接释放资源
	defer ws.Close()
	go func() {
		// 监听连接“完成”事件，其实也可以说丢失事件
		<-c.Done()
		// 删除元素
		User.SetWS(nil)

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
