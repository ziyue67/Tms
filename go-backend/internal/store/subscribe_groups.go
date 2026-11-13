package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SubscribeGroup 分组订阅里的一个分组:一组节点 + 这组节点自己的规则。
//
// 场景:一条订阅链接里,「日本1」要广告拦截+谷歌+油管,「日本」要电报+苹果。
// 渲染时每个分组变成一个策略组,分组里的规则集行把策略改写成这个组名。
type SubscribeGroup struct {
	Name       string   `json:"name"`
	NodeIDs    []string `json:"nodeIds"`
	RulesetIDs []string `json:"rulesetIds"`
	// Match 关键词(逗号分隔):按名字把「用户自己的线路」收进这个组,
	// 例如 香港,HK。全局订阅集靠它做到「每个用户收自己的节点」。
	Match string `json:"match,omitempty"`
}

// ParseGroups 解析 groups_json;空串返回 nil。
func ParseGroups(raw string) ([]SubscribeGroup, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var groups []SubscribeGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// MarshalGroups 归一化后再存:去空白、去重、丢掉空分组。
func MarshalGroups(groups []SubscribeGroup) (string, error) {
	cleaned := make([]SubscribeGroup, 0, len(groups))
	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		match := strings.TrimSpace(group.Match)
		// 既没勾节点、也没写关键词的分组没有意义,丢掉
		if name == "" || (len(group.NodeIDs) == 0 && match == "") {
			continue
		}
		if len(name) > 64 {
			name = name[:64]
		}
		cleaned = append(cleaned, SubscribeGroup{
			Name:       name,
			NodeIDs:    dedupeStrings(group.NodeIDs),
			RulesetIDs: dedupeStrings(group.RulesetIDs),
			Match:      strings.TrimSpace(group.Match),
		})
	}
	if len(cleaned) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(cleaned)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// GroupNodeIDs 汇总所有分组的节点 id(去重,保持顺序)。
func GroupNodeIDs(groups []SubscribeGroup) []int64 {
	seen := map[int64]struct{}{}
	ids := []int64{}
	for _, group := range groups {
		for _, raw := range group.NodeIDs {
			var id int64
			if _, err := fmt.Sscan(strings.TrimSpace(raw), &id); err != nil || id <= 0 {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
