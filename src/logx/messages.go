// 本文件登记 logx 包用到的消息键与其三种语言文案
// 在 init 中一次性登记，登记完成后目录只被读取，因此日志路径上的取文案不需要额外同步
//
// This file registers the message keys logx uses together with their text in three languages
// Registration happens once in init and the catalogue is only read afterwards, so fetching text on the logging path needs no extra synchronisation
package logx

import "github.com/MarchSnow-1/OptiRoute/i18n"

// KeyQueueFull 是日志队列满并丢弃记录时输出的提示键
// 该提示在本批丢弃后的第一条日志之前输出一次，参数为本次丢弃的条数
//
// KeyQueueFull is the key of the notice printed when the log queue is full and drops records
// The notice goes out once before the first record of that batch and carries the number of records dropped
const KeyQueueFull i18n.Key = "logx.queue_full"

// messages 保存本包的消息，键为 i18n.Key，值为三种语言下的文案
// messages holds this package's messages, keyed by i18n.Key with the text in three languages
var messages = map[i18n.Key]i18n.Entry{
	KeyQueueFull: i18n.New(
		"log queue full, %d records dropped",
		"日志队列已满，丢弃 %d 条",
		"日誌佇列已滿，丟棄 %d 條",
	),
}

// init 登记本包的消息，使任何日志调用之前目录已经就绪
// init registers this package's messages so that the catalogue is ready before any logging call
func init() {
	i18n.Register(messages)
}
