# TODO 任务清单

## 结论（2026-08-06）

测试机 `claw` 是 **PVE 上的 LXC 容器（CT 115）**：
- 无 `/dev/kvm`，且 PVM 需换宿主内核、容器内不可行
- `/mnt/data` 是 ext4（非 XFS）
- **CubeSandbox 无法在这台机器上运行（任何模式）**

**已选定方案 B：Docker 容器当"沙盒"**，验证控制面模式；接口契约与未来 CubeSandbox 保持一致。方案论证见 `DockerDecision.md`。

## 里程碑 1 — pingpong 控制面验证 ✅ 已完成（2026-08-09）

- [x] 环境检查（Docker 28.2.2、Go 1.22.2、端口空闲、32Gi/16核）
- [x] pingpong 镜像构建（国内镜像源：docker.xuanyuan.me / docker.1ms.run / docker.m.daocloud.io；buildx 用 `docker buildx use default` 走宿主 daemon 的镜像源）
- [x] 三接口联调：create → ping（消息+沙盒ID 签名回显）→ delete
- [x] 单实例内存实测：**~1.8 MiB**（静态 Go + alpine，5 容器合计 <10 MiB，已写入论证）
- [x] 沙盒过夜驻留验证
- [x] 代码保留在 `pingpong/`（历史参考）

## 里程碑 2 — OpenClaw 聊天沙盒（当前）✅ 代码已就绪（2026-08-09）

每个沙盒 = 一个加固的 OpenClaw 网关容器（DeepSeek 模型），浏览器网页直接对话，不分 session。

- [x] backend 重写：`/api/chat`、`/api/sandbox`、create/delete、启动时恢复沙盒端口映射
- [x] 多沙盒按 id 路由：`POST /sandbox/{id}/chat`（每个沙盒独立容器独立会话，sandboxctl 不再混 session）
- [x] `openclaw.json` 自动生成：开启 chatCompletions、token 鉴权、DeepSeek provider、默认模型 deepseek/deepseek-chat
- [x] 沙盒加固参数：`--memory 512m --cpus 1 --pids-limit 256 --cap-drop ALL --security-opt no-new-privileges`
- [x] web 聊天页（单文件、无外部依赖）
- [x] deploy.sh：镜像拉取（ghcr.io + 国内镜像回退）+ API key 管理 + 构建 + 健康检查
- [ ] **上机执行**：`./deploy.sh`（首次跑通全部流程）
- [ ] 验证：第一条消息自动建沙盒、DeepSeek 回复、对话记忆跨消息保持
- [ ] 验证：sandboxctl 两个名字各聊各的（各建一盒，改名互不影响）
- [ ] 验证：后端重启后沙盒恢复、仍可对话
- [ ] 验证：`docker stats` 内存上限 512m、过夜驻留
- [ ] 验证：删除沙盒清掉容器和数据目录

## Phase 4 — 后续演进

- [ ] 出网收窄：沙盒 egress 白名单（只放 DeepSeek API），iptables 或代理
- [ ] 只读根文件系统：`--read-only`（需先审计 OpenClaw 可写路径）
- [ ] 后端鉴权：Bearer token（当前只绑 127.0.0.1，暴露公网前必办）
- [ ] gVisor（runsc）运行时：无 KVM 下的内核级隔离（claw 可直接装）
- [ ] 找外层 PVE 管理员：嵌套虚拟化 VM（CPU type=host + nested）→ 届时可上 Kata/microVM 或原 CubeSandbox 方案
- [ ] backend 内部实现从 Docker CLI 换成 E2B 兼容 API

## 已知坑（预判）

- **ghcr.io 拉取**：中国网络可能超时，deploy.sh 已内置镜像回退（ghcr.nju.edu.cn → ghcr.m.daocloud.io）
- **容器内 user**：OpenClaw 镜像以 node（uid 1000）运行，数据目录需 chown 1000:1000，backend 已处理；若后续镜像改 uid 需同步改
- **首次回复慢**：沙盒冷启动 + DeepSeek 推理，第一条消息 1-2 分钟属正常（前端有"思考中"提示）
- **DeepSeek 模型引用**：用 `deepseek/deepseek-chat`（provider/model 格式）；若报 Unknown model，容器内跑 `openclaw models list` 排查
- **openclaw.json 校验严格**（实测）：模型条目必须带 `name` 字段；`agents.defaults.models.allow` 会被拒（已从生成配置移除）；`gateway.mode` 必填（设为 `local`）
- **多沙盒混会话**（实测已修）：早期 `/api/chat` 只对"当前沙盒"说话，sandboxctl 的 name→id 别名形同虚设、全部打进同一会话；已改为 `POST /sandbox/{id}/chat` 按真实 id 路由，`deploy.sh` 每次清空 `build/sandboxes.map`（旧别名指向已删容器）
- 后端当前无鉴权、只绑 127.0.0.1，暴露公网前需加 token（对应项目"openclaw 专用凭证"）
