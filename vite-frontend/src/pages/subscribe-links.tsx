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
import { CopyIcon, PlusIcon } from "@/components/icons";
import {
  createPanelLink,
  deletePanelLink,
  getCustomNodes,
  getPanelLinks,
  getPanelRulesets,
  getPanelTemplates,
  resetPanelLinkToken,
  updatePanelLink,
  type PanelLink,
  type PanelRuleset,
  type PanelTemplate,
} from "@/api";

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
  templateId: string;
  rulesetIds: string[];
  nodeIds: string[];
}

const EMPTY_FORM: FormState = {
  name: "",
  format: "clash",
  mode: "template",
  templateId: "",
  rulesetIds: [],
  nodeIds: [],
};

type ViewMode = "cards" | "list" | "aggregate" | "create";

export default function SubscribeLinksPage() {
  const [links, setLinks] = useState<PanelLink[]>([]);
  const [templates, setTemplates] = useState<PanelTemplate[]>([]);
  const [rulesets, setRulesets] = useState<PanelRuleset[]>([]);
  const [nodes, setNodes] = useState<PanelNodeItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [view, setView] = useState<ViewMode>("cards");

  const [editing, setEditing] = useState<PanelLink | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);

  // 聚合订阅表单
  const [aggregate, setAggregate] = useState({ name: "", remark: "", templateId: "", sources: [] as string[] });
  const [aggregating, setAggregating] = useState(false);

  const load = useCallback(async () => {
    try {
      const [lr, tr, rr, nr] = await Promise.all([
        getPanelLinks(),
        getPanelTemplates(),
        getPanelRulesets(),
        getCustomNodes(),
      ]);
      if (lr.code === 0) setLinks(lr.data || []);
      if (tr.code === 0) setTemplates(tr.data || []);
      if (rr.code === 0) setRulesets(rr.data || []);
      if (nr.code === 0) setNodes((nr.data || []) as PanelNodeItem[]);
    } catch {
      toast.error("加载失败");
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const enabledNodes = useMemo(() => nodes.filter((node) => node.status === 1), [nodes]);

  const defaultTemplateId = useMemo(() => {
    const byDefault = templates.find((tpl) => tpl.isDefault === 1);
    return byDefault ? String(byDefault.id) : "";
  }, [templates]);

  const openCreate = () => {
    setEditing(null);
    setForm({ ...EMPTY_FORM, templateId: defaultTemplateId });
    setView("create");
  };

  const openEdit = (item: PanelLink) => {
    setEditing(item);
    setForm({
      name: item.name,
      format: (item.format as FormState["format"]) || "clash",
      mode: item.rulesetIds ? "custom" : "template",
      templateId: item.templateId ? String(item.templateId) : "",
      rulesetIds: item.rulesetIds ? item.rulesetIds.split(",").filter(Boolean) : [],
      nodeIds: item.nodeIds ? item.nodeIds.split(",").filter(Boolean) : [],
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
    if (form.nodeIds.length === 0) {
      toast.error("至少选一个节点,否则订阅里没有东西");
      return;
    }
    const payload = {
      name: form.name.trim(),
      format: form.format,
      nodeIds: form.nodeIds.join(","),
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
      <div className="flex items-center gap-2">
        <Button size="sm" variant="light" onPress={() => setView("cards")}>
          ← 返回订阅链接
        </Button>
        <div>
          <div className="text-lg font-semibold">{editing ? `编辑订阅 · ${editing.name}` : "新建订阅"}</div>
          <div className="text-xs text-default-500">从节点管理中选择节点,快速生成订阅配置</div>
        </div>
      </div>

      <Card>
        <CardBody className="gap-4">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <Input
              isRequired
              label="订阅名称"
              placeholder="例如 我的 Clash"
              value={form.name}
              onValueChange={(value) => setForm({ ...form, name: value })}
            />
            <Select
              label="输出格式"
              selectedKeys={[form.format]}
              onSelectionChange={(keys) => setForm({ ...form, format: Array.from(keys)[0] as FormState["format"] })}
            >
              {Object.entries(FORMAT_LABEL).map(([key, label]) => (
                <SelectItem key={key}>{label}</SelectItem>
              ))}
            </Select>
            <div className="flex items-end pb-3 text-xs text-default-400">已选择 {form.nodeIds.length} 个节点</div>
          </div>

          <div>
            <div className="mb-2 flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">选择节点</div>
                <div className="text-xs text-default-400">从已保存的节点中选择需要添加到订阅的节点</div>
              </div>
              <div className="flex gap-2">
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
            </div>
            {enabledNodes.length === 0 ? (
              <div className="rounded-medium border border-dashed border-default-200 px-3 py-8 text-center text-xs text-default-400">
                暂无可用节点,请先在「节点管理」中添加节点。
              </div>
            ) : (
              <div className="grid max-h-56 grid-cols-1 gap-1 overflow-auto rounded-medium border border-default-200 p-2 sm:grid-cols-2 lg:grid-cols-3">
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
            )}
          </div>

          <div>
            <div className="mb-2 text-sm font-medium">规则模式</div>
            <div className="grid grid-cols-2 gap-2">
              {[
                { key: "custom", label: "自定义规则", hint: "勾选规则集,插到 MATCH 之前" },
                { key: "template", label: "使用模板", hint: "套用模板里的策略组与规则" },
              ].map((entry) => {
                const active = form.mode === entry.key;
                return (
                  <button
                    key={entry.key}
                    className={
                      "rounded-medium border px-3 py-2 text-left transition-colors " +
                      (active
                        ? "border-primary bg-primary/10"
                        : "border-default-200 hover:bg-default-50 dark:hover:bg-default-100/10")
                    }
                    type="button"
                    onClick={() => setForm({ ...form, mode: entry.key as FormState["mode"] })}
                  >
                    <div className={"text-sm " + (active ? "font-medium text-primary" : "")}>{entry.label}</div>
                    <div className="text-[11px] text-default-400">{entry.hint}</div>
                  </button>
                );
              })}
            </div>
          </div>

          <div>
            <div className="mb-2 text-sm font-medium">规则选择</div>
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
              <div className="grid max-h-52 grid-cols-1 gap-1 overflow-auto rounded-medium border border-default-200 p-2 sm:grid-cols-2 lg:grid-cols-3">
                {rulesets.map((ruleset) => (
                  <label
                    key={ruleset.id}
                    className="flex cursor-pointer items-center gap-2 rounded-small px-2 py-1.5 text-sm hover:bg-default-50 dark:hover:bg-default-100/10"
                  >
                    <input
                      checked={form.rulesetIds.includes(String(ruleset.id))}
                      className="h-4 w-4 accent-primary"
                      type="checkbox"
                      onChange={() => toggleRuleset(String(ruleset.id))}
                    />
                    <span className="flex-1 truncate">{ruleset.displayName || ruleset.name}</span>
                  </label>
                ))}
              </div>
            )}
          </div>

          {editing && (
            <div className="text-xs text-default-400">
              当前地址:{subUrl(editing.token)}(编辑不改地址;要换地址用「重置 token」)
            </div>
          )}
        </CardBody>
      </Card>

      <div className="flex items-center justify-end gap-2">
        <Button
          variant="light"
          onPress={() => setForm({ ...EMPTY_FORM, templateId: defaultTemplateId, name: form.name })}
        >
          清空
        </Button>
        <Button color="primary" isLoading={saving} onPress={submit}>
          {editing ? "保存" : "生成订阅文件"}
        </Button>
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
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-default-400">
                      <span>{nodeCount(item)} 个节点</span>
                      <span>{templateName(item)}</span>
                      <span>{rulesetCount(item)} 条规则集</span>
                      <span>访问 {item.visitCount}</span>
                    </div>
                  </div>
                  <Button isIconOnly size="sm" title="复制订阅地址" variant="light" onPress={() => copyUrl(item.token)}>
                    <CopyIcon size={16} />
                  </Button>
                </div>
                <Snippet
                  classNames={{ base: "bg-default-100/60", pre: "text-[11px] break-all whitespace-pre-wrap" }}
                  hideSymbol
                  size="sm"
                >
                  {subUrl(item.token)}
                </Snippet>
                <div className="flex items-center justify-end gap-1">
                  <Button size="sm" variant="light" onPress={() => openEdit(item)}>
                    编辑
                  </Button>
                  <Button color="warning" size="sm" variant="light" onPress={() => handleResetToken(item)}>
                    重置 token
                  </Button>
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
