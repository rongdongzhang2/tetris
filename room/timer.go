package room

import "time"

// CheckRoomActive 定时检查房间是否活跃，将长期不活跃的房间删除
func CheckRoomActive() {
	for {
		for _, v := range List {
			if time.Now().Unix()-v.LastActive > 60 {
				// 删除房间
				delete(List, v.RoomId)
			}
		}
		time.Sleep(10 * time.Second)
	}
}
