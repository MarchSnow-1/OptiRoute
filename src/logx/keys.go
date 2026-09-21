// 本文件声明 logx 包用到的消息键，各语言的文案放在 i18n/locales 下的语言文件里
//
// This file declares the message keys logx uses, and the copy of each language lives in the language files under i18n/locales
package logx

import "github.com/MarchSnow-1/OptiRoute/i18n"

// KeyQueueFull 是日志队列满并丢弃记录时输出的提示键
// 该提示在本批丢弃后的第一条日志之前输出一次，参数为本次丢弃的条数
//
// KeyQueueFull is the key of the notice printed when the log queue is full and drops records
// The notice goes out once before the first record of that batch and carries the number of records dropped
const KeyQueueFull i18n.Key = "logx.queue_full"
