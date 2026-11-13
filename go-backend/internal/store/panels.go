package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// 妙妙屋四面板的存储层:规则集(ruleset)、订阅模板(subscribe_template)、订阅链接(subscribe_link)。
//
// 转发管理不加新表 —— forward/tunnel/landing 既有表就是它的数据面,
// 面板页只是把已有的 /forward /tunnel /landing 接口聚合展示。
//
// 三张表全部走 panelTables() 建的通用结构:列表不带大字段(content),
// 详情才取,避免几十条规则集各几百 KB 一起塞进响应。

// ---------- 规则集 ----------

type Ruleset struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	Source         string `json:"source"` // manual | remote
	RemoteURL      string `json:"remoteUrl"`
	RefreshMinutes int    `json:"refreshMinutes"`
	Size           int    `json:"size"`
	LastFetchAt    int64  `json:"lastFetchAt"`
	LastFetchError string `json:"lastFetchError"`
	Status         int    `json:"status"`
	CreatedTime    int64  `json:"createdTime"`
	UpdatedTime    int64  `json:"updatedTime"`
}

type RulesetInput struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	Source         string `json:"source"`
	RemoteURL      string `json:"remoteUrl"`
	RefreshMinutes int    `json:"refreshMinutes"`
	Content        string `json:"content"`
}

func (s *Store) Rulesets(ctx context.Context) ([]Ruleset, error) {
	query := "SELECT " + s.columns("id", "name") + ", COALESCE(" + s.quote("display_name") + ",''), COALESCE(" + s.quote("source") + ",'manual'), " + s.quote("remote_url") + ", COALESCE(" + s.quote("refresh_minutes") + ",0), " + s.quote("last_fetch_at") + ", COALESCE(" + s.quote("last_fetch_error") + ",''), COALESCE(" + s.quote("status") + ",1), " + s.quote("created_time") + ", " + s.quote("updated_time") + ", LENGTH(COALESCE(" + s.quote("content") + ",'')) FROM " + s.quote("ruleset") + " ORDER BY " + s.quote("name")
	rows, err := s.db.QueryContext(ctx, s.bind(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Ruleset{}
	for rows.Next() {
		var r Ruleset
		var remote sql.NullString
		var fetched sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &r.DisplayName, &r.Source, &remote, &r.RefreshMinutes, &fetched, &r.LastFetchError, &r.Status, &r.CreatedTime, &r.UpdatedTime, &r.Size); err != nil {
			return nil, err
		}
		r.RemoteURL, r.LastFetchAt = remote.String, fetched.Int64
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *Store) RulesetByID(ctx context.Context, id int64) (Ruleset, string, error) {
	query := "SELECT " + s.columns("id", "name") + ", COALESCE(" + s.quote("display_name") + ",''), COALESCE(" + s.quote("source") + ",'manual'), " + s.quote("remote_url") + ", COALESCE(" + s.quote("refresh_minutes") + ",0), " + s.quote("last_fetch_at") + ", COALESCE(" + s.quote("last_fetch_error") + ",''), COALESCE(" + s.quote("status") + ",1), " + s.quote("created_time") + ", " + s.quote("updated_time") + ", COALESCE(" + s.quote("content") + ",'') FROM " + s.quote("ruleset") + " WHERE " + s.quote("id") + "=? LIMIT 1"
	var r Ruleset
	var remote sql.NullString
	var fetched sql.NullInt64
	var content sql.NullString
	err := s.db.QueryRowContext(ctx, s.bind(query), id).Scan(&r.ID, &r.Name, &r.DisplayName, &r.Source, &remote, &r.RefreshMinutes, &fetched, &r.LastFetchError, &r.Status, &r.CreatedTime, &r.UpdatedTime, &content)
	if err != nil {
		return r, "", err
	}
	r.RemoteURL, r.LastFetchAt = remote.String, fetched.Int64
	r.Size = len(content.String)
	return r, content.String, nil
}

func (s *Store) RulesetContentByName(ctx context.Context, name string) (string, int64, error) {
	var content string
	var updated int64
	query := "SELECT COALESCE(" + s.quote("content") + ",''), " + s.quote("updated_time") + " FROM " + s.quote("ruleset") + " WHERE " + s.quote("name") + "=? AND COALESCE(" + s.quote("status") + ",1)=1 LIMIT 1"
	err := s.db.QueryRowContext(ctx, s.bind(query), name).Scan(&content, &updated)
	return content, updated, err
}

// RulesetByName 启动播种时按名字判重。
func (s *Store) RulesetByName(ctx context.Context, name string) (Ruleset, error) {
	// 用 COALESCE 兜住老库里的 NULL:扫成 NULL 会直接报错,播种就会误判「不存在」再插一次,
	// 结果撞 ruleset_name_key 唯一键(PostgreSQL 23505)。
	query := "SELECT " + s.columns("id", "name") + ", COALESCE(" + s.quote("display_name") + ",''), COALESCE(" + s.quote("source") + ",'manual'), " + s.quote("remote_url") + ", COALESCE(" + s.quote("refresh_minutes") + ",0), " + s.quote("last_fetch_at") + ", COALESCE(" + s.quote("last_fetch_error") + ",''), COALESCE(" + s.quote("status") + ",1), " + s.quote("created_time") + ", " + s.quote("updated_time") + ", LENGTH(COALESCE(" + s.quote("content") + ",'')) FROM " + s.quote("ruleset") + " WHERE " + s.quote("name") + "=? LIMIT 1"
	var r Ruleset
	var remote sql.NullString
	var fetched sql.NullInt64
	err := s.db.QueryRowContext(ctx, s.bind(query), name).Scan(&r.ID, &r.Name, &r.DisplayName, &r.Source, &remote, &r.RefreshMinutes, &fetched, &r.LastFetchError, &r.Status, &r.CreatedTime, &r.UpdatedTime, &r.Size)
	r.RemoteURL, r.LastFetchAt = remote.String, fetched.Int64
	return r, err
}

func (s *Store) CreateRuleset(ctx context.Context, input RulesetInput) (Ruleset, error) {
	now := time.Now().UnixMilli()
	query := "INSERT INTO " + s.quote("ruleset") + " (" + s.columns("name", "display_name", "source", "remote_url", "refresh_minutes", "content", "created_time", "updated_time") + ") VALUES (?,?,?,?,?,?,?,?)"
	res, err := s.db.ExecContext(ctx, s.bind(query), input.Name, input.DisplayName, input.Source, input.RemoteURL, input.RefreshMinutes, input.Content, now, now)
	if err != nil {
		return Ruleset{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		// Postgres 的 LastInsertId 不一定实现;用 RETURNING 之外的兜底:按 name 查回。
		row := s.db.QueryRowContext(ctx, s.bind("SELECT "+s.columns("id")+" FROM "+s.quote("ruleset")+" WHERE "+s.quote("name")+"=? LIMIT 1"), input.Name)
		if scanErr := row.Scan(&id); scanErr != nil {
			return Ruleset{}, err
		}
	}
	r, _, err := s.RulesetByID(ctx, id)
	return r, err
}

func (s *Store) UpdateRuleset(ctx context.Context, id int64, input RulesetInput) (Ruleset, error) {
	now := time.Now().UnixMilli()
	// SET 子句逐列拼:columns() 只出列名清单,占位符数量必须与 args 严格对齐。
	set := s.quote("name") + "=?, " + s.quote("display_name") + "=?, " + s.quote("source") + "=?, " + s.quote("remote_url") + "=?, " + s.quote("refresh_minutes") + "=?, " + s.quote("content") + "=?, " + s.quote("updated_time") + "=?"
	query := "UPDATE " + s.quote("ruleset") + " SET " + set + " WHERE " + s.quote("id") + "=?"
	if _, err := s.db.ExecContext(ctx, s.bind(query), input.Name, input.DisplayName, input.Source, input.RemoteURL, input.RefreshMinutes, input.Content, now, id); err != nil {
		return Ruleset{}, err
	}
	r, _, err := s.RulesetByID(ctx, id)
	return r, err
}

func (s *Store) DeleteRuleset(ctx context.Context, id int64) error {
	query := "DELETE FROM " + s.quote("ruleset") + " WHERE " + s.quote("id") + "=?"
	_, err := s.db.ExecContext(ctx, s.bind(query), id)
	return err
}

func (s *Store) MarkRulesetFetched(ctx context.Context, id int64, content string, fetchErr string) error {
	now := time.Now().UnixMilli()
	if fetchErr != "" {
		query := "UPDATE " + s.quote("ruleset") + " SET " + s.quote("last_fetch_at") + "=?, " + s.quote("last_fetch_error") + "=? WHERE " + s.quote("id") + "=?"
		_, err := s.db.ExecContext(ctx, s.bind(query), now, fetchErr, id)
		return err
	}
	query := "UPDATE " + s.quote("ruleset") + " SET " + s.quote("content") + "=?, " + s.quote("last_fetch_at") + "=?, " + s.quote("last_fetch_error") + "=?, " + s.quote("updated_time") + "=? WHERE " + s.quote("id") + "=?"
	_, err := s.db.ExecContext(ctx, s.bind(query), content, now, "", now, id)
	return err
}

// ---------- 订阅模板 ----------

// SubscribeTemplate 订阅模板。client 决定它属于哪个客户端(面板按客户端分标签),
// builtin=1 表示来自 paneldefaults 的内置模板(可「恢复内置内容」),
// visible=0 表示对普通用户隐藏(管理员自己用)。
type SubscribeTemplate struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Client      string `json:"client"`  // clash | surge | loon | quanx | singbox
	Version     string `json:"version"` // v2 | v3
	IsDefault   int    `json:"isDefault"`
	Visible     int    `json:"visible"`
	Builtin     int    `json:"builtin"`
	Content     string `json:"content"`
	Remark      string `json:"remark"`
	Status      int    `json:"status"`
	CreatedTime int64  `json:"createdTime"`
	UpdatedTime int64  `json:"updatedTime"`
}

type TemplateInput struct {
	Name      string `json:"name"`
	Client    string `json:"client"`
	Version   string `json:"version"`
	IsDefault int    `json:"isDefault"`
	Visible   int    `json:"visible"`
	Content   string `json:"content"`
	Remark    string `json:"remark"`
}

func (s *Store) SubscribeTemplates(ctx context.Context) ([]SubscribeTemplate, error) {
	query := "SELECT " + s.columns("id", "name", "client", "version", "is_default", "visible", "builtin", "remark", "status", "created_time", "updated_time") + " FROM " + s.quote("subscribe_template") + " ORDER BY " + s.quote("is_default") + " DESC, " + s.quote("id") + " ASC"
	rows, err := s.db.QueryContext(ctx, s.bind(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []SubscribeTemplate{}
	for rows.Next() {
		var t SubscribeTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Client, &t.Version, &t.IsDefault, &t.Visible, &t.Builtin, &t.Remark, &t.Status, &t.CreatedTime, &t.UpdatedTime); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) SubscribeTemplateByID(ctx context.Context, id int64) (SubscribeTemplate, error) {
	query := "SELECT " + s.columns("id", "name", "client", "version", "is_default", "visible", "builtin", "content", "remark", "status", "created_time", "updated_time") + " FROM " + s.quote("subscribe_template") + " WHERE " + s.quote("id") + "=? LIMIT 1"
	var t SubscribeTemplate
	err := s.db.QueryRowContext(ctx, s.bind(query), id).Scan(&t.ID, &t.Name, &t.Client, &t.Version, &t.IsDefault, &t.Visible, &t.Builtin, &t.Content, &t.Remark, &t.Status, &t.CreatedTime, &t.UpdatedTime)
	return t, err
}

// SubscribeTemplateByName 内置模板去重/按名字恢复用。
func (s *Store) SubscribeTemplateByName(ctx context.Context, name string) (SubscribeTemplate, error) {
	query := "SELECT " + s.columns("id", "name") + ", COALESCE(" + s.quote("client") + ",'clash'), COALESCE(" + s.quote("version") + ",'v3'), COALESCE(" + s.quote("is_default") + ",0), COALESCE(" + s.quote("visible") + ",1), COALESCE(" + s.quote("builtin") + ",0), COALESCE(" + s.quote("content") + ",''), COALESCE(" + s.quote("remark") + ",''), COALESCE(" + s.quote("status") + ",1), " + s.quote("created_time") + ", " + s.quote("updated_time") + " FROM " + s.quote("subscribe_template") + " WHERE " + s.quote("name") + "=? ORDER BY " + s.quote("id") + " ASC LIMIT 1"
	var t SubscribeTemplate
	err := s.db.QueryRowContext(ctx, s.bind(query), name).Scan(&t.ID, &t.Name, &t.Client, &t.Version, &t.IsDefault, &t.Visible, &t.Builtin, &t.Content, &t.Remark, &t.Status, &t.CreatedTime, &t.UpdatedTime)
	return t, err
}

func (s *Store) CreateSubscribeTemplate(ctx context.Context, input TemplateInput) (SubscribeTemplate, error) {
	now := time.Now().UnixMilli()
	query := "INSERT INTO " + s.quote("subscribe_template") + " (" + s.columns("name", "client", "version", "is_default", "visible", "content", "remark", "created_time", "updated_time") + ") VALUES (?,?,?,?,?,?,?,?,?)"
	res, err := s.db.ExecContext(ctx, s.bind(query), input.Name, input.Client, input.Version, input.IsDefault, input.Visible, input.Content, input.Remark, now, now)
	if err != nil {
		return SubscribeTemplate{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		row := s.db.QueryRowContext(ctx, s.bind("SELECT "+s.columns("id")+" FROM "+s.quote("subscribe_template")+" WHERE "+s.quote("name")+"=? ORDER BY "+s.quote("id")+" DESC LIMIT 1"), input.Name)
		if scanErr := row.Scan(&id); scanErr != nil {
			return SubscribeTemplate{}, err
		}
	}
	return s.SubscribeTemplateByID(ctx, id)
}

// CreateBuiltinSubscribeTemplate 只给启动播种用:带 builtin=1 落库。
func (s *Store) CreateBuiltinSubscribeTemplate(ctx context.Context, input TemplateInput) (SubscribeTemplate, error) {
	now := time.Now().UnixMilli()
	query := "INSERT INTO " + s.quote("subscribe_template") + " (" + s.columns("name", "client", "version", "is_default", "visible", "builtin", "content", "remark", "created_time", "updated_time") + ") VALUES (?,?,?,?,?,?,?,?,?,?)"
	res, err := s.db.ExecContext(ctx, s.bind(query), input.Name, input.Client, input.Version, input.IsDefault, input.Visible, 1, input.Content, input.Remark, now, now)
	if err != nil {
		return SubscribeTemplate{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		row := s.db.QueryRowContext(ctx, s.bind("SELECT "+s.columns("id")+" FROM "+s.quote("subscribe_template")+" WHERE "+s.quote("name")+"=? ORDER BY "+s.quote("id")+" DESC LIMIT 1"), input.Name)
		if scanErr := row.Scan(&id); scanErr != nil {
			return SubscribeTemplate{}, err
		}
	}
	return s.SubscribeTemplateByID(ctx, id)
}

func (s *Store) UpdateSubscribeTemplate(ctx context.Context, id int64, input TemplateInput) (SubscribeTemplate, error) {
	now := time.Now().UnixMilli()
	set := s.quote("name") + "=?, " + s.quote("client") + "=?, " + s.quote("version") + "=?, " + s.quote("is_default") + "=?, " + s.quote("visible") + "=?, " + s.quote("content") + "=?, " + s.quote("remark") + "=?, " + s.quote("updated_time") + "=?"
	query := "UPDATE " + s.quote("subscribe_template") + " SET " + set + " WHERE " + s.quote("id") + "=?"
	if _, err := s.db.ExecContext(ctx, s.bind(query), input.Name, input.Client, input.Version, input.IsDefault, input.Visible, input.Content, input.Remark, now, id); err != nil {
		return SubscribeTemplate{}, err
	}
	return s.SubscribeTemplateByID(ctx, id)
}

// SetSubscribeTemplateDefault 把一个模板设成默认,其余全部取消 —— 默认模板只能有一个。
func (s *Store) SetSubscribeTemplateDefault(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, s.bind("UPDATE "+s.quote("subscribe_template")+" SET "+s.quote("is_default")+"=0 WHERE "+s.quote("is_default")+"=1")); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.bind("UPDATE "+s.quote("subscribe_template")+" SET "+s.quote("is_default")+"=1, "+s.quote("updated_time")+"=? WHERE "+s.quote("id")+"=?"), time.Now().UnixMilli(), id)
	return err
}

// ResetSubscribeTemplateContent 恢复内置模板的原始内容(名字/客户端不变)。
func (s *Store) ResetSubscribeTemplateContent(ctx context.Context, id int64, content string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, s.bind("UPDATE "+s.quote("subscribe_template")+" SET "+s.quote("content")+"=?, "+s.quote("updated_time")+"=? WHERE "+s.quote("id")+"=?"), content, now, id)
	return err
}

func (s *Store) DeleteSubscribeTemplate(ctx context.Context, id int64) error {
	query := "DELETE FROM " + s.quote("subscribe_template") + " WHERE " + s.quote("id") + "=?"
	_, err := s.db.ExecContext(ctx, s.bind(query), id)
	return err
}

// DefaultTemplateID 拿 is_default=1 的模板;没有就返回 0(渲染时走内置兜底模板)。
func (s *Store) DefaultTemplateID(ctx context.Context) (int64, error) {
	var id int64
	query := "SELECT " + s.quote("id") + " FROM " + s.quote("subscribe_template") + " WHERE " + s.quote("is_default") + "=1 AND " + s.quote("status") + "=1 ORDER BY " + s.quote("id") + " DESC LIMIT 1"
	err := s.db.QueryRowContext(ctx, s.bind(query)).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// ---------- 订阅链接 ----------

type SubscribeLink struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Token       string `json:"token"`
	Format      string `json:"format"` // clash | v2ray | singbox
	TemplateID  int64  `json:"templateId"`
	RulesetIDs  string `json:"rulesetIds"` // 逗号分隔
	NodeIDs     string `json:"nodeIds"`    // 逗号分隔,引用 custom_node.id
	UserID      int64  `json:"userId"`     // 非 0 = 输出该账号自己的节点(用户自建订阅)
	Shared      int    `json:"shared"`     // 1 = 订阅集(无账号无节点),用户可套用
	GroupsJSON  string `json:"groupsJson"` // 分组订阅:[{name,nodeIds,rulesetIds}]
	ExpireAt    int64  `json:"expireAt"`
	VisitCount  int64  `json:"visitCount"`
	Status      int    `json:"status"`
	CreatedTime int64  `json:"createdTime"`
	UpdatedTime int64  `json:"updatedTime"`
}

type LinkInput struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	TemplateID int64  `json:"templateId"`
	RulesetIDs string `json:"rulesetIds"`
	NodeIDs    string `json:"nodeIds"`
	UserID     int64  `json:"userId"`
	Shared     int    `json:"shared"`
	GroupsJSON string `json:"groupsJson"`
	ExpireAt   int64  `json:"expireAt"`
}

// newSubToken 生成 32 hex 的订阅 token。crypto/rand 失败直接报错,不用时间戳兜底 ——
// token 是订阅链接唯一的凭据,可预测等于把节点白送。
func newSubToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Store) SubscribeLinks(ctx context.Context) ([]SubscribeLink, error) {
	query := "SELECT " + s.columns("id", "name", "token", "format", "template_id", "ruleset_ids", "node_ids", "user_id", "shared", "groups_json", "expire_at", "visit_count", "status", "created_time", "updated_time") + " FROM " + s.quote("subscribe_link") + " ORDER BY " + s.quote("id") + " DESC"
	rows, err := s.db.QueryContext(ctx, s.bind(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []SubscribeLink{}
	for rows.Next() {
		var l SubscribeLink
		var templateID sql.NullInt64
		var nodeIDs sql.NullString
		var userID sql.NullInt64
		var groups sql.NullString
		if err := rows.Scan(&l.ID, &l.Name, &l.Token, &l.Format, &templateID, &l.RulesetIDs, &nodeIDs, &userID, &l.Shared, &groups, &l.ExpireAt, &l.VisitCount, &l.Status, &l.CreatedTime, &l.UpdatedTime); err != nil {
			return nil, err
		}
		l.TemplateID, l.NodeIDs, l.UserID, l.GroupsJSON = templateID.Int64, nodeIDs.String, userID.Int64, groups.String
		list = append(list, l)
	}
	return list, rows.Err()
}

func (s *Store) SubscribeLinkByID(ctx context.Context, id int64) (SubscribeLink, error) {
	query := "SELECT " + s.columns("id", "name", "token", "format", "template_id", "ruleset_ids", "node_ids", "user_id", "shared", "groups_json", "expire_at", "visit_count", "status", "created_time", "updated_time") + " FROM " + s.quote("subscribe_link") + " WHERE " + s.quote("id") + "=? LIMIT 1"
	var l SubscribeLink
	var templateID sql.NullInt64
	var nodeIDs sql.NullString
	var userID sql.NullInt64
	var groups sql.NullString
	err := s.db.QueryRowContext(ctx, s.bind(query), id).Scan(&l.ID, &l.Name, &l.Token, &l.Format, &templateID, &l.RulesetIDs, &nodeIDs, &userID, &l.Shared, &groups, &l.ExpireAt, &l.VisitCount, &l.Status, &l.CreatedTime, &l.UpdatedTime)
	l.TemplateID, l.NodeIDs, l.UserID, l.GroupsJSON = templateID.Int64, nodeIDs.String, userID.Int64, groups.String
	return l, err
}

// SubscribeLinkByToken 公开下载端点用:只认 status=1 且未过期的。
func (s *Store) SubscribeLinkByToken(ctx context.Context, token string) (SubscribeLink, error) {
	query := "SELECT " + s.columns("id", "name", "token", "format", "template_id", "ruleset_ids", "node_ids", "user_id", "shared", "groups_json", "expire_at", "visit_count", "status", "created_time", "updated_time") + " FROM " + s.quote("subscribe_link") + " WHERE " + s.quote("token") + "=? AND " + s.quote("status") + "=1 LIMIT 1"
	var l SubscribeLink
	var templateID sql.NullInt64
	var nodeIDs sql.NullString
	var userID sql.NullInt64
	var groups sql.NullString
	err := s.db.QueryRowContext(ctx, s.bind(query), token).Scan(&l.ID, &l.Name, &l.Token, &l.Format, &templateID, &l.RulesetIDs, &nodeIDs, &userID, &l.Shared, &groups, &l.ExpireAt, &l.VisitCount, &l.Status, &l.CreatedTime, &l.UpdatedTime)
	l.TemplateID, l.NodeIDs, l.UserID, l.GroupsJSON = templateID.Int64, nodeIDs.String, userID.Int64, groups.String
	if err != nil {
		return l, err
	}
	if l.ExpireAt > 0 && l.ExpireAt <= time.Now().UnixMilli() {
		return l, fmt.Errorf("订阅链接已过期")
	}
	return l, nil
}

func (s *Store) CreateSubscribeLink(ctx context.Context, input LinkInput) (SubscribeLink, error) {
	token, err := newSubToken()
	if err != nil {
		return SubscribeLink{}, err
	}
	now := time.Now().UnixMilli()
	// template_id / user_id 为 0 时写 NULL;占位符按列数自动生成,避免加列时数错。
	columns := []string{"name", "token", "format", "template_id", "ruleset_ids", "node_ids", "user_id", "shared", "groups_json", "expire_at", "created_time", "updated_time"}
	query := "INSERT INTO " + s.quote("subscribe_link") + " (" + s.columns(columns...) + ") VALUES (" + placeholders(len(columns)) + ")"
	var templateID, userID any
	if input.TemplateID > 0 {
		templateID = input.TemplateID
	}
	if input.UserID > 0 {
		userID = input.UserID
	}
	var groups any
	if input.GroupsJSON != "" {
		groups = input.GroupsJSON
	}
	res, err := s.db.ExecContext(ctx, s.bind(query), input.Name, token, input.Format, templateID, input.RulesetIDs, input.NodeIDs, userID, input.Shared, groups, input.ExpireAt, now, now)
	if err != nil {
		return SubscribeLink{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		row := s.db.QueryRowContext(ctx, s.bind("SELECT "+s.columns("id")+" FROM "+s.quote("subscribe_link")+" WHERE "+s.quote("token")+"=? LIMIT 1"), token)
		if scanErr := row.Scan(&id); scanErr != nil {
			return SubscribeLink{}, err
		}
	}
	return s.SubscribeLinkByID(ctx, id)
}

func (s *Store) UpdateSubscribeLink(ctx context.Context, id int64, input LinkInput) (SubscribeLink, error) {
	now := time.Now().UnixMilli()
	set := s.quote("name") + "=?, " + s.quote("format") + "=?, " + s.quote("ruleset_ids") + "=?, " + s.quote("node_ids") + "=?, " + s.quote("shared") + "=?, " + s.quote("groups_json") + "=?, " + s.quote("expire_at") + "=?, " + s.quote("updated_time") + "=?"
	groupsValue := any(nil)
	if input.GroupsJSON != "" {
		groupsValue = input.GroupsJSON
	}
	args := []any{input.Name, input.Format, input.RulesetIDs, input.NodeIDs, input.Shared, groupsValue, input.ExpireAt, now}
	if input.TemplateID > 0 {
		set += ", " + s.quote("template_id") + "=?"
		args = append(args, input.TemplateID)
	} else {
		set += ", " + s.quote("template_id") + "=NULL"
	}
	if input.UserID > 0 {
		set += ", " + s.quote("user_id") + "=?"
		args = append(args, input.UserID)
	}
	args = append(args, id)
	query := "UPDATE " + s.quote("subscribe_link") + " SET " + set + " WHERE " + s.quote("id") + "=?"
	if _, err := s.db.ExecContext(ctx, s.bind(query), args...); err != nil {
		return SubscribeLink{}, err
	}
	return s.SubscribeLinkByID(ctx, id)
}

// ResetSubscribeLinkToken 重置 token:旧链接立刻失效。
func (s *Store) ResetSubscribeLinkToken(ctx context.Context, id int64) (string, error) {
	token, err := newSubToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UnixMilli()
	query := "UPDATE " + s.quote("subscribe_link") + " SET " + s.quote("token") + "=?, " + s.quote("updated_time") + "=? WHERE " + s.quote("id") + "=?"
	if _, err := s.db.ExecContext(ctx, s.bind(query), token, now, id); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) DeleteSubscribeLink(ctx context.Context, id int64) error {
	query := "DELETE FROM " + s.quote("subscribe_link") + " WHERE " + s.quote("id") + "=?"
	_, err := s.db.ExecContext(ctx, s.bind(query), id)
	return err
}

func (s *Store) BumpSubscribeLinkVisits(ctx context.Context, id int64) error {
	query := "UPDATE " + s.quote("subscribe_link") + " SET " + s.quote("visit_count") + "=" + s.quote("visit_count") + "+1 WHERE " + s.quote("id") + "=?"
	_, err := s.db.ExecContext(ctx, s.bind(query), id)
	return err
}

// placeholders 生成 "?,?,?" —— 插入语句的占位符按列数自动算,避免列加了忘了补问号。
func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.Repeat("?,", count-1) + "?"
}

// NormalizeNodeIDs 与 NormalizeRulesetIDs 同款,给 custom_node.id 列表用。
func NormalizeNodeIDs(raw string) string { return NormalizeRulesetIDs(raw) }

// NormalizeRulesetIDs 把 "1, 2, 3" 之类输入洗成 "1,2";空返回空串。
func NormalizeRulesetIDs(raw string) string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

// ---------- 订阅链接可选的节点 ----------

// PanelNode 订阅链接里被选中的节点(custom_node 表)。
// 订阅输出不再要求「借用某个车友的 token」,而是直接按链接自带的 node_ids 渲染。
type PanelNode struct {
	ID       int64
	Name     string
	Protocol string
	RawLink  string
	Parsed   map[string]any
}

// PanelNodesByIDs 按 id 取节点,只取 status=1 的(停用的节点不该出现在订阅里)。
func (s *Store) PanelNodesByIDs(ctx context.Context, ids []int64) ([]PanelNode, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for index, id := range ids {
		placeholders[index] = "?"
		args[index] = id
	}
	query := "SELECT " + s.columns("id", "name", "protocol", "raw_link", "parsed_json") + " FROM " + s.quote("custom_node") + " WHERE " + s.quote("status") + "=1 AND " + s.quote("id") + " IN (" + strings.Join(placeholders, ",") + ") ORDER BY " + s.quote("id")
	rows, err := s.db.QueryContext(ctx, s.bind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []PanelNode{}
	for rows.Next() {
		var node PanelNode
		var raw, parsed sql.NullString
		if err := rows.Scan(&node.ID, &node.Name, &node.Protocol, &raw, &parsed); err != nil {
			return nil, err
		}
		node.RawLink = raw.String
		node.Parsed = map[string]any{}
		if parsed.String != "" {
			_ = json.Unmarshal([]byte(parsed.String), &node.Parsed)
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}
