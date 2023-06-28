package room

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

// Room 保存房间记录
type Room struct {
	RoomId     string   `json:"room_id"`
	Owner      string   `json:"owner"`       // 房主其实就是roomId
	UserList   []string `json:"user_list"`   // 用户列表
	GameStatus int      `json:"game_status"` // 0-未开始,1-进行中
	RandArr    []int    `json:"rand_arr"`    // 随机数数组
	Timeout    int      `json:"timeout"`     // 超时时间
	LastActive int64    `json:"last_active"` // 最后活跃时间,会用定时任务清除掉长时间未活跃的房间
}

// RoomList 房间列表
var RoomList = make(map[string]*Room)

// IsRoomExist 判断房间是否已经存在
func IsRoomExist(roomId string) bool {
	_, ok := RoomList[roomId]
	return ok
}

// 获取游戏棋盘数据时，需要加锁执行
var mutex sync.Mutex

// CreateRoom 创建房间
func CreateRoom(roomId, owner string) string {
	// 判断房间是否已经存在
	if IsRoomExist(roomId) {
		return roomId
	}

	room := &Room{
		RoomId:     roomId,
		Owner:      owner,
		UserList:   []string{},
		GameStatus: 0,
		RandArr:    []int{},
		LastActive: time.Now().Unix(),
	}
	RoomList[roomId] = room
	return roomId
}

// JoinRoom AddUser 添加用户
func JoinRoom(roomId string, userId string) error {
	// 判断房间是否存在
	room, ok := RoomList[roomId]
	if !ok {
		return errors.New("房间不存在")
	}
	// 判断游戏状态是否可以加入
	if room.GameStatus != 0 {
		return errors.New("游戏已经开始，不能加入")
	}
	// 判断是否已经存在该用户
	for _, user := range room.UserList {
		if user == userId {
			return nil // 如果存在，直接返回
		}
	}
	room.LastActive = time.Now().Unix()
	room.UserList = append(room.UserList, userId)
	return nil
}

// GetRoom 获取房间信息
func GetRoom(roomId string) (*Room, error) {
	// 判断房间是否存在
	room, ok := RoomList[roomId]
	if !ok {
		return nil, errors.New("房间不存在")
	}
	return room, nil
}

// StartGame 修改游戏状态为开始游戏
func StartGame(roomId string) error {
	// 判断房间是否存在
	room, ok := RoomList[roomId]
	if !ok {
		return errors.New("房间不存在")
	}

	// 游戏状态是否为0
	if room.GameStatus != 0 {
		return errors.New("游戏已经开始，不能重复开始")
	}

	// 游戏人数是否大于1
	if len(room.UserList) < 2 {
		return errors.New("游戏人数不足")
	}

	room.GameStatus = 1
	room.Timeout = 5 * 60
	room.LastActive = time.Now().Unix()
	room.RandArr = generateRandom()

	return nil
}

// GetRandArr 获取随机数
func GetRandArr(roomId string, index int) ([]int, error) {
	// 判断房间是否存在
	room, ok := RoomList[roomId]
	if !ok {
		return nil, errors.New("房间不存在")
	}
	// 判断游戏状态是否为1
	if room.GameStatus != 1 {
		return nil, errors.New("游戏未开始")
	}
	// 如果index已经接近尾，则再生成一批随机数
	if len(room.RandArr)-index < 20 {
		// 加锁
		mutex.Lock()
		defer mutex.Unlock()

		// 再次判断，防止重复生成
		if len(room.RandArr)-index < 20 {
			// 生成随机数
			randArr := generateRandom()
			// 追加到原来的数组中
			room.RandArr = append(room.RandArr, randArr...)
		}
	}

	room.LastActive = time.Now().Unix()
	return room.RandArr, nil
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
