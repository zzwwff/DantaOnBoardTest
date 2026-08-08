# 沙盒方案说明：为什么 onboard MVP 用 Docker（致负责人）

日期：2026-08-06
作者：onboard 沙盒服务负责人

## 一、结论先行

测试机 `claw` 的硬件条件（LXC 容器、无 KVM、非 XFS）**物理上不支持 CubeSandbox 及其 PVM 模式**，任何配置都无法绕过。Docker 方案是当前环境下唯一能跑起来的正规方案，且**它本身就是 OpenClaw 生态的标准做法**（官方和社区均如此）。MVP 用 Docker 验证控制面接口契约，生产环境（枫林服务器/独立 VM）再按隔离阶梯升级沙盒后端，接口不变。

## 二、原方案（CubeSandbox）为什么不可行

CubeSandbox 是腾讯云开源的 AI Agent 沙箱集群，核心是 **KVM MicroVM**（每沙箱一个独立 guest 内核），因此**硬性前提是宿主机存在 `/dev/kvm`**。

测试机检查结果（可复现的命令）：

```bash
ls -l /dev/kvm          # → No such file or directory（不存在）
systemd-detect-virt     # → lxc（这台机器是 PVE 上的 LXC 容器，不是独立服务器）
df -h                   # 根文件系统 /dev/mapper/pve-vm--115--disk--0（PVE 容器磁盘）
df -T /mnt/data         # → ext4
```

- **LXC 容器内无法出现 `/dev/kvm`**：KVM 设备由宿主内核提供，容器看不到、也无法安装。
- **容器内无法更换内核**：内核属于宿主机，LXC 里改不了。
- 结论：CubeSandbox 需要的 KVM 在这台机器上物理不存在，**与配置、努力无关，是硬性不可能**。

## 三、PVM 模式为什么也不可行

PVM 是 CubeSandbox 为"无 /dev/kvm 的云服务器"提供的替代模式（软件页表虚拟化），但它的三个前提当前全部不满足：

| PVM 前提 | claw 实际情况 | 结论 |
|---|---|---|
| 安装腾讯自定义宿主内核（opencloudos9.cubesandbox.pvm.host）并重启 | 我们是 LXC 容器，无法动宿主内核 | ❌ |
| `/data/cubelet` 必须为 XFS（依赖 reflink 做 CoW 快照） | `/mnt/data` 是 ext4 | ❌ |
| 仅支持 x86_64、需宿主 root 权限 | 共享测试机，动内核风险不可接受 | ❌ |

## 四、Docker 为什么能完成目标

按项目的四个能力目标逐一对照：

| 目标 | Docker 实现方式 |
|---|---|
| 新建沙盒 | `docker run` 一个 OpenClaw 容器，每用户一个（容器=沙盒实例） |
| 删除沙盒 | `docker rm -f <容器>`（可带数据卷一起清） |
| 与沙盒沟通 | 旦挞 Channel Plugin 从容器内**拨出** WebSocket 连旦挞服务端；控制层也可连容器 gateway 端口（端口可配、只绑 127.0.0.1） |
| 长期驻留 | `--restart unless-stopped` + 命名卷；记忆文件（AGENT.md/SOUL.md/MEMORY.md）在 `/home/node/.openclaw`，挂卷即持久化 |

安全基线同样可满足：`--cap-drop=ALL`、`no-new-privileges`、`--init`、cgroup 内存/CPU/PID 限制、实例间独立网络——**这套加固参数是 OpenClaw 官方多租户文档的标配**，不是我们自创。

隔离升级路径（官方"隔离阶梯"概念）：加固 Docker 容器（当前）→ gVisor/Kata/MicroVM（需要更强隔离时，在独立 VM 上）→ 敌对租户独立机器。**当前处于官方建议的第一级**。

## 五、成功案例（业内都是这么做的）

1. **OpenClaw 官方多租户方案 `openclaw fleet`**：官方文档明确"每个租户 = 一个加固 Docker 容器实例（cell）"，官方 CLI 管理创建/删除/升级，`--cap-drop=ALL`、资源限制、端口只绑 127.0.0.1 都是官方默认。
   文档：https://docs.openclaw.ai/zh-CN/gateway/multi-tenant-hosting
2. **ClawHuddle（开源团队版 OpenClaw）**：dockerode 编排，每用户一个隔离 OpenClaw 容器，秒级开通。仓库：https://github.com/allen-hsu/clawhuddle
3. **JupyterHub DockerSpawner**：高校标配，每个学生一个容器（与校园场景最相似，跑了很多年）
4. **GitHub Codespaces / Gitpod**：每个开发者一个云端容器，按需拉起
5. **AWS 官方博客**：ECS Fargate 多租户 OpenClaw 参考架构，容器即租户边界。https://aws.amazon.com/cn/blogs/china/graviton-build-enterprise-multi-tenant-ai-agent-platform-openclaw-hermes-agent-practice/

共同点：**容器边界即租户边界**——"每用户一个 OpenClaw 容器 + API 管理"是业界共识，Docker 不是降级方案。

## 六、诚实的边界说明（预判"是不是不够专业"的疑问）

- Docker 容器共享宿主内核，防的是**用户之间、用户与宿主的隔离**；MicroVM 额外多防一层"不可信代码的内核逃逸"。
- 我们的实例是**用户自己的 OpenClaw + 内网模型 API**，威胁面远小于公网"执行任意模型生成代码"的场景，容器隔离完全够用。
- 将来若需要更强隔离（如开放不可信代码执行），按隔离阶梯升级：gVisor（无需 KVM 即可用）→ 独立 VM 上的 MicroVM。**控制面接口契约不变，只换沙盒后端实现**，MVP 投入不浪费。

## 七、下一步

- MVP：claw 上部署 Docker 方案，验证"创建/删除/沟通/驻留"四个动作，固定三接口契约
- 并行：向 PVE 管理员确认能否提供一台**开嵌套虚拟化的独立 VM**（CPU type=host），作为将来更强沙盒（CubeSandbox/MicroVM）的宿主
