# Serial Gateway

一个面向本机调试的串口网关。程序只打开串口一次，并同时提供：

- 浏览器 xterm.js 终端；
- SecureCRT 可直接连接的 Telnet 终端；
- 面向二进制工具的字节透明 Raw TCP；
- 适合脚本和 AI 调试的游标式 HTTP API。

Web UI 与 xterm.js 已嵌入，编译后的 `serial-gateway.exe` 是单文件程序。

网页终端支持选中文字后右键直接复制（复制后取消选中）、未选中时右键直接粘贴。`Shift+右键` 保留浏览器菜单。直接操作需要浏览器允许剪贴板权限，并通过 localhost、127.0.0.1 或 HTTPS 访问；普通局域网 HTTP 地址会保留右键菜单。

运行后，API 文档直接访问 `http://127.0.0.1:8080/api/docs`。该页面同时链接适合 AI 读取的 Markdown 指南和 OpenAPI 3.1 JSON；把这一条链接交给新的本地 AI 上下文即可自行发现调用方式。

## 项目结构

网页终端右侧提供「常用命令」面板，可新建、编辑、删除带颜色的按钮。点击按钮会将内容发送到当前连接的串口，不自动追加回车；例如 `help\r` 会输入并执行 `help`。支持 `\r`（CR）、`\n`（LF）、`\p`（暂停一秒）、`\t`、`\e`、`\b` 和 `\\`（反斜杠），编辑框内实际换行按 CR 发送。延时期间可停止后续发送，切换串口或断线也会停止。按钮保存在当前浏览器的当前网站地址下，不会跨电脑同步。

```text
serial-gateway/
├── build/                    EXE 和运行时配置（构建生成）
├── cmd/serial-gateway/       入口、配置加载、进程生命周期
├── internal/config/          JSON 配置生成、加载和校验
├── internal/gateway/         串口、Telnet、Raw TCP、WebSocket、HTTP API
├── internal/webui/           嵌入式前端和本地 xterm.js
├── scripts/build.ps1         测试、静态检查、版本注入、构建
├── go.mod
└── README.md
```

`internal/gateway` 不依赖命令行入口，可以使用假串口独立测试。

## 运行

先构建，然后不带参数启动：

```powershell
.\scripts\build.ps1
.\build\serial-gateway.exe
```

首次启动会在 EXE 同目录创建：

```text
build/serial-gateway.json
```

程序随后直接使用默认配置运行。通常不需要手动编辑 JSON：打开 Web UI，点击右上角 **PORT SETTINGS** 即可选择启用的串口并修改串口、Web/API、Telnet 和 Raw TCP 参数。端口选择会立即生效；其他参数保存后按页面提示重启程序生效。

JSON 仍可用于脚本化部署或离线修改：

```json
{
  "serial": {
    "ports": [],
    "baud_rate": 115200,
    "data_bits": 8,
    "parity": "none",
    "stop_bits": "1"
  },
  "http": {
    "address": "127.0.0.1:8080"
  },
  "tcp": {
    "host": "127.0.0.1",
    "base_port": 7000,
    "escape_delay_ms": 5,
    "normalize_crlf": true
  },
  "telnet": {
    "enabled": true,
    "host": "127.0.0.1",
    "base_port": 8000
  },
  "scan_interval_ms": 2000
}
```

`serial.ports` 为空表示打开全部串口；只使用 COM13 时在设置面板取消“自动启用所有检测到的串口”，然后勾选 COM13。应用启动不需要且不提供运行参数。

默认仅监听本机。若主动改为 `0.0.0.0`，当前版本不会提供认证或 TLS。

## SecureCRT

1. 启动网关并查询 `GET /api/v1/ports`。
2. 在 SecureCRT 新建会话，Protocol 选择 **Telnet**。
3. Hostname 使用 `127.0.0.1`，Port 使用对应的 `telnet_address`。

Windows COM 端口采用稳定映射：`COMn → base-port + n`。默认情况下，COM3 的 Telnet 地址为 `127.0.0.1:8003`，Raw TCP 地址为 `127.0.0.1:7003`；COM13 则分别是 `8013` 和 `7013`。其他平台的设备名按发现顺序分配端口；进程运行期间拔插不会改变映射。

Telnet 入口会协商服务器回显、逐键交互、二进制传输、终端类型和窗口大小，适合 SecureCRT 的方向键、Tab 补全和命令行编辑。串口参数仍由 `serial-gateway.json` 决定。多个客户端可以同时观察一个串口，其发送数据按到达顺序串行写入。

Raw TCP 保留给需要字节透明传输的脚本和协议工具，地址见 `tcp_address`。

Telnet 和 Raw TCP 都不携带 Break、DTR、RTS 等串口带外控制信号。

默认启用交互终端兼容：连续方向键等 ESC 序列之间加入 5 ms 间隔，并把 CRLF 规范化为 CR。严格二进制透明场景把 `tcp.escape_delay_ms` 改为 `0`，并将 `tcp.normalize_crlf` 改为 `false`。

## AI 调试 API

查询服务、版本和串口默认值：

```powershell
curl.exe http://127.0.0.1:8080/api/v1/info
```

发现端口、状态、TCP 地址和当前游标：

```powershell
curl.exe http://127.0.0.1:8080/api/v1/ports
```

发送文本或十六进制数据：

```powershell
curl.exe -X POST "http://127.0.0.1:8080/api/v1/write?port=COM3" -H "Content-Type: application/json" -d '{"encoding":"text","data":"AT\r\n"}'
curl.exe -X POST "http://127.0.0.1:8080/api/v1/write?port=COM3" -H "Content-Type: application/json" -d '{"encoding":"hex","data":"41 54 0d 0a"}'
```

增量读取 RX/TX，最多等待 30 秒：

```powershell
curl.exe "http://127.0.0.1:8080/api/v1/read?port=COM3&after=0&encoding=hex&wait_ms=30000"
```

每条记录包含递增 `sequence`、UTC 时间、`rx`/`tx` 方向和数据。下一次请求使用响应中的 `next_after`；`has_more=true` 时立即继续读取，否则可以长轮询。每个端口的内存历史约为 1 MiB，`history_truncated` 表示请求游标已经早于当前保留范围。

`/api/v1/write` 也接受非 JSON 的原始请求体。`GET /healthz` 用于存活检查。

## 构建与验证

使用构建脚本，它会依次执行测试、静态检查、Git 构建标识注入和单文件构建：

```powershell
.\scripts\build.ps1
```

构建标识取当前 12 位 Git 提交号；工作区存在未提交修改时追加 `-dirty`。产物固定写入 `build/serial-gateway.exe`，启动日志和 `/api/v1/info` 会报告该标识。
