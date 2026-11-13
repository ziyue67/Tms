import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input } from "@heroui/input";
import { Chip } from "@heroui/chip";
import { Select, SelectItem } from "@heroui/select";
import { Table, TableBody, TableCell, TableColumn, TableHeader, TableRow } from "@heroui/table";
import { Spinner } from "@heroui/spinner";
import { Snippet } from "@heroui/snippet";
import toast from "react-hot-toast";
import PageWrapper from "@/components/page-wrapper";
import { RULE_PRESETS, categoryEmoji, presetRulesetIDs, type RulePreset } from "@/utils/panel-category";
import { CopyIcon, PlusIcon } from "@/components/icons";
import {
  createPanelLink,
  deletePanelLink,
  getCustomNodes,
  getPanelLinks,
  getPanelRulesets,
  getPanelTemplates,
  getAllUsers,
  resetPanelLinkToken,
  updatePanelLink,
  type PanelLink,
  type PanelRuleset,
  type PanelTemplate,
} from "@/api";
import type { User } from "@/types";

/**
 * 订阅链接(对齐妙妙屋X):卡片 / 列表 / 聚合订阅 / 新建订阅 四个视图。
 *
 * 一条订阅链接自带 node_ids —— 新建订阅时从「节点管理」里挑节点,输出就按这些节点渲染;
 * 老链接没有 node_ids,仍走「借用车友 token」的旧路径。
 *
 * custom_node.id 是 64 位雪花值,全程按字符串处理。
 */

const FORMAT_LABEL: Record<string, string> = { clash: "Clash/Mihomo", v2ray: "V2Ray", singbox: "sing-box" };
const FORMAT_CLIENTS: Record<string, string[]> = { clash: ["clash"], singbox: ["singbox"], v2ray: [] };

interface PanelNodeItem {
  id: string;
  name: string;
  protocol: string;
  status: number;
}

interface FormState {
  name: string;
  format: "clash" | "v2ray" | "singbox";
  mode: "template" | "custom";
  /** 自定义规则时套用的方案 */
  preset: RulePreset;
  templateId: string;
  rulesetIds: string[];
  nodeIds: string[];
  /** 归属账号:选了就输出该账号自己的节点,并出现在他的「我的订阅链接」 */
  userId: string;
}


const EMPTY_FORM: FormState = {
  name: "",
  format: "clash",
  // 参考面板默认就是「自定义规则 + 均衡规则(推荐)」,这里保持一致
  mode: "custom",
  preset: "balanced",
  templateId: "",
  rulesetIds: [],
  nodeIds: [],
  userId: "",
};

type ViewMode = "cards" | "list" | "aggregate" | "create";

export default function SubscribeLinksPage() {
  const [links, setLinks] = useState<PanelLink[]>([]);
  const [templates, setTemplates] = useState<PanelTemplate[]>([]);
  const [rulesets, setRulesets] = useState<PanelRuleset[]>([]);
  const [nodes, setNodes] = useState<PanelNodeItem[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [view, setView] = useState<ViewMode>("cards");

  const [editing, setEditing] = useState<PanelLink | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);

  // 聚合订阅表单
  const [aggregate, setAggregate] = useState({ name: "", remark: "", templateId: "", sources: [] as string[] });
  const [aggregating, setAggregating] = useState(false);
  const [presetCollapsed, setPresetCollapsed] = useState(false);

  const load = useCallback(async () => {
    try {
      const [lr, tr, rr, nr, ur] = await Promise.all([
        getPanelLinks(),
        getPanelTemplates(),
        getPanelRulesets(),
        getCustomNodes(),
        getAllUsers(),
      ]);
      if (lr.code === 0) setLinks(lr.data || []);
      if (tr.code === 0) setTemplates(tr.data || []);
      if (rr.code === 0) setRulesets(rr.data || []);
      if (nr.code === 0) setNodes((nr.data || []) as PanelNodeItem[]);
      if (ur.code === 0) setUsers((ur.data || []) as User[]);
    } catch {
      toast.error("加载失败");
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const enabledNodes = useMemo(() => nodes.filter((node) => node.status === 1), [nodes]);

  // 方案 -> 规则集 id 列表(公共实现,用户端同款)
  const presetIDs = useCallback((preset: RulePreset) => presetRulesetIDs(rulesets, preset), [rulesets]);

  // 规则集加载完(或切到自定义规则)时,把「均衡规则」的方案真正勾上
  useEffect(() => {
    if (form.mode !== "custom" || form.preset !== "balanced" || rulesets.length === 0) return;
    const ids = presetIDs("balanced");
    setForm((prev) =>
      prev.mode === "custom" && prev.preset === "balanced" && prev.rulesetIds.length !== ids.length
        ? { ...prev, rulesetIds: ids }
        : prev
    );
  }, [form.mode, form.preset, presetIDs, rulesets.length]);

  const defaultTemplateId = useMemo(() => {
    const byDefault = templates.find((tpl) => tpl.isDefault === 1);
    return byDefault ? String(byDefault.id) : "";
  }, [templates]);

  const openCreate = () => {
    setEditing(null);
    // 直接把「均衡规则」勾上:effect 依赖没变时不会重跑,不能指望它兜底
    setForm({ ...EMPTY_FORM, templateId: defaultTemplateId, rulesetIds: presetIDs("balanced") });
    setView("create");
  };

  const openEdit = (item: PanelLink) => {
    setEditing(item);
    setForm({
      name: item.name,
      format: (item.format as FormState["format"]) || "clash",
      mode: item.rulesetIds ? "custom" : "template",
      preset: "manual",
      templateId: item.templateId ? String(item.templateId) : "",
      rulesetIds: item.rulesetIds ? item.rulesetIds.split(",").filter(Boolean) : [],
      nodeIds: item.nodeIds ? item.nodeIds.split(",").filter(Boolean) : [],
      userId: item.userId ? String(item.userId) : "",
    });
    setView("create");
  };

  const toggleNode = (id: string) => {
    setForm((prev) => ({
      ...prev,
      nodeIds: prev.nodeIds.includes(id) ? prev.nodeIds.filter((item) => item !== id) : [...prev.nodeIds, id],
    }));
  };

  const toggleRuleset = (id: string) => {
    setForm((prev) => ({
      ...prev,
      rulesetIds: prev.rulesetIds.includes(id)
        ? prev.rulesetIds.filter((item) => item !== id)
        : [...prev.rulesetIds, id],
    }));
  };

  const submit = async () => {
    if (!form.name.trim()) {
      toast.error("给这条订阅起个名字");
      return;
    }
    if (!form.userId && form.nodeIds.length === 0) {
      toast.error("选个归属账号,或者至少挑一个节点");
      return;
    }
    const payload = {
      name: form.name.trim(),
      format: form.format,
      userId: form.userId ? Number(form.userId) : 0,
      nodeIds: form.userId ? "" : form.nodeIds.join(","),
      templateId: form.mode === "template" && form.templateId ? form.templateId : 0,
      rulesetIds: form.mode === "custom" ? form.rulesetIds.join(",") : "",
      expireAt: editing?.expireAt || 0,
    };
    setSaving(true);
    try {
      const res = editing ? await updatePanelLink(editing.id, payload) : await createPanelLink(payload);
      if (res.code === 0) {
        toast.success(editing ? "已保存" : "订阅已生成,地址在卡片里复制");
        setView("cards");
        await load();
      } else {
        toast.error(res.msg || "保存失败");
      }
    } catch {
      toast.error("保存失败");
    }
    setSaving(false);
  };

  const handleResetToken = async (item: PanelLink) => {
    if (!window.confirm(`重置「${item.name}」的 token?已导入客户端的旧链接会立即失效。`)) return;
    try {
      const res = await resetPanelLinkToken(item.id);
      if (res.code === 0) {
        toast.success("已重置,请用新链接重新导入");
        await load();
      } else {
        toast.error(res.msg || "重置失败");
      }
    } catch {
      toast.error("重置失败");
    }
  };

  const handleDelete = async (item: PanelLink) => {
    if (!window.confirm(`删除订阅「${item.name}」?`)) return;
    try {
      const res = await deletePanelLink(item.id);
      if (res.code === 0) {
        toast.success("已删除");
        await load();
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch {
      toast.error("删除失败");
    }
  };

  // 聚合订阅:把选中的几条订阅的节点并成一条新链接(取节点 id 的并集)。
  const createAggregate = async () => {
    if (!aggregate.name.trim()) {
      toast.error("给聚合订阅起个名字");
      return;
    }
    const picked = links.filter((item) => aggregate.sources.includes(String(item.id)));
    if (picked.length < 2) {
      toast.error("至少选两条源订阅");
      return;
    }
    const union = new Set<string>();
    picked.forEach((item) => (item.nodeIds || "").split(",").forEach((id) => id && union.add(id)));
    if (union.size === 0) {
      toast.error("选中的订阅都没有节点,先编辑它们选节点");
      return;
    }
    setAggregating(true);
    try {
      const res = await createPanelLink({
        name: aggregate.name.trim(),
        format: picked[0].format,
        nodeIds: Array.from(union).join(","),
        templateId: aggregate.templateId || 0,
        rulesetIds: "",
        expireAt: 0,
      });
      if (res.code === 0) {
        toast.success(`已合并 ${union.size} 个节点`);
        setAggregate({ name: "", remark: "", templateId: "", sources: [] });
        setView("cards");
        await load();
      } else {
        toast.error(res.msg || "聚合失败");
      }
    } catch {
      toast.error("聚合失败");
    }
    setAggregating(false);
  };

  const subUrl = (token: string) => `${window.location.origin}/api/v1/open_api/panel/${token}`;

  const copyUrl = async (token: string) => {
    try {
      await navigator.clipboard.writeText(subUrl(token));
      toast.success("订阅地址已复制");
    } catch {
      toast.error("复制失败");
    }
  };

  const nodeCount = (item: PanelLink) => (item.nodeIds ? item.nodeIds.split(",").filter(Boolean).length : 0);
  const rulesetCount = (item: PanelLink) => (item.rulesetIds ? item.rulesetIds.split(",").filter(Boolean).length : 0);
  const ownerName = (item: PanelLink) => {
    if (!item.userId) return "";
    const account = users.find((entry) => entry.id === item.userId);
    return account ? account.user : `#${item.userId}`;
  };
  const templateName = (item: PanelLink) =>
    item.templateId ? templates.find((tpl) => tpl.id === item.templateId)?.name || `#${item.templateId}` : "默认模板";

  const views: { key: ViewMode; label: string }[] = [
    { key: "cards", label: "卡片" },
    { key: "list", label: "列表" },
    { key: "aggregate", label: "聚合订阅" },
    { key: "create", label: editing ? "编辑订阅" : "新建订阅" },
  ];

  const createPanel = (
    <div className="flex flex-col gap-4">
      <div>
        <div className="text-lg font-semibold">{editing ? `编辑订阅 · ${editing.name}` : "新建订阅"}</div>
        <div className="text-xs text-default-500">从节点管理中选择节点,快速生成 Clash 订阅配置</div>
      </div>

      {/* 归属账号 / 选择节点 */}
      <div className="rounded-large border border-default-200">
        <div className="border-b border-default-200 px-4 py-3">
          <div className="text-sm font-medium">{form.userId ? "归属账号" : "选择节点"}</div>
          <div className="text-xs text-default-400">
            {form.userId
              ? "这条订阅会输出该账号自己的线路,并出现在他的「我的订阅链接」里"
              : `从已保存的节点中选择需要添加到订阅的节点(已选择 ${form.nodeIds.length} 个)`}
          </div>
        </div>
        <div className="p-4">
          <div className="mb-2 text-xs text-default-400">
            选账号 = 只给这个账号用;不选账号也不挑节点 = 保存成 <b>订阅集(全局规则)</b>,
            所有用户的「我的订阅」里那条 Clash 订阅链接都会自动套用这套模板和规则,用户不用做任何选择。
          </div>
          <Select
            className="mb-3"
            label="归属账号(可选)"
            selectedKeys={[form.userId || "none"]}
            onSelectionChange={(keys) => {
              const value = String(Array.from(keys)[0] ?? "none");
              setForm((prev) => ({
                ...prev,
                userId: value === "none" ? "" : value,
                nodeIds: value === "none" ? prev.nodeIds : [],
              }));
            }}
          >
            {[{ key: "none", label: "不绑定 —— 用下面挑的节点" }, ...users.map((account) => ({
              key: String(account.id),
              label: `${account.user}${account.name ? " · " + account.name : ""}`,
            }))].map((option) => (
              <SelectItem key={option.key}>{option.label}</SelectItem>
            ))}
          </Select>
          {form.userId ? (
            <div className="py-6 text-center text-xs text-default-400">
              已绑定账号:节点按账号自己的线路输出,不用在这里挑
            </div>
          ) : enabledNodes.length === 0 ? (
            <div className="py-8 text-center text-xs text-default-400">暂无可用节点,请先在节点管理中添加节点</div>
          ) : (
            <>
              <div className="mb-2 flex justify-end gap-2">
                <Button
                  size="sm"
                  variant="light"
                  onPress={() => setForm({ ...form, nodeIds: enabledNodes.map((node) => node.id) })}
                >
                  全选
                </Button>
                <Button size="sm" variant="light" onPress={() => setForm({ ...form, nodeIds: [] })}>
                  清空
                </Button>
              </div>
              <div className="grid max-h-56 grid-cols-1 gap-1 overflow-auto sm:grid-cols-2 lg:grid-cols-3">
                {enabledNodes.map((node) => (
                  <label
                    key={node.id}
                    className="flex cursor-pointer items-center gap-2 rounded-small px-2 py-1.5 text-sm hover:bg-default-50 dark:hover:bg-default-100/10"
                  >
                    <input
                      checked={form.nodeIds.includes(node.id)}
                      className="h-4 w-4 accent-primary"
                      type="checkbox"
                      onChange={() => toggleNode(node.id)}
                    />
                    <span className="flex-1 truncate">{node.name}</span>
                    <span className="text-[11px] text-default-400">{node.protocol}</span>
                  </label>
                ))}
              </div>
            </>
          )}
        </div>
      </div>

      {/* 规则模式 */}
      <div>
        <div className="mb-2 text-sm font-medium">规则模式</div>
        <div className="grid grid-cols-2 gap-2">
          {[
            { key: "custom", label: "自定义规则" },
            { key: "template", label: "使用模板" },
          ].map((entry) => {
            const active = form.mode === entry.key;
            return (
              <button
                key={entry.key}
                className={
                  "rounded-medium border px-4 py-2.5 text-sm transition-colors " +
                  (active
                    ? "border-primary bg-primary font-medium text-white"
                    : "border-default-200 text-default-600 hover:bg-default-50 dark:hover:bg-default-100/10")
                }
                type="button"
                onClick={() => {
                  const mode = entry.key as FormState["mode"];
                  setForm((prev) =>
                    mode === "custom"
                      ? { ...prev, mode, preset: "balanced", rulesetIds: presetIDs("balanced") }
                      : { ...prev, mode }
                  );
                }}
              >
                {entry.label}
              </button>
            );
          })}
        </div>
      </div>

      {/* 规则选择 */}
      <div>
        <div className="mb-2 flex items-center gap-1">
          <span className="text-sm font-medium">规则选择</span>
          <span className="text-default-400" title="规则集会在渲染时插入模板 rules 的 MATCH 之前">
            <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 17h.01M9.5 9a2.5 2.5 0 115 0c0 1.5-2.5 2-2.5 3.5" strokeLinecap="round" />
            </svg>
          </span>
        </div>

        {form.mode === "template" ? (
          <Select
            label="订阅模板"
            selectedKeys={form.templateId ? [form.templateId] : []}
            onSelectionChange={(keys) => setForm({ ...form, templateId: String(Array.from(keys)[0] ?? "") })}
          >
            {templates
              .filter((tpl) => (FORMAT_CLIENTS[form.format] || []).includes(tpl.client))
              .map((tpl) => (
                <SelectItem key={String(tpl.id)}>
                  {`${tpl.name}${tpl.isDefault === 1 ? "(默认)" : ""}`}
                </SelectItem>
              ))}
          </Select>
        ) : rulesets.length === 0 ? (
          <div className="rounded-medium border border-dashed border-default-200 px-3 py-6 text-center text-xs text-default-400">
            还没有规则集,先去「规则集」新建。
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            <Select
              selectedKeys={[form.preset]}
              onSelectionChange={(keys) => {
                const preset = Array.from(keys)[0] as FormState["preset"];
                setForm((prev) => ({ ...prev, preset, rulesetIds: presetIDs(preset) }));
              }}
            >
              {RULE_PRESETS.map((entry) => (
                <SelectItem key={entry.key}>{entry.label}</SelectItem>
              ))}
            </Select>
            <div className="text-xs text-default-400">
              {RULE_PRESETS.find((entry) => entry.key === form.preset)?.hint}
            </div>

            <div className="rounded-medium border border-default-200">
              <button
                className="flex w-full items-center justify-between px-3 py-2 text-sm"
                type="button"
                onClick={() => setPresetCollapsed((value) => !value)}
              >
                <span className="text-default-600">已选择 {form.rulesetIds.length} 个类别</span>
                <span className="text-default-400">{presetCollapsed ? "▾" : "▴"}</span>
              </button>
              {!presetCollapsed && (
                <div className="grid max-h-72 grid-cols-1 gap-x-6 gap-y-1 overflow-auto border-t border-default-200 px-3 py-3 sm:grid-cols-2 lg:grid-cols-3">
                  {rulesets.map((ruleset) => (
                    <label
                      key={ruleset.id}
                      className="flex cursor-pointer items-center gap-2 rounded-small px-1 py-1 text-sm hover:bg-default-50 dark:hover:bg-default-100/10"
                    >
                      <input
                        checked={form.rulesetIds.includes(String(ruleset.id))}
                        className="h-4 w-4 accent-primary"
                        type="checkbox"
                        onChange={() => {
                          toggleRuleset(String(ruleset.id));
                          setForm((prev) => ({ ...prev, preset: "manual" }));
                        }}
                      />
                      <span>{categoryEmoji(ruleset)}</span>
                      <span className="flex-1 truncate">{ruleset.displayName || ruleset.name}</span>
                    </label>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* 底部操作 */}
      <div className="flex items-stretch gap-2">
        <Button
          className="flex-1"
          color="primary"
          isDisabled={form.nodeIds.length === 0}
          isLoading={saving}
          onPress={submit}
        >
          {editing ? "保存订阅" : "生成订阅文件"}
        </Button>
        <Button
          variant="bordered"
          onPress={() =>
            setForm({ ...EMPTY_FORM, templateId: defaultTemplateId, name: form.name, rulesetIds: presetIDs("balanced") })
          }
        >
          清空
        </Button>
      </div>

      <div className="text-xs text-default-400">
        <button className="underline" type="button" onClick={() => setView("cards")}>
          ← 返回订阅链接
        </button>
        {editing && ` · 当前地址:${subUrl(editing.token)}(编辑不改地址,要换地址用「重置 token」)`}
      </div>
    </div>
  );

  const aggregatePanel = (
    <Card>
      <CardBody className="gap-4">
        <div className="rounded-medium bg-default-100/60 p-3 text-xs text-default-500">
          把几条订阅的节点合成一条新订阅:客户里只留一条地址,节点跟着被合并的那几条走。
        </div>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <Input
            isRequired
            label="订阅名称"
            value={aggregate.name}
            onValueChange={(value) => setAggregate({ ...aggregate, name: value })}
          />
          <Input
            label="说明(可选)"
            value={aggregate.remark}
            onValueChange={(value) => setAggregate({ ...aggregate, remark: value })}
          />
        </div>
        <div>
          <div className="mb-2 text-sm font-medium">源订阅 *</div>
          {links.length === 0 ? (
            <div className="rounded-medium border border-dashed border-default-200 px-3 py-6 text-center text-xs text-default-400">
              还没有可选的订阅,先新建几条。
            </div>
          ) : (
            <div className="grid max-h-56 grid-cols-1 gap-1 overflow-auto rounded-medium border border-default-200 p-2 md:grid-cols-2">
              {links.map((item) => (
                <label
                  key={item.id}
                  className="flex cursor-pointer items-center gap-2 rounded-small px-2 py-1.5 text-sm hover:bg-default-50 dark:hover:bg-default-100/10"
                >
                  <input
                    checked={aggregate.sources.includes(String(item.id))}
                    className="h-4 w-4 accent-primary"
                    type="checkbox"
                    onChange={() =>
                      setAggregate((prev) => ({
                        ...prev,
                        sources: prev.sources.includes(String(item.id))
                          ? prev.sources.filter((id) => id !== String(item.id))
                          : [...prev.sources, String(item.id)],
                      }))
                    }
                  />
                  <span className="flex-1 truncate">{item.name}</span>
                  <span className="text-[11px] text-default-400">{nodeCount(item)} 节点</span>
                </label>
              ))}
            </div>
          )}
        </div>
        <Select
          label="绑定模板(可选)"
          selectedKeys={aggregate.templateId ? [aggregate.templateId] : []}
          onSelectionChange={(keys) => setAggregate({ ...aggregate, templateId: String(Array.from(keys)[0] ?? "") })}
        >
          {templates
            .filter((tpl) => tpl.client === "clash")
            .map((tpl) => (
              <SelectItem key={String(tpl.id)}>{tpl.name}</SelectItem>
            ))}
        </Select>
        <div className="flex items-center justify-end gap-2">
          <Button variant="light" onPress={() => setAggregate({ name: "", remark: "", templateId: "", sources: [] })}>
            取消
          </Button>
          <Button color="primary" isLoading={aggregating} onPress={createAggregate}>
            创建聚合订阅
          </Button>
        </div>
      </CardBody>
    </Card>
  );

  return (
    <PageWrapper
      title="订阅链接"
      description="复制链接、扫码导入,也可以在这里编辑、删除自己的订阅。一条订阅 = 一批节点 + 模板或规则集。"
    >
      <Card className="mb-4">
        <CardBody className="py-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="mr-auto whitespace-nowrap text-sm text-default-500">共 {links.length} 条订阅</div>
            <div className="flex items-center overflow-hidden rounded-medium border border-default-200">
              {views.map((entry) => (
                <button
                  key={entry.key}
                  className={
                    "px-2.5 py-1.5 text-[13px] transition-colors " +
                    (view === entry.key
                      ? "bg-default-100 font-medium text-foreground"
                      : "text-default-500 hover:bg-default-50 dark:hover:bg-default-100/10")
                  }
                  type="button"
                  onClick={() => {
                    if (entry.key === "create") {
                      openCreate();
                      return;
                    }
                    setView(entry.key);
                  }}
                >
                  {entry.label}
                </button>
              ))}
            </div>
            {view !== "create" && (
              <Button color="primary" size="sm" startContent={<PlusIcon size={18} />} onPress={openCreate}>
                新建订阅
              </Button>
            )}
          </div>
        </CardBody>
      </Card>

      {loading ? (
        <Card>
          <CardBody className="flex justify-center py-16">
            <Spinner />
          </CardBody>
        </Card>
      ) : view === "create" ? (
        createPanel
      ) : view === "aggregate" ? (
        aggregatePanel
      ) : view === "list" ? (
        <Card>
          <CardBody className="p-0">
            {links.length === 0 ? (
              <div className="px-6 py-16 text-center text-default-500">还没有订阅链接。</div>
            ) : (
              <Table aria-label="订阅列表" removeWrapper>
                <TableHeader>
                  <TableColumn>名称</TableColumn>
                  <TableColumn width={110}>格式</TableColumn>
                  <TableColumn width={80}>节点</TableColumn>
                  <TableColumn>模板 / 规则集</TableColumn>
                  <TableColumn width={80}>访问</TableColumn>
                  <TableColumn align="end" width={200}>
                    操作
                  </TableColumn>
                </TableHeader>
                <TableBody>
                  {links.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell>
                        <div className="font-medium">{item.name}</div>
                        <div className="max-w-md">
                          <Snippet classNames={{ base: "bg-default-100/60", pre: "text-[11px]" }} hideSymbol size="sm">
                            {subUrl(item.token)}
                          </Snippet>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Chip size="sm" variant="flat">
                          {FORMAT_LABEL[item.format] || item.format}
                        </Chip>
                      </TableCell>
                      <TableCell className="text-sm">{nodeCount(item)}</TableCell>
                      <TableCell className="text-xs text-default-500">
                        {templateName(item)}
                        {rulesetCount(item) > 0 && ` · ${rulesetCount(item)} 条规则集`}
                      </TableCell>
                      <TableCell className="text-sm">{item.visitCount}</TableCell>
                      <TableCell>
                        <div className="flex items-center justify-end gap-1">
                          <Button size="sm" variant="light" onPress={() => copyUrl(item.token)}>
                            复制
                          </Button>
                          <Button size="sm" variant="light" onPress={() => openEdit(item)}>
                            编辑
                          </Button>
                          <Button color="danger" size="sm" variant="light" onPress={() => handleDelete(item)}>
                            删除
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardBody>
        </Card>
      ) : links.length === 0 ? (
        <Card>
          <CardBody className="py-16 text-center text-default-500">
            还没有订阅,点「新建订阅」创建一个;从节点管理里挑节点,选模板或规则集,就能拿到一条可导入的地址。
          </CardBody>
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {links.map((item) => (
            <Card key={item.id}>
              <CardBody className="gap-3">
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-base font-semibold">{item.name}</span>
                      <Chip size="sm" variant="flat">
                        {FORMAT_LABEL[item.format] || item.format}
                      </Chip>
                      {item.shared === 1 && (
                        <Chip color="secondary" size="sm" variant="flat">
                          订阅集
                        </Chip>
                      )}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-default-400">
                      <span>
                        {item.userId
                          ? `归属 ${ownerName(item)}`
                          : item.shared === 1
                            ? "订阅集(用户可套用)"
                            : `${nodeCount(item)} 个节点`}
                      </span>
                      <span>{templateName(item)}</span>
                      <span>{rulesetCount(item)} 条规则集</span>
                      <span>访问 {item.visitCount}</span>
                    </div>
                  </div>
                  <Button isIconOnly size="sm" title="复制订阅地址" variant="light" onPress={() => copyUrl(item.token)}>
                    <CopyIcon size={16} />
                  </Button>
                </div>
                {item.shared === 1 ? (
                  <div className="rounded-medium bg-secondary/5 px-3 py-2 text-xs text-default-500">
                    这是全局订阅集:它本身不输出节点,但所有用户的「我的订阅 → Clash 订阅」会自动套用这套模板和规则。
                  </div>
                ) : (
                  <Snippet
                    classNames={{ base: "bg-default-100/60", pre: "text-[11px] break-all whitespace-pre-wrap" }}
                    hideSymbol
                    size="sm"
                  >
                    {subUrl(item.token)}
                  </Snippet>
                )}
                <div className="flex items-center justify-end gap-1">
                  <Button size="sm" variant="light" onPress={() => openEdit(item)}>
                    编辑
                  </Button>
                  {item.shared !== 1 && (
                    <Button color="warning" size="sm" variant="light" onPress={() => handleResetToken(item)}>
                      重置 token
                    </Button>
                  )}
                  <Button color="danger" size="sm" variant="light" onPress={() => handleDelete(item)}>
                    删除
                  </Button>
                </div>
              </CardBody>
            </Card>
          ))}
        </div>
      )}
    </PageWrapper>
  );
}
