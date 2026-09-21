// 本文件实现消息目录：键到三种语言文案的映射、登记与取用
// 各包的 messages.go 在 init 中登记自己的条目，init 阶段结束后目录只被读取
// 一条消息在三种语言下必须使用数量与类型一致的格式符，由 main 包下的目录校验测试逐条比对
// 取文案时先按语言取字段，无参数调用直接返回原文，使文案中出现的百分号不会被当成格式符
//
// This file implements the message catalogue: the mapping from a key to text in three languages, registration and lookup
// Each package registers its own entries from init in its messages.go, and the catalogue is only read once the init phase ends
// A message must use format verbs of the same count and type in the three languages, which the catalogue test in the main package compares entry by entry
// A lookup takes the field of the language, and a call without arguments returns the text as it stands so that a percent sign in the text cannot act as a format verb
package i18n

import (
	"fmt"
	"sort"
	"sync"
)

// Key 是消息目录的键，取值由各包以自己的常量约定，形如 edge.connect.failed
// Key is a message catalogue key owned by each package as its own constants, shaped like edge.connect.failed
type Key string

// Entry 是一条消息在三种语言下的文案
// 字段不导出，使条目只能由 New 组装，避免按位置传参时把文案配到错误的语言上
// Entry is one message's text in three languages
// Its fields stay unexported so that an entry can only be built by New, which keeps a positional call from pairing text with the wrong language
type Entry struct {
	en   string
	hans string
	hant string
}

// New 按 en-US、zh-Hans、zh-Hant 的顺序组装一条消息
// New builds one message from its text in the order en-US, zh-Hans, zh-Hant
func New(en, hans, hant string) Entry {
	return Entry{en: en, hans: hans, hant: hant}
}

// text 返回该条目在指定语言下的文案
// 该语言缺文案时退回英文，英文也为空时保持空串，由 Text 决定如何暴露这一缺失
//
// text returns the entry's message in the given language
// A language without text falls back to English, and an empty English text stays empty, leaving Text to decide how the gap is surfaced
func (e Entry) text(l Lang) string {
	switch l {
	case LangZHHans:
		if e.hans != "" {
			return e.hans
		}
	case LangZHant:
		if e.hant != "" {
			return e.hant
		}
	}
	return e.en
}

// All 返回条目在三种语言下的文案，供目录校验逐条比对
// All returns the entry's text in the three languages so that the catalogue check can compare entry by entry
func (e Entry) All() map[Lang]string {
	return map[Lang]string{
		LangEN:     e.en,
		LangZHHans: e.hans,
		LangZHant:  e.hant,
	}
}

var (
	// catMu 保护 catalog，使登记与取用可以跨 goroutine 交错进行
	// catMu guards catalog so that registration and lookup may interleave across goroutines
	catMu   sync.RWMutex
	catalog = make(map[Key]Entry)
)

// Register 登记一批消息，只应由各包的 init 调用
// 同一个键以不同文案登记两次时 panic：那会静默覆盖先登记的文案，属于必须在启动前暴露的编程错误
// 同一个键以完全相同的文案重复登记视为无操作，使登记具备幂等性，测试在同一进程内重复登记同一批消息不会失败
// 冲突在 catMu 内一次查完并先释放锁，随后才渲染并抛出 panic 文案，因此 T 取文案时不会与 catMu 互相等待
//
// Register adds a batch of messages and should only be called from a package init
// A key registered twice with different text panics, because that silently replaces the text registered first and is a programming error which has to surface before start-up
// A key registered again with exactly the same text is a no-op, which makes registration idempotent so that a test registering the same batch twice in one process does not fail
// Every conflict is collected under catMu before the lock is released, and only then is the panic notice rendered and raised, so the T call inside it cannot wait on catMu
func Register(entries map[Key]Entry) {
	catMu.Lock()
	conflict, hasConflict := Key(""), false
	for k, e := range entries {
		if prev, dup := catalog[k]; dup {
			if prev == e {
				continue
			}
			conflict, hasConflict = k, true
			break
		}
		catalog[k] = e
	}
	catMu.Unlock()

	if hasConflict {
		panic(T(KeyDuplicateMessageKey, conflict))
	}
}

// Text 返回一条消息在指定语言下的文案，args 交给 fmt.Sprintf 填充
// 键未登记时返回形如 !消息键! 的标记，使缺失的文案在输出中直接可见
//
// Text returns one message in the given language and hands args to fmt.Sprintf
// An unregistered key returns a marker shaped like !message.key! so that missing text shows up in the output
func Text(l Lang, key Key, args ...any) string {
	catMu.RLock()
	e, ok := catalog[key]
	catMu.RUnlock()

	if !ok {
		return "!" + string(key) + "!"
	}
	text := e.text(l)
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

// T 返回一条消息在当前语言下的文案
// T returns one message in the current language
func T(key Key, args ...any) string {
	return Text(Current(), key, args...)
}

// Lookup 返回一条消息的条目与是否已登记，供目录校验使用
// Lookup returns a message entry and whether it is registered, for catalogue checks
func Lookup(key Key) (Entry, bool) {
	catMu.RLock()
	defer catMu.RUnlock()
	e, ok := catalog[key]
	return e, ok
}

// Keys 返回已登记的全部键，按键名字典序排列
// Keys returns every registered key in ascending key order
func Keys() []Key {
	catMu.RLock()
	out := make([]Key, 0, len(catalog))
	for k := range catalog {
		out = append(out, k)
	}
	catMu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
