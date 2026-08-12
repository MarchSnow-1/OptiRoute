package edge

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

type BusinessFirstPacket struct {
	Token     string `json:"token"`
	Timestamp int64  `json:"timestamp"`
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

func (n *Node) handleBusiness(conn net.Conn, alreadyRead []byte, deadline time.Time) {
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

	if !n.auth.VerifyToken(fp.Token, fp.Timestamp, n.ccClient().GetSelfUUID(), n.cfg.Self.TokenTTLS) {
		logger.Warn("[", remote, "] Token 验签失败，丢弃连接")
		return
	}

	conn.Write([]byte{0x01})

	originAddr := util.JoinHostPort(n.cfg.Remote.OriginAddr, n.cfg.Remote.OriginPort)
	originConn, err := net.DialTimeout("tcp", originAddr,
		time.Duration(n.cfg.Self.ConnectTimeoutMs)*time.Millisecond)
	if err != nil {
		logger.Warn("[", remote, "] 连接源站失败 err:", err)
		return
	}
	defer originConn.Close()

	clientTCPAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		logger.Warn("[", remote, "] 无法解析客户端地址")
		return
	}
	edgeTCPAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		logger.Warn("[", remote, "] 无法解析本端地址")
		return
	}
	// 写入通信密钥（Server Agent 验证）
	if _, err := originConn.Write([]byte(n.cfg.Remote.CommSecret)); err != nil {
		logger.Warn("[", remote, "] 写入通信密钥失败 err:", err)
		return
	}
	ppv2Hdr := protocol.BuildPPv2Header(
		clientTCPAddr.IP, uint16(clientTCPAddr.Port),
		edgeTCPAddr.IP, uint16(edgeTCPAddr.Port),
	)
	if _, err := originConn.Write(ppv2Hdr); err != nil {
		logger.Warn("[", remote, "] 写入 PPv2 包头失败 err:", err)
		return
	}

	logger.Info("[", remote, "] L4 转发通道已建立 origin:", originAddr)

	var counter *atomic.Int64
	if n.bwTracker != nil {
		counter = n.bwTracker.BytesAccum()
	}
	idle := time.Duration(n.cfg.Self.IdleTimeoutS) * time.Second
	util.RelayWithIdle(conn, originConn, counter, idle)
}
