# backend — 沙盒管理后端（Docker Provider MVP）

职责：暴露 3 个 HTTP 接口，内部管理"沙盒"（当前实现 = Docker 容器）。

## 接口定义

| 方法 | 路径 | 行为 |
|---|---|---|
| POST | `/sandbox` | 创建沙盒 → `{"sandbox_id": "...", "addr": "http://127.0.0.1:<port>"}` |
| DELETE | `/sandbox/{id}` | 删除指定沙盒 |
| POST | `/sandbox/{id}/ping` | body `{"message": "..."}` → `{"reply": "<message> -sandbox- <id>"}` |

## 架构（当前）

```
调用方 ──HTTP──► backend :8080 ──docker CLI──► 沙盒容器 (pingpong)
                     │                             │
                     └─ 端口映射表 (内存 map)      └─ 随机宿主端口 ← 127.0.0.1:<port>
```

- 创建：`docker run -d -p 49999`（随机宿主端口，自动避免冲突），沙盒ID 以 `SANDBOX_ID` env 注入
- 沟通：HTTP 转发到 `127.0.0.1:<宿主端口>/`
- 长期驻留：Docker 容器不删就一直活着，无空闲回收

## 依赖

- 机器上已装 Docker CLI（claw 上已有）
- 镜像 `pingpong:latest`（先构建，见 pingpong/README.md）
- 纯 Go 标准库，无第三方依赖

## 构建与运行

```bash
cd backend
go build -o backend .
./backend        # 监听 :8080
```

## 试一下

```bash
# 创建沙盒
curl -X POST localhost:8080/sandbox
# {"sandbox_id":"s12345678","addr":"http://127.0.0.1:32768"}

# 和指定沙盒沟通（pingpong）
curl -X POST localhost:8080/sandbox/s12345678/ping -d 'hello'
# {"reply":"hello -sandbox- s12345678"}

# 删除沙盒
curl -X DELETE localhost:8080/sandbox/s12345678
```

## 未来替换 CubeSandbox

接口契约不变，只需替换内部实现：
- `createSandbox` → E2B 兼容 API 创建沙盒（`E2B_API_URL` / `CUBE_TEMPLATE_ID` / `NEVER_TIMEOUT`）
- `deleteSandbox` → `sandbox.kill()`
- ping → CubeProxy `<port>-<id>.<domain>`

## 状态

✅ MVP 代码已就绪（Docker Provider），待上机联调
