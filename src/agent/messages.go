// 本文件登记 agent 包用到的消息键与其三种语言文案
// 键名按「包名.用途」命名，本包的两条角色各占一半：客户端侧键描述本地接入与透传，源站侧键描述接入校验与上游转发
// 文案保留 [<来源地址>] 这类标签，使同一条日志仍能指出它属于哪一条连接
// 在 init 中一次性登记，登记完成后目录只被读取
//
// This file registers the message keys agent uses together with their text in three languages
// A key is named package.purpose and the two roles of this package split them: the client-side keys describe local access and passthrough, while the origin-side keys describe access validation and upstream forwarding
// The text keeps tags such as [<source address>], so one log line still names the connection it belongs to
// Registration happens once in init and the catalogue is only read afterwards
package agent

import "github.com/MarchSnow-1/OptiRoute/i18n"

const (
	// KeyClientListening 是 Client Agent 的本地监听端口就绪时的键，参数为监听地址
	// KeyClientListening is the key used once the local listener of the Client Agent is ready, whose argument is the listen address
	KeyClientListening i18n.Key = "agent.client_listening"

	// KeyAcceptFailed 是 Accept 返回错误时的键，参数为 Accept 错误
	// 该键被 Client Agent 与 Server Agent 两侧的 Accept 循环共用，两处的处理方式一致
	//
	// KeyAcceptFailed is the key used when Accept returns an error, whose argument is that error
	// The key is shared by the Accept loops of the Client Agent and the Server Agent, which handle the error the same way
	KeyAcceptFailed i18n.Key = "agent.accept_failed"

	// KeyLocalAccessRateLimited 是单个来源 IP 的本地接入超过限速时的键，参数为该来源主机
	// KeyLocalAccessRateLimited is the key used when the local access of one source IP exceeds the rate limit, whose argument is that source host
	KeyLocalAccessRateLimited i18n.Key = "agent.local_access_rate_limited"

	// KeyLocalAccessConcurrencyFull 是本地接入额度用尽时的键，不带参数
	// KeyLocalAccessConcurrencyFull is the key used when the local access slots run out, and it takes no argument
	KeyLocalAccessConcurrencyFull i18n.Key = "agent.local_access_concurrency_full"

	// KeyAccessFlowFailed 是本地连接的接入流程失败时的键，参数依次为来源地址与失败原因
	// KeyAccessFlowFailed is the key used when the access flow of a local connection fails, with the source address and the failure reason as arguments
	KeyAccessFlowFailed i18n.Key = "agent.access_flow_failed"

	// KeyRelayEstablished 是本地连接与目标 Edge 之间的透传通道建立时的键，参数为来源地址
	// KeyRelayEstablished is the key used once the passthrough channel between a local connection and the target Edge is established, whose argument is the source address
	KeyRelayEstablished i18n.Key = "agent.relay_established"

	// KeyProbeItemListReceived 是收到引导节点下发的探测项列表时的键，参数依次为来源地址与探测项条数
	// KeyProbeItemListReceived is the key used when the probe item list delivered by the bootstrap node arrives, with the source address and the item count as arguments
	KeyProbeItemListReceived i18n.Key = "agent.probe_item_list_received"

	// KeyRedirectReceived 是收到 Redirect 命令时的键，参数依次为来源地址、目标 IP 与业务端口
	// KeyRedirectReceived is the key used when the Redirect command arrives, with the source address, the target IP and the service port as arguments
	KeyRedirectReceived i18n.Key = "agent.redirect_received"

	// KeyServerListening 是 Server Agent 的监听端口就绪时的键，参数为监听地址
	// KeyServerListening is the key used once the listener of the Server Agent is ready, whose argument is the listen address
	KeyServerListening i18n.Key = "agent.server_listening"

	// KeyServerAuthConcurrencyFull 是源站侧认证前握手额度用尽时的键，不带参数
	// KeyServerAuthConcurrencyFull is the key used when the pre-authentication handshake slots on the origin side run out, and it takes no argument
	KeyServerAuthConcurrencyFull i18n.Key = "agent.server_auth_concurrency_full"

	// KeyServerAuthRateLimited 是源站侧认证前连接超过每 IP 限速时的键，参数为来源地址
	// KeyServerAuthRateLimited is the key used when a pre-authentication connection on the origin side exceeds the per-IP rate limit, whose argument is the source address
	KeyServerAuthRateLimited i18n.Key = "agent.server_auth_rate_limited"

	// KeyCommSecretReadFailed 是读取通信密钥失败且错误不是 io.EOF 时的键，参数依次为来源地址与读取错误
	// KeyCommSecretReadFailed is the key used when reading the comm secret fails with an error other than io.EOF, with the source address and the read error as arguments
	KeyCommSecretReadFailed i18n.Key = "agent.comm_secret_read_failed"

	// KeyCommSecretAuthFailed 是通信密钥与配置不一致时的键，参数为来源地址
	// KeyCommSecretAuthFailed is the key used when the comm secret differs from the configured one, whose argument is the source address
	KeyCommSecretAuthFailed i18n.Key = "agent.comm_secret_auth_failed"

	// KeyAckWriteFailed 是回写 Server 确认帧失败时的键，参数依次为来源地址与写入错误
	// KeyAckWriteFailed is the key used when writing the Server ack frame back fails, with the source address and the write error as arguments
	KeyAckWriteFailed i18n.Key = "agent.ack_write_failed"

	// KeyPPv2HeaderReadFailed 是读取 PPv2 固定包头失败且错误不是 io.EOF 时的键，参数依次为来源地址与读取错误
	// KeyPPv2HeaderReadFailed is the key used when reading the fixed PPv2 header fails with an error other than io.EOF, with the source address and the read error as arguments
	KeyPPv2HeaderReadFailed i18n.Key = "agent.ppv2_header_read_failed"

	// KeyPPv2AddrReadFailed 是按附加地址长度补读 PPv2 地址数据失败时的键，参数依次为来源地址与读取错误
	// KeyPPv2AddrReadFailed is the key used when reading the PPv2 address data according to the extra address length fails, with the source address and the read error as arguments
	KeyPPv2AddrReadFailed i18n.Key = "agent.ppv2_addr_read_failed"

	// KeyPPv2HeaderInvalid 是 PPv2 包头未通过解析校验时的键，参数依次为来源地址与解析错误
	// KeyPPv2HeaderInvalid is the key used when the PPv2 header fails its parse check, with the source address and the parse error as arguments
	KeyPPv2HeaderInvalid i18n.Key = "agent.ppv2_header_invalid"

	// KeyClientRealAddrResolved 是 PPv2 解析出客户端真实地址时的键，参数依次为来源地址、客户端 IP 与客户端端口
	// 该键只在 Self.LogRealIP 为真时输出
	//
	// KeyClientRealAddrResolved is the key used once the PPv2 header yields the client real address, with the source address, the client IP and the client port as arguments
	// The key is logged only while Self.LogRealIP is true
	KeyClientRealAddrResolved i18n.Key = "agent.client_real_addr_resolved"

	// KeyUpstreamConnectFailed 是连接上游失败时的键，参数依次为来源地址与拨号错误
	// KeyUpstreamConnectFailed is the key used when dialling the upstream fails, with the source address and the dial error as arguments
	KeyUpstreamConnectFailed i18n.Key = "agent.upstream_connect_failed"

	// KeyPPv2UpstreamWriteFailed 是向上游写入 PPv2 包头失败时的键，参数依次为来源地址与写入错误
	// KeyPPv2UpstreamWriteFailed is the key used when writing the PPv2 header to the upstream fails, with the source address and the write error as arguments
	KeyPPv2UpstreamWriteFailed i18n.Key = "agent.ppv2_upstream_write_failed"

	// KeyPPv2Injected 是向上游注入 PPv2 包头成功时的键，参数依次为来源地址与客户端 IP
	// KeyPPv2Injected is the key used once the PPv2 header has been injected towards the upstream, with the source address and the client IP as arguments
	KeyPPv2Injected i18n.Key = "agent.ppv2_injected"

	// KeyRelayUpstreamEstablished 是源站侧转发通道建立时的键，参数依次为来源地址、客户端 IP 与上游地址
	// KeyRelayUpstreamEstablished is the key used once the forwarding channel on the origin side is established, with the source address, the client IP and the upstream address as arguments
	KeyRelayUpstreamEstablished i18n.Key = "agent.relay_upstream_established"
)

// messages 保存本包的消息，键为 i18n.Key，值为三种语言下的文案
// messages holds this package's messages, keyed by i18n.Key with the text in three languages
var messages = map[i18n.Key]i18n.Entry{
	KeyClientListening: i18n.New(
		"Client Agent is listening addr:%s",
		"Client Agent 监听中 addr:%s",
		"Client Agent 監聽中 addr:%s",
	),
	KeyAcceptFailed: i18n.New(
		"Accept failed err:%v",
		"Accept 错误 err:%v",
		"Accept 錯誤 err:%v",
	),
	KeyLocalAccessRateLimited: i18n.New(
		"local access exceeded the per-IP rate limit, rejecting the connection remote:%s",
		"本地接入超过每 IP 限速，拒绝连接 remote:%s",
		"本地接入超過每 IP 限速，拒絕連線 remote:%s",
	),
	KeyLocalAccessConcurrencyFull: i18n.New(
		"the local access concurrency is full, rejecting the connection",
		"本地接入并发已满，拒绝连接",
		"本地接入並行已滿，拒絕連線",
	),
	KeyAccessFlowFailed: i18n.New(
		"[%s] the access flow failed err:%v",
		"[%s] 接入流程失败 err:%v",
		"[%s] 接入流程失敗 err:%v",
	),
	KeyRelayEstablished: i18n.New(
		"[%s] passthrough channel established",
		"[%s] 透传通道已建立",
		"[%s] 透傳通道已建立",
	),
	KeyProbeItemListReceived: i18n.New(
		"[%s] probe item list received count:%d",
		"[%s] 收到探测项列表 count:%d",
		"[%s] 收到探測項列表 count:%d",
	),
	KeyRedirectReceived: i18n.New(
		"[%s] Redirect received target:%s port:%d",
		"[%s] 收到 Redirect target:%s port:%d",
		"[%s] 收到 Redirect target:%s port:%d",
	),
	KeyServerListening: i18n.New(
		"Server Agent is listening addr:%s",
		"Server Agent 监听中 addr:%s",
		"Server Agent 監聽中 addr:%s",
	),
	KeyServerAuthConcurrencyFull: i18n.New(
		"the pre-authentication handshake concurrency of the Server Agent is full, rejecting the new connection",
		"Server Agent 认证前握手并发已满，拒绝新连接",
		"Server Agent 認證前握手並行已滿，拒絕新連線",
	),
	KeyServerAuthRateLimited: i18n.New(
		"[%s] the pre-authentication connection exceeded the rate limit, rejecting it",
		"[%s] 认证前连接超过限速，拒绝",
		"[%s] 認證前連線超過限速，拒絕",
	),
	KeyCommSecretReadFailed: i18n.New(
		"[%s] reading the comm secret failed err:%v",
		"[%s] 读取通信密钥失败 err:%v",
		"[%s] 讀取通信密鑰失敗 err:%v",
	),
	KeyCommSecretAuthFailed: i18n.New(
		"[%s] comm secret authentication failed",
		"[%s] 通信密钥认证失败",
		"[%s] 通信密鑰驗證失敗",
	),
	KeyAckWriteFailed: i18n.New(
		"[%s] writing the ack frame failed err:%v",
		"[%s] 回确认帧失败 err:%v",
		"[%s] 回確認幀失敗 err:%v",
	),
	KeyPPv2HeaderReadFailed: i18n.New(
		"[%s] reading the PPv2 header failed err:%v",
		"[%s] 读取 PPv2 包头失败 err:%v",
		"[%s] 讀取 PPv2 包頭失敗 err:%v",
	),
	KeyPPv2AddrReadFailed: i18n.New(
		"[%s] reading the PPv2 address data failed err:%v",
		"[%s] 读取 PPv2 地址数据失败 err:%v",
		"[%s] 讀取 PPv2 位址資料失敗 err:%v",
	),
	KeyPPv2HeaderInvalid: i18n.New(
		"[%s] PPv2 header validation failed err:%v",
		"[%s] PPv2 包头校验失败 err:%v",
		"[%s] PPv2 包頭校驗失敗 err:%v",
	),
	KeyClientRealAddrResolved: i18n.New(
		"[%s] client real address ip:%s port:%d",
		"[%s] 客户端真实地址 ip:%s port:%d",
		"[%s] 用戶端真實位址 ip:%s port:%d",
	),
	KeyUpstreamConnectFailed: i18n.New(
		"[%s] connecting the game server failed err:%v",
		"[%s] 连接游戏服务器失败 err:%v",
		"[%s] 連線遊戲伺服器失敗 err:%v",
	),
	KeyPPv2UpstreamWriteFailed: i18n.New(
		"[%s] writing the PPv2 header to the upstream failed err:%v",
		"[%s] 写入 PPv2 包头至上游失败 err:%v",
		"[%s] 寫入 PPv2 包頭至上游失敗 err:%v",
	),
	KeyPPv2Injected: i18n.New(
		"[%s] PPv2 header injected client_ip:%s",
		"[%s] 已注入 PPv2 包头 client_ip:%s",
		"[%s] 已注入 PPv2 包頭 client_ip:%s",
	),
	KeyRelayUpstreamEstablished: i18n.New(
		"[%s] forwarding channel established client_ip:%s upstream:%s",
		"[%s] 转发通道已建立 client_ip:%s upstream:%s",
		"[%s] 轉發通道已建立 client_ip:%s upstream:%s",
	),
}

// init 登记本包的消息，使任何日志调用之前目录已经就绪
// init registers this package's messages so that the catalogue is ready before any logging call
func init() {
	i18n.Register(messages)
}
