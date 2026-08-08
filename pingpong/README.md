# pingpong — 沙盒内常驻服务（Docker MVP）

职责：沙盒内部的 HTTP 服务，实现 pingpong 签名回显。

## 行为

- 监听 `:49999`
- 收到请求 → 返回 `"<原消息> -sandbox- <沙箱ID>"`
- 沙箱ID 通过环境变量 `SANDBOX_ID` 注入（由 backend 创建时传入）

## 镜像构建

```bash
cd pingpong
docker build -t pingpong:latest .
```

多阶段构建（golang:alpine 编译 → alpine 运行），产物只有 1 个小二进制。

## 单独运行验证（不用后端）

```bash
docker run -d --name test-sbx -e SANDBOX_ID=test123 -p 49999 pingpong:latest
curl 127.0.0.1:49999/ -d 'hello'   # hello -sandbox- test123
docker rm -f test-sbx
```

## 状态

✅ MVP 代码已就绪，待上机构建验证
