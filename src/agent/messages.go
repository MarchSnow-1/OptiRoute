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

import (
	"fmt"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

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

	// KeyClientListenFailed 是 Client Agent 本地监听失败时的键，参数依次为监听地址与监听错误
	// KeyClientListenFailed is the key used when the local listener of the Client Agent fails, with the listen address and the listen error as arguments
	KeyClientListenFailed i18n.Key = "agent.client_listen_failed"

	// KeyEdgeDialFailed 是 Client Agent 连接目标 Edge 业务端口失败时的键，参数为拨号错误
	// KeyEdgeDialFailed is the key used when the Client Agent fails to dial the business port of the target Edge, whose argument is the dial error
	KeyEdgeDialFailed i18n.Key = "agent.edge_dial_failed"

	// KeyFirstPacketWriteFailed 是发送 Token 首包失败时的键，参数为写入错误
	// KeyFirstPacketWriteFailed is the key used when sending the Token first packet fails, whose argument is the write error
	KeyFirstPacketWriteFailed i18n.Key = "agent.first_packet_write_failed"

	// KeyAckWaitTimeout 是等待验签确认字节超时时的键，参数为读超时错误
	// 该分支只在下层错误满足 net.Error 且 Timeout 为真时成立
	//
	// KeyAckWaitTimeout is the key used when waiting for the verification ack byte times out, whose argument is the read timeout error
	// The branch is taken only while the underlying error satisfies net.Error with Timeout reporting true
	KeyAckWaitTimeout i18n.Key = "agent.ack_wait_timeout"

	// KeyAckWaitFailed 是等待验签确认字节时连接中断的键，参数为读取错误
	// KeyAckWaitFailed is the key used when the connection drops while waiting for the verification ack byte, whose argument is the read error
	KeyAckWaitFailed i18n.Key = "agent.ack_wait_failed"

	// KeyAckByteCountAbnormal 是读到的确认字节数不是 1 时的键，参数为实际读到的字节数
	// KeyAckByteCountAbnormal is the key used when the ack byte count read is not 1, whose argument is the count actually read
	KeyAckByteCountAbnormal i18n.Key = "agent.ack_byte_count_abnormal"

	// KeyAckByteUnexpected 是确认字节取值不是 0x01 时的键，参数为该字节
	// KeyAckByteUnexpected is the key used when the ack byte holds a value other than 0x01, whose argument is that byte
	KeyAckByteUnexpected i18n.Key = "agent.ack_byte_unexpected"

	// KeyBootstrapDialFailed 是连接引导节点失败时的键，参数为拨号错误
	// KeyBootstrapDialFailed is the key used when dialling the bootstrap node fails, whose argument is the dial error
	KeyBootstrapDialFailed i18n.Key = "agent.bootstrap_dial_failed"

	// KeyMagicWriteFailed 是发送引导 Magic 失败时的键，参数为写入错误
	// KeyMagicWriteFailed is the key used when sending the bootstrap Magic fails, whose argument is the write error
	KeyMagicWriteFailed i18n.Key = "agent.magic_write_failed"

	// KeyNodeListReadFailed 是接收引导节点下发的节点列表失败时的键，参数为读取错误
	// KeyNodeListReadFailed is the key used when receiving the node list delivered by the bootstrap node fails, whose argument is the read error
	KeyNodeListReadFailed i18n.Key = "agent.node_list_read_failed"

	// KeyProbeItemListParseFailed 是探测项列表解析失败时的键，参数为解析错误
	// KeyProbeItemListParseFailed is the key used when the probe item list fails to parse, whose argument is the parse error
	KeyProbeItemListParseFailed i18n.Key = "agent.probe_item_list_parse_failed"

	// KeyProbeAllFailed 是全部探测项都失败时的键，不带参数
	// 该分支使本次接入流程直接失败，不上报任何结果
	//
	// KeyProbeAllFailed is the key used when every probe item failed, and it takes no argument
	// The branch fails the whole access flow without reporting any result
	KeyProbeAllFailed i18n.Key = "agent.probe_all_failed"

	// KeyProbeResultWriteFailed 是上报探测结果失败时的键，参数为写入错误
	// KeyProbeResultWriteFailed is the key used when reporting the probe results fails, whose argument is the write error
	KeyProbeResultWriteFailed i18n.Key = "agent.probe_result_write_failed"

	// KeyRedirectReadFailed 是接收 Redirect 命令失败时的键，参数为读取错误
	// KeyRedirectReadFailed is the key used when receiving the Redirect command fails, whose argument is the read error
	KeyRedirectReadFailed i18n.Key = "agent.redirect_read_failed"

	// KeyRedirectParseFailed 是 Redirect 命令解析失败时的键，参数为解析错误
	// KeyRedirectParseFailed is the key used when the Redirect command fails to parse, whose argument is the parse error
	KeyRedirectParseFailed i18n.Key = "agent.redirect_parse_failed"

	// KeyServerSecretLengthInvalid 是数据面密钥长度不是 requiredCommSecretLen 时的键，参数依次为要求长度与实际长度
	// 该检查在监听之前完成，使长度为零的密钥不会带着「空切片比较判定相等」的语义进入服务
	//
	// KeyServerSecretLengthInvalid is the key used when the data-plane secret is not requiredCommSecretLen bytes long, with the required length and the actual length as arguments
	// The check runs before listening, which keeps a zero-length secret out of the service, where comparing two empty slices reports equality
	KeyServerSecretLengthInvalid i18n.Key = "agent.server_secret_length_invalid"

	// KeyServerListenFailed 是 Server Agent 监听失败时的键，参数依次为监听地址与监听错误
	// KeyServerListenFailed is the key used when the listener of the Server Agent fails, with the listen address and the listen error as arguments
	KeyServerListenFailed i18n.Key = "agent.server_listen_failed"

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
	KeyClientListenFailed: i18n.New(
		"local port listen failed %s: %w",
		"本地端口监听失败 %s: %w",
		"本機連接埠監聽失敗 %s: %w",
	),
	KeyEdgeDialFailed: i18n.New(
		"connecting the target edge node failed: %w",
		"连接目标边缘节点失败: %w",
		"連線目標邊緣節點失敗: %w",
	),
	KeyFirstPacketWriteFailed: i18n.New(
		"sending the Token first packet failed: %w",
		"发送 Token 首包失败: %w",
		"傳送 Token 首包失敗: %w",
	),
	KeyAckWaitTimeout: i18n.New(
		"timed out waiting for the signature check: %w",
		"等待验签超时: %w",
		"等待驗簽逾時: %w",
	),
	KeyAckWaitFailed: i18n.New(
		"the signature check failed (connection interrupted): %w",
		"等待验签失败（连接中断）: %w",
		"等待驗簽失敗（連線中斷）: %w",
	),
	KeyAckByteCountAbnormal: i18n.New(
		"the signature check failed (the server returned no ack byte, %d bytes received)",
		"验签未通过（服务端未返回确认字节，收到 %d 字节）",
		"驗簽未通過（伺服端未回傳確認位元組，收到 %d 位元組）",
	),
	KeyAckByteUnexpected: i18n.New(
		"the signature check failed (the server returned the ack byte 0x%02X)",
		"验签未通过（服务端返回的确认字节为 0x%02X）",
		"驗簽未通過（伺服端回傳的確認位元組為 0x%02X）",
	),
	KeyBootstrapDialFailed: i18n.New(
		"connecting the bootstrap node failed: %w",
		"连接引导节点失败: %w",
		"連線引導節點失敗: %w",
	),
	KeyMagicWriteFailed: i18n.New(
		"sending the Magic failed: %w",
		"发送 Magic 失败: %w",
		"傳送 Magic 失敗: %w",
	),
	KeyNodeListReadFailed: i18n.New(
		"receiving the node list failed: %w",
		"接收节点列表失败: %w",
		"接收節點清單失敗: %w",
	),
	KeyProbeItemListParseFailed: i18n.New(
		"parsing the probe item list failed: %w",
		"解析探测项列表失败: %w",
		"解析探測項列表失敗: %w",
	),
	KeyProbeAllFailed: i18n.New(
		"every probe item failed",
		"所有探测项均失败",
		"所有探測項均失敗",
	),
	KeyProbeResultWriteFailed: i18n.New(
		"reporting the probe results failed: %w",
		"上报探测结果失败: %w",
		"上報探測結果失敗: %w",
	),
	KeyRedirectReadFailed: i18n.New(
		"receiving the Redirect command failed: %w",
		"接收 Redirect 命令失败: %w",
		"接收 Redirect 命令失敗: %w",
	),
	KeyRedirectParseFailed: i18n.New(
		"parsing the Redirect command failed: %w",
		"解析 Redirect 命令失败: %w",
		"解析 Redirect 命令失敗: %w",
	),
	KeyServerSecretLengthInvalid: i18n.New(
		"the data-plane secret has to be %d bytes long, currently %d bytes",
		"数据面密钥长度必须是 %d 字节，当前 %d 字节",
		"資料面密鑰長度必須是 %d 位元組，目前 %d 位元組",
	),
	KeyServerListenFailed: i18n.New(
		"Server Agent listen failed %s: %w",
		"Server Agent 监听失败 %s: %w",
		"Server Agent 監聽失敗 %s: %w",
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

// errf 按消息键取当前语言的格式串并交给 fmt.Errorf
// 取的是未填充的格式串而不是已渲染的文本，使 %w 仍由 fmt.Errorf 处理，错误链得以保留
// 这些错误会被 agent 的调用方经 %w 嵌入日志行，因此文案同样取自目录
//
// errf takes the format string of a message key in the current language and hands it to fmt.Errorf
// The unfilled format string is used rather than rendered text, so a %w verb is still handled by fmt.Errorf and the error chain survives
// A caller of agent embeds these errors in a log line through %w, so their text comes from the catalogue as well
func errf(key i18n.Key, args ...any) error {
	return fmt.Errorf(i18n.T(key), args...)
}

// init 登记本包的消息，使任何日志调用之前目录已经就绪
// init registers this package's messages so that the catalogue is ready before any logging call
func init() {
	i18n.Register(messages)
}
