package edge

import (
	"context"
	"net"

	"github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/util"
)

func (n *Node) runProbeServer(ctx context.Context) {
	addr := util.JoinHostPort("", n.cfg.Self.ProbePort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal("探测端口监听失败 err:", err)
	}
	logger.Info("探测端口监听中 addr:", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		conn.Close()
	}
}
