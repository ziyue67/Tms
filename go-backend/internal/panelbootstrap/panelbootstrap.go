// Package panelbootstrap 在进程启动时把内置模板/内置规则集播种进数据库,
// 并顺手把内置规则集的远程内容抓一遍(抓不到不影响启动)。
//
// 之所以放在独立包里:播种要同时摸 subscribe_template 和 ruleset 两张表,
// 塞进 httpapi 或 store 都会让那两层多背一份「首次启动」的业务含义。
package panelbootstrap

import (
	"context"
	"strings"
	"fmt"

	"github.com/ziyue67/tms/go-backend/internal/paneldefaults"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

// Store 播种需要的最小存储面。
type Store interface {
	SubscribeTemplateByName(context.Context, string) (store.SubscribeTemplate, error)
	CreateBuiltinSubscribeTemplate(context.Context, store.TemplateInput) (store.SubscribeTemplate, error)
	SetSubscribeTemplateDefault(context.Context, int64) error
	RulesetByName(context.Context, string) (store.Ruleset, error)
	CreateRuleset(context.Context, store.RulesetInput) (store.Ruleset, error)
	MarkRulesetFetched(context.Context, int64, string, string) error
}

// Result 播种结果,只用于打日志。
type Result struct {
	TemplatesAdded int
	RulesetsAdded  int
	DefaultFixed   bool
}

// Seed 幂等:同名模板/规则集已存在就跳过 —— 管理员改过的内容不会被升级覆盖。
func Seed(ctx context.Context, s Store) (Result, error) {
	var result Result
	defaultSeen := false
	for _, tpl := range paneldefaults.Templates() {
		existing, err := s.SubscribeTemplateByName(ctx, tpl.Name)
		if err == nil && existing.ID > 0 {
			if existing.IsDefault == 1 {
				defaultSeen = true
			}
			continue
		}
		input := store.TemplateInput{
			Name: tpl.Name, Client: tpl.Client, Version: tpl.Version,
			IsDefault: 0, Visible: 1, Content: tpl.Content, Remark: tpl.Remark,
		}
		created, err := s.CreateBuiltinSubscribeTemplate(ctx, input)
		if err != nil {
			return result, fmt.Errorf("播种内置模板 %s: %w", tpl.Name, err)
		}
		result.TemplatesAdded++
		if tpl.Default && !defaultSeen {
			if err := s.SetSubscribeTemplateDefault(ctx, created.ID); err != nil {
				return result, fmt.Errorf("设置默认模板 %s: %w", tpl.Name, err)
			}
			defaultSeen = true
			result.DefaultFixed = true
		}
	}
	// 一个默认模板都没有(比如用户把默认的删了)时,把内置推荐模板顶上,
	// 否则所有「没选模板」的订阅链接都会走内置兜底配置。
	if !defaultSeen {
		for _, tpl := range paneldefaults.Templates() {
			if !tpl.Default {
				continue
			}
			if existing, err := s.SubscribeTemplateByName(ctx, tpl.Name); err == nil && existing.ID > 0 {
				if err := s.SetSubscribeTemplateDefault(ctx, existing.ID); err != nil {
					return result, fmt.Errorf("补默认模板: %w", err)
				}
				result.DefaultFixed = true
			}
			break
		}
	}

	for _, rs := range paneldefaults.Rulesets() {
		if existing, err := s.RulesetByName(ctx, rs.Name); err == nil && existing.ID > 0 {
			continue
		}
		if _, err := s.CreateRuleset(ctx, store.RulesetInput{
			Name: rs.Name, DisplayName: rs.DisplayName, Source: rs.Source,
			RemoteURL: rs.RemoteURL, RefreshMinutes: rs.RefreshMinutes, Content: rs.Content,
		}); err != nil {
			// 并发播种 / 判重时字段扫失败都会走到这里;撞唯一键说明已经存在,跳过即可,
			// 不能因为一条把剩下 20 多个分类全拦住。
			if isDuplicateKey(err) {
				continue
			}
			return result, fmt.Errorf("播种内置规则集 %s: %w", rs.Name, err)
		}
		result.RulesetsAdded++
	}
	return result, nil
}

// isDuplicateKey 判断唯一键冲突。MySQL 1062 / PostgreSQL 23505 / SQLite 都覆盖。
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "duplicate") ||
		strings.Contains(text, "23505") ||
		strings.Contains(text, "unique constraint") ||
		strings.Contains(text, "duplicate key")
}
