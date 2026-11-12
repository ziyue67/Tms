// Package paneldefaults 提供「妙妙屋四面板」的内置模板与内置规则集。
//
// 面板首次启动时把这些内容写进 subscribe_template / ruleset 两张表,
// 让模板管理页开箱就有东西可用 —— 之前空表意味着管理员要自己从零手写
// 一份 mihomo 骨架才订阅得出配置。
//
// 内置内容全部 embed 进二进制:不依赖网络、不依赖部署目录里的散文件。
// clash 模板抄自 iluobei/miaomiaowuX 的 rule_templates,保持与上游同名同骨架,
// 只有 __PROXY_NODES__ / __PROXY_PROVIDERS__ / proxies: null 三处占位会由
// 渲染器替换成真实节点。
package paneldefaults

import (
	"embed"
	"fmt"
)

//go:embed assets/*
var assets embed.FS

// Template 一个内置订阅模板。
type Template struct {
	Name    string
	Client  string // clash | surge | loon | quanx | singbox
	Version string // v3 = 结构化骨架(JSON/YAML),v2 = 纯文本
	Remark  string
	Default bool
	Content string
}

// Ruleset 一个内置规则集。内置规则集都是远程来源:内容为空,由主进程启动后
// 或管理员点「刷新」时抓取;抓不到也只是显示未抓取,不影响订阅渲染。
type Ruleset struct {
	Name           string
	DisplayName    string
	Source         string
	RemoteURL      string
	RefreshMinutes int
	Content        string
}

// Templates 返回内置模板。顺序即面板里的展示顺序(默认模板排最前)。
func Templates() []Template {
	return []Template{
		{
			Name: "fake_ip__v3.yaml", Client: "clash", Version: "v3", Default: true,
			Remark:  "fake-ip DNS + 全套策略组(推荐默认)",
			Content: mustRead("assets/fake_ip__v3.yaml"),
		},
		{
			Name: "icon_ruleset__v3.yaml", Client: "clash", Version: "v3",
			Remark:  "fake_ip 骨架 + 策略组图标",
			Content: mustRead("assets/icon_ruleset__v3.yaml"),
		},
		{
			Name: "redirhost__v3.yaml", Client: "clash", Version: "v3",
			Remark:  "redir-host DNS,兼容老设备",
			Content: mustRead("assets/redirhost__v3.yaml"),
		},
		{
			Name: "default__singbox.json", Client: "singbox", Version: "v3",
			Remark:  "sing-box 骨架(TUN + mixed 入站)",
			Content: mustRead("assets/default__singbox.json"),
		},
		{
			Name: "surge_cn__surge.conf", Client: "surge", Version: "v2",
			Remark:  "Surge 5 国内优先配置",
			Content: mustRead("assets/surge_cn__surge.conf"),
		},
		{
			Name: "starter__qx.qx", Client: "quanx", Version: "v2",
			Remark:  "Quantumult X 策略组 + 分流",
			Content: mustRead("assets/starter__qx.qx"),
		},
	}
}

// Rulesets 返回内置规则集。
//
// 都是「手动」来源的完整规则行(带策略),渲染时直接插入模板 rules 的 MATCH 之前 ——
// 上游那种只有 `DOMAIN-SUFFIX,xxx`、没有策略的 rule-provider 清单不能直接内联,
// 想用的话请自己在面板里建远程规则集并确认格式。
func Rulesets() []Ruleset {
	return []Ruleset{
		{Name: "advertising.lst", DisplayName: "广告拦截", Source: "manual", Content: mustRead("assets/advertising.lst")},
		{Name: "cn-direct.lst", DisplayName: "国内直连", Source: "manual", Content: mustRead("assets/cn-direct.lst")},
		{Name: "proxy-common.lst", DisplayName: "常用代理域名", Source: "manual", Content: mustRead("assets/proxy-common.lst")},
	}
}

// TemplateByName 按名字取内置模板,「恢复内置」用。
func TemplateByName(name string) (Template, bool) {
	for _, t := range Templates() {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}

func mustRead(path string) string {
	data, err := assets.ReadFile(path)
	if err != nil {
		// embed 是编译期写入的,读不到只可能是文件名写错 —— 直接 panic 比
		// 静默返回空模板好排查。
		panic(fmt.Sprintf("paneldefaults: %s: %v", path, err))
	}
	return string(data)
}
