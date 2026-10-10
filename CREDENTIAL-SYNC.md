# 即时凭证同步（本地改版）

基线：上游 `c1ccab9`。配套后端见相邻 `ppanel-server` 项目的
`docs/node-credential-sync.md`。本次不升级上游已固定的核心版本。

- 使用现有 ApiHost / ServerID / SecretKey，不增加节点入站端口。
- 每个启用的协议控制器建立一条 WebSocket，空闲时回应心跳。
- 收到凭证更新，只获取该用户的最新授权记录；热更新完成并确认核心
  认证表可见后，才回复成功。重复 UUID 不重复查询和修改核心。
- 定时拉取与即时更新串行执行，避免旧列表覆盖新凭证。
- 断线采用带随机抖动的退避重连（约 1～60 秒）；重连后补拉完整名单。
  原有定时拉取间隔不变，旧后端没有新接口时仍通过定时拉取运行。
- 删除旧凭证可能主动断开原有连接，本功能不承诺无感不断流。
- 公网必须使用 HTTPS/WSS，反向代理放行 WebSocket Upgrade，空闲超时
  建议大于 75 秒。不要记录 `X-Node-Secret` 请求头。
- 同一 ServerID / protocol 不应复用于多个独立节点副本，本版本没有
  实现同身份多副本的逐实例确认。

测试命令：`go test ./api/panel ./node ./core ./limiter`。
支持 CGO 的环境应追加 `go test -race ./node ./api/panel ./core ./limiter`。
正式上线前，先对一台测试节点验证重连、暂停/删除、UUID 轮换与各协议。
