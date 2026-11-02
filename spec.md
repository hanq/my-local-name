# MY LOCAL NAME · 局域网命名台 — 产品与技术规格

> 版本 v0.1 · 2026-11-02
> 一句话：给这台机器在局域网里起一个名字，像树莓派的 `raspberrypi.local` 一样，输入即广播，点停即消失。

---

## 1. 背景与目标

### 1.1 问题

树莓派、NAS、打印机接入局域网后，可以通过 `<hostname>.local` 直接访问，不依赖 IP、不依赖路由器后台、不需要 DNS 服务器。普通 Windows / macOS / Linux 电脑默认只能用自己的系统主机名，无法临时声明一个额外的、可自定义的局域网名字。

### 1.2 目标

做一个跨平台桌面小工具，用 mDNS（Multicast DNS, RFC 6762）把「自定义主机名 + 本机局域网 IPv4 地址」广播到局域网：

1. 输入名字，点击「开始广播」，局域网内其他设备即可用 `<name>.local` 解析到本机。
2. 点击「停止广播」，发送 goodbye 包，名字立即失效。
3. 默认后缀 `.local`，可自定义（如 `.lan`、`.home`、`.studio`）。
4. 常驻系统托盘；关闭窗口不退出。
5. 支持开机自启动（可选开机后自动开始广播）。
6. 优先 Windows，代码结构兼容 macOS / Linux。

### 1.3 非目标（v1 明确不做）

- 不做通用 DNS 服务器、不做递归解析、不代理其他设备的名字。
- 不做 IPv6 AAAA 广播（v1 仅 IPv4；协议层预留扩展）。
- 不做 `_http._tcp` / `_ssh._tcp` 等服务发现记录（PTR/SRV/TXT）。
- 不做端口转发、远程访问穿透、HTTPS。
- 不修改系统 hosts、不接管系统 DNS、不需要管理员权限。
- 不做 Windows 服务 / macOS LaunchDaemon（仅用户级登录自启）。

---

## 2. 用户与场景

| 用户 | 场景 | 期望 |
|---|---|---|
| 开发者 | 本机跑了一个 Web 服务，想让手机/同事用 `devbox.local:8080` 打开 | 30 秒内完成，名字固定不变 |
| 家庭实验室玩家 | 多台旧电脑/开发板，IP 由 DHCP 随机分配 | 名字好记、重启后仍生效 |
| 演示者 | 临时把笔记本映射成 `demo.local` | 一键开始、一键停止，停止后不残留 |
| 运维/测试 | 验证设备 mDNS 行为 | 能看到广播状态、IP、报文计数、冲突提示 |

### 2.1 关键用户故事

- **US-1** 我输入 `hanq-pi`，后缀保持默认 `.local`，点「开始广播」，界面显示 `hanq-pi.local` 与我的局域网 IP；另一台电脑 `ping hanq-pi.local` 成功。
- **US-2** 我把后缀改成 `.lan`，开始广播；使用支持该域名的 mDNS 客户端能解析（界面明确提示 OS 原生解析器通常只认 `.local`）。
- **US-3** 我点「停止广播」，1 秒内其他设备解析失败；界面回到待机态。
- **US-4** 局域网里已有一台设备叫 `hanq-pi.local`，我启动时收到明确的冲突提示，并被建议改用 `hanq-pi-2.local`。
- **US-5** 我关掉窗口，程序留在托盘；托盘菜单能查看状态、开始/停止、复制地址、退出。窗口内按 `F9` 可快速开始/停止。
- **US-6** 我勾选「开机自启」和「自启后自动广播」，下次登录后程序静默驻留托盘并已开始广播。
- **US-7** 我切换 Wi-Fi / 插网线，程序检测到地址变化并自动重新广播，不需要手动重启。

---

## 3. 功能需求

### FR-1 名称配置

- 一个主输入框填写**前缀**，紧跟一个可编辑的**后缀**字段（默认 `local`，界面显示为 `.local`）。
- 默认前缀：取系统主机名，转小写、非法字符替换为 `-`，追加 `-lan` 并截断到 63 字节（如 `jhq-lan`）；避免与系统自带的 mDNS 主机名抢占。取不到时用 `my-device-lan`。
- 默认后缀：`local`。
- 实时预览完整名字：`prefix.suffix`。
- 前缀规则：
  - 1–63 字节；仅 `a-z 0-9 -`（大小写均可输入，统一转小写展示/发送）；
  - 不能以 `-` 开头或结尾；不能包含 `.`、空格、下划线；
  - 允许多段后缀（`home.arpa`），每段 1–63 字节，总长 ≤ 253 字节。
- 后缀规则：可带或不带前导 `.`，统一规范为不带点的存储形式；不允许为空；不允许连续点；段字符集同前缀。
- 校验失败时输入框下方显示中文错误，按钮禁用。
- 名称保存到配置文件，下次启动恢复。

### FR-2 开始广播

点击「开始广播」后状态机：

```
idle → probing（最多 750ms）→ announcing（约 1s）→ broadcasting
```

- **probing**：向 `224.0.0.251:5353` 发送 3 次针对目标名字的 ANY 查询（间隔 250ms），监听冲突应答。
- **announcing**：发送 2 次无请求应答（A 记录，cache-flush，TTL 120s），间隔 1s。
- **broadcasting**：正常应答查询；每 60s 重新宣告一次；每 5s 检查接口地址变化，变化则立即重新宣告。
- 成功后界面显示：完整 FQDN、广播中的 IPv4 列表、接口数量、运行时长、应答/宣告计数。
- 广播期间输入框锁定（避免"显示的名字"与"广播的名字"不一致），停止后可编辑。

### FR-3 停止广播

- 向所有选定接口发送 A 记录 TTL=0 的 goodbye 报文（发送 2 次，间隔 50ms）。
- 关闭 socket、停止定时器，状态回 `idle`，清空运行时长与计数。
- 停止过程 ≤ 300ms，不阻塞 UI。

### FR-4 冲突检测

- probing 阶段若收到任何 `QR=1` 且包含目标名字记录的响应，判定冲突。
- broadcasting 阶段若收到**其他主机**对目标名字的 A 记录且地址不在本机地址集合内，判定冲突：记录日志、停止广播、进入 `conflict` 状态。
- 冲突时返回建议名：在原前缀后追加 `-2`、`-3`……（保持 ≤63 字节），UI 提供「采用建议名」按钮。
- 本机自己发出的报文（源 IP 属于本机、或地址集合内）不触发冲突。

### FR-5 网络接口

- 自动枚举可用接口：`Up && Multicast && !Loopback`，且拥有至少一个可用 IPv4 地址。
- 默认选择全部；用户可在设置抽屉中多选，选择持久化。
- 只广播所选接口上的 IPv4 地址（排除 169.254/16 链路本地、0.0.0.0、回环）。
- 若当前无可用接口：报错「没有可用的局域网接口」，不进入广播态。
- 广播中接口地址变化：更新 A 记录集合并重新宣告（不重建 socket）。
- 接口选择变更时：若正在广播，先 goodbye 旧地址，再以新地址重启广播。

### FR-6 系统托盘

- 托盘图标常驻；tooltip 显示 `MY LOCAL NAME · hanq-pi.local`（待机时为 `MY LOCAL NAME · 未广播`）。
- 菜单项：
  - 状态标题（禁用）：`● 广播中 · hanq-pi.local` / `○ 待机` / `! 名称冲突`
  - 显示主窗口
  - 开始广播 / 停止广播（根据状态切换，单一项）
  - 复制地址（复制 FQDN）
  - 开机自启（复选，实时反映 OS 注册状态）
  - 退出
- 左键单击托盘图标：显示并聚焦主窗口；再次点击可选隐藏（Windows 采用显示/聚焦语义）。
- 关闭窗口 = 隐藏到托盘，不退出；退出仅通过托盘菜单或 `Alt+F4` 长按？不，`Alt+F4` 同样只隐藏。程序退出仅托盘「退出」。
- 托盘菜单状态与前端界面状态双向同步。

### FR-7 开机自启动

- 使用 Wails v3 内置 `Autostart`（Windows: `HKCU\...\Run`；macOS: LaunchAgent/SMAppService；Linux: XDG autostart）。
- 注册参数包含 `--autostart`，标识符 `cn.hanq.mylocalname`。
- 开关立即生效，无需重启；界面显示当前是否已注册以及注册机制（可选展示路径）。
- 自启启动时：
  - 窗口默认隐藏，仅托盘驻留（可由 `startMinimized` 配置覆盖）；
  - 若 `autoBroadcast=true` 且保存的名字合法，则自动开始广播；
  - 自动广播失败（冲突/端口占用）时弹出窗口并显示错误。
- 单实例：重复启动只唤醒已有实例，不重复广播。

### FR-8 状态与日志

- 状态枚举：`idle` / `probing` / `announcing` / `broadcasting` / `conflict` / `error`。
- 状态变更事件推送到前端（`state:update`），托盘同步刷新。
- 日志：内存环形缓冲 200 条，级别 `info/warn/error`，包含时间、类别（socket/probe/announce/query/conflict/config/autostart）；UI 显示最近 8 条。
- 同时写入 `%AppData%/my-local-name/my-local-name.log`（macOS/Linux 对应配置目录），单文件超过 1MB 轮转为 `.log.1`。
- 计数：`queriesAnswered`、`announcementsSent`、`conflictsDetected`、`lastAnnounceAt`。

### FR-9 诊断与帮助

设置抽屉内展示：

- 当前 FQDN、监听端口 `5353/UDP`、mDNS 组播组 `224.0.0.251`；
- 本机地址与接口列表；
- 平台对应的验证命令：
  - Windows: `ping <fqdn>` / `Resolve-DnsName <fqdn> -Type A`
  - macOS: `dns-sd -G v4 <fqdn>` / `ping <fqdn>`
  - Linux: `avahi-resolve -n <fqdn>` / `resolvectl query <fqdn>`
- 自定义后缀提醒：OS 原生解析器通常只处理 `.local`；其他后缀需要客户端 mDNS 解析器显式支持；
- Windows 防火墙提示：首次监听 `UDP 5353` 若弹出防火墙询问，需允许「专用网络」入站。

### FR-10 异常处理

| 场景 | 行为 |
|---|---|
| UDP 5353 绑定失败 | 报错并提示可能有其他 mDNS 程序独占端口；不崩溃 |
| 无可用接口 | 报错，停留在 idle |
| 名称冲突 | conflict 状态 + 建议名 |
| 自启动注册失败 | 开关回滚，toast 报错（含系统返回信息） |
| 配置文件损坏 | 使用默认配置启动，日志警告，不覆盖原文件直至下次保存 |
| 广播中网络断开 | 日志警告，保持状态并等待地址恢复；恢复后重新宣告 |

### FR-11 可访问性与交互

- 全键盘可用：Tab 顺序、Enter 开始/停止、Esc 关闭设置抽屉。
- 输入框有显式 `<label>`；按钮有 `aria-pressed` / `aria-busy`；状态区 `aria-live="polite"`。
- 颜色对比：正文与背景 ≥ 4.5:1；不依赖颜色单独传达状态（同时有文字/图标）。
- 遵循 `prefers-reduced-motion`，关闭雷达扫描与脉冲动画。

---

## 4. mDNS 协议设计

### 4.1 报文

- 组播地址：`224.0.0.251`，端口 `5353/UDP`（IPv4）。
- 查询：`flags=0x0000`；探测查询 QTYPE=ANY(255)，QCLASS=IN，不设 QU 位（要求组播应答）。
- 应答：`flags=0x8400`（QR + AA）；A 记录 owner 为目标 FQDN，CLASS=IN + cache-flush 位（0x8001），TTL=120。
- 停止时：A 记录 TTL=0（goodbye），发送 2 次。
- 传统单播查询（源端口 ≠ 5353）：单播回复，ID 回显，TTL=10。
- 名字比较大小写不敏感；发送时保留归一化后的小写形式。
- 不实现 name compression 输出；解析支持 compression pointer（防环）。
- 不实现 known-answer suppression（v1 可接受少量冗余）。

### 4.2 状态机

```
                 ┌──────────── conflict ────────────┐
                 │                                  │
idle ──start──▶ probing ──ok──▶ announcing ──▶ broadcasting
  ▲                 │                                  │
  └────── stop ─────┴──────────── stop ───────────────┘
```

### 4.3 时序

| 动作 | 时机 |
|---|---|
| 探测查询 | 0 / 250 / 500 ms（组播应答），监听至 750ms |
| 首次宣告 | 探测通过后立即 + 1s 后各一次 |
| 周期宣告 | 每 60s（TTL 120s 的 50%） |
| 地址检查 | 每 5s |
| goodbye | 停止时 0 / 50 ms |

### 4.4 平台共存

- Windows 10/11 自带 mDNS 解析器（Dnscache）也监听 5353。必须使用 `SO_REUSEADDR`（Unix 同时 `SO_REUSEPORT`）绑定，不能独占端口。
- 已验证：绑定 `0.0.0.0:5353` + 加入组播组后，Windows 可立即解析我们宣告的 `.local` 名字。
- macOS/Linux 的 mDNSResponder / Avahi 同样共享端口；名字冲突由 probing 发现。
- 组播回环必须开启，否则本机解析器收不到自己的宣告。

### 4.5 自定义后缀的现实约束

mDNS 标准域是 `.local`。本工具在协议层会应答任意配置后缀的查询，但：

- Windows / macOS 原生解析器默认只把 `.local` 交给 mDNS；
- Linux 的 `nss-mdns` 默认也仅处理 `.local`；
- 自定义后缀要全局可解析，需要客户端配置（如 systemd-resolved 的域名路由、或应用自带 mDNS 库查询）。

因此：`.local` 为推荐默认；自定义后缀面向「客户端可控」的场景，UI 明确提示，不制造"任何后缀都能被系统解析"的误解。

---

## 5. 技术架构

### 5.1 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Wails v3.1.0（原生系统托盘、Autostart、单实例、事件、服务绑定） |
| 后端 | Go 1.26，标准库 + `golang.org/x/net/ipv4` + `golang.org/x/sys` |
| 前端 | Vue 3 + TypeScript + Vite |
| 样式 | 手写 CSS（设计令牌 + 大色块布局），不使用 UI 组件库 |
| 图标 | 内联线性 SVG（24×24，stroke=currentColor，stroke-width 2） |
| 字体 | 系统字体栈（离线可用）：衬线 Display + Mono + 中文宋体回退 |

> 不使用任何 CDN / 远程字体，保证内网离线可用。

### 5.2 目录

```
my-local-name/
├── main.go                     # Wails 启动、窗口、托盘、单实例、启动参数
├── app_service.go              # 绑定给前端的服务：状态/启动/停止/配置/诊断
├── app_state.go                # DTO：AppState、InterfaceOption、LogLine
├── internal/
│   ├── mdns/
│   │   ├── dns_message.go      # DNS 编解码（压缩指针解析、A 记录）
│   │   ├── hostname.go         # 名称规范化与校验
│   │   ├── mdns_engine.go      # 状态机、探测、宣告、应答、goodbye
│   │   ├── net_iface.go        # 接口枚举与 IPv4 地址筛选
│   │   ├── socket_windows.go   # SO_REUSEADDR
│   │   └── socket_unix.go      # SO_REUSEADDR + SO_REUSEPORT
│   ├── config/                 # 配置读写、默认值、迁移
│   └── applog/                 # 内存环形日志 + 文件轮转
├── assets/                     # 托盘图标（PNG / macOS 模板图）
├── build/                      # Wails 构建资产（图标、manifest、Info.plist）
├── frontend/
│   ├── src/
│   │   ├── App.vue             # 布局与状态编排
│   │   ├── style.css           # 设计令牌与全局样式
│   │   ├── types.ts            # 前端类型
│   │   ├── api.ts              # bindings 封装 + 事件订阅
│   │   └── components/         # name_console / signal_panel / settings_drawer / glyph
│   └── bindings/               # wails3 生成，勿手改
├── spec.md
└── AGENTS.md
```

### 5.3 边界

- `internal/mdns` **不依赖 Wails**，可独立单元测试、可被 CLI 复用。
- 只有 `app_service.go` / `main.go` 接触 Wails。
- 前端不直接拼造状态，以 `GetState()` + `state:update` 事件为唯一事实来源。
- 网络写操作串行化在 engine 内部（`sendMu`），避免并发写同一 socket。

### 5.4 数据流

```
UI 输入 ──StartBroadcast──▶ AppService ──▶ mdns.Engine.Start
                                  │                │
                                  │      状态/日志回调
                                  ▼                ▼
                           Event.Emit ◀──── AppService.syncTray
                                  │
                       前端 state:update / log:line
```

### 5.5 前端 API（Go 服务方法）

| 方法 | 返回 | 说明 |
|---|---|---|
| `GetState()` | `AppState` | 全量状态，挂载时调用一次 |
| `StartBroadcast(prefix, suffix)` | `AppState, error` | 校验 + 探测 + 广播 |
| `StopBroadcast()` | `AppState, error` | goodbye + 关闭 |
| `ApplyName(prefix, suffix)` | `AppState, error` | 仅校验并保存（待机时） |
| `UseSuggestedName()` | `AppState` | 冲突后采用建议名 |
| `ListInterfaces()` | `[]InterfaceOption` | 接口与地址 |
| `SetInterfaces(ids)` | `AppState, error` | 保存并在必要时重启广播 |
| `SetAutostart(bool)` | `bool, error` | OS 级注册 |
| `SetAutoBroadcast(bool)` | `AppState, error` | 配置项 |
| `SetStartMinimized(bool)` | `AppState, error` | 配置项 |
| `GetDiagnostics()` | `Diagnostics` | 命令、端口、防火墙提示 |
| `HideWindow()` | `void` | 隐藏到托盘 |
| `Quit()` | `void` | 退出应用 |
| `OpenLogFolder()` | `void` | 打开日志目录 |

事件：

- `state:update`（`AppState`）：任何状态/统计/配置变化；
- `log:line`（`LogLine`）：新日志。

### 5.6 配置

路径：`os.UserConfigDir()/my-local-name/config.json`

```json
{
  "prefix": "hanq-pi",
  "suffix": "local",
  "interfaces": [],
  "autoBroadcast": false,
  "startMinimized": false,
  "showWindowOnStart": true
}
```

- `interfaces: []` 表示自动（全部可用接口）。
- 未知字段保留（向前兼容）；解析失败回退默认值并记录日志。
- 写入采用临时文件 + rename，避免半文件。

### 5.7 启动参数

| 参数 | 含义 |
|---|---|
| `--autostart` | 由登录自启拉起：默认隐藏窗口、按配置自动广播 |
| `--show` | 强制显示窗口（覆盖隐藏逻辑） |

---

## 6. 视觉与交互规格

### 6.1 设计基因

- **主基因：巴洛克聚光（Tenebrism）** — 深色舞台、单一光源、暖黑底、金色细线、衬线大标题。
- **辅基因：洛可可粉彩** — 蓬巴杜粉、奶油白、玫瑰金、曲线分区，但压低饱和度避免甜腻。
- **点缀基因：赛博霓虹（仅结构语言）** — 等宽字体、网格纹理、状态读数、雷达扫描；不用荧光堆叠。
- **大色块**：奶油色信息块 / 墨色状态块 / 粉色行动块，三块硬边拼接；不用三张等宽圆角卡片。

### 6.2 色板

```css
--ink:        #140a11;  /* 暖黑紫，主背景 */
--ink-2:      #1d0d18;  /* 次级面 */
--plum:       #2b1122;  /* 深梅色 */
--cream:      #f7e9e4;  /* 奶油纸面 */
--cream-2:    #eed6cf;  /* 奶油暗面 */
--pink:       #ff5d8f;  /* 蓬巴杜粉，主行动色 */
--pink-deep:  #d61f5c;  /* 深玫瑰 */
--pink-soft:  #ffb3c7;  /* 高亮粉 */
--rose-gold:  #c08497;  /* 玫瑰金线 */
--gold:       #c9a227;  /* 巴洛克金线，仅描边 */
--mint:       #7ef0c0;  /* 仅表示「广播中」状态 */
--danger:     #ff5a52;  /* 冲突/错误 */
--text-dim:   #a98d9c;  /* 暗底上的次要文字 */
```

使用比例：墨色 ≥ 45%，奶油 ≥ 30%，粉色 ≤ 15%，金线 < 3%。

### 6.3 字体

```css
--font-display: "Playfair Display", "Cormorant Garamond", "Noto Serif SC",
                "Source Han Serif SC", "Songti SC", SimSun, Georgia, serif;
--font-ui:      "Inter", "Segoe UI", "Microsoft YaHei", system-ui, sans-serif;
--font-mono:    "JetBrains Mono", "Cascadia Mono", Consolas, "Courier New", monospace;
```

- 中文标题用宋体族（有衬线质感）；英文 Display 用 Playfair 的回退栈。
- 标签用 Mono 大写 + 0.22em 字距。
- 正文禁止纯黑（`#000`）；奶油底上用 `#3a1c2a`。

### 6.4 布局（默认 800×600，可缩放，最小 340×540）

小工具定位：单列、紧凑、打开即用。上方奶油操作区，下方墨色状态区；不做海报式大标题和大雷达。

```
┌────────────────────────────────────────────────────────────┐
│  ◉ MY LOCAL NAME       mDNS NAMING DESK      ● 状态    ⚙  –  ×  │  ← 44px 顶栏
├────────────────────────────────────────────────────────────┤
│  奶油操作区                                                 │
│  NAME YOUR MACHINE · MDNS                                   │
│  局域网名字                                                  │
│  同一网络里的设备，可用 jhq.local 找到这台机器。              │
│  ┌────────────────────────────────────┬─────────┐           │
│  │ hanq-pi                            │ .local  │           │
│  └────────────────────────────────────┴─────────┘           │
│  小写字母 · 数字 · 连字符 / 后缀默认 .local          9/253   │
│  [ ▶ 开始广播 ]   [ 复制地址 ]                               │
│  待机 · 尚未广播                                             │
│  mDNS · RFC 6762 · UDP 5353 · 224.0.0.251                   │
├────────────────────────────────────────────────────────────┤
│  墨色状态区                                                 │
│  SIGNAL                                              IDLE   │
│  ◎   jhq.local                                              │
│      192.168.4.126  WLAN                                    │
│  查询 0  │  宣告 0  │  冲突 0  │  运行 00:00                │
│  ┌ PACKET LOG ────────────────────────────────────────────┐ │
│  │ 16:54:45  INFO  探测通过，开始宣告 jhq.local            │ │
│  └────────────────────────────────────────────────────────┘ │
├────────────────────────────────────────────────────────────┤
│  UDP 5353   IFACE 2   JHQ.LOCAL        SIGNAL LOCKED  V0.1.0│  ← 22px 状态条
└────────────────────────────────────────────────────────────┘
```

- 单列两段式：奶油操作区约 55%，墨色状态区约 45%；≥1000px 宽时自动切换为左右两栏。
- 奶油块保留 1px 玫瑰金内框与左上 / 右下两处小角饰；墨色区保留粉色聚光与 5% 网格。
- 前缀输入 32px 衬线，后缀 14px Mono；主按钮高约 34px；状态区使用 42px 信号环代替大雷达。
- 顶部 44px 可拖拽栏；底部 22px 状态条（端口、接口数、FQDN、状态、版本）。
- 设置抽屉从右侧滑入，宽 352px；Esc / 点击遮罩关闭。
- ≤560px 收紧字号、间距和按钮；≤420px 隐藏副标题/页脚；≤570px 高隐藏统计格；最小 340×540 仍能完成开始/停止。
- 宽屏（≥1000px）自动切换左右两栏：左侧名字操作，右侧状态与日志，避免大屏留白。

### 6.5 状态视觉

| 状态 | 色彩/动效 | 文案 |
|---|---|---|
| idle | 奶油底、暗玫瑰文字、无光晕 | `待机 · 尚未广播` |
| probing | 金色脉冲环 | `正在探测名字冲突…` |
| announcing | 粉色快速脉冲 | `正在宣告…` |
| broadcasting | 薄荷色 LED + 缓慢信号环扫描 + 粉色光晕 | `广播中 · 自 14:02:01` |
| conflict | 红色描边 + 抖动一次 | `名称冲突 · 建议 hanq-pi-2.local` |
| error | 红色文字 + 详情 | `启动失败 · 端口被占用` |

### 6.6 组件与交互

- **名称铭牌**：前缀输入（衬线 32px）+ 后缀输入（Mono 14px，前置 `.` 固定显示）；聚焦时粉色底边，不改变布局。
- **主按钮**：粉色实心块、墨色粗体字、2px 墨色描边、右下 4px 硬阴影（非模糊）；按下位移 2px。广播中变为墨底粉字「停止广播」。
- **复制按钮**：线性 SVG + `复制地址` 文案；成功后 toast 显示「已复制」。
- **设置抽屉**：开机自启、自启后自动广播、启动时最小化、接口多选、验证命令、日志目录。
- **Toast**：底部居中，墨底奶油字，2.5s 自动消失；同一时刻最多一条。
- **窗口控制**（无边框窗口）：右上角线性图标「最小化 / 隐藏到托盘」，悬停显示提示；不使用 emoji。
- 所有图标：24×24 viewBox、stroke=currentColor、stroke-width=2、round cap/join。

### 6.7 动效

- 信号环扫描 5s/圈；LED 呼吸 2.4s；探测脉冲 0.9s。
- 状态切换淡入 180ms；抽屉 220ms cubic-bezier(0.22, 1, 0.36, 1)。
- `prefers-reduced-motion: reduce` 时全部停顿，仅保留状态色变化。

---

## 7. 跨平台矩阵

| 能力 | Windows 10/11 | macOS 13+ | Linux |
|---|---|---|---|
| mDNS 广播 | ✅ 已验证 | ✅（共享 5353） | ✅（共享 5353） |
| 系统托盘 | ✅ 原生 | ✅ 菜单栏（模板图标） | ✅ 需托盘环境（AppIndicator/StatusNotifier） |
| 关闭到托盘 | ✅ | ✅ | ✅ |
| 开机自启 | HKCU Run | LaunchAgent / SMAppService | XDG autostart |
| 单实例 | ✅ | ✅ | ✅ |
| 构建方式 | `wails3 build`（WebView2） | `wails3 build`（Xcode CLI） | `wails3 build`（GTK/WebKit 依赖） |
| 系统解析 `.local` | ✅ Dnscache | ✅ mDNSResponder | 需 Avahi/nss-mdns |
| 自定义后缀系统解析 | ❌（需客户端支持） | ❌ | 需 resolver 配置 |

### 7.1 Windows 重点

- 无边框窗口 + WebView2；`build/windows/wails.exe.manifest` 已含 DPI 感知。
- UDP 5353 绑定必须允许与系统 mDNS 共存，已用 `SO_REUSEADDR` 验证。
- 首次运行防火墙可能询问；界面提供说明，不自动提权、不改防火墙规则。
- 打包：`wails3 package` 产出 NSIS 安装包（可选）。

### 7.2 macOS

- 菜单栏应用（`ActivationPolicyAccessory`），无 Dock 图标。
- 托盘图标使用模板图（黑/透明），自动适配深浅色。
- 自启优先使用 Wails 内置策略。
- 未签名分发时需在「隐私与安全性」放行；文档说明。

### 7.3 Linux

- 运行依赖：`libgtk-3-0`、`libwebkit2gtk-4.1-0`。
- 托盘依赖桌面环境的 StatusNotifier 支持（GNOME 需扩展）。
- 自启：写入 `~/.config/autostart/my-local-name.desktop`。
- 自定义后缀可配 `systemd-resolved` 的 `Domains=~lan` 路由到 mDNS。

---

## 8. 安全与权限

- 仅监听 UDP 5353，仅应答**本程序声明的唯一名字**的 A/ANY 查询；不缓存、不转发、不代理。
- 默认只响应来自局域网的请求（组播范围）；不做开放递归解析。
- 不需要管理员/root 权限；不写系统 hosts；不改防火墙。
- 自启注册仅写当前用户范围（HKCU / LaunchAgent / 用户 autostart）。
- 日志不记录完整 MAC、不记录外部查询来源 IP（仅调试级别记录接口名）。
- 输入严格校验，防止把任意 DNS 记录注入广播。

---

## 9. 测试计划

### 9.1 单元测试（Go）

- DNS 编解码：名称 round-trip、压缩指针解析、畸形包不 panic、A 记录 cache-flush/TTL。
- 名称校验：合法/非法字符、长度边界、前缀点号、后缀多点、大小写归一。
- 构建报文：查询/应答/goodbye 的 flags、计数、TTL。
- 状态迁移：mock 时钟下的 probe 冲突判定（可选）。
- 配置：缺省、损坏 JSON、原子写入。

### 9.2 集成测试（本机，端口可注入）

- engine 使用可配置端口（默认 5353，测试用 15353）启动，测试客户端发查询并校验 A 应答。
- 停止后再次查询无应答（goodbye 不必须被测试端消费，但引擎必须不再响应）。
- 模拟冲突应答，断言 `ErrNameConflict` 与建议名。
- 多接口/无接口场景。

### 9.3 手工验收（Windows）

1. 启动应用，输入 `my-local-name-test`，开始广播。
2. `Resolve-DnsName my-local-name-test.local -Type A` 返回本机 IPv4。
3. `ping my-local-name-test.local` 成功。
4. 另一台手机/电脑访问 `http://my-local-name-test.local:<任意本地端口>` 成功。
5. 停止广播后 1–3 秒内解析失败（受缓存 TTL 影响，可用 `Clear-DnsClientCache` 后验证）。
6. 用另一台设备广播同名，本应用提示冲突并给出建议名。
7. 关闭窗口 → 托盘仍存在；托盘菜单开始/停止生效；托盘状态与 UI 同步。
8. 重启电脑（或注销重登）验证自启与自动广播。
9. 切换 Wi-Fi 后 IP 变化，UI 地址更新且解析到新 IP。
10. 托盘「退出」后进程消失、名字失效。

### 9.4 视觉验收

- 800×600、340×540 与 1200×600 三档布局不溢出、不重叠。
- 状态六态截图对比；reduced-motion 下无动画。
- 键盘全流程可操作；对比度检查通过。

---

## 10. 风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| Windows 系统 mDNS 占用 5353 | 无法绑定/收不到查询 | SO_REUSEADDR 共享绑定；已 POC 验证；失败时给出明确错误与建议 |
| 与其他 mDNS 软件互抢名字 | 解析不稳定 | RFC 6762 probing + 冲突提示；冲突即停，不恶意抢占 |
| 防火墙拦截入站组播 | 其他设备解析不到 | 诊断提示允许专用网络 UDP 5353；本机解析不受影响 |
| 自定义后缀不被系统解析 | 用户误判功能失效 | UI 明确提示 + 推荐 `.local` + 平台验证命令 |
| 休眠/网络切换后记录失效 | 名字短暂不可用 | 每 5s 检查地址、每 60s 重宣告；恢复后立即宣告 |
| Wails v3 版本较新 | API 变动 | 锁定 v3.1.0；引擎与 UI 解耦，壳层薄 |
| Linux 托盘环境差异 | 托盘不显示 | 文档说明依赖；窗口模式仍可用 |
| 多网卡广播到错误网段 | 解析到不可达 IP | 广播所选接口地址集合，客户端择优；支持手动选接口 |

---

## 11. 开发计划（逐项执行）

### Phase 0 — 项目与文档
- [x] 需求分析与可行性验证（Windows mDNS 解析 POC 通过）
- [x] 生成 `spec.md`
- [x] 生成 `AGENTS.md`
- [x] Wails v3 + Vue 脚手架落位

### Phase 1 — mDNS 核心（可独立测试）
- [x] DNS 报文编解码 + 单元测试
- [x] 名称规范化/校验 + 单元测试
- [x] 接口枚举与 IPv4 筛选
- [x] 跨平台 socket（REUSEADDR/REUSEPORT）+ 组播加入
- [x] 引擎：探测、宣告、应答、goodbye、周期宣告、地址监测
- [x] 冲突检测与建议名
- [x] 引擎集成测试（注入端口）

### Phase 2 — 配置、日志、自启
- [x] config 读写与原子保存
- [x] 环形日志 + 文件轮转
- [x] 开机自启（Wails Autostart + `--autostart`）
- [x] 单实例与二次启动唤醒

### Phase 3 — 应用壳与托盘
- [x] AppService：GetState/Start/Stop/配置/诊断
- [x] 事件推送 state:update / log:line
- [x] 无边框窗口、关闭隐藏、托盘菜单与状态同步
- [x] 应用图标、托盘图标、macOS 模板图

### Phase 4 — 前端界面
- [x] 设计令牌与全局样式（巴洛克粉 / 大色块 / 极客）
- [x] 名称铭牌 + 校验 + 开始/停止
- [x] 状态面板、雷达、计数器、日志
- [x] 设置抽屉、接口多选、诊断命令
- [x] 响应式、键盘、reduced-motion、toast

### Phase 5 — 构建与验收
- [x] `go test ./...`、`npm run build`、`wails3 build`
- [x] Windows 本机端到端验证（解析、ping、停止、冲突；托盘/自启为 Wails 原生能力）
- [x] 视觉截图检查与调整
- [x] README（使用说明、自启、防火墙、跨平台限制）
- [ ] 打包配置（NSIS / macOS bundle / Linux 依赖说明）

### Phase 6 — 后续（非 v1 阻塞）
- [ ] AAAA / IPv6（ff02::fb）
- [ ] `_http._tcp` 等服务发现记录
- [ ] Bonjour 浏览器式设备列表
- [ ] 多名字配置（多个 FQDN 同时广播）

---

## 12. 验收标准（Definition of Done）

- [x] 输入合法名字 → 广播成功，`<name>.local` 在 Windows 本机与至少一台同网设备上可解析。
- [x] 点击停止 → 不再应答，goodbye 已发送，UI/托盘回到待机。
- [x] 默认后缀 `.local`，可改为自定义后缀并被协议层正确应答。
- [x] 冲突可检测、可恢复（建议名一键采用）。
- [x] 托盘常驻，关闭窗口不退出，菜单可开始/停止/复制/自启/退出。
- [x] 开机自启可开关，自启后可静默广播。
- [x] Windows 构建产物可直接运行；macOS/Linux 代码路径完整且有构建说明。
- [x] 无数据竞争（`go test -race`）、无 panic、无阻塞 UI。
- [x] 视觉符合：巴洛克聚光 + 粉系 + 大色块 + 极客读数；无 emoji 图标；离线可用。
