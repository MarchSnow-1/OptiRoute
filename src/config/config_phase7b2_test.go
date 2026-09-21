// 本文件锁住 Phase 7b-2 引入的 max_connections 默认值与校验语义
// 默认值同时存在于 config 的 defaults() 与 edge 的 defaultMaxConnections：两处必须一致，否则「未配置」在不同入口会得到不同上限
// 该断言放在 config 包内，避免 config 反向依赖 edge
//
// This file pins the default value and the validation semantics of max_connections introduced in Phase 7b-2
// The default lives both in config.defaults() and in edge.defaultMaxConnections, and the two have to agree, otherwise "not configured" would yield a different cap depending on the entry point
// The assertion sits in the config package so that config does not have to depend on edge
package config

import "testing"

// expectedDefaultMaxConnections 是 max_connections 的默认值，与 edge.defaultMaxConnections 必须一致
// 该常量的唯一作用是让两处默认值的比对在 config 包内可以表达
//
// expectedDefaultMaxConnections is the default of max_connections and has to match edge.defaultMaxConnections
// The constant exists so that the comparison of the two defaults can be expressed inside the config package
const expectedDefaultMaxConnections = 4096

// TestMaxConnectionsDefaults 覆盖 max_connections 的默认值与显式 0 的区分
// TestMaxConnectionsDefaults covers the default of max_connections and how it differs from an explicit 0
func TestMaxConnectionsDefaults(t *testing.T) {
	c := &Config{}
	c.defaults()
	if c.Self.MaxConnections == nil {
		t.Fatal("defaults() should fill max_connections")
	}
	if *c.Self.MaxConnections != expectedDefaultMaxConnections {
		t.Fatalf("default max_connections = %d, want %d", *c.Self.MaxConnections, expectedDefaultMaxConnections)
	}

	// 显式 0 表示不限制，必须保留而不是被默认值覆盖
	// An explicit 0 means no limit and has to survive instead of being overwritten by the default
	zero := 0
	c = &Config{}
	c.Self.MaxConnections = &zero
	c.defaults()
	if c.Self.MaxConnections == nil || *c.Self.MaxConnections != 0 {
		t.Fatalf("an explicit max_connections of 0 was overwritten: %v", c.Self.MaxConnections)
	}

	// 非零的显式取值同样保留
	// A non-zero explicit value survives as well
	custom := 128
	c = &Config{}
	c.Self.MaxConnections = &custom
	c.defaults()
	if *c.Self.MaxConnections != 128 {
		t.Fatalf("an explicit max_connections of 128 was overwritten: %d", *c.Self.MaxConnections)
	}
}

// TestMaxConnectionsRejectsNegative 覆盖负值被校验拒绝，而 0 与正值通过
// TestMaxConnectionsRejectsNegative covers a negative value being rejected while 0 and positive ones pass
func TestMaxConnectionsRejectsNegative(t *testing.T) {
	build := func(v int) *Config {
		c := &Config{}
		c.defaults()
		c.Self.MaxConnections = &v
		return c
	}

	if err := build(-1).validateCommonNumerics(); err == nil {
		t.Fatal("a negative max_connections should be rejected")
	}
	if err := build(0).validateCommonNumerics(); err != nil {
		t.Fatalf("an explicit 0 should be accepted: %v", err)
	}
	if err := build(64).validateCommonNumerics(); err != nil {
		t.Fatalf("a positive value should be accepted: %v", err)
	}
}
