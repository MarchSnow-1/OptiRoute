// 本文件登记 edge 包用到的消息键与其三种语言文案
// 键名按「包名.用途」命名，原文案里的 [%s] 远端地址前缀与 err:、addr: 一类字段标签保留在文案里，使日志仍能指出产生它的位置与上下文
// 在 init 中一次性登记，登记完成后目录只被读取
//
// This file registers the message keys edge uses together with their text in three languages
// A key is named package.purpose, while the [%s] remote-address prefix and the field labels such as err: and addr: of the original text stay inside it, so a log line still shows where it came from and what it refers to
// Registration happens once in init and the catalogue is only read afterwards
package edge

import "github.com/MarchSnow-1/OptiRoute/i18n"

const (
	// KeyTopoCacheWriteFailed 是异步写盘失败时的键，参数为写盘错误
	// KeyTopoCacheWriteFailed is the key used when an asynchronous disk write fails, whose argument is the write error
	KeyTopoCacheWriteFailed i18n.Key = "edge.topo_cache_write_failed"

	// KeyTopoCacheFlushFailed 是关闭前冲刷缓存失败时的键，参数为写盘错误
	// KeyTopoCacheFlushFailed is the key used when the flush before close fails, whose argument is the write error
	KeyTopoCacheFlushFailed i18n.Key = "edge.topo_cache_flush_failed"

	// KeyTopoCacheLoaded 是从缓存文件加载拓扑成功时的键，参数依次为节点数与缓存写入时刻（Unix 秒）
	// KeyTopoCacheLoaded is the key used after the topology was loaded from the cache file, with the node count and the cache write instant in Unix seconds as arguments
	KeyTopoCacheLoaded i18n.Key = "edge.topo_cache_loaded"

	// KeyProbePortUnset 是 probe_port 为 0 因而跳过探测服务时的键，不带参数
	// KeyProbePortUnset is the key used when the probe service is skipped because probe_port is 0, and it carries no argument
	KeyProbePortUnset i18n.Key = "edge.probe_port_unset"

	// KeyProbeICMPNoListener 是 probe_proto 为 icmp 因而不启动监听时的键，不带参数
	// KeyProbeICMPNoListener is the key used when no listener is started because probe_proto is icmp, and it carries no argument
	KeyProbeICMPNoListener i18n.Key = "edge.probe_icmp_no_listener"

	// KeyProbeTCPListenFailed 是 TCP 探测端口监听失败时的键，参数为监听错误
	// KeyProbeTCPListenFailed is the key used when the TCP probe port fails to listen, whose argument is the listen error
	KeyProbeTCPListenFailed i18n.Key = "edge.probe_tcp_listen_failed"

	// KeyProbeTCPListening 是 TCP 探测端口开始监听时的键，参数为监听地址
	// KeyProbeTCPListening is the key used once the TCP probe port listens, whose argument is the listen address
	KeyProbeTCPListening i18n.Key = "edge.probe_tcp_listening"

	// KeyProbeTCPRateLimited 是 TCP 探测请求超过每来源 IP 限速时的键，参数为来源主机
	// KeyProbeTCPRateLimited is the key used when a TCP probe request exceeds the per-source-IP rate limit, whose argument is the source host
	KeyProbeTCPRateLimited i18n.Key = "edge.probe_tcp_rate_limited"

	// KeyProbeTCPGateFull 是 TCP 探测并发闸已满时的键，参数为来源主机
	// KeyProbeTCPGateFull is the key used when the TCP probe concurrency gate is full, whose argument is the source host
	KeyProbeTCPGateFull i18n.Key = "edge.probe_tcp_gate_full"

	// KeyProbeUDPListenFailed 是 UDP 探测端口监听失败时的键，参数为监听错误
	// KeyProbeUDPListenFailed is the key used when the UDP probe port fails to listen, whose argument is the listen error
	KeyProbeUDPListenFailed i18n.Key = "edge.probe_udp_listen_failed"

	// KeyProbeUDPListening 是 UDP 探测端口开始监听时的键，参数为监听地址
	// KeyProbeUDPListening is the key used once the UDP probe port listens, whose argument is the listen address
	KeyProbeUDPListening i18n.Key = "edge.probe_udp_listening"

	// KeyCenterConnectAttempt 是每次尝试连接中心节点前输出的键，参数依次为本次序号与总次数
	// KeyCenterConnectAttempt is the key printed before every Center connection attempt, with the current attempt number and the total as arguments
	KeyCenterConnectAttempt i18n.Key = "edge.center_connect_attempt"

	// KeyCenterConnectFailed 是连接中心节点失败时的键，参数为连接错误
	// KeyCenterConnectFailed is the key used when connecting to Center fails, whose argument is the connection error
	KeyCenterConnectFailed i18n.Key = "edge.center_connect_failed"

	// KeyCenterWaitRegistrationFailed 是启动期等待注册结果失败时的键，参数为等待错误
	// KeyCenterWaitRegistrationFailed is the key used when waiting for the registration result fails during start-up, whose argument is the wait error
	KeyCenterWaitRegistrationFailed i18n.Key = "edge.center_wait_registration_failed"

	// KeyCenterRejectedStopStartupRetry 是注册被拒且开关要求停止时结束启动重试的键，不带参数
	// KeyCenterRejectedStopStartupRetry is the key used when the start-up retries stop because registration was rejected and the switch asks for it, and it carries no argument
	KeyCenterRejectedStopStartupRetry i18n.Key = "edge.center_rejected_stop_startup_retry"

	// KeySecretLoadedFromCache 是降级模式从缓存取到 shared_secret 时的键，不带参数
	// KeySecretLoadedFromCache is the key used when degraded mode obtained a shared_secret from the cache, and it carries no argument
	KeySecretLoadedFromCache i18n.Key = "edge.secret_loaded_from_cache"

	// KeySecretMissingInCache 是缓存中没有 shared_secret 时的键，不带参数
	// KeySecretMissingInCache is the key used when the cache holds no shared_secret, and it carries no argument
	KeySecretMissingInCache i18n.Key = "edge.secret_missing_in_cache"

	// KeyEnterDegradedMode 是重试耗尽并加载本地缓存后进入降级模式时的键，不带参数
	// KeyEnterDegradedMode is the key used when the retries are exhausted and the node enters degraded mode after loading the local cache, and it carries no argument
	KeyEnterDegradedMode i18n.Key = "edge.enter_degraded_mode"

	// KeyStartedDegraded 是各服务启动完毕且处于降级模式时的键，不带参数
	// KeyStartedDegraded is the key used once the services are up while the node is in degraded mode, and it carries no argument
	KeyStartedDegraded i18n.Key = "edge.started_degraded"

	// KeyStartedNormal 是各服务启动完毕且已连接中心节点时的键，不带参数
	// KeyStartedNormal is the key used once the services are up with a Center connection in place, and it carries no argument
	KeyStartedNormal i18n.Key = "edge.started_normal"

	// KeyShuttingDown 是 ctx 结束、节点进入关闭流程时的键，不带参数
	// KeyShuttingDown is the key used when ctx ends and the node starts shutting down, and it carries no argument
	KeyShuttingDown i18n.Key = "edge.shutting_down"

	// KeyBackgroundReconnectAttempt 是降级模式下每次后台重连前输出的键，不带参数
	// KeyBackgroundReconnectAttempt is the key printed before every background reconnect in degraded mode, and it carries no argument
	KeyBackgroundReconnectAttempt i18n.Key = "edge.background_reconnect_attempt"

	// KeyBackgroundReconnectFailed 是后台重连拨号失败时的键，参数为连接错误
	// KeyBackgroundReconnectFailed is the key used when the background reconnect fails to dial, whose argument is the connection error
	KeyBackgroundReconnectFailed i18n.Key = "edge.background_reconnect_failed"

	// KeyBackgroundReconnectWaitFailed 是后台重连等待注册结果失败时的键，参数为等待错误
	// KeyBackgroundReconnectWaitFailed is the key used when the background reconnect fails while waiting for the registration result, whose argument is the wait error
	KeyBackgroundReconnectWaitFailed i18n.Key = "edge.background_reconnect_wait_failed"

	// KeyCenterRejectedStopBackgroundReconnect 是注册被拒且开关要求时结束后台重连的键，不带参数
	// KeyCenterRejectedStopBackgroundReconnect is the key used when the background reconnect stops because registration was rejected and the switch asks for it, and it carries no argument
	KeyCenterRejectedStopBackgroundReconnect i18n.Key = "edge.center_rejected_stop_background_reconnect"

	// KeyLeftDegradedMode 是后台重连成功并退出降级模式时的键，不带参数
	// KeyLeftDegradedMode is the key used when the background reconnect succeeded and the node left degraded mode, and it carries no argument
	KeyLeftDegradedMode i18n.Key = "edge.left_degraded_mode"

	// KeyLinkStatusChanged 是链路状态发生变迁时的键，参数依次为原状态、新状态、窗口丢包率与配置阈值
	// KeyLinkStatusChanged is the key used when the link status transitions, with the previous status, the new status, the windowed loss rate and the configured threshold as arguments
	KeyLinkStatusChanged i18n.Key = "edge.link_status_changed"

	// KeyFakeIPSelfAddressSkipped 是 FAKE-IP 与本机地址相同时跳过该项的键，参数为该 FAKE-IP
	// KeyFakeIPSelfAddressSkipped is the key used when a FAKE-IP is skipped for being this node's own address, whose argument is that FAKE-IP
	KeyFakeIPSelfAddressSkipped i18n.Key = "edge.fake_ip_self_address_skipped"

	// KeyWriteToCenterFailed 是控制面连接写消息失败时的键，参数为写入错误
	// KeyWriteToCenterFailed is the key used when writing a message on the control-plane connection fails, whose argument is the write error
	KeyWriteToCenterFailed i18n.Key = "edge.write_to_center_failed"

	// KeyWriteQueueFull 是写队列已满并主动断开当前连接时的键，参数为触发本次断开的消息类型
	// KeyWriteQueueFull is the key used when the write queue is full and the current connection is dropped for it, whose argument is the message type that triggered the drop
	KeyWriteQueueFull i18n.Key = "edge.write_queue_full"

	// KeyCenterConnectionLost 是控制面连接断开且注册未被拒绝时的键，参数为读取错误
	// KeyCenterConnectionLost is the key used when the control-plane connection drops while registration was not rejected, whose argument is the read error
	KeyCenterConnectionLost i18n.Key = "edge.center_connection_lost"

	// KeyRejectedStopAutoReconnect 是注册被拒且开关为真因而不再自动重连时的键，不带参数
	// KeyRejectedStopAutoReconnect is the key used when no automatic reconnect starts because registration was rejected and the switch is true, and it carries no argument
	KeyRejectedStopAutoReconnect i18n.Key = "edge.rejected_stop_auto_reconnect"

	// KeyRegisteredPayloadParseFailed 是注册结果载荷解析失败时的键，参数为解析错误
	// KeyRegisteredPayloadParseFailed is the key used when the registration result payload fails to parse, whose argument is the parse error
	KeyRegisteredPayloadParseFailed i18n.Key = "edge.registered_payload_parse_failed"

	// KeyRegistrationRejected 是中心节点拒绝注册时的键，参数依次为本节点 UUID 与拒绝原因
	// KeyRegistrationRejected is the key used when Center rejects the registration, with this node's UUID and the reason as arguments
	KeyRegistrationRejected i18n.Key = "edge.registration_rejected"

	// KeyRegistrationSucceeded 是注册成功时的键，参数为本节点 UUID
	// KeyRegistrationSucceeded is the key used after a successful registration, whose argument is this node's UUID
	KeyRegistrationSucceeded i18n.Key = "edge.registration_succeeded"

	// KeyTopoResponseParseFailed 是拓扑响应解析失败时的键，参数为解析错误
	// 解析失败时保留上一次的拓扑，因此这里只记录本次错误
	//
	// KeyTopoResponseParseFailed is the key used when the topology response fails to parse, whose argument is the parse error
	// The previous topology stays in place on such a failure, so only this error is recorded here
	KeyTopoResponseParseFailed i18n.Key = "edge.topo_response_parse_failed"

	// KeyTopologyUpdated 是拓扑替换完成时的键，参数依次为节点数与中心的客户端信息采集开关
	// KeyTopologyUpdated is the key used after the topology was replaced, with the node count and the client-info collection switch of Center as arguments
	KeyTopologyUpdated i18n.Key = "edge.topology_updated"

	// KeySecretPushParseFailed 是密钥推送载荷解析失败时的键，参数为解析错误
	// 解析失败时保留上一次的 secret，因此这里只记录本次错误
	//
	// KeySecretPushParseFailed is the key used when the secret push payload fails to parse, whose argument is the parse error
	// The previous secret stays in place on such a failure, so only this error is recorded here
	KeySecretPushParseFailed i18n.Key = "edge.secret_push_parse_failed"

	// KeySecretPushInvalid 是密钥推送内容非法时的键，参数依次为 hex 解码错误与解码出的字节数
	// 推送长度不是 32 字节时解码错误为 nil，此时只看字节数即可判断原因
	//
	// KeySecretPushInvalid is the key used when the pushed secret is invalid, with the hex decoding error and the decoded byte count as arguments
	// A push that decodes without error but is not 32 bytes long carries a nil error, and the byte count alone then tells the reason
	KeySecretPushInvalid i18n.Key = "edge.secret_push_invalid"

	// KeySharedSecretUpdated 是收到并接受一次密钥推送后的键，参数为推送版本号
	// KeySharedSecretUpdated is the key used after a secret push was received and accepted, whose argument is the pushed version
	KeySharedSecretUpdated i18n.Key = "edge.shared_secret_updated"

	// KeyUnknownMessageType 是下行消息类型未被识别时的键，参数为该消息类型
	// KeyUnknownMessageType is the key used when a downstream message type is not recognised, whose argument is that message type
	KeyUnknownMessageType i18n.Key = "edge.unknown_message_type"

	// KeyCenterRejectedStopReconnect 是注册被拒且开关要求时结束重连的键，不带参数
	// KeyCenterRejectedStopReconnect is the key used when the reconnect stops because registration was rejected and the switch asks for it, and it carries no argument
	KeyCenterRejectedStopReconnect i18n.Key = "edge.center_rejected_stop_reconnect"

	// KeyTryingReconnect 是每次重连尝试拨号前输出的键，不带参数
	// KeyTryingReconnect is the key printed before every reconnect attempt dials, and it carries no argument
	KeyTryingReconnect i18n.Key = "edge.trying_reconnect"

	// KeyBusinessPortListening 是业务端口开始监听时的键，参数为监听地址
	// KeyBusinessPortListening is the key used once the business port listens, whose argument is the listen address
	KeyBusinessPortListening i18n.Key = "edge.business_port_listening"

	// KeyBusinessAcceptFailed 是业务端口 Accept 失败时的键，参数为 Accept 错误
	// KeyBusinessAcceptFailed is the key used when the business port Accept fails, whose argument is the accept error
	KeyBusinessAcceptFailed i18n.Key = "edge.business_accept_failed"

	// KeyHandshakeConcurrencyFull 是未认证与引导握手并发闸已满时的键，不带参数
	// KeyHandshakeConcurrencyFull is the key used when the concurrency gate of unauthenticated and bootstrap handshakes is full, and it carries no argument
	KeyHandshakeConcurrencyFull i18n.Key = "edge.handshake_concurrency_full"

	// KeyBootstrapRateLimited 是引导请求超过每来源 IP 限速时的键，参数依次为远端地址
	// KeyBootstrapRateLimited is the key used when a bootstrap request exceeds the per-source-IP rate limit, whose first argument is the remote address
	KeyBootstrapRateLimited i18n.Key = "edge.bootstrap_rate_limited"

	// KeyBootstrapDegradedMode 是降级模式下仍为客户端引导时的键，参数依次为远端地址
	// KeyBootstrapDegradedMode is the key used when a client is bootstrapped while the node is degraded, whose first argument is the remote address
	KeyBootstrapDegradedMode i18n.Key = "edge.bootstrap_degraded_mode"

	// KeyBootstrapNoProbeItems 是拓扑里没有任何探测项时的键，参数依次为远端地址
	// KeyBootstrapNoProbeItems is the key used when the topology yields no probe item at all, whose first argument is the remote address
	KeyBootstrapNoProbeItems i18n.Key = "edge.bootstrap_no_probe_items"

	// KeyBootstrapSendListFailed 是下发探测项列表失败时的键，参数依次为远端地址与写入错误
	// KeyBootstrapSendListFailed is the key used when sending the probe item list fails, with the remote address and the write error as arguments
	KeyBootstrapSendListFailed i18n.Key = "edge.bootstrap_send_list_failed"

	// KeyBootstrapReadResultsFailed 是读取客户端探测结果失败时的键，参数依次为远端地址与读取错误
	// KeyBootstrapReadResultsFailed is the key used when reading the client probe results fails, with the remote address and the read error as arguments
	KeyBootstrapReadResultsFailed i18n.Key = "edge.bootstrap_read_results_failed"

	// KeyBootstrapParseResultsFailed 是探测结果解析失败时的键，参数依次为远端地址与解析错误
	// KeyBootstrapParseResultsFailed is the key used when the probe results fail to parse, with the remote address and the parse error as arguments
	KeyBootstrapParseResultsFailed i18n.Key = "edge.bootstrap_parse_results_failed"

	// KeyBootstrapResultCountAbnormal 是探测结果数量超过下发项数时的键，参数依次为远端地址、结果条数与下发项数
	// KeyBootstrapResultCountAbnormal is the key used when the probe result count exceeds the number of items sent, with the remote address, the result count and the item count as arguments
	KeyBootstrapResultCountAbnormal i18n.Key = "edge.bootstrap_result_count_abnormal"

	// KeyBootstrapRTTRejected 是探测结果中的 RTT 为负数或超出可信上界而被忽略时的键，参数依次为远端地址、编码与 RTT 毫秒数
	// KeyBootstrapRTTRejected is the key used when an RTT in the probe results is negative or above the trusted bound and is dropped, with the remote address, the code and the RTT in milliseconds as arguments
	KeyBootstrapRTTRejected i18n.Key = "edge.bootstrap_rtt_rejected"

	// KeyBootstrapAllOverloaded 是过滤后无候选节点因而回退使用 overloaded 节点时的键，参数依次为远端地址
	// KeyBootstrapAllOverloaded is the key used when the filtering leaves no candidate and the overloaded nodes are used as a fallback, whose first argument is the remote address
	KeyBootstrapAllOverloaded i18n.Key = "edge.bootstrap_all_overloaded"

	// KeyBootstrapNoCandidate 是连 overloaded 节点也没有时的键，参数依次为远端地址
	// KeyBootstrapNoCandidate is the key used when even the overloaded nodes yield nothing, whose first argument is the remote address
	KeyBootstrapNoCandidate i18n.Key = "edge.bootstrap_no_candidate"

	// KeyBootstrapTokenFailed 是签发路由 Token 失败时的键，参数依次为远端地址与签发错误
	// KeyBootstrapTokenFailed is the key used when issuing the route Token fails, with the remote address and the issuing error as arguments
	KeyBootstrapTokenFailed i18n.Key = "edge.bootstrap_token_failed"

	// KeyBootstrapSendRedirectFailed 是下发 Redirect 命令失败时的键，参数依次为远端地址与写入错误
	// KeyBootstrapSendRedirectFailed is the key used when sending the Redirect command fails, with the remote address and the write error as arguments
	KeyBootstrapSendRedirectFailed i18n.Key = "edge.bootstrap_send_redirect_failed"

	// KeyBootstrapCompleted 是一次引导交互完成时的键，参数依次为远端地址、目标节点 UUID 与全链路 RTT 毫秒数
	// KeyBootstrapCompleted is the key used once one bootstrap interaction is done, with the remote address, the target node UUID and the total path RTT in milliseconds as arguments
	KeyBootstrapCompleted i18n.Key = "edge.bootstrap_completed"

	// KeyBusinessReadFirstPacketFailed 是业务首帧读取失败时的键，参数依次为远端地址与读取错误
	// KeyBusinessReadFirstPacketFailed is the key used when reading the business first frame fails, with the remote address and the read error as arguments
	KeyBusinessReadFirstPacketFailed i18n.Key = "edge.business_read_first_packet_failed"

	// KeyBusinessFirstPacketJSONFailed 是业务首帧 JSON 解析失败时的键，参数依次为远端地址
	// KeyBusinessFirstPacketJSONFailed is the key used when the business first frame fails to parse as JSON, whose first argument is the remote address
	KeyBusinessFirstPacketJSONFailed i18n.Key = "edge.business_first_packet_json_failed"

	// KeyBusinessClientAddrFailed 是连接远端地址无法断言为 TCP 地址时的键，参数依次为远端地址
	// KeyBusinessClientAddrFailed is the key used when the remote address of the connection cannot be asserted as a TCP address, whose first argument is the remote address
	KeyBusinessClientAddrFailed i18n.Key = "edge.business_client_addr_failed"

	// KeyBusinessTokenRejected 是路由 Token 验签不通过时的键，参数依次为远端地址
	// KeyBusinessTokenRejected is the key used when the route Token fails verification, whose first argument is the remote address
	KeyBusinessTokenRejected i18n.Key = "edge.business_token_rejected"

	// KeyBusinessForwardCapReached 是已建立转发连接数达到上限时的键，参数依次为远端地址
	// KeyBusinessForwardCapReached is the key used when the established forwarding connections reach the cap, whose first argument is the remote address
	KeyBusinessForwardCapReached i18n.Key = "edge.business_forward_cap_reached"

	// KeyBusinessWriteAckFailed 是向客户端写验签确认失败时的键，参数依次为远端地址与写入错误
	// KeyBusinessWriteAckFailed is the key used when writing the verification acknowledgement to the client fails, with the remote address and the write error as arguments
	KeyBusinessWriteAckFailed i18n.Key = "edge.business_write_ack_failed"

	// KeyBusinessDialOriginFailed 是连接源站失败时的键，参数依次为远端地址与连接错误
	// KeyBusinessDialOriginFailed is the key used when dialling the origin fails, with the remote address and the dial error as arguments
	KeyBusinessDialOriginFailed i18n.Key = "edge.business_dial_origin_failed"

	// KeyBusinessLocalAddrFailed 是本端地址无法断言为 TCP 地址时的键，参数依次为远端地址
	// KeyBusinessLocalAddrFailed is the key used when the local address cannot be asserted as a TCP address, whose first argument is the remote address
	KeyBusinessLocalAddrFailed i18n.Key = "edge.business_local_addr_failed"

	// KeyBusinessWriteCommSecretFailed 是向源站写通信密钥失败时的键，参数依次为远端地址与写入错误
	// KeyBusinessWriteCommSecretFailed is the key used when writing the communication secret to the origin fails, with the remote address and the write error as arguments
	KeyBusinessWriteCommSecretFailed i18n.Key = "edge.business_write_comm_secret_failed"

	// KeyBusinessWritePPv2Failed 是向源站写 PPv2 包头失败时的键，参数依次为远端地址与写入错误
	// KeyBusinessWritePPv2Failed is the key used when writing the PPv2 header to the origin fails, with the remote address and the write error as arguments
	KeyBusinessWritePPv2Failed i18n.Key = "edge.business_write_ppv2_failed"

	// KeyBusinessReadServerAckFailed 是读取 Server Agent 确认帧失败时的键，参数依次为远端地址与读取错误
	// KeyBusinessReadServerAckFailed is the key used when reading the Server Agent ack frame fails, with the remote address and the read error as arguments
	KeyBusinessReadServerAckFailed i18n.Key = "edge.business_read_server_ack_failed"

	// KeyBusinessServerAckParseFailed 是 Server Agent 确认帧解析失败时的键，参数依次为远端地址与解析错误
	// KeyBusinessServerAckParseFailed is the key used when the Server Agent ack frame fails to parse, with the remote address and the parse error as arguments
	KeyBusinessServerAckParseFailed i18n.Key = "edge.business_server_ack_parse_failed"

	// KeyBusinessRelayEstablished 是双向 L4 透传建立后的键，参数依次为远端地址、源站地址、客户端版本与 Server Agent 版本
	// KeyBusinessRelayEstablished is the key used once the bidirectional L4 relay is up, with the remote address, the origin address, the client version and the Server Agent version as arguments
	KeyBusinessRelayEstablished i18n.Key = "edge.business_relay_established"
)

// messages 保存本包的消息，键为 i18n.Key，值为三种语言下的文案
// messages holds this package's messages, keyed by i18n.Key with the text in three languages
var messages = map[i18n.Key]i18n.Entry{
	KeyTopoCacheWriteFailed: i18n.New(
		"asynchronous topology cache write failed err:%v",
		"拓扑缓存异步写入失败 err:%v",
		"拓撲快取非同步寫入失敗 err:%v",
	),
	KeyTopoCacheFlushFailed: i18n.New(
		"topology cache flush before close failed err:%v",
		"拓扑缓存关闭前冲刷失败 err:%v",
		"拓撲快取關閉前沖刷失敗 err:%v",
	),
	KeyTopoCacheLoaded: i18n.New(
		"loaded the topology from the local cache nodes:%d updated_at:%d",
		"已从本地缓存加载拓扑 nodes:%d updated_at:%d",
		"已從本機快取載入拓撲 nodes:%d updated_at:%d",
	),
	KeyProbePortUnset: i18n.New(
		"probe_port is not configured, skipping the probe service (fakeip-only or icmp mode)",
		"probe_port 未配置，跳过探测服务（fakeip-only 或 icmp 模式）",
		"probe_port 未設定，略過探測服務（fakeip-only 或 icmp 模式）",
	),
	KeyProbeICMPNoListener: i18n.New(
		"probe_proto is icmp, so no probe listener is started (the client pings this host's IP directly)",
		"probe_proto 为 icmp，不启动探测监听（客户端直接 ping 本机 IP）",
		"probe_proto 為 icmp，不啟動探測監聽（用戶端直接 ping 本機 IP）",
	),
	KeyProbeTCPListenFailed: i18n.New(
		"TCP probe port listen failed err:%v",
		"TCP 探测端口监听失败 err:%v",
		"TCP 探測埠監聽失敗 err:%v",
	),
	KeyProbeTCPListening: i18n.New(
		"TCP probe port listening addr:%s",
		"TCP 探测端口监听中 addr:%s",
		"TCP 探測埠監聽中 addr:%s",
	),
	KeyProbeTCPRateLimited: i18n.New(
		"TCP probe request over the per-IP rate limit, rejected remote:%s",
		"TCP 探测请求超过每 IP 限速，拒绝 remote:%s",
		"TCP 探測請求超過每 IP 限速，拒絕 remote:%s",
	),
	KeyProbeTCPGateFull: i18n.New(
		"TCP probe concurrency is full, rejected remote:%s",
		"TCP 探测并发已满，拒绝 remote:%s",
		"TCP 探測併發已滿，拒絕 remote:%s",
	),
	KeyProbeUDPListenFailed: i18n.New(
		"UDP probe port listen failed err:%v",
		"UDP 探测端口监听失败 err:%v",
		"UDP 探測埠監聽失敗 err:%v",
	),
	KeyProbeUDPListening: i18n.New(
		"UDP probe port listening addr:%s",
		"UDP 探测端口监听中 addr:%s",
		"UDP 探測埠監聽中 addr:%s",
	),
	KeyCenterConnectAttempt: i18n.New(
		"connecting to Center attempt %d/%d",
		"连接中心节点 第%d/%d次",
		"連線中心節點 第%d/%d次",
	),
	KeyCenterConnectFailed: i18n.New(
		"connecting to Center failed err:%v",
		"连接中心节点失败 err:%v",
		"連線中心節點失敗 err:%v",
	),
	KeyCenterWaitRegistrationFailed: i18n.New(
		"waiting for the Center registration result failed err:%v",
		"等待中心注册结果失败 err:%v",
		"等待中心註冊結果失敗 err:%v",
	),
	KeyCenterRejectedStopStartupRetry: i18n.New(
		"Center rejected the registration and stop_reconnect_on_reject is true, stopping the start-up retries and entering degraded mode",
		"Center 已拒绝注册且 stop_reconnect_on_reject 为真，停止启动重试并进入降级模式",
		"Center 已拒絕註冊且 stop_reconnect_on_reject 為真，停止啟動重試並進入降級模式",
	),
	KeySecretLoadedFromCache: i18n.New(
		"shared_secret loaded from the cache, so degraded mode can sign Tokens",
		"已从缓存加载 shared_secret，降级模式可签发 Token",
		"已從快取載入 shared_secret，降級模式可簽發 Token",
	),
	KeySecretMissingInCache: i18n.New(
		"the cache holds no shared_secret, so degraded mode cannot sign Tokens (a normal connection to Center has to happen first)",
		"缓存中无 shared_secret，降级模式无法签发 Token（需先以正常模式连接过中心）",
		"快取中沒有 shared_secret，降級模式無法簽發 Token（需先以正常模式連線過中心）",
	),
	KeyEnterDegradedMode: i18n.New(
		"Center is unreachable, the local cache has been loaded and the node enters degraded mode",
		"无法连接中心节点，已加载本地缓存，进入降级模式",
		"無法連線中心節點，已載入本機快取，進入降級模式",
	),
	KeyStartedDegraded: i18n.New(
		"Edge started in degraded mode and waits for Center to reconnect...",
		"边缘节点以降级模式启动，等待中心节点重连...",
		"邊緣節點以降級模式啟動，等待中心節點重新連線...",
	),
	KeyStartedNormal: i18n.New(
		"Edge started in normal mode",
		"边缘节点以正常模式启动",
		"邊緣節點以正常模式啟動",
	),
	KeyShuttingDown: i18n.New(
		"Edge is shutting down...",
		"边缘节点正在关闭...",
		"邊緣節點正在關閉...",
	),
	KeyBackgroundReconnectAttempt: i18n.New(
		"reconnecting to Center in the background...",
		"后台尝试重连中心节点...",
		"在背景嘗試重新連線中心節點...",
	),
	KeyBackgroundReconnectFailed: i18n.New(
		"background reconnect failed err:%v",
		"后台重连失败 err:%v",
		"背景重連失敗 err:%v",
	),
	KeyBackgroundReconnectWaitFailed: i18n.New(
		"the background reconnect failed while waiting for the registration result err:%v",
		"后台重连等待注册结果失败 err:%v",
		"背景重連等待註冊結果失敗 err:%v",
	),
	KeyCenterRejectedStopBackgroundReconnect: i18n.New(
		"Center rejected the registration and stop_reconnect_on_reject is true, stopping the background reconnect",
		"Center 已拒绝注册且 stop_reconnect_on_reject 为真，停止后台重连",
		"Center 已拒絕註冊且 stop_reconnect_on_reject 為真，停止背景重連",
	),
	KeyLeftDegradedMode: i18n.New(
		"reconnected to Center and left degraded mode",
		"已重新连接中心节点，退出降级模式",
		"已重新連線中心節點，退出降級模式",
	),
	KeyLinkStatusChanged: i18n.New(
		"link status changed from:%s to:%s loss_rate:%v threshold:%v",
		"链路状态变化 from:%s to:%s loss_rate:%v threshold:%v",
		"鏈路狀態變化 from:%s to:%s loss_rate:%v threshold:%v",
	),
	KeyFakeIPSelfAddressSkipped: i18n.New(
		"this FAKE-IP equals the host's own address and was skipped ip:%s",
		"FAKE-IP 与本机地址相同，已跳过 ip:%s",
		"FAKE-IP 與本機位址相同，已略過 ip:%s",
	),
	KeyWriteToCenterFailed: i18n.New(
		"writing to Center failed err:%v",
		"写入中心节点失败 err:%v",
		"寫入中心節點失敗 err:%v",
	),
	KeyWriteQueueFull: i18n.New(
		"the write queue is full, dropping the Center connection to trigger a reconnect type:%s",
		"写队列已满，主动断开中心连接以触发重连 type:%s",
		"寫入佇列已滿，主動斷開中心連線以觸發重連 type:%s",
	),
	KeyCenterConnectionLost: i18n.New(
		"the Center connection dropped and a reconnect follows err:%v",
		"中心节点连接断开，将重连 err:%v",
		"中心節點連線斷開，即將重連 err:%v",
	),
	KeyRejectedStopAutoReconnect: i18n.New(
		"registration was rejected by Center and stop_reconnect_on_reject is true, so no automatic reconnect is started",
		"注册已被 Center 拒绝且 stop_reconnect_on_reject 为真，不启动自动重连",
		"註冊已被 Center 拒絕且 stop_reconnect_on_reject 為真，不啟動自動重連",
	),
	KeyRegisteredPayloadParseFailed: i18n.New(
		"parsing RegisteredPayload failed err:%v",
		"RegisteredPayload 解析失败 err:%v",
		"RegisteredPayload 解析失敗 err:%v",
	),
	KeyRegistrationRejected: i18n.New(
		"Center rejected the registration uuid:%s reason:%s",
		"中心节点拒绝注册 uuid:%s reason:%s",
		"中心節點拒絕註冊 uuid:%s reason:%s",
	),
	KeyRegistrationSucceeded: i18n.New(
		"registration succeeded uuid:%s",
		"注册成功 uuid:%s",
		"註冊成功 uuid:%s",
	),
	KeyTopoResponseParseFailed: i18n.New(
		"parsing TopoResponse failed, keeping the previous topology err:%v",
		"TopoResponse 解析失败，保留旧拓扑 err:%v",
		"TopoResponse 解析失敗，保留舊拓撲 err:%v",
	),
	KeyTopologyUpdated: i18n.New(
		"topology updated nodes:%d collect_client_info:%v",
		"拓扑已更新 nodes:%d collect_client_info:%v",
		"拓撲已更新 nodes:%d collect_client_info:%v",
	),
	KeySecretPushParseFailed: i18n.New(
		"parsing SecretPushPayload failed, keeping the previous secret err:%v",
		"SecretPushPayload 解析失败，保留旧 secret err:%v",
		"SecretPushPayload 解析失敗，保留舊 secret err:%v",
	),
	KeySecretPushInvalid: i18n.New(
		"the secret in SecretPushPayload is invalid and the push is ignored err:%v len:%d",
		"SecretPushPayload secret 非法，忽略推送 err:%v len:%d",
		"SecretPushPayload 的 secret 不合法，忽略推送 err:%v len:%d",
	),
	KeySharedSecretUpdated: i18n.New(
		"shared_secret updated version:%d",
		"shared_secret 已更新 version:%d",
		"shared_secret 已更新 version:%d",
	),
	KeyUnknownMessageType: i18n.New(
		"unknown message type type:%s",
		"未知消息类型 type:%s",
		"未知訊息類型 type:%s",
	),
	KeyCenterRejectedStopReconnect: i18n.New(
		"Center rejected the registration and stop_reconnect_on_reject is true, stopping the reconnect",
		"Center 已拒绝注册且 stop_reconnect_on_reject 为真，停止重连",
		"Center 已拒絕註冊且 stop_reconnect_on_reject 為真，停止重連",
	),
	KeyTryingReconnect: i18n.New(
		"trying to reconnect to Center",
		"尝试重连中心节点",
		"嘗試重新連線中心節點",
	),
	KeyBusinessPortListening: i18n.New(
		"business port listening addr:%s",
		"业务端口监听中 addr:%s",
		"業務埠監聽中 addr:%s",
	),
	KeyBusinessAcceptFailed: i18n.New(
		"business port Accept failed err:%v",
		"Accept 错误 err:%v",
		"Accept 錯誤 err:%v",
	),
	KeyHandshakeConcurrencyFull: i18n.New(
		"the concurrency cap of unauthenticated and bootstrap handshakes is full, rejecting the new connection",
		"未认证/引导握手并发已满，拒绝新连接",
		"未認證或引導握手的併發數已滿，拒絕新連線",
	),
	KeyBootstrapRateLimited: i18n.New(
		"[%s] bootstrap request over the rate limit, rejected",
		"[%s] 引导请求超过限速，拒绝",
		"[%s] 引導請求超過限速，拒絕",
	),
	KeyBootstrapDegradedMode: i18n.New(
		"[%s] bootstrapping the client in degraded mode with the cached topology",
		"[%s] 降级模式下引导客户端，使用缓存拓扑",
		"[%s] 在降級模式下引導用戶端，使用快取拓撲",
	),
	KeyBootstrapNoProbeItems: i18n.New(
		"[%s] no probe item is available",
		"[%s] 无可用探测项",
		"[%s] 沒有可用的探測項",
	),
	KeyBootstrapSendListFailed: i18n.New(
		"[%s] sending the probe item list failed err:%v",
		"[%s] 下发探测项列表失败 err:%v",
		"[%s] 下發探測項列表失敗 err:%v",
	),
	KeyBootstrapReadResultsFailed: i18n.New(
		"[%s] reading the probe results failed err:%v",
		"[%s] 读取探测结果失败 err:%v",
		"[%s] 讀取探測結果失敗 err:%v",
	),
	KeyBootstrapParseResultsFailed: i18n.New(
		"[%s] parsing the probe results failed err:%v",
		"[%s] 探测结果解析失败 err:%v",
		"[%s] 探測結果解析失敗 err:%v",
	),
	KeyBootstrapResultCountAbnormal: i18n.New(
		"[%s] unexpected number of probe results results:%d items:%d",
		"[%s] 探测结果数量异常 results:%d items:%d",
		"[%s] 探測結果數量異常 results:%d items:%d",
	),
	KeyBootstrapRTTRejected: i18n.New(
		"[%s] dropping an abnormal RTT code:%s rtt_ms:%d",
		"[%s] 忽略异常 RTT code:%s rtt_ms:%d",
		"[%s] 忽略異常 RTT code:%s rtt_ms:%d",
	),
	KeyBootstrapAllOverloaded: i18n.New(
		"[%s] every node is overloaded, falling back to the overloaded nodes",
		"[%s] 所有节点均已满载，回退使用 overloaded 节点",
		"[%s] 所有節點均已滿載，退回使用 overloaded 節點",
	),
	KeyBootstrapNoCandidate: i18n.New(
		"[%s] no candidate node is available",
		"[%s] 无可用候选节点",
		"[%s] 沒有可用的候選節點",
	),
	KeyBootstrapTokenFailed: i18n.New(
		"[%s] issuing the Token failed err:%v",
		"[%s] Token 生成失败 err:%v",
		"[%s] Token 產生失敗 err:%v",
	),
	KeyBootstrapSendRedirectFailed: i18n.New(
		"[%s] sending the Redirect command failed err:%v",
		"[%s] 下发 Redirect 命令失败 err:%v",
		"[%s] 下發 Redirect 命令失敗 err:%v",
	),
	KeyBootstrapCompleted: i18n.New(
		"[%s] bootstrap completed target:%s total_rtt_ms:%d",
		"[%s] 引导完成 target:%s total_rtt_ms:%d",
		"[%s] 引導完成 target:%s total_rtt_ms:%d",
	),
	KeyBusinessReadFirstPacketFailed: i18n.New(
		"[%s] reading the first packet failed err:%v",
		"[%s] 读取首包失败 err:%v",
		"[%s] 讀取首包失敗 err:%v",
	),
	KeyBusinessFirstPacketJSONFailed: i18n.New(
		"[%s] parsing the first packet JSON failed",
		"[%s] 首包 JSON 解析失败",
		"[%s] 首包 JSON 解析失敗",
	),
	KeyBusinessClientAddrFailed: i18n.New(
		"[%s] the client address cannot be resolved",
		"[%s] 无法解析客户端地址",
		"[%s] 無法解析用戶端位址",
	),
	KeyBusinessTokenRejected: i18n.New(
		"[%s] Token verification failed, dropping the connection",
		"[%s] Token 验签失败，丢弃连接",
		"[%s] Token 驗簽失敗，丟棄連線",
	),
	KeyBusinessForwardCapReached: i18n.New(
		"[%s] the established forwarding connections reached the cap, rejecting the connection",
		"[%s] 已建立转发连接数已达上限，拒绝连接",
		"[%s] 已建立的轉發連線數已達上限，拒絕連線",
	),
	KeyBusinessWriteAckFailed: i18n.New(
		"[%s] writing the verification acknowledgement failed err:%v",
		"[%s] 写入验签确认失败 err:%v",
		"[%s] 寫入驗簽確認失敗 err:%v",
	),
	KeyBusinessDialOriginFailed: i18n.New(
		"[%s] dialling the origin failed err:%v",
		"[%s] 连接源站失败 err:%v",
		"[%s] 連線源站失敗 err:%v",
	),
	KeyBusinessLocalAddrFailed: i18n.New(
		"[%s] the local address cannot be resolved",
		"[%s] 无法解析本端地址",
		"[%s] 無法解析本端位址",
	),
	KeyBusinessWriteCommSecretFailed: i18n.New(
		"[%s] writing the communication secret failed err:%v",
		"[%s] 写入通信密钥失败 err:%v",
		"[%s] 寫入通訊密鑰失敗 err:%v",
	),
	KeyBusinessWritePPv2Failed: i18n.New(
		"[%s] writing the PPv2 header failed err:%v",
		"[%s] 写入 PPv2 包头失败 err:%v",
		"[%s] 寫入 PPv2 標頭失敗 err:%v",
	),
	KeyBusinessReadServerAckFailed: i18n.New(
		"[%s] reading the Server ack frame failed err:%v (is the Server Agent version too old?)",
		"[%s] 读取 Server 确认帧失败 err:%v（Server Agent 版本过旧？）",
		"[%s] 讀取 Server 確認幀失敗 err:%v（Server Agent 版本過舊？）",
	),
	KeyBusinessServerAckParseFailed: i18n.New(
		"[%s] parsing the Server ack frame failed err:%v",
		"[%s] Server 确认帧解析失败 err:%v",
		"[%s] Server 確認幀解析失敗 err:%v",
	),
	KeyBusinessRelayEstablished: i18n.New(
		"[%s] L4 relay established origin:%s client_v:%s server_v:%s",
		"[%s] L4 转发通道已建立 origin:%s client_v:%s server_v:%s",
		"[%s] L4 轉發通道已建立 origin:%s client_v:%s server_v:%s",
	),
}

// init 登记本包的消息，使任何日志调用之前目录已经就绪
// init registers this package's messages so that the catalogue is ready before any logging call
func init() {
	i18n.Register(messages)
}
