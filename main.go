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
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"tetris/middlewares"
	"tetris/util"
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
		// 加入房间 todo 这里可以给房间主人推送有人加入房间的消息
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
}

// 获取游戏棋盘
// todo 因为这里可能存在并发获取问题，所需要加一个锁
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

	// 获取房间的随机数
	val := rdb.HGet(ctx, "tetris_room_randArr", roomId).Val()
	randArr := make([]int, 0)
	if val == "" {
		// 创建一批随机数 100个
		randArr = generateRandom()

		// 将数组的int转换string
		var strArr []string
		for _, v := range randArr {
			strArr = append(strArr, fmt.Sprintf("%d", v))
		}

		// 将随机数存入redis
		err := rdb.HSet(ctx, "tetris_room_randArr", roomId, strings.Join(strArr, ",")).Err()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"msg":  err.Error(),
			})
			return
		}
	} else {
		randArri := strings.Split(val, ",")
		for _, v := range randArri {
			// 将v转换为int
			num, _ := strconv.Atoi(v)
			randArr = append(randArr, num)
		}
	}
	// 数组长度
	randLen := len(randArr)

	// 查看当前已经进行到的索引位置
	if randLen-index < 20 {
		// 生成新的随机数
		randArrNew := generateRandom()
		// 合并数组
		randArr = append(randArr, randArrNew...)
		// 将数组的int转换string
		var strArrNew []string
		for _, v := range randArr {
			strArrNew = append(strArrNew, fmt.Sprintf("%d", v))
		}

		// 保存到Redis
		err := rdb.HSet(ctx, "tetris_room_randArr", roomId, strings.Join(strArrNew, ",")).Err()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"msg":  err.Error(),
			})
			return
		}
	}

	// 返回随机数组
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": JSON{"randArr": randArr},
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

	// 创建一批随机数 100个
	randArr := generateRandom()

	// 将数组的int转换string
	var strArr []string
	for _, v := range randArr {
		strArr = append(strArr, fmt.Sprintf("%d", v))
	}

	// 将随机数存入redis
	err := rdb.HSet(ctx, "tetris_room_randArr", roomId, strings.Join(strArr, ",")).Err()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 获取房间内的所有用户，并且推送游戏开始指令
	result, err := rdb.HGet(ctx, "tetris_room", roomId).Result()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "1" + err.Error(),
		})
		return
	}

	// 开始游戏的消息内容，包括随机数组和开始游戏的标识
	jsonData := JSON{
		"command": "start_game",
		"data": JSON{
			"randArr": randArr,
			"timeout": 5 * 60, // 5分钟游戏时间
		},
	}

	val, err := json.Marshal(jsonData)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}
	// 解析result为数组
	arr := strings.Split(result, ",")
	for _, v := range arr {
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

// 创建一批随机数
func generateRandom() []int {
	var arr []int
	for i := 0; i < 100; i++ {
		// 创建一个随机数 1-20
		randNum := rand.Intn(20) + 1
		arr = append(arr, randNum)
	}
	return arr
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
	// 去重
	arr = util.RemoveDuplicates(arr)

	// 拼接result和token
	value := strings.Join(arr, ",")

	err = rdb.HSet(ctx, "tetris_room", roomId, value).Err()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
		})
		return
	}

	// 循环uniqueSlice
	for _, v := range arr {
		message, _ := json.Marshal(JSON{
			"command": "join_room",
			"data": JSON{
				"token": token,
			},
		})
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

	// 将棋盘数据发送给房间内的其他用户
	// 获取房间内的所有用户
	result, err := rdb.HGet(ctx, "tetris_room", roomId).Result()
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
		},
	})

	arr := strings.Split(result, ",")

	for _, v := range arr {
		// 跳过当前用户
		if v == token {
			continue
		}
		// 获取连接
		conn := wsconnect[v]
		if conn == nil {
			continue
		}

		// 发送消息
		err = conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			fmt.Println(err)
		}
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
