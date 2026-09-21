// 本文件实现消息目录：从内嵌的语言文件加载键到文案的映射，并按语言取用
// 文案的唯一来源是 locales 目录下的 JSON，一门语言一个文件，键名形如 edge.connect_failed
// 每个 JSON 条目含 text 与 note 两个字段：text 是文案，note 说明该消息在什么条件下输出、参数各是什么，供译者与维护者阅读
// 键仍由各包以 Go 常量声明，因此调用点的键有编译期校验，而言辞改动只落在 JSON 上
// 新增一门语言只需加一个 JSON 文件并登记到 supported，取文案的路径不含按语言分支的 switch
// 加载在 init 中完成一次，之后目录只被读取，因此日志与报错路径上的取文案不需要额外同步
//
// This file implements the message catalogue: it loads the mapping from keys to text out of the embedded language files and looks text up by language
// The single source of the copy is the JSON under locales, one file per language, with keys shaped like edge.connect_failed
// An entry carries text and note: text is the copy, while note states the condition under which the message is emitted and what its arguments are, for translators and maintainers to read
// Keys stay declared as Go constants in each package, so a key at a call site is checked at compile time while a wording change lands in the JSON alone
// Adding a language takes one more JSON file and one entry in supported, and the lookup path holds no per-language switch
// Loading happens once in init and the catalogue is only read afterwards, so a lookup on the logging and error paths needs no extra synchronisation
package i18n

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// localeFiles 嵌入 locales 目录下的一门语言一个文件
// 用 go:embed 而不是在运行期读盘，使二进制自带文案、启动不依赖外部文件，也不引入任何平台相关的路径处理
//
// localeFiles embeds the locales directory, which holds one file per language
// go:embed is used rather than reading from disk at run time, so the binary carries its own copy, start-up depends on no external file, and no platform-specific path handling is involved
//
//go:embed locales/*.json
var localeFiles embed.FS

// localeDir 是嵌入路径的前缀，与 go:embed 的指令保持一致
// localeDir is the prefix of the embedded paths and matches the go:embed directive
const localeDir = "locales/"

// Key 是消息目录的键，由各包以自己的常量声明，形如 edge.connect_failed
// Key is a message catalogue key declared as a constant by each package, shaped like edge.connect_failed
type Key string

// Entry 是一条已加载的消息：各语言的文案，加上供人阅读的说明
// texts 以 Lang 为键，因此取文案是按语言查表而不是按位置取字段，新增语言不需要在这里加分支
//
// Entry is one loaded message: its text per language plus the note meant for a reader
// texts is keyed by Lang, so a lookup is a table read rather than a positional field access, and a new language needs no branch here
type Entry struct {
	texts map[Lang]string
	note  string
}

// text 返回该条目在指定语言下的文案
// 该语言缺文案时退回默认语言，默认语言也为空时保持空串，由 Text 决定如何暴露这一缺失
//
// text returns the entry's text in the given language
// A language without text falls back to the default one, and an empty default stays empty, leaving Text to decide how the gap is surfaced
func (e Entry) text(l Lang) string {
	if t := e.texts[l]; t != "" {
		return t
	}
	return e.texts[Default()]
}

// All 返回条目在已支持语言下的文案，供目录校验逐条比对
// All returns the entry's text in every supported language so that the catalogue check can compare entry by entry
func (e Entry) All() map[Lang]string {
	out := make(map[Lang]string, len(e.texts))
	for l, t := range e.texts {
		out[l] = t
	}
	return out
}

// Note 返回该条目供人阅读的说明
// Note returns the note of this entry, which is meant for a reader
func (e Entry) Note() string { return e.note }

var (
	// catMu 保护 catalog，使加载之后的取用与测试可能发起的重新加载互不阻塞
	// catMu guards catalog so that lookups after loading and a reload a test may trigger do not block one another
	catMu   sync.RWMutex
	catalog = make(map[Key]Entry)
)

// loadError 描述加载语言文件失败的原因
// 该错误只在二进制自带的语言文件本身损坏时出现，属构建缺陷：CI 的目录门禁会在合并之前拦下它
// 因此它的文本不取自目录也不做本地化，否则渲染报错就需要那份加载失败的目录
//
// loadError describes why loading a language file failed
// The failure happens only when a language file carried by the binary is itself broken, which is a build defect the catalogue gate in CI stops before a merge
// Its text therefore comes from neither the catalogue nor a translation, because rendering such an error would need the very catalogue that failed to load
type loadError struct {
	lang string
	file string
	key  string
	kind string
	err  error
}

// Error 实现 error
// Error implements error
func (e loadError) Error() string {
	switch e.kind {
	case "read":
		return "i18n: cannot read " + localeDir + e.file + ": " + e.err.Error()
	case "parse":
		return "i18n: cannot parse " + localeDir + e.file + ": " + e.err.Error()
	case "duplicate":
		return "i18n: " + localeDir + e.file + " declares the key " + e.key + " more than once"
	case "empty":
		return "i18n: " + localeDir + e.file + " carries no entry for " + e.key
	default:
		return "i18n: cannot load " + localeDir + e.file
	}
}

// decodeLocale 解析一个语言文件的正文
// 用词法扫描而不是一次 Unmarshal，是为了显式发现重复键：encoding/json 对重复键静默取后者，那会让一个键的文案被另一条覆盖而不报错
// 同时要求每个条目的 text 非空，空文案在输出里等同于缺失，放在加载期报出比在运行期看到 !键! 标记更早
//
// decodeLocale parses the body of one language file
// It scans tokens instead of a single Unmarshal to surface a duplicate key explicitly, because encoding/json silently keeps the last one and that would let one entry overwrite another without an error
// It also requires a non-empty text, since empty copy is indistinguishable from missing output, and reporting it while loading comes earlier than seeing a !key! marker at run time
func decodeLocale(lang Lang, body []byte) (map[Key]Entry, error) {
	fileName := lang.String() + ".json"
	dec := json.NewDecoder(bytes.NewReader(body))

	open, err := dec.Token()
	if err != nil {
		return nil, loadError{lang: string(lang), file: fileName, kind: "parse", err: err}
	}
	if delim, ok := open.(json.Delim); !ok || delim != '{' {
		return nil, loadError{lang: string(lang), file: fileName, kind: "parse", err: fmt.Errorf("the top level is not an object")}
	}

	out := make(map[Key]Entry)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, loadError{lang: string(lang), file: fileName, kind: "parse", err: err}
		}
		raw, ok := tok.(string)
		if !ok {
			return nil, loadError{lang: string(lang), file: fileName, kind: "parse", err: fmt.Errorf("a key is not a string")}
		}
		if _, dup := out[Key(raw)]; dup {
			return nil, loadError{lang: string(lang), file: fileName, key: raw, kind: "duplicate"}
		}
		var item struct {
			Text string `json:"text"`
			Note string `json:"note"`
		}
		if err := dec.Decode(&item); err != nil {
			return nil, loadError{lang: string(lang), file: fileName, key: raw, kind: "parse", err: err}
		}
		if strings.TrimSpace(item.Text) == "" {
			return nil, loadError{lang: string(lang), file: fileName, key: raw, kind: "empty"}
		}
		out[Key(raw)] = Entry{texts: map[Lang]string{lang: item.Text}, note: item.Note}
	}
	return out, nil
}

// Load 读取全部内嵌语言文件并组装目录
// 同一条消息在多个语言文件里出现时把各语言文案并入同一条目，因此一个键的三语文案来自三个文件
// 返回错误时目录保持原样，调用方据错误决定是 panic 还是留给测试报告
//
// Load reads every embedded language file and assembles the catalogue
// A message appearing in several language files has its texts merged into one entry, so the three texts of a key come from three files
// On an error the catalogue stays as it was, leaving the caller to decide between a panic and a test report
func Load() error {
	assembled := make(map[Key]Entry)

	for _, lang := range supported {
		body, err := localeFiles.ReadFile(localeDir + lang.String() + ".json")
		if err != nil {
			return loadError{lang: string(lang), file: lang.String() + ".json", kind: "read", err: err}
		}
		entries, err := decodeLocale(lang, body)
		if err != nil {
			return err
		}
		for k, e := range entries {
			existing, ok := assembled[k]
			if !ok {
				// 首个语言文件建立条目，其 note 即该条目的说明，其余语言文件只补文案
				// The first language file creates the entry and its note becomes that entry's note, while the others only add text
				assembled[k] = e
				continue
			}
			existing.texts[lang] = e.text(lang)
			assembled[k] = existing
		}
	}

	catMu.Lock()
	catalog = assembled
	catMu.Unlock()
	return nil
}

// init 加载内嵌语言文件
// 加载失败时 panic：那说明二进制自带的语言文件损坏，进程带着缺文案的目录继续运行只会把 !键! 标记写进日志
//
// init loads the embedded language files
// A failed load panics: it means a language file carried by the binary is broken, and letting the process run with a partly empty catalogue would only write !key! markers into the log
func init() {
	if err := Load(); err != nil {
		panic(err.Error())
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
