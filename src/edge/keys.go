// 本文件声明 edge 包用到的消息键，各语言的文案放在 i18n/locales 下的语言文件里
// 键名按「包名.用途」命名，原文案里的 [%s] 远端地址前缀与 err:、addr: 一类字段标签保留在文案里，使日志仍能指出产生它的位置与上下文
//
// This file declares the message keys edge uses, and the copy of each language lives in the language files under i18n/locales
// A key is named package.purpose, while the [%s] remote-address prefix and the field labels such as err: and addr: of the original text stay inside it, so a log line still shows where it came from and what it refers to
package edge

import (
	"fmt"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

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

	// KeyCenterConnectionLost 是控制面连接读取失败时的键，参数为读取错误
	// 该消息在判定是否重连之前输出，因此文案不声明随后是否重连；重连的结果由紧随其后的 KeyRejectedStopAutoReconnect 或 KeyTryingReconnect 给出
	//
	// KeyCenterConnectionLost is the key used when reading the control-plane connection fails, whose argument is the read error
	// The message goes out before the reconnect decision, so its text states nothing about whether a reconnect follows; the outcome is reported by the KeyRejectedStopAutoReconnect or KeyTryingReconnect line that comes next
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

	// 下面五个键把 Center 下发的原因码渲染成本机语言的文本，不带参数
	// 跨进程只传码，语言因此各按各的 lang 决定；未知码由 KeyRejectReasonUnknown 原样回显，使新增码不需要本机同时升级
	//
	// The five keys below render the reason code delivered by the Center into text in the local language and carry no argument
	// Only the code crosses the process boundary, so each side follows its own lang, and an unknown code is echoed as it stands by KeyRejectReasonUnknown, which keeps a new code from requiring a simultaneous local upgrade

	// KeyRejectReasonPayloadInvalid 是原因码 payload_invalid 的文本
	// KeyRejectReasonPayloadInvalid is the text of the payload_invalid reason code
	KeyRejectReasonPayloadInvalid i18n.Key = "edge.reject_reason_payload_invalid"

	// KeyRejectReasonFieldsInvalid 是原因码 fields_invalid 的文本
	// KeyRejectReasonFieldsInvalid is the text of the fields_invalid reason code
	KeyRejectReasonFieldsInvalid i18n.Key = "edge.reject_reason_fields_invalid"

	// KeyRejectReasonRateLimited 是原因码 rate_limited 的文本
	// KeyRejectReasonRateLimited is the text of the rate_limited reason code
	KeyRejectReasonRateLimited i18n.Key = "edge.reject_reason_rate_limited"

	// KeyRejectReasonCapacityReached 是原因码 capacity_reached 的文本
	// KeyRejectReasonCapacityReached is the text of the capacity_reached reason code
	KeyRejectReasonCapacityReached i18n.Key = "edge.reject_reason_capacity_reached"

	// KeyRejectReasonUnknown 是收到本机不认识的原因码时的键，参数为该码
	// KeyRejectReasonUnknown is the key used for a reason code this build does not know, whose argument is that code
	KeyRejectReasonUnknown i18n.Key = "edge.reject_reason_unknown"

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

	// KeyBusinessListenFailed 是业务端口监听失败时的键，参数依次为监听地址与监听错误
	// 该错误由调用方按正常退出处理，因此文案说明的是监听这一步失败
	//
	// KeyBusinessListenFailed is the key used when the business port fails to listen, with the listen address and the listen error as arguments
	// The caller treats it as a normal exit, so the text names the listening step as the one that failed
	KeyBusinessListenFailed i18n.Key = "edge.business_listen_failed"

	// KeyFirstPacketShortPrefix 是判定阶段移交的已读前缀不足 4 字节时的键，不带参数
	// 该分支只在调用方移交了短于帧长度字段的前缀时成立
	//
	// KeyFirstPacketShortPrefix is the key used when the prefix handed over by the classification stage is shorter than 4 bytes, and it takes no argument
	// The branch needs a prefix shorter than the frame length field, which the caller is expected not to hand over
	KeyFirstPacketShortPrefix i18n.Key = "edge.first_packet_short_prefix"

	// KeyFirstPacketOverLimit 是业务首帧声明的帧长超过 util.MaxFrameSize 时的键，参数为该帧长
	// 该检查在读载荷之前完成，因此为帧体分配的缓冲区不超过 1 MB
	//
	// KeyFirstPacketOverLimit is the key used when the frame length declared by the business first frame exceeds util.MaxFrameSize, whose argument is that length
	// The check runs before the payload is read, so the buffer allocated for the frame body stays within 1 MB
	KeyFirstPacketOverLimit i18n.Key = "edge.first_packet_over_limit"

	// KeyFirstPacketPrefixOverflow 是已读前缀携带的数据超过帧长时的键，参数依次为前缀含有的帧体字节数与帧长
	// 该分支为纯防御代码：classifyConnection 移交的 4 字节与 16 字节两种形态都进不到这里
	//
	// KeyFirstPacketPrefixOverflow is the key used when the data carried by the read prefix exceeds the frame length, with the frame body byte count inside the prefix and the frame length as arguments
	// The branch is pure defensive code, because neither the 4-byte nor the 16-byte handoff of classifyConnection reaches it
	KeyFirstPacketPrefixOverflow i18n.Key = "edge.first_packet_prefix_overflow"

	// KeyRegistrationFlowNotStarted 是 Connect 尚未执行因而等待注册结果无门时的键，不带参数
	// Connect 未执行时 regCh 为 nil，等待无从建立
	//
	// KeyRegistrationFlowNotStarted is the key used when the registration result cannot be awaited because Connect has not run, and it takes no argument
	// regCh is nil while Connect has not run, which leaves nothing to wait on
	KeyRegistrationFlowNotStarted i18n.Key = "edge.registration_flow_not_started"

	// KeyRegistrationConnClosed 是连接在注册结果到达前关闭时的键，不带参数
	// KeyRegistrationConnClosed is the key used when the connection closes before the registration result arrives, and it takes no argument
	KeyRegistrationConnClosed i18n.Key = "edge.registration_conn_closed"

	// KeyRegistrationWaitTimeout 是等待注册结果超时时的键，不带参数
	// KeyRegistrationWaitTimeout is the key used when waiting for the registration result times out, and it takes no argument
	KeyRegistrationWaitTimeout i18n.Key = "edge.registration_wait_timeout"

	// KeyDegradedNoCacheDir 是重试耗尽且未配置拓扑缓存目录时的键，不带参数
	// 该分支在降级之前返回错误，因为无缓存目录即无从加载本地拓扑
	//
	// KeyDegradedNoCacheDir is the key used when the retries are exhausted and no topology cache directory is configured, and it takes no argument
	// The branch fails before degraded mode, since without a cache directory there is no local topology to load
	KeyDegradedNoCacheDir i18n.Key = "edge.degraded_no_cache_dir"

	// KeyDegradedNoUsableCache 是重试耗尽且本地缓存不可用时的键，参数为加载缓存错误
	// KeyDegradedNoUsableCache is the key used when the retries are exhausted and the local cache is unusable, whose argument is the cache loading error
	KeyDegradedNoUsableCache i18n.Key = "edge.degraded_no_usable_cache"

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

// errf 按消息键取当前语言的格式串并交给 fmt.Errorf
// 取的是未填充的格式串而不是已渲染的文本，使 %w 仍由 fmt.Errorf 处理，错误链得以保留
// 这些错误会经 %w 嵌入 main 的日志行，因此文案同样取自目录
//
// errf takes the format string of a message key in the current language and hands it to fmt.Errorf
// The unfilled format string is used rather than rendered text, so a %w verb is still handled by fmt.Errorf and the error chain survives
// main embeds these errors in a log line through %w, so their text comes from the catalogue as well
func errf(key i18n.Key, args ...any) error {
	return fmt.Errorf(i18n.T(key), args...)
}
