# TODO 任务清单

## 结论（2026-08-06）

测试机 `claw` 是 **PVE 上的 LXC 容器（CT 115）**：
- 无 `/dev/kvm`，且 PVM 需换宿主内核、容器内不可行
- `/mnt/data` 是 ext4（非 XFS）
- **CubeSandbox 无法在这台机器上运行（任何模式）**

**已选定方案 B：Docker 容器当"沙盒"**，验证控制面模式；接口契约与未来 CubeSandbox 保持一致。

## 当前任务（Docker MVP）

### Phase 0 — 环境检查（claw 上执行）

- [ ] `docker version && docker info --format '{{.ServerVersion}} / {{.OperatingSystem}}'`
- [ ] `docker ps`（确认 8080 无冲突）
- [ ] `ss -tlnp | grep -E '8080|49999'`（期望无输出）
- [ ] `go version`（有则服务器编译；无则用 Windows 交叉编译产物）
- [ ] `free -h`、`df -h /root`、`nproc`

### Phase 1 — 构建 pingpong 镜像 ✅ 代码已就绪（2026-08-08）

- [x] pingpong/main.go + Dockerfile 已写好（pingpong/）
- [ ] 上机执行：`cd pingpong && docker build -t pingpong:latest .`
- [ ] 单独验证：`docker run -d -e SANDBOX_ID=test123 -p 49999 pingpong:latest` + curl

### Phase 2 — 后端 ✅ 代码已就绪（2026-08-08）

- [x] backend/main.go + go.mod 已写好（纯标准库，LISTEN_ADDR 可配，默认 127.0.0.1:8080）
- [ ] 上机执行：`cd backend && go build -o backend . && ./backend`（或拷贝 Linux 交叉编译产物）

### Phase 3 — 端到端联调

- [ ] `POST /sandbox` → 返回 sandbox_id + addr
- [ ] `POST /sandbox/{id}/ping` → 返回"msg -sandbox- id"
- [ ] 沙盒过夜驻留后仍可 ping
- [ ] `DELETE /sandbox/{id}` → 容器被删除
- [ ] 记录真实资源占用（单个沙盒容器内存）

### Phase 4 — 后续演进（不动接口，只换实现）

- [ ] 找外层 PVE 管理员：能否开一台嵌套虚拟化 VM（CPU type=host + nested）→ 届时 `/dev/kvm` 可用
- [ ] 在那台 VM 上部署 CubeSandbox 集群（回头参考 README 里 CubeSandbox 架构说明）
- [ ] backend 内部实现从 Docker CLI 换成 E2B 兼容 API

## 已知坑（预判）

- **CubeSandbox 的 PVM 模式在此机器不可行**（LXC 内无法换宿主内核）
- 嵌套虚拟化需要外层 PVE 配合，这是未来部署 CubeSandbox 的前提
- 后端当前无鉴权、只绑 127.0.0.1，上线前需加 token（对应项目"openclaw 专用凭证"）
