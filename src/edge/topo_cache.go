package edge

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	logger "github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/protocol"
)

type topoCacheFile struct {
	Nodes     []protocol.NodeInfo `json:"nodes"`
	UpdatedAt int64               `json:"updated_at"`
}

type TopoCache struct {
	mu            sync.RWMutex
	nodes         []protocol.NodeInfo
	cacheFilePath string // 非空时 Update 自动写盘
}

func NewTopoCache(cacheFilePath string) *TopoCache {
	return &TopoCache{cacheFilePath: cacheFilePath}
}

// Update 用中心节点下发的最新拓扑替换本地缓存，若配置了缓存路径则自动写盘
func (t *TopoCache) Update(nodes []protocol.NodeInfo) {
	t.mu.Lock()
	t.nodes = nodes
	t.mu.Unlock()

	if t.cacheFilePath != "" {
		if err := t.SaveToFile(); err != nil {
			logger.Warn("拓扑缓存写入文件失败 err:", err)
		}
	}
}

func (t *TopoCache) GetAll() []protocol.NodeInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := make([]protocol.NodeInfo, len(t.nodes))
	copy(cp, t.nodes)
	return cp
}

func (t *TopoCache) GetClientVisible() []protocol.ClientNodeInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]protocol.ClientNodeInfo, len(t.nodes))
	for i, n := range t.nodes {
		out[i] = protocol.ClientNodeInfo{
			IP:        n.IP,
			ProbePort: n.ProbePort,
		}
	}
	return out
}

// SaveToFile 将当前拓扑原子写入缓存文件（先写 .tmp 再 rename）
func (t *TopoCache) SaveToFile() error {
	t.mu.RLock()
	data := topoCacheFile{
		Nodes:     t.nodes,
		UpdatedAt: time.Now().Unix(),
	}
	t.mu.RUnlock()

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := t.cacheFilePath + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, t.cacheFilePath)
}

// LoadFromFile 从缓存文件加载拓扑到内存
func (t *TopoCache) LoadFromFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var data topoCacheFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if len(data.Nodes) == 0 {
		return os.ErrInvalid
	}

	t.mu.Lock()
	t.nodes = data.Nodes
	t.mu.Unlock()

	logger.Info("已从本地缓存加载拓扑 nodes:", len(data.Nodes), " updated_at:", data.UpdatedAt)
	return nil
}
