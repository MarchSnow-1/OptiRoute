// 本文件加载仓库 examples/ 下的四个示例配置并逐个走 parse 与 Validate，使示例与实际校验规则保持一致
// 建立原因：示例是用户直接复制的内容，一旦某个字段名或取值与实现脱节，问题会出现在用户侧而不是 CI 侧
// 用例通过相对路径定位 examples/，因此只在仓库目录结构内成立；缺失示例文件会直接失败并指明路径
//
// This file loads the four example configurations under examples/ and runs each one through parse and Validate, keeping the examples and the actual validation rules in step
// Reason for its existence: the examples are what users copy, and a field name or value that drifts from the implementation would surface on the user side rather than in CI
// The cases locate examples/ through a relative path, so they hold inside the repository layout, and a missing example fails with the path spelled out
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// exampleDir 是示例配置所在目录，相对本包目录
// exampleDir is the directory holding the example configurations, relative to this package directory
const exampleDir = "../../examples"

// TestExamplesLoadAndValidate 覆盖四个示例配置：每个都能被 parse 读出，并通过 Validate
// TestExamplesLoadAndValidate covers the four example configurations: each one parses and passes Validate
func TestExamplesLoadAndValidate(t *testing.T) {
	for _, name := range []string{"center.json", "edge.json", "client.json", "server.json"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(exampleDir, name)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("the example should exist at %s: %v", path, err)
			}

			cfg, err := parse(raw)
			if err != nil {
				t.Fatalf("the example should parse: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("the example should pass validation: %v", err)
			}

			// 角色必须与文件名一致，否则用户会拿到一份名不符实的示例
			// The role has to match the file name, otherwise a user would get an example that does not match its name
			wantRole := Role(name[:len(name)-len(".json")])
			if cfg.Self.Role != wantRole {
				t.Fatalf("role = %q, want %q to match the file name", cfg.Self.Role, wantRole)
			}
		})
	}
}

// TestExamplesUseGenericPlaceholders 覆盖示例中的取值不指向真实主机
// 示例地址一律落在 RFC 5737 的文档用地址段内，避免把某台真实主机写进仓库
//
// TestExamplesUseGenericPlaceholders covers the values in the examples not pointing at a real host
// Every example address sits inside the RFC 5737 documentation ranges, which keeps a real host out of the repository
func TestExamplesUseGenericPlaceholders(t *testing.T) {
	// 允许的地址前缀：RFC 5737 的 192.0.2.0/24、198.51.100.0/24 与 203.0.113.0/24，加上回环与本机默认值
	// The accepted address prefixes are the RFC 5737 blocks 192.0.2.0/24, 198.51.100.0/24 and 203.0.113.0/24, plus loopback and local defaults
	allowed := []string{"192.0.2.", "198.51.100.", "203.0.113.", "127.0.0.", "::1", ""}

	for _, name := range []string{"center.json", "edge.json", "client.json", "server.json"} {
		raw, err := os.ReadFile(filepath.Join(exampleDir, name))
		if err != nil {
			t.Fatalf("the example should exist: %v", err)
		}
		cfg, err := parse(raw)
		if err != nil {
			t.Fatal(err)
		}

		addrs := []string{
			cfg.Self.Addr, cfg.Self.ListenAddr,
			cfg.Remote.CenterAddr, cfg.Remote.OriginAddr,
			cfg.Remote.BootstrapAddr, cfg.Remote.UpstreamAddr,
		}
		for _, f := range cfg.Self.FakeIPs {
			addrs = append(addrs, f.IP)
		}

		for _, addr := range addrs {
			if !hasAnyPrefix(addr, allowed) {
				t.Fatalf("%s carries the address %q, which is outside the documentation ranges", name, addr)
			}
		}
	}
}

// hasAnyPrefix 判断 s 是否以 prefixes 中的任一项开头，空字符串视为允许
// hasAnyPrefix reports whether s starts with any entry of prefixes, and an empty string counts as accepted
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return s == ""
}
