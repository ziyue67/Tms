import type { PanelRuleset } from "@/api";

/**
 * 「新建订阅 → 规则选择」的展示辅助:分类 emoji + 预设方案。
 * 管理端和用户端共用,避免两套 emoji 表跑偏。
 */

const EMOJI_TABLE: [RegExp, string][] = [
  [/广告|拦截/, "🛑"],
  [/hentai/i, "🔞"],
  [/AI/i, "🤖"],
  [/哔哩|bilibili/i, "📺"],
  [/油管|youtube/i, "📹"],
  [/谷歌|google/i, "🔍"],
  [/私有|内网/, "🏠"],
  [/国内|直连/, "🔒"],
  [/电报|telegram/i, "📲"],
  [/github/i, "🐱"],
  [/微软|microsoft/i, "Ⓜ️"],
  [/苹果|apple/i, "🍏"],
  [/社交/, "🌐"],
  [/流媒体/, "🎬"],
  [/游戏/, "🎮"],
  [/教育/, "📚"],
  [/金融/, "💰"],
  [/云服务/, "☁️"],
  [/tiktok/i, "🎶"],
  [/spotify/i, "🎵"],
  [/pixiv/i, "🎨"],
  [/abema/i, "📡"],
  [/代理媒体/, "🎭"],
  [/代理/, "🔀"],
  [/非中国/, "🌍"],
  [/tracker/i, "🔗"],
  [/PT/, "🎞️"],
];

export function categoryEmoji(item: Pick<PanelRuleset, "displayName" | "name">) {
  const label = `${item.displayName || ""}${item.name}`;
  for (const [pattern, emoji] of EMOJI_TABLE) {
    if (pattern.test(label)) return emoji;
  }
  return "🔖";
}

export type RulePreset = "balanced" | "ads" | "manual";

export const RULE_PRESETS: { key: RulePreset; label: string; hint: string }[] = [
  { key: "balanced", label: "均衡规则(推荐)", hint: "已自动选择 7 个常用规则类别,可以手动调整" },
  { key: "ads", label: "只拦广告", hint: "只勾广告拦截类规则集" },
  { key: "manual", label: "手动选择", hint: "自己决定勾哪些" },
];

// 参考面板「均衡规则」默认勾的那几类
const BALANCED = ["广告拦截", "国内直连", "私有网络", "谷歌服务", "油管视频", "Github", "苹果服务"];

export function presetRulesetIDs(rulesets: PanelRuleset[], preset: RulePreset): string[] {
  if (preset === "balanced") {
    const picked = rulesets.filter((item) => BALANCED.some((name) => (item.displayName || "").includes(name)));
    return (picked.length > 0 ? picked : rulesets).map((item) => String(item.id));
  }
  if (preset === "ads") {
    return rulesets
      .filter((item) => /广告|拦截|ad/i.test(`${item.displayName || ""}${item.name}`))
      .map((item) => String(item.id));
  }
  return [];
}
