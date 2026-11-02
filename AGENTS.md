# AGENTS.md — MY LOCAL NAME 开发指南

本文件面向在本仓库工作的 AI/人类开发者。先读 `spec.md` 了解产品与技术规格；本文件只讲**怎么改、怎么验、什么不能碰**。

## 1. 项目速览

- **产品**：局域网 mDNS 命名台。把「自定义主机名 + 本机 IPv4」广播为 `<name>.local`，可开始/停止、托盘驻留、开机自启。
- **技术栈**：Go 1.26 + Wails v3.1.0 + Vue 3 + TypeScript + Vite。
- **平台优先级**：Windows 10/11 首先保证；macOS / Linux 保持代码路径可用，不写 Windows-only 逻辑到公共层。
- **界面语言**：简体中文（技术读数可中英混排）。

## 2. 常用命令

```bash
# 安装前端依赖（首次）
cd frontend && npm install

# 开发（热重载，含绑定生成）
wails3 dev

# 只跑 Go 测试
go test ./...

# 竞态检测（提交前必跑）
go test -race ./...

# 前端类型检查 + 构建
cd frontend && npm run build

# 生产构建（Windows）
wails3 build

# 生成前端 bindings（改了导出的服务方法后）
wails3 generate bindings

# 打包
wails3 package
```

> `wails3` 需要 `$(go env GOPATH)/bin` 在 PATH 中。

## 3. 目录与职责

```
main.go                 Wails 启动、窗口、托盘、单实例、启动参数。保持薄。
app_service.go          唯一绑定给前端的服务；编排 engine/config/autostart/事件/托盘。
app_state.go            AppState / InterfaceOption / LogLine / Diagnostics DTO。
internal/mdns/          纯 Go mDNS 实现。禁止 import Wails 或前端相关包。
internal/config/        配置读写。原子写入，损坏时回退默认。
internal/applog/        内存环形日志 + 文件轮转。
assets/                 托盘图标（PNG + macOS 模板 PNG），通过 go:embed 引入。
frontend/src/           Vue 界面。
frontend/bindings/      Wails 生成代码，禁止手改。
build/                  平台构建资产；改 config.yml 后运行对应 update 任务。
```

## 4. 架构边界（必须遵守）

1. **mDNS 核心与 UI 解耦**：`internal/mdns` 只依赖标准库 + `golang.org/x/*`。状态通过回调（`OnState` / `OnLog`）向外抛，不引用 Wails。
2. **单一事实来源**：前端状态只能来自 `GetState()` 与 `state:update` 事件；不要在 Vue 里推断"应该正在广播"。
3. **托盘与 UI 同源**：任何 Start/Stop/配置变更都要走 `AppService` 的方法，由服务层统一刷新托盘 + 发事件。禁止在托盘回调里直接操作 engine。
4. **写 socket 串行化**：engine 内所有发送经同一把 `sendMu`，避免多个 goroutine 并发写导致报文交错。
5. **停止必须发 goodbye**：任何停止路径（用户停止、接口变更重启、冲突、退出）都要先发 TTL=0 的 A 记录，再关 socket。
6. **只应答自己的名字**：不得响应其他 FQDN、不得做递归、不得代理。查询匹配大小写不敏感。
7. **不抢名字**：探测到冲突立即失败并给出建议名，不允许"抢先宣告"。

## 5. mDNS 实现约束

- 组播：`224.0.0.251:5353/UDP`（IPv4）。v1 不做 IPv6。
- 记录：仅 A（QTYPE A / ANY 时应答），CLASS=IN，cache-flush 位，TTL=120s。
- 探测：3 次 ANY 查询（0/250/500ms，不设 QU 位，要求组播应答），监听 750ms；任何针对目标名字的响应都算冲突。Windows 多进程共享 5353 时，单播应答可能投递给错误 socket。
- 宣告：成功后立即 + 1s 两次；之后每 60s 一次；地址变化立即重发。
- goodbye：TTL=0，发 2 次（0/50ms），然后关闭。
- 传统单播查询（源端口 ≠ 5353）：单播回复，回显 ID，TTL=10。
- 解析 DNS 报文必须支持 compression pointer 且防环；编码不使用压缩。
- 端口可注入（测试用 15353），生产默认 5353。
- **Windows 必须 SO_REUSEADDR**，Unix 追加 SO_REUSEPORT，否则无法与系统 mDNS 共存。
- 组播回环（multicast loopback）必须开启，否则本机解析器收不到自己的宣告。
- 忽略自己的报文：源 IP 属于本机地址集合时不触发冲突。
- 自定义后缀只在协议层应答；不要宣称操作系统一定能解析非 `.local` 域名。

## 6. 命名与代码风格

- 新建文件名用 **snake_case**（`dns_message.go`、`socket_windows.go`、`name_console.vue`），测试文件 `<被测>_test.go`。既有文件（`main.go`、`App.vue`）不改名。
- Go：标准 `gofmt`；错误用 `%w` 包装；导出类型/方法写注释；不要在注释里引用 `spec.md` 或任何 md 文件，注释要自包含。
- Vue：`<script setup lang="ts">`；组件 props/emits 显式声明类型；不引入 UI 组件库。
- 图标：**禁止 emoji 当功能图标**；统一 24×24 内联 SVG，`stroke="currentColor"`、`stroke-width="2"`、round cap/join。可参考 IconPark / iconfont 造型。
- 颜色、间距、字体必须走 `frontend/src/style.css` 的 CSS 变量，禁止在组件里写散落色值（一次性装饰除外）。

## 7. 前端视觉约束

- 设计基因：**巴洛克聚光 + 洛可可粉彩 + 极客读数 + 大色块**。
- 定位是**小工具**：默认 800×600，最小 340×540；窄屏单列两段式（上操作 / 下状态），≥1000px 自动左右两栏。不做海报式大标题和大雷达；不要为了装饰挤压名字输入与开始/停止。
- 比例：墨色 ≥45%、奶油 ≥30%、粉 ≤15%、金线 <3%。不要三张等宽圆角卡片，不要蓝紫渐变，不要玻璃拟态。
- 色板、字体、布局见 `spec.md` §6；改视觉先改 tokens，再改组件。
- 状态必须"文字 + 颜色"双通道表达，不能只靠颜色。
- 所有动画必须在 `prefers-reduced-motion: reduce` 下停用。
- 无远程字体/CDN；离线可用。

## 8. 托盘 / 自启 / 窗口

- 无边框窗口；关闭按钮 = 隐藏到托盘（`WindowClosing` hook 里 `Hide()` + `Cancel()`）。
- 退出只允许托盘「退出」/ 程序退出流程；退出前必须 `Stop()` 发 goodbye。
- 托盘菜单状态：`● 广播中 · name.local` / `○ 待机` / `! 名称冲突`；开始/停止是同一菜单项。
- 自启用 Wails `app.Autostart`，标识符 `cn.hanq.mylocalname`，参数 `--autostart`；不要自己写注册表/plist（除非 Wails 在目标平台不支持且已有测试）。
- 默认前缀 = 系统主机名 + `-lan`，避免与系统自身 mDNS 主机名抢名。
- 窗口内 `F9` 触发开始/停止（与托盘同一 `ToggleBroadcast` 路径）。
- `--autostart` 启动：隐藏窗口；若 `autoBroadcast=true` 则自动 `StartBroadcast`；失败时显示窗口并报错。
- 单实例 ID：`cn.hanq.mylocalname`；二次启动唤醒并聚焦主窗口。

## 9. 配置与日志

- 配置路径 `os.UserConfigDir()/my-local-name/config.json`；写入用临时文件 + rename。
- 配置损坏：回退默认 + 日志警告，不删除用户原文件。
- 日志：内存 200 条；文件 `my-local-name.log`，>1MB 轮转 `.log.1`。
- 不要把外部查询来源 IP 写入常规日志（隐私）；接口名/地址可以。

## 10. 测试要求

- 新增/修改 `internal/mdns` 必须补单元测试；DNS 编解码要覆盖畸形包（不 panic）。
- `build/ios` 有非 iOS stub，保证 `go test ./...` 在桌面上可运行；不要删。
- 提交前：`go test ./...` + `cd frontend && npm run build` 必须通过。
- 涉及并发/定时器改动：`go test -race ./...`。
- 手工验收（Windows）：
  1. 启动广播 `my-local-name-test.local`；
  2. `Resolve-DnsName my-local-name-test.local -Type A` 返回本机 IP；
  3. `ping my-local-name-test.local` 成功；
  4. 停止后 `Clear-DnsClientCache; Resolve-DnsName ...` 失败；
  5. 托盘开始/停止/退出可用；
  6. 关闭窗口进程仍在。
- 改 UI 后至少检查 800×600、340×540 与 1200×600 三档，无溢出/重叠。

## 11. 已知坑

- **端口 5353 共享**：Windows 自带 Dnscache 常驻；不用 reuse 选项会 `address in use`。macOS/Linux 同理（mDNSResponder/Avahi）。
- **Windows 防火墙**：首次监听可能弹窗；被拒绝时本机可解析、别的设备可能不行。不要静默失败，诊断面板要给提示。
- **解析缓存**：停止广播后系统可能仍缓存最多 TTL 120s；验收时先 `Clear-DnsClientCache`（Win）/ `sudo killall -HUP mDNSResponder`（macOS）。
- **自定义后缀**：Windows/macOS 原生解析器只对 `.local` 走 mDNS；不要把它当成 bug 修，改成提示文案。
- **Wails 事件时序**：窗口加载完成前发出的事件会丢；前端挂载时必须先 `GetState()` 再订阅事件（或订阅后立刻拉一次）。
- **Vue 与 bindings**：改了 Go 服务方法签名后必须重新生成 bindings，否则前端类型报错。
- **构建资产**：`build/config.yml` 的 info 段是生成平台元数据的来源；改了要跑 `wails3 task common:update:build-assets`（Windows 下 `wails3 task common:update:build-assets`）。

## 12. 提交前检查清单

- [ ] `gofmt` / `go vet ./...` 干净
- [ ] `go test ./...` 通过（核心改动再加 `-race`）
- [ ] `cd frontend && npm run build` 通过
- [ ] 无 emoji 功能图标、无远程资源
- [ ] Start/Stop 路径都发 goodbye
- [ ] 冲突不会进入 broadcasting
- [ ] 托盘与 UI 状态一致
- [ ] Windows 端到端手工验证（至少解析 + 停止）
- [ ] 配置/日志路径在三个平台都使用 `os.UserConfigDir()` 派生
