# Serial Gateway

一个面向本机调试的串口网关。程序只打开串口一次，并同时提供：

- 浏览器 xterm.js 终端；
- SecureCRT 可连接的字节透明 Raw TCP；
- 适合脚本和 AI 调试的游标式 HTTP API。

Web UI 与 xterm.js 已嵌入，编译后的 `serial-gateway.exe` 是单文件程序。

## 项目结构

```text
serial-gateway/
├── cmd/serial-gateway/       CLI、参数校验、进程生命周期
├── internal/gateway/         串口、Raw TCP、WebSocket、HTTP API
├── internal/webui/           嵌入式前端和本地 xterm.js
├── scripts/build.ps1         测试、静态检查、版本注入、构建
├── go.mod
└── README.md
```

`internal/gateway` 不依赖命令行入口，可以使用假串口独立测试。

## 运行

```powershell
.\serial-gateway.exe -baud 115200
```

从源码运行：

```powershell
go run ./cmd/serial-gateway -baud 115200
```

主要参数：

```text
-baud 115200                 波特率
-data-bits 8                 数据位：5、6、7、8
-parity none                 校验：none、odd、even、mark、space
-stop-bits 1                 停止位：1、1.5、2
-http 127.0.0.1:8080         Web UI 和 API 地址
-tcp-host 127.0.0.1          Raw TCP 监听地址
-tcp-base 7000               Raw TCP 基准端口
-scan-interval 2s            热插拔扫描周期
-ports COM13,COM14           只打开指定串口；默认打开全部
-version                     显示版本和构建信息
```

默认仅监听本机。若主动改为 `0.0.0.0`，当前版本不会提供认证或 TLS。

## SecureCRT

1. 启动网关并查询 `GET /api/v1/ports`。
2. 在 SecureCRT 新建会话，Protocol 选择 **Raw**，不能选择 Telnet。
3. Hostname 使用 `127.0.0.1`，Port 使用对应的 `tcp_address`。

Windows COM 端口采用稳定映射：`COMn → tcp-base + n`。默认情况下，`COM3` 对应 `127.0.0.1:7003`。其他平台的设备名按发现顺序分配端口；进程运行期间拔插不会改变映射。

TCP 双向传输任意原始字节。串口参数由网关命令行决定，SecureCRT Raw 会话中的串口选项不生效。多个客户端可以同时观察一个串口，其发送数据按到达顺序串行写入。

Raw TCP 不携带 Break、DTR、RTS 等串口带外控制信号。

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

推荐使用构建脚本，它会依次执行测试、静态检查、版本注入和单文件构建：

```powershell
.\scripts\build.ps1 -Version 0.2.0
```

手动执行：

```powershell
go test ./...
go vet ./...
go build -o serial-gateway.exe ./cmd/serial-gateway
```
