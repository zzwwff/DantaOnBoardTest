# OnBoard Test — CubeSandbox MVP 验证

在测试服务器上部署 CubeSandbox 沙盒集群，实现最小可用的"一人一沙盒"雏形。

## 目标（四个能力）

1. **新建沙盒** — 后端创建沙盒实例，返回 `sandbox_id`
2. **删除沙盒** — 后端销毁指定沙盒
3. **与指定沙盒沟通** — pingpong：发一条消息，沙盒返回"消息 + 沙盒ID 签名"
4. **长期驻留** — 沙盒不因空闲被回收（timeout = `NEVER_TIMEOUT`）

## 架构

```
后端 (backend/)
  ├─ REST → CubeAPI :3000     创建/删除沙盒
  └─ HTTP → CubeProxy :443    沟通（按 Host 头路由到沙箱内端口）
              │
        <port>-<sandbox_id>.<domain>
              │
       沙箱 (MicroVM，独立内核)
         ├─ envd :49983    （平台自带，供平台管理沙箱）
         └─ pingpong :49999（由 pingpong/ 目录构建的镜像提供）
```

## 目录结构

```
OnBoardTest/
├── README.md       ← 本文档（整体说明）
├── TODO.md         ← 任务清单
├── backend/        ← Go 后端服务（3 个接口）
└── pingpong/       ← 沙箱内 pingpong 服务 + Dockerfile
```

## 通信链路（核心是 HTTP）

| 动作 | 路径 | 说明 |
|---|---|---|
| 创建/删除沙盒 | 后端 → CubeAPI (`:3000`) | E2B 兼容 REST API |
| pingpong 沟通 | 后端 → CubeProxy (`:443`) | mkcert TLS，按 Host 头 `<port>-<id>.<domain>` 路由进沙箱内 `:49999` |

## 路线图概览

1. 检查测试机硬件（KVM / 内存 / 磁盘）→ 决定安装模式
2. 一键安装 CubeSandbox 集群
3. 写 pingpong 服务 → 构建镜像 → 本地 registry → 创建模板
4. 写 Go 后端三个接口
5. 配置联调：CA 证书、域名解析、NEVER_TIMEOUT
6. 端到端验证 + 长期驻留验证

详细步骤见 [TODO.md](TODO.md)。

## 验收标准

- [ ] 能创建一个沙盒并返回 `sandbox_id`
- [ ] 能对指定沙盒 ping，返回"原消息 + `-sandbox-` + 沙盒ID"
- [ ] 沙盒存活过夜后仍可 ping（长期驻留成立）
- [ ] 能删除沙盒
- [ ] 沙盒删除后重建，可写层数据保留

## 时间估计

- 测试机有 KVM：2~3 天
- 无 KVM 走 PVM 模式：+1 天
