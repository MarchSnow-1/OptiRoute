// 本文件声明 config 包用到的消息键，各语言的文案放在 i18n/locales 下的语言文件里
// 覆盖配置加载失败与 Validate 各分支的报错文案，使启动期的错误文本与日志一样随 self.lang 或 --lang 切换
// 同构的报错合并为共用键：字段名作为参数传入，例如 self.connect_timeout_ms 与 self.max_edges 共用「%s 必须 > 0，当前 %d」
// 文案是未填充的格式串，由 config.errf 交给 fmt.Errorf，使 %w 仍能保留错误链
//
// This file declares the message keys config uses, and the copy of each language lives in the language files under i18n/locales
// It covers the failures of configuration loading and the messages of the Validate arms, so that start-up errors follow self.lang or --lang just like logs do
// Structurally identical messages share one key with the field name passed as an argument, so self.connect_timeout_ms and self.max_edges both use "%s must be > 0, current %d"
// A message is an unfilled format string handed to fmt.Errorf by config.errf, which lets a %w verb keep the error chain intact
package config

import "github.com/MarchSnow-1/OptiRoute/i18n"

const (
	// KeyConfigReadFailed 是配置文件读取失败时的键，参数依次为路径与底层错误
	// KeyConfigReadFailed is the key used when the configuration file cannot be read, with the path and the underlying error as arguments
	KeyConfigReadFailed i18n.Key = "config.read_failed"

	// KeyConfigBase64Failed 是 base64 配置串解码失败时的键，参数为底层错误
	// KeyConfigBase64Failed is the key used when the base64 configuration string fails to decode, whose argument is the underlying error
	KeyConfigBase64Failed i18n.Key = "config.base64_failed"

	// KeyJSONParseFailed 是配置 JSON 解析失败时的键，参数为底层错误
	// KeyJSONParseFailed is the key used when the configuration JSON fails to decode, whose argument is the underlying error
	KeyJSONParseFailed i18n.Key = "config.json_parse_failed"

	// KeyJSONMissingSelf 是配置缺少 self 段时的键
	// KeyJSONMissingSelf is the key used when the configuration carries no self section
	KeyJSONMissingSelf i18n.Key = "config.json_missing_self"

	// KeyRoleRequired 是 self.role 为空时的键
	// KeyRoleRequired is the key used when self.role is empty
	KeyRoleRequired i18n.Key = "config.role_required"

	// KeyRoleUnknown 是 self.role 不在四种角色之内时的键，参数为实际取值
	// KeyRoleUnknown is the key used when self.role names none of the four roles, whose argument is the value read
	KeyRoleUnknown i18n.Key = "config.role_unknown"

	// KeyLogLevelUnknown 是 log_level 不在枚举内时的键，参数依次为实际取值与允许取值
	// KeyLogLevelUnknown is the key used when log_level falls outside the enumeration, with the value read and the accepted values as arguments
	KeyLogLevelUnknown i18n.Key = "config.log_level_unknown"

	// KeyPortOutOfRange 是端口越界时的键，参数依次为字段名与实际端口
	// KeyPortOutOfRange is the key used when a port falls outside its range, with the field name and the port as arguments
	KeyPortOutOfRange i18n.Key = "config.port_out_of_range"

	// KeySecretRequired 是密钥为空时的键，参数依次为角色名与字段名
	// KeySecretRequired is the key used when a secret is empty, with the role and the field name as arguments
	KeySecretRequired i18n.Key = "config.secret_required"

	// KeySecretLength 是密钥长度不是 32 字节时的键，参数依次为字段名与实际字节数
	// KeySecretLength is the key used when a secret is not 32 bytes long, with the field name and the byte count as arguments
	KeySecretLength i18n.Key = "config.secret_length"

	// KeyAddrBracketUnclosed 是地址以左方括号开头却没有右方括号时的键，参数依次为字段名与实际取值
	// KeyAddrBracketUnclosed is the key used when an address opens a bracket without closing it, with the field name and the value as arguments
	KeyAddrBracketUnclosed i18n.Key = "config.addr_bracket_unclosed"

	// KeyAddrBracketNotIPv6 是方括号内不是合法 IPv6 时的键，参数依次为字段名与实际取值
	// KeyAddrBracketNotIPv6 is the key used when the brackets hold no valid IPv6 address, with the field name and the value as arguments
	KeyAddrBracketNotIPv6 i18n.Key = "config.addr_bracket_not_ipv6"

	// KeyAddrIPv6NeedsBrackets 是 IPv6 地址缺少方括号时的键，参数依次为字段名与实际取值
	// KeyAddrIPv6NeedsBrackets is the key used when an IPv6 address carries no brackets, with the field name and the value as arguments
	KeyAddrIPv6NeedsBrackets i18n.Key = "config.addr_ipv6_needs_brackets"

	// KeyFieldMustBePositive 是数值字段要求大于 0 时的共用键，参数依次为字段名与实际取值
	// KeyFieldMustBePositive is the shared key for a numeric field that has to be greater than 0, with the field name and the value as arguments
	KeyFieldMustBePositive i18n.Key = "config.field_must_be_positive"

	// KeyFieldMustBeNonNegative 是整数段要求不小于 0 时的共用键，参数依次为字段名与实际取值
	// KeyFieldMustBeNonNegative is the shared key for an integer field that may not go below 0, with the field name and the value as arguments
	KeyFieldMustBeNonNegative i18n.Key = "config.field_must_be_non_negative"

	// KeyFieldMustBeNonNegativeFloat 是浮点字段要求不小于 0 时的共用键，参数依次为字段名与实际取值
	// KeyFieldMustBeNonNegativeFloat is the shared key for a floating-point field that may not go below 0, with the field name and the value as arguments
	KeyFieldMustBeNonNegativeFloat i18n.Key = "config.field_must_be_non_negative_float"

	// KeyFieldBetweenZeroAndOne 是比例字段要求落在 0 到 1 之间时的共用键，参数依次为字段名与实际取值
	// KeyFieldBetweenZeroAndOne is the shared key for a ratio field that has to fall between 0 and 1, with the field name and the value as arguments
	KeyFieldBetweenZeroAndOne i18n.Key = "config.field_between_zero_and_one"

	// KeyBWRatiosInverted 是告警比例大于过载比例时的键
	// KeyBWRatiosInverted is the key used when the warning ratio exceeds the overload ratio
	KeyBWRatiosInverted i18n.Key = "config.bw_ratios_inverted"

	// KeyCenterListenPortRequired 是 center 角色缺少 self.listen_port 时的键
	// KeyCenterListenPortRequired is the key used when the center role carries no self.listen_port
	KeyCenterListenPortRequired i18n.Key = "config.center_listen_port_required"

	// KeyEdgeUUIDRequired 是 edge 角色缺少 self.uuid 时的键
	// KeyEdgeUUIDRequired is the key used when the edge role carries no self.uuid
	KeyEdgeUUIDRequired i18n.Key = "config.edge_uuid_required"

	// KeyEdgeAddrRequired 是 edge 角色缺少 self.addr 时的键
	// KeyEdgeAddrRequired is the key used when the edge role carries no self.addr
	KeyEdgeAddrRequired i18n.Key = "config.edge_addr_required"

	// KeyEdgeCenterRequired 是 edge 角色缺少中心地址或端口时的键
	// KeyEdgeCenterRequired is the key used when the edge role carries no center address or port
	KeyEdgeCenterRequired i18n.Key = "config.edge_center_required"

	// KeyEdgeOriginRequired 是 edge 角色缺少源站地址或端口时的键
	// KeyEdgeOriginRequired is the key used when the edge role carries no origin address or port
	KeyEdgeOriginRequired i18n.Key = "config.edge_origin_required"

	// KeyEdgeProbePortRequired 是 edge 角色在非 icmp-only 模式下缺少 self.probe_port 时的键，参数依次为探测模式与探测协议
	// KeyEdgeProbePortRequired is the key used when the edge role carries no self.probe_port outside icmp-only mode, with the probe mode and the probe protocol as arguments
	KeyEdgeProbePortRequired i18n.Key = "config.edge_probe_port_required"

	// KeyEdgeSecretsMustDiffer 是 center_secret 与 comm_secret 取同一值时的键
	// KeyEdgeSecretsMustDiffer is the key used when center_secret and comm_secret hold the same value
	KeyEdgeSecretsMustDiffer i18n.Key = "config.edge_secrets_must_differ"

	// KeyProbeModeUnknown 是 probe_mode 不在枚举内时的键，参数为实际取值
	// KeyProbeModeUnknown is the key used when probe_mode falls outside the enumeration, whose argument is the value read
	KeyProbeModeUnknown i18n.Key = "config.probe_mode_unknown"

	// KeyProbeProtoUnknown 是 probe_proto 不在枚举内时的键，参数为实际取值
	// KeyProbeProtoUnknown is the key used when probe_proto falls outside the enumeration, whose argument is the value read
	KeyProbeProtoUnknown i18n.Key = "config.probe_proto_unknown"

	// KeyFakeIPMaxCountTooSmall 是 fake_ip_max_count 小于 1 时的键，参数为实际取值
	// KeyFakeIPMaxCountTooSmall is the key used when fake_ip_max_count is below 1, whose argument is the value read
	KeyFakeIPMaxCountTooSmall i18n.Key = "config.fake_ip_max_count_too_small"

	// KeyFakeIPMaxCountTooLarge 是 fake_ip_max_count 超出上限时的键，参数依次为实际取值与上限
	// KeyFakeIPMaxCountTooLarge is the key used when fake_ip_max_count exceeds its cap, with the value read and the cap as arguments
	KeyFakeIPMaxCountTooLarge i18n.Key = "config.fake_ip_max_count_too_large"

	// KeyFakeIPInvalid 是 fake_ips 某项 IP 非法时的键，参数依次为下标与实际取值
	// KeyFakeIPInvalid is the key used when one fake_ips entry carries an invalid IP, with the index and the value as arguments
	KeyFakeIPInvalid i18n.Key = "config.fake_ip_invalid"

	// KeyFakeIPProtoInvalid 是 fake_ips 某项 proto 非法时的键，参数依次为下标与实际取值
	// KeyFakeIPProtoInvalid is the key used when one fake_ips entry carries an invalid proto, with the index and the value as arguments
	KeyFakeIPProtoInvalid i18n.Key = "config.fake_ip_proto_invalid"

	// KeyFakeIPWeightNegative 是 fake_ips 某项 weight 为负时的键，参数依次为下标与实际取值
	// KeyFakeIPWeightNegative is the key used when one fake_ips entry carries a negative weight, with the index and the value as arguments
	KeyFakeIPWeightNegative i18n.Key = "config.fake_ip_weight_negative"

	// KeyFakeIPPortRequired 是 fake_ips 某项缺少 port 时的键，参数依次为下标与该项的 proto
	// KeyFakeIPPortRequired is the key used when one fake_ips entry carries no port, with the index and that entry's proto as arguments
	KeyFakeIPPortRequired i18n.Key = "config.fake_ip_port_required"

	// KeyFakeIPPortRange 是 fake_ips 某项 port 越界时的键，参数依次为下标与实际取值
	// KeyFakeIPPortRange is the key used when one fake_ips entry carries an out-of-range port, with the index and the value as arguments
	KeyFakeIPPortRange i18n.Key = "config.fake_ip_port_range"

	// KeyFakeIPRTTFallbackNegative 是 fake_ips 某项 rtt_fallback_ms 为负时的键，参数为下标
	// KeyFakeIPRTTFallbackNegative is the key used when one fake_ips entry carries a negative rtt_fallback_ms, whose argument is the index
	KeyFakeIPRTTFallbackNegative i18n.Key = "config.fake_ip_rtt_fallback_negative"

	// KeyClientListenPortRequired 是 client 角色缺少 self.listen_port 时的键
	// KeyClientListenPortRequired is the key used when the client role carries no self.listen_port
	KeyClientListenPortRequired i18n.Key = "config.client_listen_port_required"

	// KeyClientBootstrapRequired 是 client 角色缺少引导地址或端口时的键
	// KeyClientBootstrapRequired is the key used when the client role carries no bootstrap address or port
	KeyClientBootstrapRequired i18n.Key = "config.client_bootstrap_required"

	// KeyServerUUIDRequired 是 server 角色缺少 self.uuid 时的键
	// KeyServerUUIDRequired is the key used when the server role carries no self.uuid
	KeyServerUUIDRequired i18n.Key = "config.server_uuid_required"

	// KeyServerPortsRequired 是 server 角色缺少监听端口或上游端口时的键
	// KeyServerPortsRequired is the key used when the server role carries no listen port or upstream port
	KeyServerPortsRequired i18n.Key = "config.server_ports_required"
)
