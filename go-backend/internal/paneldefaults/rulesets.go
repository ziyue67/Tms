package paneldefaults

import (
	"io/fs"
	"path"
	"sort"
)

// 规则分类:对应参考面板「新建订阅 → 规则选择」里那一格格带 emoji 的类别。
//
// 每个分类都是一份「带策略的完整规则行」,注入模板 rules 的 MATCH 之前。
// 策略用内置 clash 模板里真实存在的策略组名(🚀 手动选择 / 🎯 全球直连 /
// 🍎 苹果服务 / Ⓜ️ 微软服务 / 🎮 游戏平台 / REJECT);换成自己的模板时
// 要么保留这些组名,要么在「规则集」里改策略。
type ruleCategory struct {
	file    string
	display string
	policy  string
	domains []string
	extra   []string // 额外规则行(IP-CIDR、GEOIP 之类)
}

const (
	policyProxy  = "🚀 手动选择"
	policyDirect = "🎯 全球直连"
	policyApple  = "🍎 苹果服务"
	policyMS     = "Ⓜ️ 微软服务"
	policyGame   = "🎮 游戏平台"
	policyReject = "REJECT"
)

var ruleCategories = []ruleCategory{
	{file: "advertising.lst", display: "广告拦截", policy: policyReject, domains: []string{
		"doubleclick.net", "googlesyndication.com", "googleadservices.com", "adservice.google.com",
		"ads.twitter.com", "advertising.com", "adnxs.com", "adcolony.com", "applovin.com",
		"unityads.unity3d.com", "app-measurement.com", "crashlytics.com", "adjust.com",
		"umeng.com", "cnzz.com", "talkingdata.com", "analytics.126.net",
	}},
	{file: "category-ai.lst", display: "AI 服务", policy: policyProxy, domains: []string{
		"openai.com", "chatgpt.com", "anthropic.com", "claude.ai", "gemini.google.com",
		"bard.google.com", "perplexity.ai", "midjourney.com", "poe.com", "cohere.ai",
	}},
	{file: "category-bilibili.lst", display: "哔哩哔哩", policy: policyDirect, domains: []string{
		"bilibili.com", "bilivideo.com", "hdslb.com", "biliapi.net", "biliapi.com", "b23.tv",
	}},
	{file: "category-youtube.lst", display: "油管视频", policy: policyProxy, domains: []string{
		"youtube.com", "youtu.be", "ytimg.com", "googlevideo.com", "youtube-nocookie.com", "yt.be",
	}},
	{file: "category-google.lst", display: "谷歌服务", policy: policyProxy, domains: []string{
		"google.com", "googleapis.com", "gstatic.com", "googleusercontent.com", "ggpht.com",
		"gmail.com", "googlemail.com", "withgoogle.com",
	}},
	{file: "category-private.lst", display: "私有网络", policy: policyDirect, domains: []string{
		"local", "lan", "localhost", "home.arpa", "router.asus.com", "miwifi.com",
	}, extra: []string{
		"IP-CIDR,10.0.0.0/8," + policyDirect + ",no-resolve",
		"IP-CIDR,172.16.0.0/12," + policyDirect + ",no-resolve",
		"IP-CIDR,192.168.0.0/16," + policyDirect + ",no-resolve",
		"IP-CIDR,127.0.0.0/8," + policyDirect + ",no-resolve",
	}},
	{file: "cn-direct.lst", display: "国内直连", policy: policyDirect, domains: []string{
		"baidu.com", "bdstatic.com", "qq.com", "gtimg.com", "taobao.com", "tmall.com",
		"alicdn.com", "alipay.com", "jd.com", "zhihu.com", "weibo.com", "163.com",
		"126.net", "sohu.com", "iqiyi.com", "youku.com", "douyin.com", "meituan.com",
		"dianping.com", "xiaohongshu.com",
	}, extra: []string{"GEOIP,CN," + policyDirect + ",no-resolve"}},
	{file: "category-telegram.lst", display: "电报消息", policy: policyProxy, domains: []string{
		"telegram.org", "t.me", "telegram.me", "telesco.pe", "telegra.ph", "tg.dev",
	}, extra: []string{
		"IP-CIDR,91.108.4.0/22," + policyProxy + ",no-resolve",
		"IP-CIDR,91.108.8.0/22," + policyProxy + ",no-resolve",
		"IP-CIDR,91.108.16.0/22," + policyProxy + ",no-resolve",
		"IP-CIDR,149.154.160.0/20," + policyProxy + ",no-resolve",
	}},
	{file: "category-github.lst", display: "Github", policy: policyProxy, domains: []string{
		"github.com", "github.io", "githubusercontent.com", "githubassets.com", "ghcr.io", "git.io",
	}},
	{file: "category-microsoft.lst", display: "微软服务", policy: policyMS, domains: []string{
		"microsoft.com", "live.com", "office.com", "office365.com", "windowsupdate.com",
		"msftncsi.com", "msftconnecttest.com", "bing.com", "msn.com", "xboxlive.com",
	}},
	{file: "category-apple.lst", display: "苹果服务", policy: policyApple, domains: []string{
		"apple.com", "icloud.com", "mzstatic.com", "cdn-apple.com", "applemusic.com",
		"itunes.com", "me.com", "apple-cloudkit.com",
	}},
	{file: "category-social.lst", display: "社交媒体", policy: policyProxy, domains: []string{
		"x.com", "twitter.com", "twimg.com", "facebook.com", "fbcdn.net", "instagram.com",
		"reddit.com", "redd.it", "discord.com", "discordapp.com", "linkedin.com",
	}},
	{file: "category-streaming.lst", display: "流媒体", policy: policyProxy, domains: []string{
		"netflix.com", "nflxvideo.net", "nflximg.net", "disneyplus.com", "dssott.com",
		"hulu.com", "primevideo.com", "hbomax.com", "max.com", "tv.apple.com",
	}},
	{file: "category-game.lst", display: "游戏平台", policy: policyGame, domains: []string{
		"steamcommunity.com", "steampowered.com", "steamcontent.com", "epicgames.com",
		"ea.com", "blizzard.com", "battle.net", "playstation.com", "nintendo.com", "xbox.com",
	}},
	{file: "category-edu.lst", display: "教育资源", policy: policyProxy, domains: []string{
		"coursera.org", "edx.org", "khanacademy.org", "udemy.com", "brilliant.org",
		"mit.edu", "stanford.edu", "arxiv.org", "scholar.google.com",
	}},
	{file: "category-finance.lst", display: "金融服务", policy: policyProxy, domains: []string{
		"paypal.com", "stripe.com", "coinbase.com", "binance.com", "okx.com",
		"wise.com", "revolut.com", "interactivebrokers.com",
	}},
	{file: "category-cloud.lst", display: "云服务", policy: policyProxy, domains: []string{
		"aws.amazon.com", "amazonaws.com", "cloudflare.com", "digitalocean.com",
		"azure.com", "vercel.com", "netlify.com", "heroku.com", "vultr.com", "linode.com",
	}},
	{file: "category-tiktok.lst", display: "Tiktok", policy: policyProxy, domains: []string{
		"tiktok.com", "tiktokcdn.com", "tiktokv.com", "musical.ly", "byteoversea.com",
	}},
	{file: "category-spotify.lst", display: "Spotify", policy: policyProxy, domains: []string{
		"spotify.com", "scdn.co", "spotifycdn.com", "spotify.link",
	}},
	{file: "category-pixiv.lst", display: "Pixiv", policy: policyProxy, domains: []string{
		"pixiv.net", "pximg.net", "fanbox.cc",
	}},
	{file: "category-abema.lst", display: "Abema", policy: policyProxy, domains: []string{
		"abema.tv", "abematv.akamaized.net", "abema-tv.com",
	}},
	{file: "proxy-common.lst", display: "代理服务", policy: policyProxy, domains: []string{
		"clash.razord.top", "yacd.haishan.me", "sub-web.netlify.app", "gh-proxy.com", "ghfast.top",
	}},
	{file: "category-proxy-media.lst", display: "代理媒体", policy: policyProxy, domains: []string{
		"vimeo.com", "dailymotion.com", "twitch.tv", "ttvnw.net", "soundcloud.com", "bandcamp.com",
	}},
	{file: "category-ehentai.lst", display: "E-Hentai", policy: policyProxy, domains: []string{
		"e-hentai.org", "exhentai.org", "ehgt.org", "hentaiverse.org",
	}},
	{file: "category-pt-tracker.lst", display: "PT Tracker", policy: policyProxy, domains: []string{
		"tracker.openbittorrent.com", "tracker.opentrackr.org", "open.tracker.cl",
		"tracker.torrent.eu.org", "explodie.org", "tracker.gbitt.info",
	}},
	{file: "category-pt-site.lst", display: "PT 站点", policy: policyProxy, domains: []string{
		"hdchina.org", "chdbits.co", "pt.sjtu.edu.cn", "totheglory.im", "hdatmos.club",
		"pterclub.com", "ourbits.club", "hdarea.club",
	}},
}

func buildRulesetContent(category ruleCategory) string {
	lines := []string{"# " + category.display + "(内置示例规则集,可自行增删)", "# 行格式:TYPE,value,policy[,no-resolve]"}
	for _, domain := range category.domains {
		lines = append(lines, "DOMAIN-SUFFIX,"+domain+","+category.policy)
	}
	lines = append(lines, category.extra...)
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	return content
}

// Rulesets 返回内置规则集 —— 27 个分类,和参考面板「规则选择」里的类别一一对应。
func Rulesets() []Ruleset {
	list := make([]Ruleset, 0, len(ruleCategories))
	for _, category := range ruleCategories {
		list = append(list, Ruleset{
			Name:        category.file,
			DisplayName: category.display,
			Source:      "manual",
			Content:     buildRulesetContent(category),
		})
	}
	return list
}

// AssetNames 仅用于测试:列出 assets 目录里的文件,检查有没有漏挂的。
func AssetNames() []string {
	entries, err := fs.ReadDir(assets, "assets")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, path.Base(entry.Name()))
	}
	sort.Strings(names)
	return names
}
