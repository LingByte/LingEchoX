package sfu

import (
	"github.com/LingByte/LingEchoX/pkg/models"
	"github.com/LingByte/LingEchoX/pkg/protocol"
	"go.uber.org/zap"
)

// RoleAssigner 处理智能角色分配
type RoleAssigner struct {
	config *Config
	logger *zap.Logger
}

// NewRoleAssigner 创建新的角色分配器
func NewRoleAssigner(config *Config, logger *zap.Logger) *RoleAssigner {
	return &RoleAssigner{
		config: config,
		logger: logger,
	}
}

// AssignRole 基于指标和系统状态为节点分配最优角色
func (ra *RoleAssigner) AssignRole(node *models.Node, allNodes []*models.Node, room *models.Room) (protocol.Role, []protocol.StreamInfo) {
	// 检查节点是否能够转发
	if !node.CanForward() {
		return protocol.RoleReceiver, nil
	}

	// 计算系统负载
	totalNodes := len(allNodes)
	if totalNodes < 2 {
		return protocol.RoleReceiver, nil
	}

	// 统计当前转发者数量
	forwarderCount := 0
	for _, n := range allNodes {
		if n.Role == protocol.RoleForwarder || n.Role == protocol.RoleHybrid {
			forwarderCount++
		}
	}

	// 确定最优转发者数量（目标为节点总数的 20-30%）
	optimalForwarders := max(1, totalNodes/5)
	if optimalForwarders > 10 {
		optimalForwarders = 10 // 最多 10 个转发者
	}

	// 如果需要更多转发者且该节点合适
	if forwarderCount < optimalForwarders {
		// 检查节点的适用性评分
		score := ra.calculateForwardingScore(node)

		// 与其他潜在转发者比较
		betterNodes := 0
		for _, n := range allNodes {
			if n.ID != node.ID && n.CanForward() {
				otherScore := ra.calculateForwardingScore(n)
				if otherScore > score {
					betterNodes++
				}
			}
		}

		// 基于评分和需求分配角色
		if betterNodes < optimalForwarders-forwarderCount {
			// 分配要转发的流
			streams := ra.assignStreamsToForward(node, allNodes, room)
			if len(streams) > 0 {
				return protocol.RoleHybrid, streams
			}
			return protocol.RoleForwarder, nil
		}
	}

	// 默认为接收者
	return protocol.RoleReceiver, nil
}

// calculateForwardingScore 计算转发适用性评分
func (ra *RoleAssigner) calculateForwardingScore(node *models.Node) float64 {
	score := 0.0

	// 带宽评分（越高越好，归一化到 0-1）
	bandwidthScore := min(1.0, node.Metrics.BandwidthMbps/ra.config.MinBandwidthForForwarding)
	score += bandwidthScore * 0.4

	// CPU 评分（越低越好，需要反转）
	cpuScore := 1.0 - min(1.0, node.Metrics.CPUUsagePercent/100.0)
	score += cpuScore * 0.3

	// 内存评分（越低越好，需要反转）
	memoryScore := 1.0 - min(1.0, node.Metrics.MemoryUsagePercent/100.0)
	score += memoryScore * 0.2

	// 网络质量评分（RTT 和丢包率越低越好）
	rttScore := 1.0 - min(1.0, float64(node.Metrics.RTTMs)/200.0)          // 归一化到 200ms
	packetLossScore := 1.0 - min(1.0, node.Metrics.PacketLossPercent/10.0) // 归一化到 10%
	networkScore := (rttScore + packetLossScore) / 2.0
	score += networkScore * 0.1

	return score
}

// assignStreamsToForward 为节点分配要转发的特定流
func (ra *RoleAssigner) assignStreamsToForward(node *models.Node, allNodes []*models.Node, room *models.Room) []protocol.StreamInfo {
	var streams []protocol.StreamInfo

	// 获取房间中的可用流
	allStreams := room.GetAllStreams()

	// 构建已被其他节点转发的流的映射
	forwardedStreams := make(map[string]int) // streamID -> 转发者数量
	for _, n := range allNodes {
		if n.ID == node.ID {
			continue
		}
		// 检查该节点是否是转发者并获取其转发流列表
		if n.Role == protocol.RoleForwarder || n.Role == protocol.RoleHybrid {
			forwardingList := n.GetForwardingStreams()
			for _, streamID := range forwardingList {
				forwardedStreams[streamID]++
			}
		}
	}

	// 构建需要转发的流列表，优先选择转发者较少的流
	type streamCandidate struct {
		stream         *models.Stream
		forwarderCount int
		priority       int
	}

	candidates := make([]streamCandidate, 0)
	for _, stream := range allStreams {
		// 不分配节点自己的流
		if stream.SourceNodeID == node.ID {
			continue
		}

		// 计算优先级（转发者数量越少，优先级越高）
		forwarderCount := forwardedStreams[stream.ID]
		priority := 100 - forwarderCount*10 // 简单的优先级计算

		candidates = append(candidates, streamCandidate{
			stream:         stream,
			forwarderCount: forwarderCount,
			priority:       priority,
		})
	}

	// 按优先级排序（优先级高的在前）
	// 对小列表使用简单的冒泡排序
	for i := 0; i < len(candidates)-1; i++ {
		for j := 0; j < len(candidates)-i-1; j++ {
			if candidates[j].priority < candidates[j+1].priority {
				candidates[j], candidates[j+1] = candidates[j+1], candidates[j]
			}
		}
	}

	// 根据节点能力限制流的数量
	maxStreams := int(node.Capabilities.MaxForwardingStreams)
	if maxStreams > len(candidates) {
		maxStreams = len(candidates)
	}

	// 选择优先级最高的流
	for i := 0; i < maxStreams; i++ {
		stream := candidates[i].stream
		streams = append(streams, protocol.StreamInfo{
			StreamID:     stream.ID,
			SourceNodeID: stream.SourceNodeID,
			StreamType:   stream.Type,
		})

		ra.logger.Debug("为转发者分配流",
			zap.String("node_id", node.ID),
			zap.String("stream_id", stream.ID),
			zap.Int("existing_forwarders", candidates[i].forwarderCount),
			zap.Int("priority", candidates[i].priority))
	}

	return streams
}
