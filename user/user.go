package user

import (
	"errors"
	"github.com/gorilla/websocket"
	"time"
)

type User struct {
	UserId     string          `json:"token"`
	roomId     string          `json:"room_id"`
	ws         *websocket.Conn `json:"_"`           // 不需要转换为json
	lastActive int64           `json:"last_active"` // 最后活跃时间,会用定时任务清除掉长时间未活跃的房间
	Message    chan []byte     `json:"_"`           // 不需要转换为json
}

// List  用户列表
var List = make(map[string]*User)

// IsUserExist 判断用户是否已经存在
func IsUserExist(userId string) bool {
	_, ok := List[userId]
	return ok
}

// CreateUser 创建用户
func CreateUser(userId string) *User {
	// 判断用户是否已经存在
	if IsUserExist(userId) {
		return List[userId]
	}

	user := &User{
		UserId:     userId,
		Message:    make(chan []byte, 10),
		lastActive: time.Now().Unix(),
	}
	List[userId] = user
	return user
}

// GetUser 获取用户
func GetUser(userId string) (*User, error) {
	// 判断用户是否已经存在
	if !IsUserExist(userId) {
		return nil, errors.New("用户不存在")
	}

	return List[userId], nil
}

// SetWS 设置用户 ws
func (u *User) SetWS(ws *websocket.Conn) {
	u.ws = ws
	go u.WaitMessage()
}

// WaitMessage 等待 Message chan 发送消息
func (u *User) WaitMessage() {
	// 使用通道的做法，是为了解决并发问题
	// concurrent write to websocket connection

	for message := range u.Message {
		u.ws.WriteMessage(websocket.TextMessage, message)
	}
}

// SetRoomId 设置用户房间号
func (u *User) SetRoomId(roomId string) {
	u.roomId = roomId
}

// GetRoomId 获取用户房间号
func (u *User) GetRoomId() string {
	return u.roomId
}

// GetToken get token
func (u *User) GetToken() string {
	return u.UserId
}

// ToJson 转换为json
func (u *User) ToJson() map[string]interface{} {
	return map[string]interface{}{
		"token":       u.UserId,
		"room_id":     u.roomId,
		"last_active": u.lastActive,
	}
}
