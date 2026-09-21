// 本文件登记 center 包用到的消息键与其三种语言文案
// 键名按「包名.用途」命名，每条对应中心节点的一条处理路径，日志因此仍能直接指出发生的是哪一步
// 在 init 中一次性登记，登记完成后目录只被读取
//
// This file registers the message keys center uses together with their text in three languages
// A key is named package.purpose and each one belongs to one handling path of the center node, so a log line still names the step that produced it
// Registration happens once in init and the catalogue is only read afterwards
package center

import (
	"fmt"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

const (
	// KeyWriteEdgeFailed 是向单个边缘节点连接写入消息失败时的键，参数依次为节点 UUID 与写入错误
	// 该写入由 EdgeRecord.writeLoop 执行，失败时该写循环返回
	//
	// KeyWriteEdgeFailed is the key used when writing a message to one edge connection fails, with the node UUID and the write error as arguments
	// The write runs in EdgeRecord.writeLoop, which returns once it fails
	KeyWriteEdgeFailed i18n.Key = "center.write_edge_failed"

	// KeyWSUpgradeFailed 是 WebSocket 升级失败时的键，参数为升级错误
	// KeyWSUpgradeFailed is the key used when the WebSocket upgrade fails, whose argument is the upgrade error
	KeyWSUpgradeFailed i18n.Key = "center.ws_upgrade_failed"

	// KeyOpenAPIEnabled 是 web_api_key 非空并完成开放 API 注册时的键，不带参数
	// KeyOpenAPIEnabled is the key used when web_api_key is not empty and the open API routes are registered, and it takes no argument
	KeyOpenAPIEnabled i18n.Key = "center.open_api_enabled"

	// KeyShuttingDown 是收到退出信号并关闭监听时的键，不带参数
	// KeyShuttingDown is the key used when the exit signal arrives and the listener is closed, and it takes no argument
	KeyShuttingDown i18n.Key = "center.shutting_down"

	// KeyStarted 是 HTTP 服务进入监听时的键，参数为监听地址
	// KeyStarted is the key used once the HTTP service starts listening, whose argument is the listen address
	KeyStarted i18n.Key = "center.started"

	// KeySecretGenerateFailed 是生成新的 shared_secret 失败时的键，参数为随机源错误
	// KeySecretGenerateFailed is the key used when generating a new shared_secret fails, whose argument is the random-source error
	KeySecretGenerateFailed i18n.Key = "center.secret_generate_failed"

	// KeyRegisterUUIDInvalid 是注册消息的 UUID 为空或超过 128 字节时的键，不带参数
	// KeyRegisterUUIDInvalid is the key used when the UUID of a registration is empty or longer than 128 bytes, and it takes no argument
	KeyRegisterUUIDInvalid i18n.Key = "center.register_uuid_invalid"

	// KeyRegisterIPInvalid 是注册消息的节点 IP 未通过地址校验时的键，参数为地址校验错误
	// 地址校验本身也走目录，因此这里的参数是已经本地化的文本，返回的错误链上两层都是当前语言
	//
	// KeyRegisterIPInvalid is the key used when the node IP of a registration fails the address check, whose argument is the address validation error
	// The address check reads the catalogue as well, so the argument is already localised text and both layers of the returned error chain follow the current language
	KeyRegisterIPInvalid i18n.Key = "center.register_ip_invalid"

	// KeyRegisterGroupTooLong 是注册消息的 group 超过 128 字节时的键，不带参数
	// KeyRegisterGroupTooLong is the key used when the group of a registration is longer than 128 bytes, and it takes no argument
	KeyRegisterGroupTooLong i18n.Key = "center.register_group_too_long"

	// KeyRegisterProbeModeUnknown 是注册消息的 probe_mode 不在枚举内时的键，参数为实际取值
	// KeyRegisterProbeModeUnknown is the key used when the probe_mode of a registration falls outside the enumeration, whose argument is the value read
	KeyRegisterProbeModeUnknown i18n.Key = "center.register_probe_mode_unknown"

	// KeyRegisterProbeProtoUnknown 是注册消息的 probe_proto 不在枚举内时的键，参数为实际取值
	// KeyRegisterProbeProtoUnknown is the key used when the probe_proto of a registration falls outside the enumeration, whose argument is the value read
	KeyRegisterProbeProtoUnknown i18n.Key = "center.register_probe_proto_unknown"

	// KeyRegisterBusinessPortOutOfRange 是注册消息的 business_port 超出 1-65535 时的键，参数为该端口
	// KeyRegisterBusinessPortOutOfRange is the key used when the business_port of a registration falls outside 1-65535, whose argument is that port
	KeyRegisterBusinessPortOutOfRange i18n.Key = "center.register_business_port_out_of_range"

	// KeyRegisterProbePortOutOfRange 是注册消息的 probe_port 超出 0-65535 时的键，参数为该端口
	// KeyRegisterProbePortOutOfRange is the key used when the probe_port of a registration falls outside 0-65535, whose argument is that port
	KeyRegisterProbePortOutOfRange i18n.Key = "center.register_probe_port_out_of_range"

	// KeyRegisterProbePortRequired 是 direct 或 mixed 模式下非 ICMP 探测未提供 probe_port 时的键，不带参数
	// KeyRegisterProbePortRequired is the key used when a direct or mixed mode registration carries no probe_port for a non-ICMP probe, and it takes no argument
	KeyRegisterProbePortRequired i18n.Key = "center.register_probe_port_required"

	// KeyRegisterVersionTooLong 是注册消息的 version 超过 64 字节时的键，不带参数
	// KeyRegisterVersionTooLong is the key used when the version of a registration is longer than 64 bytes, and it takes no argument
	KeyRegisterVersionTooLong i18n.Key = "center.register_version_too_long"

	// KeyFakeItemsCountExceeded 是 FAKE-IP 上报条数超过单节点上限时的键，参数依次为条数与上限
	// KeyFakeItemsCountExceeded is the key used when the reported FAKE-IP count exceeds the per-node cap, with the count and the cap as arguments
	KeyFakeItemsCountExceeded i18n.Key = "center.fake_items_count_exceeded"

	// KeyFakeItemIPInvalid 是 FAKE-IP 上报项的 IP 非法时的键，参数依次为下标与该 IP
	// KeyFakeItemIPInvalid is the key used when one FAKE-IP report carries an invalid IP, with the index and that IP as arguments
	KeyFakeItemIPInvalid i18n.Key = "center.fake_item_ip_invalid"

	// KeyFakeItemProtoInvalid 是 FAKE-IP 上报项的 proto 非法时的键，参数依次为下标与该 proto
	// KeyFakeItemProtoInvalid is the key used when one FAKE-IP report carries an invalid proto, with the index and that proto as arguments
	KeyFakeItemProtoInvalid i18n.Key = "center.fake_item_proto_invalid"

	// KeyFakeItemPortOutOfRange 是 FAKE-IP 上报项的 port 越界时的键，参数依次为下标与该端口
	// KeyFakeItemPortOutOfRange is the key used when the port of one FAKE-IP report falls outside its range, with the index and that port as arguments
	KeyFakeItemPortOutOfRange i18n.Key = "center.fake_item_port_out_of_range"

	// KeyFakeItemPortRequired 是 tcp 或 udp 的 FAKE-IP 上报项缺少 port 时的键，参数依次为下标与该 proto
	// KeyFakeItemPortRequired is the key used when a tcp or udp FAKE-IP report carries no port, with the index and that proto as arguments
	KeyFakeItemPortRequired i18n.Key = "center.fake_item_port_required"

	// KeyFakeItemWeightNegative 是 FAKE-IP 上报项的 weight 为负时的键，参数为下标
	// KeyFakeItemWeightNegative is the key used when the weight of one FAKE-IP report is negative, whose argument is the index
	KeyFakeItemWeightNegative i18n.Key = "center.fake_item_weight_negative"

	// KeyFakeItemRTTFallbackNegative 是 FAKE-IP 上报项的 rtt_fallback_ms 为负时的键，参数为下标
	// KeyFakeItemRTTFallbackNegative is the key used when the rtt_fallback_ms of one FAKE-IP report is negative, whose argument is the index
	KeyFakeItemRTTFallbackNegative i18n.Key = "center.fake_item_rtt_fallback_negative"

	// KeyNodeAddrEmpty 是节点地址为空时的键，不带参数
	// KeyNodeAddrEmpty is the key used when a node address is empty, and it takes no argument
	KeyNodeAddrEmpty i18n.Key = "center.node_addr_empty"

	// KeyNodeAddrBracketUnclosed 是 IPv6 方括号未闭合时的键，参数为该地址
	// KeyNodeAddrBracketUnclosed is the key used when the brackets of an IPv6 address do not close, whose argument is that address
	KeyNodeAddrBracketUnclosed i18n.Key = "center.node_addr_bracket_unclosed"

	// KeyNodeAddrUnspecifiedOrMulticast 是被拒的未指定或组播地址时的键，参数为该地址
	// KeyNodeAddrUnspecifiedOrMulticast is the key used for a rejected unspecified or multicast address, whose argument is that address
	KeyNodeAddrUnspecifiedOrMulticast i18n.Key = "center.node_addr_unspecified_or_multicast"

	// KeyNodeAddrNotIPOrHostname 是取值既不是合法 IP 也不是合法域名时的键，参数为该地址
	// KeyNodeAddrNotIPOrHostname is the key used when a value is neither a valid IP nor a valid host name, whose argument is that value
	KeyNodeAddrNotIPOrHostname i18n.Key = "center.node_addr_not_ip_or_hostname"

	// KeySecretRotated 是完成一次 shared_secret 轮换时的键，参数为新的版本号
	// KeySecretRotated is the key used after one shared_secret rotation, whose argument is the new version
	KeySecretRotated i18n.Key = "center.secret_rotated"

	// KeyMessageDecodeFailed 是控制连接上收到的帧不是合法信封 JSON 时的键，参数为解析错误
	// KeyMessageDecodeFailed is the key used when a frame on a control connection is not a valid envelope JSON, whose argument is the decoding error
	KeyMessageDecodeFailed i18n.Key = "center.message_decode_failed"

	// KeyUnknownMessageType 是信封类型不在已处理范围内时的键，参数为收到的类型名
	// KeyUnknownMessageType is the key used when the envelope type is outside the handled set, whose argument is the type name received
	KeyUnknownMessageType i18n.Key = "center.unknown_message_type"

	// KeyRegisterRejectFailed 是向未登记连接直接下发注册拒绝失败时的键，参数为写入错误
	// KeyRegisterRejectFailed is the key used when sending a registration rejection directly to an unregistered connection fails, whose argument is the write error
	KeyRegisterRejectFailed i18n.Key = "center.register_reject_failed"

	// KeyRegisterPayloadDecodeFailed 是注册消息体不是合法 JSON 时的键，参数为解析错误
	// KeyRegisterPayloadDecodeFailed is the key used when the register payload is not valid JSON, whose argument is the decoding error
	KeyRegisterPayloadDecodeFailed i18n.Key = "center.register_payload_decode_failed"

	// KeyRegisterValidateFailed 是注册字段边界校验失败时的键，参数依次为节点 UUID 与校验错误
	// KeyRegisterValidateFailed is the key used when the field range check of a registration fails, with the node UUID and the validation error as arguments
	KeyRegisterValidateFailed i18n.Key = "center.register_validate_failed"

	// KeyRegisterRateLimited 是单个 UUID 的注册频率超过限速时的键，参数为节点 UUID
	// KeyRegisterRateLimited is the key used when one UUID exceeds the registration rate limit, whose argument is the node UUID
	KeyRegisterRateLimited i18n.Key = "center.register_rate_limited"

	// KeyRegisterReusedConnection 是同一连接重复注册同一 UUID 并原地复用该连接时的键，参数为节点 UUID
	// KeyRegisterReusedConnection is the key used when one connection registers the same UUID again and that connection is reused in place, whose argument is the node UUID
	KeyRegisterReusedConnection i18n.Key = "center.register_reused_connection"

	// KeyRegisterEdgeLimitReached 是在线 Edge 数量已达上限时的键，参数依次为节点 UUID 与当前在线数
	// KeyRegisterEdgeLimitReached is the key used when the number of online edges has reached the limit, with the node UUID and the current online count as arguments
	KeyRegisterEdgeLimitReached i18n.Key = "center.register_edge_limit_reached"

	// KeyRegisterSucceeded 是注册成功并完成 secret 推送时的键，参数依次为节点 UUID、节点 IP 与分组
	// KeyRegisterSucceeded is the key used once a registration succeeds and the secret has been pushed, with the node UUID, the node IP and the group as arguments
	KeyRegisterSucceeded i18n.Key = "center.register_succeeded"

	// KeyRTTReportDecodeFailed 是 rtt_report 消息体不是合法 JSON 时的键，参数为解析错误
	// KeyRTTReportDecodeFailed is the key used when the rtt_report payload is not valid JSON, whose argument is the decoding error
	KeyRTTReportDecodeFailed i18n.Key = "center.rtt_report_decode_failed"

	// KeyRTTReportOriginRTTMsOutOfRange 是上报的源站 RTT 超出取值范围时的键，参数为上报的 RTT 毫秒数
	// KeyRTTReportOriginRTTMsOutOfRange is the key used when the reported origin RTT falls outside its range, whose argument is the reported RTT in milliseconds
	KeyRTTReportOriginRTTMsOutOfRange i18n.Key = "center.rtt_report_origin_rtt_ms_out_of_range"

	// KeyRTTReportFakeCountExceeded 是上报的 f2n 条数超过上限时的键，参数依次为条数与上限
	// KeyRTTReportFakeCountExceeded is the key used when the reported f2n entry count exceeds the cap, with the count and the cap as arguments
	KeyRTTReportFakeCountExceeded i18n.Key = "center.rtt_report_fake_count_exceeded"

	// KeyRTTReportFakeKeysCapped 是 record 内累计的 FAKE-IP 延迟键数已达上限时的键，参数依次为节点 UUID、上限与本次被丢弃的键数
	// 该键只在确实丢弃了新键时输出，且每条消息至多一次，因此不会随丢弃次数刷屏
	//
	// KeyRTTReportFakeKeysCapped is the key used when the FAKE-IP latency keys accumulated in one record have reached the cap, with the node UUID, the cap and the number of keys this message dropped as arguments
	// It is emitted only when new keys were actually dropped, at most once per message, so it cannot flood the log with the number of drops
	KeyRTTReportFakeKeysCapped i18n.Key = "center.rtt_report_fake_keys_capped"

	// KeyRTTReportFakeIPInvalid 是上报的 f2n 条目携带非法 IP 时的键，参数为该 IP
	// KeyRTTReportFakeIPInvalid is the key used when a reported f2n entry carries an invalid IP, whose argument is that IP
	KeyRTTReportFakeIPInvalid i18n.Key = "center.rtt_report_fake_ip_invalid"

	// KeyRTTReportFakeRTTMsOutOfRange 是上报的 f2n 延迟超出取值范围时的键，参数依次为该条目 IP 与延迟毫秒数
	// KeyRTTReportFakeRTTMsOutOfRange is the key used when a reported f2n latency falls outside its range, with the entry IP and the latency in milliseconds as arguments
	KeyRTTReportFakeRTTMsOutOfRange i18n.Key = "center.rtt_report_fake_rtt_ms_out_of_range"

	// KeyRTTReportUnknownLinkStatus 是上报的链路状态不在 protocol 定义范围内时的键，参数为上报的取值
	// KeyRTTReportUnknownLinkStatus is the key used when the reported link status is outside the values defined by protocol, whose argument is the reported value
	KeyRTTReportUnknownLinkStatus i18n.Key = "center.rtt_report_unknown_link_status"

	// KeyFakeUpdateDecodeFailed 是 fake_update 消息体不是合法 JSON 时的键，参数为解析错误
	// KeyFakeUpdateDecodeFailed is the key used when the fake_update payload is not valid JSON, whose argument is the decoding error
	KeyFakeUpdateDecodeFailed i18n.Key = "center.fake_update_decode_failed"

	// KeyFakeUpdateValidateFailed 是 fake_update 列表未通过校验时的键，参数为校验错误
	// KeyFakeUpdateValidateFailed is the key used when the fake_update list fails validation, whose argument is the validation error
	KeyFakeUpdateValidateFailed i18n.Key = "center.fake_update_validate_failed"

	// KeyFakeListUpdated 是 FAKE-IP 列表写入 record 之后时的键，参数依次为节点 UUID 与本次条数
	// KeyFakeListUpdated is the key used after a FAKE-IP list is stored in the record, with the node UUID and the entry count as arguments
	KeyFakeListUpdated i18n.Key = "center.fake_list_updated"

	// KeyVersionReportDecodeFailed 是 version_report 消息体不是合法 JSON 时的键，参数为解析错误
	// KeyVersionReportDecodeFailed is the key used when the version_report payload is not valid JSON, whose argument is the decoding error
	KeyVersionReportDecodeFailed i18n.Key = "center.version_report_decode_failed"

	// KeyVersionReportApplied 是版本上报写入 record 之后时的键，参数依次为节点 UUID、客户端条数与上报中的 Server Agent 记录
	// KeyVersionReportApplied is the key used after a version report is stored in the record, with the node UUID, the client record count and the Server Agent record of that report as arguments
	KeyVersionReportApplied i18n.Key = "center.version_report_applied"

	// KeyEdgeOffline 是连接断开并清理注册信息时的键，参数为节点 UUID
	// KeyEdgeOffline is the key used when a connection drops and its registration is cleaned up, whose argument is the node UUID
	KeyEdgeOffline i18n.Key = "center.edge_offline"

	// KeyWriteQueueFull 是该连接的写队列已满并因此关闭连接时的键，参数依次为节点 UUID 与消息类型
	// KeyWriteQueueFull is the key used when the write queue of a connection is full and the connection is closed for that reason, with the node UUID and the message type as arguments
	KeyWriteQueueFull i18n.Key = "center.write_queue_full"
)

// messages 保存本包的消息，键为 i18n.Key，值为三种语言下的文案
// messages holds this package's messages, keyed by i18n.Key with the text in three languages
var messages = map[i18n.Key]i18n.Entry{
	KeyWriteEdgeFailed: i18n.New(
		"writing to the edge node failed uuid:%s err:%v",
		"写入边缘节点失败 uuid:%s err:%v",
		"寫入邊緣節點失敗 uuid:%s err:%v",
	),
	KeyWSUpgradeFailed: i18n.New(
		"WebSocket upgrade failed err:%v",
		"WebSocket 升级失败 err:%v",
		"WebSocket 升級失敗 err:%v",
	),
	KeyOpenAPIEnabled: i18n.New(
		"open API is enabled (Bearer authentication)",
		"开放 API 已开启（Bearer 鉴权）",
		"開放 API 已開啟（Bearer 驗證）",
	),
	KeyShuttingDown: i18n.New(
		"center node is shutting down...",
		"中心节点正在关闭...",
		"中心節點正在關閉...",
	),
	KeyStarted: i18n.New(
		"center node started addr:%s",
		"中心节点启动 addr:%s",
		"中心節點啟動 addr:%s",
	),
	KeySecretGenerateFailed: i18n.New(
		"generating the secret failed: %w",
		"生成 secret 失败: %w",
		"產生 secret 失敗: %w",
	),
	KeyRegisterUUIDInvalid: i18n.New(
		"the uuid is empty or too long",
		"uuid 为空或超长",
		"uuid 為空或超長",
	),
	KeyRegisterIPInvalid: i18n.New(
		"the ip is invalid: %w",
		"ip 非法: %w",
		"ip 不合法: %w",
	),
	KeyRegisterGroupTooLong: i18n.New(
		"the group is too long",
		"group 超长",
		"group 超長",
	),
	KeyRegisterProbeModeUnknown: i18n.New(
		"unknown probe mode: %s",
		"未知探测模式: %s",
		"未知探測模式: %s",
	),
	KeyRegisterProbeProtoUnknown: i18n.New(
		"unknown probe protocol: %s",
		"未知探测协议: %s",
		"未知探測協定: %s",
	),
	KeyRegisterBusinessPortOutOfRange: i18n.New(
		"business_port out of range: %d",
		"business_port 超出范围: %d",
		"business_port 超出範圍: %d",
	),
	KeyRegisterProbePortOutOfRange: i18n.New(
		"probe_port out of range: %d",
		"probe_port 超出范围: %d",
		"probe_port 超出範圍: %d",
	),
	KeyRegisterProbePortRequired: i18n.New(
		"direct or mixed mode with a non-ICMP probe has to carry probe_port",
		"direct/mixed 模式且非 ICMP 探测必须提供 probe_port",
		"direct/mixed 模式且非 ICMP 探測必須提供 probe_port",
	),
	KeyRegisterVersionTooLong: i18n.New(
		"the version is too long",
		"version 超长",
		"version 超長",
	),
	KeyFakeItemsCountExceeded: i18n.New(
		"fake_items over the count limit: %d > %d",
		"fake_items 数量超限: %d > %d",
		"fake_items 數量超限: %d > %d",
	),
	KeyFakeItemIPInvalid: i18n.New(
		"fake_items[%d].ip is invalid: %s",
		"fake_items[%d].ip 非法: %s",
		"fake_items[%d].ip 不合法: %s",
	),
	KeyFakeItemProtoInvalid: i18n.New(
		"fake_items[%d].proto is invalid: %s",
		"fake_items[%d].proto 非法: %s",
		"fake_items[%d].proto 不合法: %s",
	),
	KeyFakeItemPortOutOfRange: i18n.New(
		"fake_items[%d].port out of range: %d",
		"fake_items[%d].port 超出范围: %d",
		"fake_items[%d].port 超出範圍: %d",
	),
	KeyFakeItemPortRequired: i18n.New(
		"fake_items[%d] (%s) has to carry a port",
		"fake_items[%d]（%s）必须提供 port",
		"fake_items[%d]（%s）必須提供 port",
	),
	KeyFakeItemWeightNegative: i18n.New(
		"fake_items[%d].weight has to be >= 0",
		"fake_items[%d].weight 必须 ≥ 0",
		"fake_items[%d].weight 必須 ≥ 0",
	),
	KeyFakeItemRTTFallbackNegative: i18n.New(
		"fake_items[%d].rtt_fallback_ms has to be >= 0",
		"fake_items[%d].rtt_fallback_ms 必须 ≥ 0",
		"fake_items[%d].rtt_fallback_ms 必須 ≥ 0",
	),
	KeyNodeAddrEmpty: i18n.New(
		"the address is empty",
		"地址为空",
		"位址為空",
	),
	KeyNodeAddrBracketUnclosed: i18n.New(
		"the IPv6 bracket is not closed: %s",
		"IPv6 方括号未闭合: %s",
		"IPv6 方括號未閉合: %s",
	),
	KeyNodeAddrUnspecifiedOrMulticast: i18n.New(
		"an unspecified or multicast address is not allowed: %s",
		"不允许使用未指定/组播地址: %s",
		"不允許使用未指定或群播位址: %s",
	),
	KeyNodeAddrNotIPOrHostname: i18n.New(
		"neither a valid IP nor a host name: %s",
		"不是合法 IP 或域名: %s",
		"不是合法 IP 或網域名稱: %s",
	),
	KeySecretRotated: i18n.New(
		"shared_secret rotated version:%d",
		"shared_secret 已轮转 version:%d",
		"shared_secret 已輪換 version:%d",
	),
	KeyMessageDecodeFailed: i18n.New(
		"message decoding failed err:%v",
		"消息解析失败 err:%v",
		"訊息解析失敗 err:%v",
	),
	KeyUnknownMessageType: i18n.New(
		"unknown message type type:%s",
		"未知消息类型 type:%s",
		"未知訊息類型 type:%s",
	),
	KeyRegisterRejectFailed: i18n.New(
		"sending the registration rejection failed err:%v",
		"下发注册拒绝失败 err:%v",
		"下發註冊拒絕失敗 err:%v",
	),
	KeyRegisterPayloadDecodeFailed: i18n.New(
		"edge registration failed: decoding the register message failed err:%v",
		"边缘节点注册失败：注册消息解析失败 err:%v",
		"邊緣節點註冊失敗：註冊訊息解析失敗 err:%v",
	),
	KeyRegisterValidateFailed: i18n.New(
		"edge registration validation failed uuid:%s err:%v",
		"边缘节点注册校验失败 uuid:%s err:%v",
		"邊緣節點註冊校驗失敗 uuid:%s err:%v",
	),
	KeyRegisterRateLimited: i18n.New(
		"edge registration failed: the registration rate limit was exceeded uuid:%s",
		"边缘节点注册失败：注册频率超限 uuid:%s",
		"邊緣節點註冊失敗：註冊頻率超限 uuid:%s",
	),
	KeyRegisterReusedConnection: i18n.New(
		"the edge registered again on the same connection, which is reused in place uuid:%s",
		"边缘节点在同一连接上重复注册，已原地复用该连接 uuid:%s",
		"邊緣節點在同一連線上重複註冊，已原地沿用該連線 uuid:%s",
	),
	KeyRegisterEdgeLimitReached: i18n.New(
		"edge registration failed: the online edge limit has been reached uuid:%s count:%d",
		"边缘节点注册失败：在线 Edge 数量已达上限 uuid:%s count:%d",
		"邊緣節點註冊失敗：線上 Edge 數量已達上限 uuid:%s count:%d",
	),
	KeyRegisterSucceeded: i18n.New(
		"edge registered uuid:%s ip:%s group:%s",
		"边缘节点注册成功 uuid:%s ip:%s group:%s",
		"邊緣節點註冊成功 uuid:%s ip:%s group:%s",
	),
	KeyRTTReportDecodeFailed: i18n.New(
		"decoding the RTTReport failed err:%v",
		"RTTReport 解析失败 err:%v",
		"RTTReport 解析失敗 err:%v",
	),
	KeyRTTReportOriginRTTMsOutOfRange: i18n.New(
		"the origin RTT of the RTTReport is out of range, dropping the whole report rtt_ms:%d",
		"RTTReport 的源站 RTT 超出范围，整条丢弃 rtt_ms:%d",
		"RTTReport 的源站 RTT 超出範圍，整筆丟棄 rtt_ms:%d",
	),
	KeyRTTReportFakeCountExceeded: i18n.New(
		"the RTTReport carries too many f2n entries, dropping the whole report count:%d limit:%d",
		"RTTReport 的 f2n 条数超限，整条丢弃 count:%d limit:%d",
		"RTTReport 的 f2n 筆數超限，整筆丟棄 count:%d limit:%d",
	),
	KeyRTTReportFakeKeysCapped: i18n.New(
		"the f2n keys kept for this node have reached the cap, only existing keys are updated uuid:%s limit:%d dropped:%d",
		"该节点累计的 f2n 键数已达上限，只更新已有键 uuid:%s limit:%d dropped:%d",
		"該節點累計的 f2n 鍵數已達上限，只更新既有鍵 uuid:%s limit:%d dropped:%d",
	),
	KeyRTTReportFakeIPInvalid: i18n.New(
		"the RTTReport carries an invalid f2n IP, dropping the whole report ip:%s",
		"RTTReport 含非法 f2n IP，整条丢弃 ip:%s",
		"RTTReport 含非法 f2n IP，整筆丟棄 ip:%s",
	),
	KeyRTTReportFakeRTTMsOutOfRange: i18n.New(
		"the RTTReport carries an out-of-range f2n latency, dropping the whole report ip:%s rtt_ms:%d",
		"RTTReport 含超出范围的 f2n 延迟，整条丢弃 ip:%s rtt_ms:%d",
		"RTTReport 含超出範圍的 f2n 延遲，整筆丟棄 ip:%s rtt_ms:%d",
	),
	KeyRTTReportUnknownLinkStatus: i18n.New(
		"the RTTReport carries an unknown link status, ignoring that field link_status:%s",
		"RTTReport 含未知链路状态，忽略该字段 link_status:%s",
		"RTTReport 含未知鏈路狀態，忽略該欄位 link_status:%s",
	),
	KeyFakeUpdateDecodeFailed: i18n.New(
		"decoding the FakeUpdate failed err:%v",
		"FakeUpdate 解析失败 err:%v",
		"FakeUpdate 解析失敗 err:%v",
	),
	KeyFakeUpdateValidateFailed: i18n.New(
		"the FakeUpdate failed validation, dropping the whole update err:%v",
		"FakeUpdate 校验失败，整条丢弃 err:%v",
		"FakeUpdate 校驗失敗，整筆丟棄 err:%v",
	),
	KeyFakeListUpdated: i18n.New(
		"FAKE-IP list updated uuid:%s count:%d",
		"FAKE-IP 列表更新 uuid:%s count:%d",
		"FAKE-IP 列表更新 uuid:%s count:%d",
	),
	KeyVersionReportDecodeFailed: i18n.New(
		"decoding the VersionReport failed err:%v",
		"VersionReport 解析失败 err:%v",
		"VersionReport 解析失敗 err:%v",
	),
	KeyVersionReportApplied: i18n.New(
		"version report applied uuid:%s clients:%d server_uuid:%v",
		"版本上报 uuid:%s clients:%d server_uuid:%v",
		"版本上報 uuid:%s clients:%d server_uuid:%v",
	),
	KeyEdgeOffline: i18n.New(
		"edge went offline uuid:%s",
		"边缘节点下线 uuid:%s",
		"邊緣節點下線 uuid:%s",
	),
	KeyWriteQueueFull: i18n.New(
		"the write queue of the edge node is full, closing the connection to trigger a reconnect uuid:%s type:%s",
		"边缘节点写队列已满，关闭连接以触发重连 uuid:%s type:%s",
		"邊緣節點寫入佇列已滿，關閉連線以觸發重連 uuid:%s type:%s",
	),
}

// errf 按消息键取当前语言的格式串并交给 fmt.Errorf
// 取的是未填充的格式串而不是已渲染的文本，使 %w 仍由 fmt.Errorf 处理，错误链得以保留
// 这些错误会被 center 的日志行经 %v 嵌入，因此文案同样取自目录
//
// errf takes the format string of a message key in the current language and hands it to fmt.Errorf
// The unfilled format string is used rather than rendered text, so a %w verb is still handled by fmt.Errorf and the error chain survives
// A log line of center embeds these errors through %v, so their text comes from the catalogue as well
func errf(key i18n.Key, args ...any) error {
	return fmt.Errorf(i18n.T(key), args...)
}

// init 登记本包的消息，使任何日志调用之前目录已经就绪
// init registers this package's messages so that the catalogue is ready before any logging call
func init() {
	i18n.Register(messages)
}
