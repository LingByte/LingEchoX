# WebRTC Hybrid SFU Optimization Plan

## 1. Goal and Scope

This document describes a practical optimization path for real-time communication based on WebRTC + SFU, without relying on client devices as temporary relay nodes.

The objective is to:

- avoid single-egress bottlenecks in centralized SFU architecture;
- maintain stability and controllability under high concurrency;
- reduce bandwidth and compute pressure with layered media strategies;
- support progressive evolution from small-scale PoC to large-room scenarios.

This plan focuses on a hybrid server-side architecture:

- horizontally scalable SFU clusters;
- region-aware access and routing;
- cascaded SFU for cross-region rooms;
- adaptive stream subscription (Simulcast/SVC + Last-N).

## 2. Core Architecture

### 2.1 Logical Components

- `Edge SFU`: media entry/exit close to users in each region.
- `Room Router`: deterministic room-to-SFU mapping and failover routing.
- `Control Plane`: signaling, session metadata, node health, scaling decisions.
- `Inter-SFU Backbone`: controlled forwarding links between SFU nodes/regions.
- `Telemetry & Autoscaler`: metrics collection, alerting, scale-out/scale-in actions.

### 2.2 Key Design Principles

1. **Multi-ingress and multi-egress**
   - A room maps to one active SFU shard at a time (or one per region in cascade mode), avoiding a global single-output hotspot.

2. **Media-layer adaptation first**
   - Prefer reducing unnecessary traffic (subscription and layer control) before adding more machines.

3. **Regional affinity**
   - Users connect to nearest edge SFU whenever possible; cross-region traffic remains on server backbone.

4. **Control/data plane decoupling**
   - Room state and scheduling are independent from media forwarding processes.

5. **Server-only reliability baseline**
   - All traffic paths have server-managed fallback; no dependency on client relay quality.

## 3. End-to-End Workflow

### 3.1 Session Join Flow

1. Client sends join request with room ID, region hint, and network profile.
2. Control Plane queries `Room Router`:
   - if room is new: assign target SFU shard by consistent hashing + region policy;
   - if room exists: return current room SFU endpoint.
3. Client establishes signaling + WebRTC transport to assigned Edge SFU.
4. SFU publishes participant media metadata to Control Plane for subscription coordination.

### 3.2 Media Publish/Subscribe Flow

1. Publisher sends Simulcast/SVC layers (for example: low/medium/high).
2. SFU evaluates each subscriber state:
   - bandwidth estimate;
   - viewport or active speaker context;
   - packet loss and RTT trend.
3. SFU forwards only required layers and applies Last-N policy in large rooms.
4. Subscriber layer target is adjusted continuously according to congestion signals.

### 3.3 Cross-Region Room Flow (Cascade)

1. Same room has participants in multiple regions.
2. Each region keeps local participants on local Edge SFU.
3. Regional SFUs exchange only selected media set over backbone links.
4. Each region fan-outs locally, reducing long-haul duplicate traffic.

### 3.4 Failure and Recovery Flow

1. Telemetry detects SFU overload or node failure risk:
   - CPU, memory, packet queue, retransmission spikes.
2. Control Plane marks node as draining and stops assigning new sessions.
3. Active rooms are migrated to healthy nodes with re-offer/reconnect strategy.
4. Clients recover through fallback endpoint list and short retry policy.

## 4. Optimization Strategies by Priority

### Phase A (highest ROI, immediate)

- Multi-SFU horizontal scaling with room sharding.
- Simulcast/SVC enabled by default for video publish.
- Last-N + Active Speaker policy for medium/large rooms.
- Region-aware nearest access.

### Phase B (scale growth)

- Cascaded SFU for cross-region room distribution.
- Independent autoscaling for signaling/control/media layers.
- Hot-room detection and proactive room migration.

### Phase C (advanced)

- Session handoff minimization (faster renegotiation paths).
- Separate optimization profiles for audio-first and video-first scenarios.
- Backbone route quality selection by real-time link metrics.

## 5. Suggested PoC Work Breakdown

### Week 1: Baseline and Metrics

- Deploy at least 2 SFU instances and one router service.
- Implement deterministic room-to-SFU mapping.
- Expose key metrics:
  - per-room participants;
  - outbound bitrate per SFU;
  - packet loss / RTT / NACK count;
  - SFU process CPU and memory.

### Week 2: Traffic Reduction Features

- Enable Simulcast (or SVC if codec/toolchain supports it).
- Implement subscriber adaptive layer selection.
- Add Last-N (for example 6~9 videos in large room mode).

### Week 3: Stability and Failover

- Implement node draining and room migration controls.
- Add overload protection thresholds and alerts.
- Verify reconnect and fallback behavior under chaos tests.

### Week 4: Cross-Region Cascade (optional in first PoC)

- Build inter-SFU forwarding between two regions.
- Validate end-to-end latency and long-haul traffic reduction.
- Compare centralized vs cascaded topology metrics.

## 6. Operational KPIs

Track these KPIs continuously to evaluate optimization quality:

- P95 end-to-end one-way media latency;
- packet loss and retransmission ratio;
- frozen-video ratio and average freeze duration;
- successful join rate and reconnect success rate;
- SFU node utilization distribution (hotspot ratio);
- bandwidth cost per participant-minute.

## 7. Risks and Mitigations

- **Risk: control plane becomes bottleneck**
  - Mitigation: stateless API + distributed room state cache + partitioned routing data.

- **Risk: aggressive adaptation causes quality oscillation**
  - Mitigation: hysteresis and minimum hold time for layer switching.

- **Risk: room migration impacts user experience**
  - Mitigation: drain-first policy, migration windows, and staged reconnect fallback.

- **Risk: cross-region links degrade under burst traffic**
  - Mitigation: rate caps, priority queues (audio first), and route fallback.

## 8. Implementation Checklist

- [ ] Multi-SFU deployment and room sharding in place.
- [ ] Simulcast/SVC publish path enabled.
- [ ] Adaptive subscription + Last-N policy validated.
- [ ] Monitoring dashboard and alert thresholds configured.
- [ ] Draining + migration + fallback flow tested.
- [ ] Optional cascaded SFU prototype benchmarked.

---

This architecture keeps reliability on the server side while still achieving meaningful cost and performance optimization for real-time communication workloads.

## 9. 实现思路与落地步骤（中文）

### 9.1 实现思路（要解决什么问题、怎么解）

1. **把「单 SFU 扛全量」改成「按房间分片的多 SFU」**  
   每个房间有稳定的「主 SFU 分片」，用户媒体只进该分片（或级联拓扑下的区域主分片），从根上消除「多入口、单出口」的结构性瓶颈。

2. **先减流量，再加机器**  
   用 Simulcast/SVC 让上行一次编码多层；SFU 按订阅者网络与业务策略只转发需要的层；大房间用 Last-N / 主讲人优先，把下行 fan-out 压到可控范围。

3. **就近接入 + 跨区再级联**  
   同区用户只连本区 Edge SFU；跨区房间再引入 SFU 间链路，把长途复制变成「区间一条、区内分发」，降低公网重复流量。

4. **控制面与媒体面解耦**  
   路由、房间元数据、扩缩容决策与 RTP 转发进程分离，避免信令或调度逻辑成为隐性单点。

5. **可观测 + 可迁移**  
   用指标识别热点房间与热点节点；支持 draining（只出不进）与房间迁移，保证故障与扩容时仍有服务器侧兜底路径。

### 9.2 实现步骤（按阶段、可执行任务）

以下为建议顺序；每步完成都应有可量化验收（见第 6 节 KPI）。

#### 阶段 0：基线与接口约定（约 3～5 天）

- [ ] 明确房间 ID、用户 ID、区域 ID 的数据模型与信令协议（join、leave、publish、subscribe）。
- [ ] 定义「房间 → SFU」路由 API：创建房间、查询房间当前 SFU、健康检查。
- [ ] 搭建最小双 SFU 实例（同区即可），验证两实例均可独立承载不同房间。

#### 阶段 1：房间分片路由（约 1～2 周）

- [ ] 实现确定性映射：例如 `hash(room_id) % N` 或一致性哈希，并支持**节点列表变更时的再平衡策略**（文档化即可，首版可简单取模）。
- [ ] Join 流程改造：客户端只拿到「当前房间绑定的 SFU 地址」，禁止写死单节点。
- [ ] 在控制面持久化或缓存「room → sfu_id」映射，保证同一房间并发加入时一致。
- [ ] 验收：压测下多房间均匀分布到多 SFU，单机 CPU/出口带宽曲线随房间分散而下降。

#### 阶段 2：分层媒体与订阅策略（约 1～2 周）

- [ ] 发布端开启 Simulcast（或 SVC，视编解码与 SFU 能力而定），约定各层码率/分辨率上限。
- [ ] SFU 侧实现或接入「按订阅选层」：依据 TWCC/丢包/RTT 与业务规则切换层。
- [ ] 大房间策略：Last-N（如 6～9 路视频）+ Active Speaker 提升优先级；其余仅音频或低帧占位。
- [ ] 验收：同等人数下 SFU 出口带宽与转发 CPU 显著下降；弱网用户可维持音频连续。

#### 阶段 3：就近接入与多区域部署（约 1～2 周）

- [ ] 客户端上报区域/延迟探测结果；DNS 或调度服务返回最近 Edge SFU。
- [ ] 多区域部署控制面与 SFU；路由表包含区域维度（同 room 在不同区可先「各自 SFU」再进入阶段 4 级联）。
- [ ] 验收：跨城用户 RTT 分布改善；同区流量占比提升。

#### 阶段 4：SFU 级联（跨区房间）（约 2～4 周，复杂度最高）

- [ ] 定义级联链路：区域 A SFU ↔ 区域 B SFU 只同步「被选中的远端流集合」（配合 Last-N）。
- [ ] 信令上区分「本地参与者」与「远端桥接轨道」，订阅关系在两侧 SFU 对齐。
- [ ] 验收：跨区房间长途带宽对比「单中心 SFU」方案下降；端到端延迟可接受（对比 KPI）。

#### 阶段 5：容量、故障与迁移（持续迭代）

- [ ] 暴露 SFU 与按房间聚合指标；配置告警阈值（CPU、队列、重传、每房间出口码率）。
- [ ] 实现节点 draining：新房间不调度至该节点；存量房间分批迁移。
- [ ] 客户端实现重连与备用 SFU 列表；混沌测试：随机杀 SFU、断网、切换 Wi‑Fi。
- [ ] 验收：故障场景下自动恢复时间与失败率达标。

### 9.3 建议的首个 PoC 范围（最小闭环）

只做 **阶段 0 + 阶段 1 + 阶段 2（Simulcast + Last-N）**，暂不级联跨区。用压测工具模拟「多房间、每房间多人订阅」，验证「分片 + 订阅裁剪」是否带来预期的带宽与 CPU 收益后，再投入阶段 4。
