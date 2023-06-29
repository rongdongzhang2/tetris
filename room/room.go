package room

import (
	"encoding/json"
	"errors"
	"math/rand"
	"sync"
	"tetris/user"
	"time"
)

// Room 保存房间记录
// 这里应该都是被保护的变量，不应该直接被访问，所以首字母小写
type Room struct {
	RoomId     string                `json:"room_id"`
	Owner      *user.User            `json:"owner"`       // 房主其实就是roomId
	UserList   map[string]*user.User `json:"user_list"`   // 房间内的用户列表
	GameStatus int                   `json:"game_status"` // 0-未开始,1-进行中
	RandArr    []int                 `json:"rand_arr"`    // 随机数数组
	Timeout    int                   `json:"timeout"`     // 超时时间,毫秒
	LastActive int64                 `json:"last_active"` // 最后活跃时间,会用定时任务清除掉长时间未活跃的房间
}

// List RoomList 房间列表
var List = make(map[string]*Room)

// 获取游戏棋盘数据时，需要加锁执行
var mutex sync.Mutex

// CreateRoom 创建房间
func CreateRoom(roomId string, owner *user.User) *Room {
	// 判断房间是否已经存在
	if _, ok := List[roomId]; ok {
		return List[roomId]
	}

	room := &Room{
		RoomId:     roomId,
		Owner:      owner,
		UserList:   map[string]*user.User{},
		GameStatus: 0,
		RandArr:    []int{},
		LastActive: time.Now().Unix(),
	}
	List[roomId] = room
	owner.SetRoomId(roomId)
	room.JoinRoom(owner)
	return room
}

// JoinRoom 加入房间
func (Room *Room) JoinRoom(User *user.User) error {
	// 判断游戏状态是否可以加入
	if Room.GameStatus != 0 {
		return errors.New("游戏已经开始，不能加入")
	}
	// 判断是否已经存在该用户
	if _, ok := Room.UserList[User.UserId]; ok {
		return nil // 如果存在，直接返回
	}

	Room.LastActive = time.Now().Unix()
	Room.UserList[User.UserId] = User
	User.SetRoomId(Room.RoomId)
	return nil
}

// GetRoom 获取房间信息
func GetRoom(roomId string) (*Room, error) {
	// 判断房间是否存在
	room, ok := List[roomId]
	if !ok {
		return nil, errors.New("房间不存在")
	}
	return room, nil
}

// StartGame 修改游戏状态为开始游戏
func (Room *Room) StartGame() error {
	// 游戏状态是否为0
	if Room.GameStatus != 0 {
		return errors.New("游戏已经开始，不能重复开始")
	}

	// 游戏人数是否大于1
	if len(Room.UserList) < 2 {
		return errors.New("游戏人数不足")
	}

	Room.GameStatus = 1
	// Timeout 5分钟 毫秒
	Room.Timeout = 5 * 60 * 1000
	Room.LastActive = time.Now().Unix()
	Room.RandArr = generateRandom()

	go func() {
		// 每个100毫秒 调用一次 SendGameStatus
		ticker := time.NewTicker(100 * time.Millisecond)

		for {
			select {
			case <-ticker.C:
				// 如果TimeOut时间到了，就停止定时器
				if Room.Timeout <= 0 {
					ticker.Stop()
					Room.GameOver()
					return
				}
				// 每次执行，TimeOut减少100毫秒
				Room.Timeout -= 100

				// 将房间信息发送给房间内的每个用户
				Room.SendGameStatus()
			}
		}

	}()

	return nil
}

// GameOver 游戏结束
func (Room *Room) GameOver() {
	// 修改游戏状态
	Room.GameStatus = 0
	Room.Timeout = 0
	Room.RandArr = []int{}
}

// GetRandArr 获取随机数
func (Room *Room) GetRandArr(index int) ([]int, error) {
	// 判断游戏状态是否为1
	if Room.GameStatus != 1 {
		return nil, errors.New("游戏未开始")
	}
	// 如果index已经接近尾，则再生成一批随机数
	if len(Room.RandArr)-index < 20 {
		// 加锁
		mutex.Lock()
		defer mutex.Unlock()

		// 再次判断，防止重复生成
		if len(Room.RandArr)-index < 20 {
			// 生成随机数
			randArr := generateRandom()
			// 追加到原来的数组中
			Room.RandArr = append(Room.RandArr, randArr...)
		}
	}

	Room.LastActive = time.Now().Unix()
	return Room.RandArr, nil
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

// SendMessage send message to all users in the room
// 排除部分用户
func (Room Room) SendMessage(message []byte, excludeUser []*user.User) {
	for _, User := range Room.UserList {
		// 排除部分用户
		for _, exclude := range excludeUser {
			if User == exclude {
				continue
			}
		}

		/*
			concurrent write to websocket connection
			需要解决并发写入的问题
		*/
		//conn.WriteMessage(websocket.TextMessage, message)

		User.Message <- message
	}
}

// SendGameStatus 发送游戏状态给所有用户
func (Room *Room) SendGameStatus() {
	// 获取游戏状态
	msg := make(map[string]any)
	// 游戏状态
	msg["game_status"] = Room.GameStatus
	// 游戏剩余时间
	msg["timeout"] = Room.Timeout // 毫秒
	// 每个用户的棋盘和分数
	msg["user_list"] = []any{}
	for _, User := range Room.UserList {
		userInfo := make(map[string]any)
		userInfo["token"] = User.UserId
		userInfo["score"] = User.Score
		userInfo["board"] = User.Board
		msg["user_list"] = append(msg["user_list"].([]any), userInfo)
	}

	jsonData := make(map[string]any)
	jsonData["command"] = "sync_game_status"
	jsonData["data"] = msg

	// 将msg json化
	message, _ := json.Marshal(jsonData)
	Room.SendMessage(message, nil)
}

// room info to json
func (Room *Room) ToJson() map[string]interface{} {
	// 获取游戏状态
	msg := make(map[string]interface{})
	// 游戏状态
	msg["game_status"] = Room.GameStatus
	// 游戏剩余时间
	msg["timeout"] = Room.Timeout // 毫秒
	// 随机数
	msg["rand_arr"] = Room.RandArr
	// 房主
	msg["owner"] = Room.Owner.UserId

	// 每个用户的棋盘和分数
	msg["user_list"] = []interface{}{}
	for _, User := range Room.UserList {
		userInfo := make(map[string]interface{})
		userInfo["token"] = User.UserId
		userInfo["score"] = User.Score
		userInfo["board"] = User.Board
		msg["user_list"] = append(msg["user_list"].([]interface{}), userInfo)
	}

	return msg
}
