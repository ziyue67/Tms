package panelbootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/ziyue67/tms/go-backend/internal/paneldefaults"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

// fakeStore 只实现播种路径要用的几个方法,够验证「幂等 + 默认模板」。
type fakeStore struct {
	templates map[string]store.SubscribeTemplate
	rulesets  map[string]store.Ruleset
	defaultID int64
	nextID    int64
	links     []store.SubscribeLink
}

func newFakeStore() *fakeStore {
	return &fakeStore{templates: map[string]store.SubscribeTemplate{}, rulesets: map[string]store.Ruleset{}, nextID: 1}
}

func (f *fakeStore) SubscribeTemplateByName(_ context.Context, name string) (store.SubscribeTemplate, error) {
	if item, ok := f.templates[name]; ok {
		return item, nil
	}
	return store.SubscribeTemplate{}, errors.New("not found")
}

func (f *fakeStore) CreateBuiltinSubscribeTemplate(_ context.Context, input store.TemplateInput) (store.SubscribeTemplate, error) {
	item := store.SubscribeTemplate{
		ID: f.nextID, Name: input.Name, Client: input.Client, Version: input.Version,
		IsDefault: input.IsDefault, Visible: input.Visible, Builtin: 1, Content: input.Content, Remark: input.Remark,
	}
	f.nextID++
	f.templates[input.Name] = item
	return item, nil
}

func (f *fakeStore) SetSubscribeTemplateDefault(_ context.Context, id int64) error {
	for name, item := range f.templates {
		item.IsDefault = 0
		if item.ID == id {
			item.IsDefault = 1
		}
		f.templates[name] = item
	}
	f.defaultID = id
	return nil
}

func (f *fakeStore) RulesetByName(_ context.Context, name string) (store.Ruleset, error) {
	if item, ok := f.rulesets[name]; ok {
		return item, nil
	}
	return store.Ruleset{}, errors.New("not found")
}

func (f *fakeStore) CreateRuleset(_ context.Context, input store.RulesetInput) (store.Ruleset, error) {
	item := store.Ruleset{ID: f.nextID, Name: input.Name, DisplayName: input.DisplayName, Source: input.Source, Size: len(input.Content)}
	f.nextID++
	f.rulesets[input.Name] = item
	return item, nil
}

func (f *fakeStore) MarkRulesetFetched(_ context.Context, _ int64, _ string, _ string) error {
	return nil
}

func TestSeedWritesBuiltinTemplatesAndRulesets(t *testing.T) {
	fake := newFakeStore()
	result, err := Seed(context.Background(), fake)
	if err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	if result.TemplatesAdded != len(paneldefaults.Templates()) {
		t.Fatalf("模板数量不对: %d", result.TemplatesAdded)
	}
	if result.RulesetsAdded != len(paneldefaults.Rulesets()) {
		t.Fatalf("规则集数量不对: %d", result.RulesetsAdded)
	}
	item, ok := fake.templates["fake_ip__v3.yaml"]
	if !ok {
		t.Fatal("默认模板没播种")
	}
	if item.IsDefault != 1 {
		t.Fatal("fake_ip__v3.yaml 应该是默认模板")
	}
	if item.Builtin != 1 || item.Content == "" {
		t.Fatal("内置模板的 builtin/content 不对")
	}
}

// 再跑一次不能重复插入,也不能把默认模板改掉。
func TestSeedIsIdempotent(t *testing.T) {
	fake := newFakeStore()
	if _, err := Seed(context.Background(), fake); err != nil {
		t.Fatal(err)
	}
	before := len(fake.templates)
	second, err := Seed(context.Background(), fake)
	if err != nil {
		t.Fatal(err)
	}
	if second.TemplatesAdded != 0 || second.RulesetsAdded != 0 {
		t.Fatalf("第二次播种又插入了数据: %+v", second)
	}
	if len(fake.templates) != before {
		t.Fatal("模板数量变了")
	}
	if fake.templates["fake_ip__v3.yaml"].IsDefault != 1 {
		t.Fatal("默认模板被改掉了")
	}
}

// 所有模板的默认位都被清掉时,播种要把推荐模板补回默认。
func TestSeedRestoresMissingDefault(t *testing.T) {
	fake := newFakeStore()
	if _, err := Seed(context.Background(), fake); err != nil {
		t.Fatal(err)
	}
	for name, item := range fake.templates {
		item.IsDefault = 0
		fake.templates[name] = item
	}
	fake.defaultID = 0
	if _, err := Seed(context.Background(), fake); err != nil {
		t.Fatal(err)
	}
	if fake.templates["fake_ip__v3.yaml"].IsDefault != 1 {
		t.Fatal("默认模板没有被补回来")
	}
}

// 判重时字段扫失败(老库 NULL 列)会误判「不存在」;此时插入撞唯一键要能跳过,
// 不能把剩下 26 个分类全拦住(线上 PostgreSQL 就是这样报 23505 的)。
type duplicateStore struct{ *fakeStore }

func (d *duplicateStore) RulesetByName(context.Context, string) (store.Ruleset, error) {
	return store.Ruleset{}, errors.New("扫描失败")
}

func (d *duplicateStore) CreateRuleset(context.Context, store.RulesetInput) (store.Ruleset, error) {
	return store.Ruleset{}, errors.New(`ERROR: duplicate key value violates unique constraint "ruleset_name_key" (SQLSTATE 23505)`)
}

func TestSeedSkipsDuplicateRuleset(t *testing.T) {
	store := &duplicateStore{newFakeStore()}
	result, err := Seed(context.Background(), store)
	if err != nil {
		t.Fatalf("撞唯一键不该让播种失败: %v", err)
	}
	if result.RulesetsAdded != 0 {
		t.Fatalf("重复的不该计入新增: %d", result.RulesetsAdded)
	}
	if result.TemplatesAdded == 0 {
		t.Fatal("模板应该照常播种")
	}
}

// 空表时会预置一条「全局订阅规则」:默认模板 + 均衡规则,让老订阅链接直接带规则。
func TestSeedCreatesGlobalSetWhenNoLinks(t *testing.T) {
	fake := newFakeStore()
	result, err := Seed(context.Background(), fake)
	if err != nil {
		t.Fatal(err)
	}
	if !result.GlobalSetCreated || len(fake.links) != 1 {
		t.Fatalf("应该预置一条全局订阅规则: %+v links=%d", result, len(fake.links))
	}
	set := fake.links[0]
	if set.Shared != 1 || set.TemplateID == 0 || set.RulesetIDs == "" {
		t.Fatalf("全局订阅规则内容不对: %+v", set)
	}
	// 第二次播种不能再建一条
	second, err := Seed(context.Background(), fake)
	if err != nil {
		t.Fatal(err)
	}
	if second.GlobalSetCreated || len(fake.links) != 1 {
		t.Fatal("已有链接时不该再建全局订阅规则")
	}
}

func (f *fakeStore) SubscribeLinks(context.Context) ([]store.SubscribeLink, error) {
	return f.links, nil
}

func (f *fakeStore) CreateSubscribeLink(_ context.Context, input store.LinkInput) (store.SubscribeLink, error) {
	item := store.SubscribeLink{
		ID: f.nextID, Name: input.Name, Format: input.Format, TemplateID: input.TemplateID,
		RulesetIDs: input.RulesetIDs, Shared: input.Shared, UserID: input.UserID,
	}
	f.nextID++
	f.links = append(f.links, item)
	return item, nil
}
