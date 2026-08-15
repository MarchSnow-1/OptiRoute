package edge

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	"github.com/donnie4w/go-logger/logger"
)

type BusinessFirstPacket struct {
	Token     string `json:"token"`
	Timestamp int64  `json:"timestamp"`
	Version   string `json:"version,omitempty"` // Client Agent 版本（center 采集用）
}

// readFirstPacketWithPrefix 读取业务首帧：alreadyRead 为判定阶段已读的前缀
// （4 或 16 字节），从其中解析帧长并在连接上补齐剩余数据。
// deadline 为绝对时间点，复用调用方传入的同一超时窗口；零值表示不设超时。
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
		// 已读前缀超过帧长，说明前缀中含下一条帧的字节，raw conn 无法回推，只能关闭。
		// 注：当前 16 字节移交形态下该分支不可达——16 字节移交意味着首 4 字节为
		// "ORTE"(0x4F525445 ≈ 1.33GB)，必先被上面的帧长上限拒绝；保留为纯防御代码。
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

func (n *Node) handleBusiness(conn net.Conn, alreadyRead []byte, deadline time.Time, release func()) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()

	firstPacket, err := readFirstPacketWithPrefix(conn, alreadyRead, deadline)
	if err != nil {
		logger.Warn("[", remote, "] 读取首包失败 err:", err)
		return
	}

	var fp BusinessFirstPacket
	if err := json.Unmarshal(firstPacket, &fp); err != nil {
		logger.Warn("[", remote, "] 首包 JSON 解析失败")
		return
	}

	clientTCPAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		logger.Warn("[", remote, "] 无法解析客户端地址")
		return
	}

	// Token 绑定客户端 IP 与 nonce；IP 绑定可通过 token_bind_client_ip 关闭。
	verifyIP := ""
	if n.cfg.Self.TokenBindClientIP == nil || *n.cfg.Self.TokenBindClientIP {
		verifyIP = clientTCPAddr.IP.String()
	}
	if !n.auth.VerifyRouteToken(fp.Token, fp.Timestamp, n.ccClient().GetSelfUUID(), verifyIP, n.cfg.Self.TokenTTLS) {
		logger.Warn("[", remote, "] Token 验签失败，丢弃连接")
		return
	}

	// 验签通过后释放握手额度；后续长连接透传不再占用并发限制。
	if release != nil {
		release()
	}

	if err := util.WriteWithDeadline(conn, []byte{0x01}, 5*time.Second); err != nil {
		logger.Warn("[", remote, "] 写入验签确认失败 err:", err)
		return
	}

	originAddr := util.JoinHostPort(n.cfg.Remote.OriginAddr, n.cfg.Remote.OriginPort)
	originConn, err := net.DialTimeout("tcp", originAddr,
		time.Duration(n.cfg.Self.ConnectTimeoutMs)*time.Millisecond)
	if err != nil {
		logger.Warn("[", remote, "] 连接源站失败 err:", err)
		return
	}
	defer originConn.Close()

	edgeTCPAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		logger.Warn("[", remote, "] 无法解析本端地址")
		return
	}
	// 写入通信密钥（Server Agent 验证）
	if err := util.WriteWithDeadline(originConn, []byte(n.cfg.Remote.CommSecret), 5*time.Second); err != nil {
		logger.Warn("[", remote, "] 写入通信密钥失败 err:", err)
		return
	}
	ppv2Hdr := protocol.BuildPPv2Header(
		clientTCPAddr.IP, uint16(clientTCPAddr.Port),
		edgeTCPAddr.IP, uint16(edgeTCPAddr.Port),
	)
	if err := util.WriteWithDeadline(originConn, ppv2Hdr, 5*time.Second); err != nil {
		logger.Warn("[", remote, "] 写入 PPv2 包头失败 err:", err)
		return
	}

	// 读取 Server 确认帧（含 UUID + 版本）。Server Agent 在密钥校验后、读 PPv2 前回此帧；
	// 读不到/解析失败则断连（老版本 Server Agent 不兼容，强制同步升级）。
	var serverVersion, serverUUID string
	if ackData, err := util.ReadFrame(originConn, 3*time.Second); err != nil {
		logger.Warn("[", remote, "] 读取 Server 确认帧失败 err:", err, "（Server Agent 版本过旧？）")
		return
	} else {
		var ack protocol.ServerAck
		if err := json.Unmarshal(ackData, &ack); err != nil {
			logger.Warn("[", remote, "] Server 确认帧解析失败 err:", err)
			return
		}
		serverVersion = ack.Version
		serverUUID = ack.UUID
	}

	// 记录 Server Agent 信息（UUID + 版本 + IP，IP 取 edge 视角的源站连接远端地址）
	if cc := n.ccClient(); cc != nil {
		cc.SetServerReport(&protocol.ServerVersionReport{
			UUID:    serverUUID,
			IP:      originConn.RemoteAddr().String(),
			Version: serverVersion,
		})
	}

	// 记录客户端接入信息（中心开启采集时）
	if fp.Version != "" {
		if cc := n.ccClient(); cc != nil && cc.CollectClientInfo() {
			cc.AddClientInfo(protocol.ClientVersionReport{
				IP:        clientTCPAddr.IP.String(),
				Version:   fp.Version,
				Timestamp: time.Now().Unix(),
			})
		}
	}

	logger.Info("[", remote, "] L4 转发通道已建立 origin:", originAddr, " client_v:", fp.Version, " server_v:", serverVersion)

	var counter *atomic.Int64
	if n.bwTracker != nil {
		counter = n.bwTracker.BytesAccum()
	}
	idle := time.Duration(0)
	if n.cfg.Self.IdleTimeoutS != nil && *n.cfg.Self.IdleTimeoutS > 0 {
		idle = time.Duration(*n.cfg.Self.IdleTimeoutS) * time.Second
	}
	util.RelayWithIdle(conn, originConn, counter, idle)
}
