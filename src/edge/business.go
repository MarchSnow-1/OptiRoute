// 本文件实现业务转发面：业务连接首帧携带 Token 与时间戳，Edge 在读取首个长度前缀帧、校验 Token 之后释放握手额度，再连接源站建立双向 L4 透传
// 与源站建连时先写通信密钥、再写 PPv2 包头，随后读取 Server Agent 的确认帧以采集其 UUID 与版本；确认帧读不到或解析失败即断开，从而要求 Server Agent 与新版本同步
// 首帧读取直接从连接判定阶段已读的前缀继续，避免把已经读出的字节丢弃后再重新读取
//
// This file implements the business relay plane: the first frame of a business connection carries a Token and a timestamp, and Edge reads the first length-prefixed frame, verifies the Token, releases the handshake slot and then dials the origin to set up a bidirectional L4 relay
// When dialling the origin it writes the communication secret first and the PPv2 header second, then reads the Server Agent ack frame to collect its UUID and version; a missing or unparsable ack frame drops the connection, which keeps Server Agent in step with this version
// The first-frame read continues directly from the prefix already read during classification, so those bytes are not discarded and read again
package edge

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	"io"
	"net"
	"sync/atomic"
	"time"
)

// BusinessFirstPacket 是业务连接的首帧载荷，由 Client Agent 构造
// 字段名与 JSON 标签按协议固定，Version 用于 Center 的客户端版本采集
//
// BusinessFirstPacket is the payload of the first frame on a business connection, built by the Client Agent
// The field names and JSON tags are fixed by the protocol, and Version feeds the client version collection of Center
type BusinessFirstPacket struct {
	Token     string `json:"token"`
	Timestamp int64  `json:"timestamp"`
	Version   string `json:"version,omitempty"` // Client Agent 版本（center 采集用） / Client Agent version, collected by Center
}

// readFirstPacketWithPrefix 读取业务首帧：alreadyRead 为判定阶段已读的前缀
// （4 或 16 字节），从其中解析帧长并在连接上补齐剩余数据
// deadline 为绝对时间点，复用调用方传入的同一超时窗口；零值表示不设超时
//
// readFirstPacketWithPrefix reads the business first frame: alreadyRead holds the prefix read during classification
// (4 or 16 bytes), the frame length is parsed from it and the remaining bytes are filled in from the connection
// deadline is an absolute instant reusing the caller's timeout window, and a zero value means no timeout
func readFirstPacketWithPrefix(conn net.Conn, alreadyRead []byte, deadline time.Time) ([]byte, error) {
	if len(alreadyRead) < 4 {
		return nil, fmt.Errorf("已读数据不足 4 字节，无法解析帧长度")
	}
	frameLen := binary.BigEndian.Uint32(alreadyRead[:4])
	if frameLen > util.MaxFrameSize {
		return nil, fmt.Errorf("帧长度超限: %d", frameLen)
	}
	got := uint32(len(alreadyRead) - 4)
	if got > frameLen {
		// 已读前缀超过帧长，说明前缀中含下一条帧的字节，raw conn 无法回推，只能关闭
		// 该分支只在已读前缀长于 4 字节且帧长小于 / 4 字节时成立，classifyConnection 移交的两种形态都进不到这里：
		// 移交 4 字节时 got 等于 0，无符号比较下 0 不会大于 frameLen；移交 16 字节意味着前 4 字节命中 "ORTE"（0x4F525445，约 1.33GB），先被上面的 MaxFrameSize 上限拒绝
		// 保留为纯防御代码
		//
		// A read prefix longer than the frame means the prefix holds bytes of the next frame, and a raw conn cannot push them back, so the only option is to close
		// The branch needs a prefix longer than 4 bytes with a frame length below / 4 bytes, and neither shape handed over by classifyConnection reaches it:
		// with a 4-byte handoff got is 0 and 0 cannot exceed frameLen in an unsigned comparison, while a 16-byte handoff means the first 4 bytes hit "ORTE" (0x4F525445, about 1.33GB) and the MaxFrameSize check above rejects it first
		// It is kept as pure defensive code
		return nil, fmt.Errorf("alreadyRead 包含的数据超过帧长度: got=%d, frameLen=%d", got, frameLen)
	}
	remaining := frameLen - got

	if !deadline.IsZero() {
		conn.SetReadDeadline(deadline)
		defer conn.SetReadDeadline(time.Time{})
	}

	buf := make([]byte, frameLen)
	copy(buf, alreadyRead[4:])
	if remaining > 0 {
		if _, err := io.ReadFull(conn, buf[got:]); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// handleBusiness 处理一条业务连接：读首帧、验签、连源站并建立双向透传
// 验签通过后立即释放握手额度，随后的长连接不再占用并发限制；函数返回时关闭两端连接
//
// handleBusiness serves one business connection: it reads the first frame, verifies the token, dials the origin and sets up the bidirectional relay
// The handshake slot is released right after the token check passes so the long-lived relay no longer counts against the concurrency cap, and both connections are closed when the function returns
func (n *Node) handleBusiness(conn net.Conn, alreadyRead []byte, deadline time.Time, release func()) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()

	firstPacket, err := readFirstPacketWithPrefix(conn, alreadyRead, deadline)
	if err != nil {
		logx.Warn("[", remote, "] 读取首包失败 err:", err)
		return
	}

	var fp BusinessFirstPacket
	if err := json.Unmarshal(firstPacket, &fp); err != nil {
		logx.Warn("[", remote, "] 首包 JSON 解析失败")
		return
	}

	clientTCPAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		logx.Warn("[", remote, "] 无法解析客户端地址")
		return
	}

	// Token 校验客户端 IP 与 nonce，其中 IP 比对由 token_bind_client_ip 控制
	// 该开关只在验证方生效：签发方在 edge.handleBootstrap 中把客户端 IP 交给 GenerateRouteToken 写入 claims，因此关闭绑定时是「跳过比对」而不是「claims 中没有 IP」
	// 开关为 nil 时按绑定处理；关闭绑定后同一 Token 可从其它来源 IP 使用，这是配置方明确接受的取舍
	//
	// The Token check covers the client IP and the nonce, with token_bind_client_ip steering the IP comparison
	// That switch acts on the verifying side only: edge.handleBootstrap hands the client IP to GenerateRouteToken, which writes it into the claims, so turning the binding off skips the comparison instead of leaving the claims without an IP
	// A nil switch means binding; with the binding off the same Token can be used from another source IP, which is the trade-off the operator opted into
	bindClientIP := n.cfg.Self.TokenBindClientIP == nil || *n.cfg.Self.TokenBindClientIP
	if !n.auth.VerifyRouteToken(fp.Token, fp.Timestamp, n.ccClient().GetSelfUUID(), clientTCPAddr.IP.String(), bindClientIP, n.cfg.Self.TokenTTLS) {
		logx.Warn("[", remote, "] Token 验签失败，丢弃连接")
		return
	}

	// 验签通过后释放握手额度；后续长连接透传不再占用握手闸，但要占用已建立连接的上限
	// The handshake slot is released after the token check passes, so the long-lived relay no longer counts against the handshake gate while it does count against the established-connection cap
	if release != nil {
		release()
	}

	// 已建立的转发连接数达到上限时拒绝新连接
	// 该检查放在验签之后，使未通过验签的连接仍然只占用握手闸，不会把额度浪费在垃圾连接上
	//
	// A connection is refused once the established forwarding connections reach the cap
	// The check sits after the token check, which keeps an unverified connection on the handshake gate alone and stops it from spending the allowance
	if !n.acquireForwardSlot() {
		logx.Warn("[", remote, "] 已建立转发连接数已达上限，拒绝连接")
		return
	}
	// 从获取额度到转发结束之间无论走哪条分支都要归还，避免上限被失败的连接逐渐占满
	// The slot is returned on every branch from acquiring it to the end of forwarding, so failed connections cannot fill the cap up over time
	defer n.releaseForwardSlot()

	if err := util.WriteWithDeadline(conn, []byte{0x01}, 5*time.Second); err != nil {
		logx.Warn("[", remote, "] 写入验签确认失败 err:", err)
		return
	}

	originAddr := util.JoinHostPort(n.cfg.Remote.OriginAddr, n.cfg.Remote.OriginPort)
	originConn, err := net.DialTimeout("tcp", originAddr,
		time.Duration(n.cfg.Self.ConnectTimeoutMs)*time.Millisecond)
	if err != nil {
		logx.Warn("[", remote, "] 连接源站失败 err:", err)
		return
	}
	defer originConn.Close()

	edgeTCPAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		logx.Warn("[", remote, "] 无法解析本端地址")
		return
	}
	// 写入通信密钥（Server Agent 验证）
	// Write the communication secret for the Server Agent to verify
	if err := util.WriteWithDeadline(originConn, []byte(n.cfg.Remote.CommSecret), 5*time.Second); err != nil {
		logx.Warn("[", remote, "] 写入通信密钥失败 err:", err)
		return
	}
	ppv2Hdr := protocol.BuildPPv2Header(
		clientTCPAddr.IP, uint16(clientTCPAddr.Port),
		edgeTCPAddr.IP, uint16(edgeTCPAddr.Port),
	)
	if err := util.WriteWithDeadline(originConn, ppv2Hdr, 5*time.Second); err != nil {
		logx.Warn("[", remote, "] 写入 PPv2 包头失败 err:", err)
		return
	}

	// 读取 Server 确认帧（含 UUID + 版本）。Server Agent 在密钥校验后、读 PPv2 前回此帧；
	// 读不到/解析失败则断连（老版本 Server Agent 不兼容，强制同步升级）
	//
	// Read the Server ack frame carrying the UUID and the version. The Server Agent replies after checking the secret and before reading PPv2,
	// and a missing or unparsable frame drops the connection because an older Server Agent is not compatible and has to be upgraded together
	var serverVersion, serverUUID string
	if ackData, err := util.ReadFrame(originConn, 3*time.Second); err != nil {
		logx.Warn("[", remote, "] 读取 Server 确认帧失败 err:", err, "（Server Agent 版本过旧？）")
		return
	} else {
		var ack protocol.ServerAck
		if err := json.Unmarshal(ackData, &ack); err != nil {
			logx.Warn("[", remote, "] Server 确认帧解析失败 err:", err)
			return
		}
		serverVersion = ack.Version
		serverUUID = ack.UUID
	}

	// 记录 Server Agent 信息（UUID + 版本 + IP，IP 取 edge 视角的源站连接远端地址）
	// Record the Server Agent info: UUID, version and IP, where the IP is the remote address of the origin connection as seen by Edge
	if cc := n.ccClient(); cc != nil {
		cc.SetServerReport(&protocol.ServerVersionReport{
			UUID:    serverUUID,
			IP:      originConn.RemoteAddr().String(),
			Version: serverVersion,
		})
	}

	// 记录客户端接入信息（中心开启采集时）
	// Record the client access entry when Center has collection enabled
	if fp.Version != "" {
		if cc := n.ccClient(); cc != nil && cc.CollectClientInfo() {
			cc.AddClientInfo(protocol.ClientVersionReport{
				IP:        clientTCPAddr.IP.String(),
				Version:   fp.Version,
				Timestamp: time.Now().Unix(),
			})
		}
	}

	logx.Info("[", remote, "] L4 转发通道已建立 origin:", originAddr, " client_v:", fp.Version, " server_v:", serverVersion)

	var counter *atomic.Int64
	if n.bwTracker != nil {
		counter = n.bwTracker.BytesAccum()
	}
	// idle_timeout_s 为 nil 或非正数时不设空闲超时，即透传不因空闲被中断
	// A nil or non-positive idle_timeout_s means no idle timeout, so the relay is not interrupted while idle
	idle := time.Duration(0)
	if n.cfg.Self.IdleTimeoutS != nil && *n.cfg.Self.IdleTimeoutS > 0 {
		idle = time.Duration(*n.cfg.Self.IdleTimeoutS) * time.Second
	}
	util.RelayWithIdle(conn, originConn, counter, idle)
}
