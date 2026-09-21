// 本文件登记 i18n 包自己用到的消息键与其三种语言文案
// 目录本身只承载一条文案：同一个键以不同文本重复登记时的 panic 提示，它在启动期的 init 阶段就可能触发
// 该文案中英拼在一行的写法已拆成三种语言的条目，使目录内的文案与其余包遵循同一套排版规则
// 在 init 中一次性登记，登记完成后目录只被读取
//
// This file registers the message key i18n itself uses together with its text in three languages
// The catalogue carries one message of its own: the panic notice for a key registered twice with different text, which can fire during the init phase of start-up
// The text used to sit in one line holding Chinese and English together, and splitting it into three language entries keeps the copy inside the catalogue under the same layout rule as every other package
// Registration happens once in init and the catalogue is only read afterwards
package i18n

const (
	// KeyDuplicateMessageKey 是同一个键以不同文案重复登记时的键，参数为该键
	// 该分支在 Register 中 panic：静默覆盖先登记的文案属于必须在启动前暴露的编程错误
	//
	// KeyDuplicateMessageKey is the key used when the same key is registered twice with different text, whose argument is that key
	// The branch panics inside Register, because silently replacing the text registered first is a programming error that has to surface before start-up
	KeyDuplicateMessageKey Key = "i18n.duplicate_message_key"
)

// messages 保存本包的消息，键为 Key，值为三种语言下的文案
// messages holds this package's messages, keyed by Key with the text in three languages
var messages = map[Key]Entry{
	KeyDuplicateMessageKey: New(
		"i18n: duplicate message key with different text: %s",
		"i18n: 消息键重复且文案不同: %s",
		"i18n: 訊息鍵重複且文案不同: %s",
	),
}

// init 登记本包的消息，使 Register 的 panic 文案在首次登记之前即可取用
// init registers this package's messages so that the panic text of Register is available before the first registration
func init() {
	Register(messages)
}
