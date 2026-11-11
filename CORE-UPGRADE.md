# LazyCat 节点核心升级记录

## 固定版本与范围

核心从 `wyx2685/xray-core 83ad74c46335`（报告版本 26.7.28）升级到
`e087e74a1bf2`（报告版本 26.9.30），与原版节点的 2026-11 核心选择一致。
对应 Go 伪版本为 `v0.0.0-20260930151924-e087e74a1bf2`。
实际核心由 go.mod 的 replace 指定，不能仅根据 require 行判断。

本次只同步核心及必要适配，不合并原版的全局设备准入和 Nowhere 协议。
保持现有面板 API、配置文件、凭证同步/定时拉取、用户撤销、限速与设备限制逻辑，
保留配置变更失败重试、重载回退、流量补报及 Hy2 端口跳跃实现。

## 必需适配

- TUIC 兼容监听器改用 FinalMask.ListenPacket；保留本地等待认证完成的修复，
  避免认证与首个 TCP/UDP 请求并发时丢失请求。
- SS2022 初始化用户使用 AES-128/256 对应的 16/32 字节密钥；构建时不再修改
  面板传入的 ServerKey，避免重载复用配置时再次编码。
- FakeDNS 错误构造适配新核心 API。
- 构建默认设置 `godebug multipathtcp=0`，并验证 Go 与 Xray 的 TCP 监听。
  这是应用层默认值，不代替宿主机 Linux 内核更新；运行环境 GODEBUG 或显式
  socket 设置仍可覆盖默认值。参考官方 26.10.31 安全公告。

## 回归覆盖

- VLESS、VMess、传统 SS、SS2022 AES-128/256、Trojan、AnyTLS、Hy2、TUIC：
  实际连接与数据收发、用户流量统计及删除用户。
- Hy2 Salamander，以及 TUIC TCP/两种 UDP 模式在有/无 Salamander 时的行为。
- SS2022 UDP 实际数据收发与用户流量统计。
- 现有节点用户更新、凭证同步、限额、流量补报和重载失败恢复测试。
- Linux 隔离网络：nft/iptables 的 IPv4/IPv6 转发、改范围和目标、清理。
- Linux 隔离网络：真实 Hy2 客户端从另一个网络命名空间经过主端口和两个
  跳跃端口连接本节点，覆盖 IPv4/IPv6、有/无 Salamander。
  此项检查各端口上的新连接，不等同于长时间公网跨端口迁移稳定性测试。

工作流完整测试与构建成功后才发布 amd64/arm64 安装包。
Windows 上不验证 POSIX 0600 权限，相关原有测试由 Linux CI 全部执行。

## 已知上游边界

原版记录的 Hy2 `udpSessionManager.clean/run` 对 `closed` 的并发读写问题
仍存在于固定核心中，旧核心也有相同形态。此次不修改第三方核心源码，也不宣称
升级修复了它；上述测试不能替代对该关闭路径的专项压力验证。
TUIC 上游出站仍未实现，不能据此宣称支持 TUIC 出站中继。

生产验收仍应先用一台节点验证现有客户端、端口跳跃和云安全组/NAT 配置。

## 来源

- 原版适配：https://github.com/perfect-panel/ppanel-node/commit/57dddf2fbdfcde2f10adce10e4be4f25a1f36aab
- 核心差异：https://github.com/wyx2685/Xray-core/compare/83ad74c46335...e087e74a1bf2
- 安全公告：https://github.com/XTLS/Xray-core/releases/tag/v26.10.31

升级前 LazyCat 基线：`f0bb7db22bc5a2aaf0d25de8b7a6be3a287b86db`。
