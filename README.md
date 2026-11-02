# MY LOCAL NAME · 局域网命名台

像树莓派一样，给这台机器一个局域网名字：输入 `hanq-pi`，点击开始广播，同一网络里的设备就能用 `hanq-pi.local` 访问它。

Wails v3 + Go + Vue 3 桌面应用，Windows 优先，macOS / Linux 代码路径完整。界面是巴洛克聚光 + 洛可可粉彩 + 大色块 + 极客读数。

## 功能

- 自定义主机名 + 默认 `.local` 后缀，可配置其他后缀（如 `.lan`、`.home`）
- mDNS 广播 A 记录（RFC 6762），支持探测冲突、周期宣告、goodbye
- 开始 / 停止广播；停止后立即不再应答
- 系统托盘常驻：查看状态、开始/停止、复制地址、开机自启、退出
- 关闭窗口只隐藏到托盘，不退出；退出前自动发送 goodbye
- 开机自启（Windows HKCU Run / macOS LaunchAgent / Linux XDG autostart），可自启后自动广播
- 多网卡选择、地址变化自动重新宣告
- 内存 + 文件日志、诊断命令、冲突建议名
- `F9` 快速开始 / 停止
- 窗口默认 800×600，可缩至 340×540；宽屏自动左右两栏，窄屏自动收紧
- 全离线：无 CDN、无远程字体、无遥测

## 快速使用

1. 启动应用，在左侧填写名字前缀（默认是系统主机名 + `-lan`）。
2. 后缀保持 `.local`，点击「开始广播」。
3. 在另一台设备上执行：
   - Windows: `Resolve-DnsName <名字>.local -Type A` 或 `ping <名字>.local`
   - macOS: `dns-sd -G v4 <名字>.local` 或 `ping <名字>.local`
   - Linux: `avahi-resolve -n <名字>.local` 或 `resolvectl query <名字>.local`
4. 点击「停止广播」，发送 TTL=0 的 goodbye；系统缓存最长 120 秒，验证前可清理缓存。

## 开发

```bash
# 前端依赖（首次）
cd frontend && npm install

# 开发模式（热重载 + 自动生成 bindings）
wails3 dev

# Go 测试
go test ./...

# 竞态检测
go test -race ./...

# 前端构建（含类型检查）
cd frontend && npm run build

# Windows 生产构建
wails3 build
```

构建产物：`bin/my-local-name.exe`。

`wails3` 需要 `$(go env GOPATH)/bin` 在 PATH 中。

## 目录

```
main.go              应用启动、窗口、托盘、单实例、F9
app_service.go       绑定给前端的服务：状态、启动/停止、配置、诊断
app_state.go         DTO 定义
internal/mdns/       mDNS 协议实现（纯 Go，可独立测试）
internal/config/     配置读写（原子写入）
internal/applog/     环形日志 + 文件轮转
assets/              托盘图标 / macOS 模板图标
frontend/src/        Vue 界面
frontend/bindings/   Wails 自动生成，勿手改
```

## 平台说明

### Windows

- 需要 WebView2 Runtime（Windows 11 自带，Windows 10 一般已安装）。
- 首次监听 UDP 5353 可能触发防火墙提示；请允许「专用网络」入站，否则其他设备可能解析不到（本机解析不受影响）。
- 与系统自带 mDNS 共享端口，使用 `SO_REUSEADDR`；已验证 `Resolve-DnsName` 与 `ping`。

### macOS

- 菜单栏应用（无 Dock 图标），托盘图标使用模板图。
- 与 mDNSResponder 共享 5353；系统自带主机名可能已被占用，默认名字带 `-lan`。
- 未签名分发时需在「系统设置 → 隐私与安全性」放行。

### Linux

- 运行依赖：`libgtk-3-0`、`libwebkit2gtk-4.1-0`。
- 托盘依赖桌面环境的 StatusNotifier / AppIndicator 支持（GNOME 需扩展）。
- 自启写入 `~/.config/autostart/my-local-name.desktop`。
- 原生解析 `.local` 需要 Avahi / nss-mdns。

### 自定义后缀

mDNS 标准域是 `.local`。本工具会在协议层应答任意配置后缀，但 Windows / macOS 原生解析器默认只把 `.local` 交给 mDNS。自定义后缀需要客户端解析器支持（如 systemd-resolved 的域名路由）。界面会在后缀不是 `.local` 时给出提示。

## 配置文件与日志

- 配置：`os.UserConfigDir()/my-local-name/config.json`
  - Windows: `%AppData%\my-local-name\config.json`
  - macOS: `~/Library/Application Support/my-local-name/config.json`
  - Linux: `~/.config/my-local-name/config.json`
- 日志：同目录 `my-local-name.log`，超过 1 MB 轮转为 `.log.1`。
- 配置损坏时回退默认值并记录警告，不删除原文件。

## 验证清单（Windows）

1. 启动广播 `my-local-name-test.local`。
2. `Resolve-DnsName my-local-name-test.local -Type A` 返回本机 IPv4。
3. `ping my-local-name-test.local` 成功。
4. 停止后 `Clear-DnsClientCache; Resolve-DnsName ...` 不再返回结果。
5. 用另一台设备占用同名，启动时提示冲突并给出 `-2` 建议。
6. 关闭窗口后进程仍在托盘；托盘菜单可开始/停止/退出。
7. 勾选开机自启并注销/重启，确认登录后静默驻留（如启用自动广播则自动开始）。

## 许可

代码 MIT。界面设计基因来自巴洛克聚光、洛可可粉彩与赛博霓虹的极客结构语言。
