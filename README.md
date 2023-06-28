# 俄罗斯方块游戏

一个在线的俄罗斯方块游戏，可以邀请你的好友和你对战。

## 游戏玩法

单人纯玩模式
多人对战模式(房间模式)

## 功能

1. 创建房间
2. 邀请加入(可前端直接复制邀请链接)
3. 房间状态(各个选手的准备状态)
4. 开始游戏
5. 游戏数据同步
6. 游戏结束(公布结果)

## 功能设计

- [ ] 进入对战页面即登录，会返回一个唯一的id(token)，服务器会写入到cookie中
- [ ] 通过token去链接ws
- [ ] 以上步骤都成功之后才可以创建房间或者加入房间
- [ ] 创建房间，会的到一个房间id(room_id)，会直接用创建人的token来作为房间id (此时房间处于等待状态)
- [ ] 在房间中，显示邀请链接，可以邀请好友加入房间
- [ ] 加入房间后，通知在房间内的其他用户，有人加入房间(记录当前用户正在的房间号，方便刷新时也在房间内)
- [ ] 房主开始游戏，通过ws发送通知给其他用户。并且发送游戏棋盘数据(随机生成一批方块)
- [ ] 定时同步游戏棋盘数据给其他用户(包括游戏状态(正在玩，还是已经挂了))
- [ ] 游戏结束，发送游戏结果给其他用户


## 2. 前端设计

前端下载地址
https://codepen.io/zxllxl/pen/NmEmja


