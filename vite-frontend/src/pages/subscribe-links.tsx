import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input } from "@heroui/input";
import { Chip } from "@heroui/chip";
import { Select, SelectItem } from "@heroui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
} from "@heroui/table";
import { Spinner } from "@heroui/spinner";
import { Snippet } from "@heroui/snippet";
import toast from "react-hot-toast";
import PageWrapper from "@/components/page-wrapper";
import { CopyIcon, DeleteIcon, PlusIcon } from "@/components/icons";
import {
  createPanelLink,
  deletePanelLink,
  getAllUsers,
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
import type { User } from "@/types";
import {
  RULE_PRESETS,
  categoryEmoji,
  presetRulesetIDs,
  type RulePreset,
} from "@/utils/panel-category";

/**
 * 订阅链接(对齐妙妙屋X):卡片 / 列表 / 聚合订阅 / 新建订阅。
 *
 * 两种建法:
 *  - 自定义规则(分组):一条链接里多个「节点组 + 各自规则」,每个分组在客户端里是一个策略组;
 *    同一个节点可以放进多个分组(客户端里多个策略组都包含它),互不冲突;
 *  - 使用模板:一批节点 + 一份模板(旧行为);「全体用户」是一套全局规则。
 *
 * custom_node.id 是 64 位雪花值,全程按字符串处理。
 */

const FORMAT_LABEL: Record<string, string> = {
  clash: "Clash/Mihomo",
  v2ray: "V2Ray",
  singbox: "sing-box",
};
const FORMAT_CLIENTS: Record<string, string[]> = {
  clash: ["clash"],
  singbox: ["singbox"],
  v2ray: [],
};

interface PanelNodeItem {
  id: string;
  name: string;
  protocol: string;
  status: number;
}

interface GroupForm {
  name: string;
  nodeIds: string[];
  rulesetIds: string[];
}

interface FormState {
  name: string;
  format: "clash" | "v2ray" | "singbox";
  mode: "custom" | "template";
  templateId: string;
  userId: string;
  nodeIds: string[];
  groups: GroupForm[];
}

const EMPTY_FORM: FormState = {
  name: "",
  format: "clash",
  mode: "custom",
  templateId: "",
  userId: "",
  nodeIds: [],
  groups: [{ name: "分组1", nodeIds: [], rulesetIds: [] }],
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

  const [aggregate, setAggregate] = useState({
    name: "",
    remark: "",
    templateId: "",
    sources: [] as string[],
  });
  const [aggregating, setAggregating] = useState(false);

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

  const enabledNodes = useMemo(
    () => nodes.filter((node) => node.status === 1),
    [nodes],
  );

  const defaultTemplateId = useMemo(() => {
    const byDefault = templates.find((tpl) => tpl.isDefault === 1);
    return byDefault ? String(byDefault.id) : "";
  }, [templates]);

  const openCreate = () => {
    setEditing(null);
    setForm({
      ...EMPTY_FORM,
      templateId: defaultTemplateId,
      groups: [
        {
          name: "分组1",
          nodeIds: [],
          rulesetIds: presetRulesetIDs(rulesets, "balanced"),
        },
      ],
    });
    setView("create");
  };

  const openEdit = (item: PanelLink) => {
    let groups: GroupForm[] = [];
    if (item.groupsJson) {
      try {
        const parsed = JSON.parse(item.groupsJson) as GroupForm[];
        groups = parsed.map((group) => ({
          name: group.name,
          nodeIds: (group.nodeIds || []).map(String),
          rulesetIds: (group.rulesetIds || []).map(String),
        }));
      } catch {
        groups = [];
      }
    }
    // 老数据:单组(节点 + 规则集)转成一个分组
    if (groups.length === 0 && item.rulesetIds) {
      groups = [
        {
          name: "分组1",
          nodeIds: item.nodeIds ? item.nodeIds.split(",").filter(Boolean) : [],
          rulesetIds: item.rulesetIds.split(",").filter(Boolean),
        },
      ];
    }
    setEditing(item);
    setForm({
      name: item.name,
      format: (item.format as FormState["format"]) || "clash",
      mode: groups.length > 0 ? "custom" : "template",
      templateId: item.templateId ? String(item.templateId) : "",
      userId: item.userId
        ? String(item.userId)
        : item.shared === 1
          ? "all"
          : "",
      nodeIds: item.nodeIds ? item.nodeIds.split(",").filter(Boolean) : [],
      groups: groups.length > 0 ? groups : EMPTY_FORM.groups,
    });
    setView("create");
  };

  const updateGroup = (index: number, patch: Partial<GroupForm>) => {
    setForm((prev) => ({
      ...prev,
      groups: prev.groups.map((group, i) =>
        i === index ? { ...group, ...patch } : group,
      ),
    }));
  };

  const toggleInGroup = (
    index: number,
    key: "nodeIds" | "rulesetIds",
    id: string,
  ) => {
    setForm((prev) => ({
      ...prev,
      groups: prev.groups.map((group, i) => {
        if (i !== index) return group;
        const list = group[key];
        return {
          ...group,
          [key]: list.includes(id)
            ? list.filter((item) => item !== id)
            : [...list, id],
        };
      }),
    }));
  };

  const applyPreset = (index: number, preset: RulePreset) => {
    updateGroup(index, { rulesetIds: presetRulesetIDs(rulesets, preset) });
  };

  const addGroup = () => {
    setForm((prev) => ({
      ...prev,
      groups: [
        ...prev.groups,
        { name: `分组${prev.groups.length + 1}`, nodeIds: [], rulesetIds: [] },
      ],
    }));
  };

  const removeGroup = (index: number) => {
    setForm((prev) => ({
      ...prev,
      groups: prev.groups.filter((_, i) => i !== index),
    }));
  };

  const toggleNode = (id: string) => {
    setForm((prev) => ({
      ...prev,
      nodeIds: prev.nodeIds.includes(id)
        ? prev.nodeIds.filter((item) => item !== id)
        : [...prev.nodeIds, id],
    }));
  };

  const submit = async () => {
    if (!form.name.trim()) {
      toast.error("给这条订阅起个名字");
      return;
    }
    let payload;
    if (form.mode === "custom") {
      const groups = form.groups.filter((group) => group.nodeIds.length > 0);
      if (groups.length === 0) {
        toast.error("每个分组至少挑一个节点");
        return;
      }
      if (groups.some((group) => !group.name.trim())) {
        toast.error("分组要有名字");
        return;
      }
      payload = {
        name: form.name.trim(),
        format: form.format,
        groupsJson: JSON.stringify(groups),
        nodeIds: "",
        rulesetIds: "",
        templateId: 0,
        userId: 0,
        expireAt: editing?.expireAt || 0,
      };
    } else {
      if (!form.userId && form.nodeIds.length === 0) {
        toast.error("选个归属账号,或者至少挑一个节点");
        return;
      }
      payload = {
        name: form.name.trim(),
        format: form.format,
        userId: form.userId && form.userId !== "all" ? Number(form.userId) : 0,
        nodeIds: form.userId ? "" : form.nodeIds.join(","),
        rulesetIds: "",
        templateId: form.templateId ? form.templateId : 0,
        expireAt: editing?.expireAt || 0,
      };
    }
    setSaving(true);
    try {
      const res = editing
        ? await updatePanelLink(editing.id, payload)
        : await createPanelLink(payload);
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
    if (
      !window.confirm(
        `重置「${item.name}」的 token?已导入客户端的旧链接会立即失效。`,
      )
    )
      return;
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

  const createAggregate = async () => {
    if (!aggregate.name.trim()) {
      toast.error("给聚合订阅起个名字");
      return;
    }
    const picked = links.filter((item) =>
      aggregate.sources.includes(String(item.id)),
    );
    if (picked.length < 2) {
      toast.error("至少选两条源订阅");
      return;
    }
    const union = new Set<string>();
    picked.forEach((item) =>
      (item.nodeIds || "").split(",").forEach((id) => id && union.add(id)),
    );
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
        userId: 0,
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

  const subUrl = (token: string) =>
    `${window.location.origin}/api/v1/open_api/panel/${token}`;

  const copyUrl = async (token: string) => {
    try {
      await navigator.clipboard.writeText(subUrl(token));
      toast.success("订阅地址已复制");
    } catch {
      toast.error("复制失败");
    }
  };

  const nodeCount = (item: PanelLink) =>
    item.nodeIds ? item.nodeIds.split(",").filter(Boolean).length : 0;
  const groupCount = (item: PanelLink) => {
    if (!item.groupsJson) return 0;
    try {
      return (JSON.parse(item.groupsJson) as unknown[]).length;
    } catch {
      return 0;
    }
  };
  const ownerName = (item: PanelLink) => {
    if (!item.userId) return "";
    const account = users.find((entry) => entry.id === item.userId);
    return account ? account.user : `#${item.userId}`;
  };
  const templateName = (item: PanelLink) =>
    item.templateId
      ? templates.find((tpl) => tpl.id === item.templateId)?.name ||
        `#${item.templateId}`
      : "默认模板";

  const views: { key: ViewMode; label: string }[] = [
    { key: "cards", label: "卡片" },
    { key: "list", label: "列表" },
    { key: "aggregate", label: "聚合订阅" },
    { key: "create", label: editing ? "编辑订阅" : "新建订阅" },
  ];

  const groupEditor = (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2 text-xs text-default-400">
        <span>共 {enabledNodes.length} 个可用节点</span>
        <span>
          · 同一个节点可以放进多个分组(客户端里就是多个策略组都包含它)
        </span>
      </div>

      {form.groups.map((group, index) => (
        <div key={index} className="rounded-large border border-default-200">
          <div className="flex flex-wrap items-center gap-3 border-b border-default-200 px-4 py-2">
            <Input
              className="w-48"
              label="分组名称"
              size="sm"
              value={group.name}
              onValueChange={(value) => updateGroup(index, { name: value })}
            />
            <span className="text-xs text-default-400">
              已选 {group.nodeIds.length} 个节点 · {group.rulesetIds.length}{" "}
              个类别
            </span>
            <div className="flex-1" />
            <Button
              isIconOnly
              color="danger"
              isDisabled={form.groups.length <= 1}
              size="sm"
              title="删除这个分组"
              variant="light"
              onPress={() => removeGroup(index)}
            >
              <DeleteIcon size={16} />
            </Button>
          </div>

          <div className="grid gap-4 p-4 lg:grid-cols-2">
            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs text-default-500">
                  选择节点(可重复用)
                </span>
                <div className="flex gap-1">
                  <Button
                    size="sm"
                    variant="light"
                    onPress={() =>
                      updateGroup(index, {
                        nodeIds: enabledNodes.map((node) => node.id),
                      })
                    }
                  >
                    全选
                  </Button>
                  <Button
                    size="sm"
                    variant="light"
                    onPress={() => updateGroup(index, { nodeIds: [] })}
                  >
                    清空
                  </Button>
                </div>
              </div>
              {enabledNodes.length === 0 ? (
                <div className="rounded-medium border border-dashed border-default-200 px-3 py-6 text-center text-xs text-default-400">
                  暂无可用节点,请先在「节点管理」添加节点。
                </div>
              ) : (
                <div className="grid max-h-44 grid-cols-1 gap-1 overflow-auto rounded-medium border border-default-200 p-2 md:grid-cols-2">
                  {enabledNodes.map((node) => (
                    <label
                      key={node.id}
                      className="flex cursor-pointer items-center gap-2 rounded-small px-1.5 py-1 text-sm hover:bg-default-50 dark:hover:bg-default-100/10"
                      title={node.name}
                    >
                      <input
                        checked={group.nodeIds.includes(node.id)}
                        className="h-4 w-4 accent-primary"
                        type="checkbox"
                        onChange={() =>
                          toggleInGroup(index, "nodeIds", node.id)
                        }
                      />
                      <span className="flex-1 truncate">{node.name}</span>
                      <span className="text-[11px] text-default-400">
                        {node.protocol}
                      </span>
                    </label>
                  ))}
                </div>
              )}
            </div>

            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs text-default-500">这个分组的规则</span>
                <Select
                  aria-label="规则方案"
                  className="w-40"
                  placeholder="套用方案"
                  selectedKeys={[]}
                  size="sm"
                  onSelectionChange={(keys) =>
                    applyPreset(index, Array.from(keys)[0] as RulePreset)
                  }
                >
                  {RULE_PRESETS.map((entry) => (
                    <SelectItem key={entry.key}>{entry.label}</SelectItem>
                  ))}
                </Select>
              </div>
              <div className="grid max-h-44 grid-cols-1 gap-1 overflow-auto rounded-medium border border-default-200 p-2 md:grid-cols-2">
                {rulesets.map((ruleset) => (
                  <label
                    key={ruleset.id}
                    className="flex cursor-pointer items-center gap-2 rounded-small px-1.5 py-1 text-sm hover:bg-default-50 dark:hover:bg-default-100/10"
                  >
                    <input
                      checked={group.rulesetIds.includes(String(ruleset.id))}
                      className="h-4 w-4 accent-primary"
                      type="checkbox"
                      onChange={() =>
                        toggleInGroup(index, "rulesetIds", String(ruleset.id))
                      }
                    />
                    <span>{categoryEmoji(ruleset)}</span>
                    <span className="flex-1 truncate">
                      {ruleset.displayName || ruleset.name}
                    </span>
                  </label>
                ))}
              </div>
            </div>
          </div>
        </div>
      ))}
      <Button
        startContent={<PlusIcon size={16} />}
        variant="bordered"
        onPress={addGroup}
      >
        添加分组(每个分组 = 一批节点 + 自己的规则)
      </Button>
    </div>
  );

  const templateEditor = (
    <div className="flex flex-col gap-4">
      <div className="rounded-large border border-default-200">
        <div className="border-b border-default-200 px-4 py-3">
          <div className="text-sm font-medium">
            {form.userId ? "归属账号" : "选择节点"}
          </div>
          <div className="text-xs text-default-400">
            {form.userId === "all"
              ? "全体用户:所有用户的「我的订阅」里那条 Clash 链接都会套用这套模板,不用挑节点"
              : form.userId
                ? "已绑定账号:节点按账号自己的线路输出"
                : `从已保存的节点中选择(已选择 ${form.nodeIds.length} 个)`}
          </div>
        </div>
        <div className="p-4">
          <Select
            className="mb-3"
            label="归属账号(可选)"
            selectedKeys={[form.userId || "none"]}
            onSelectionChange={(keys) => {
              const value = String(Array.from(keys)[0] ?? "none");
              const nextUserId = value === "none" ? "" : value;
              setForm((prev) => ({
                ...prev,
                userId: nextUserId,
                nodeIds: nextUserId ? [] : prev.nodeIds,
              }));
            }}
          >
            {[
              { key: "all", label: "全体用户(全局规则)" },
              { key: "none", label: "不绑定 —— 用下面挑的节点" },
              ...users.map((account) => ({
                key: String(account.id),
                label: `${account.user}${account.name ? " · " + account.name : ""}`,
              })),
            ].map((option) => (
              <SelectItem key={option.key}>{option.label}</SelectItem>
            ))}
          </Select>
          {form.userId ? (
            <div className="py-4 text-center text-xs text-default-400">
              节点由账号决定,不用在这里挑
            </div>
          ) : enabledNodes.length === 0 ? (
            <div className="py-8 text-center text-xs text-default-400">
              暂无可用节点,请先在节点管理中添加节点
            </div>
          ) : (
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
                  <span className="text-[11px] text-default-400">
                    {node.protocol}
                  </span>
                </label>
              ))}
            </div>
          )}
        </div>
      </div>

      <Select
        label="订阅模板"
        selectedKeys={form.templateId ? [form.templateId] : []}
        onSelectionChange={(keys) =>
          setForm({ ...form, templateId: String(Array.from(keys)[0] ?? "") })
        }
      >
        {templates
          .filter((tpl) =>
            (FORMAT_CLIENTS[form.format] || []).includes(tpl.client),
          )
          .map((tpl) => (
            <SelectItem key={String(tpl.id)}>
              {`${tpl.name}${tpl.isDefault === 1 ? "(默认)" : ""}`}
            </SelectItem>
          ))}
      </Select>
    </div>
  );

  const createPanel = (
    <div className="flex flex-col gap-4">
      <div>
        <div className="text-lg font-semibold">
          {editing ? `编辑订阅 · ${editing.name}` : "新建订阅"}
        </div>
        <div className="text-xs text-default-500">
          节点按分组走各自的规则,也可以套一份模板
        </div>
      </div>

      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
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
          onSelectionChange={(keys) =>
            setForm({
              ...form,
              format: Array.from(keys)[0] as FormState["format"],
            })
          }
        >
          {Object.entries(FORMAT_LABEL).map(([key, label]) => (
            <SelectItem key={key}>{label}</SelectItem>
          ))}
        </Select>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium">规则模式</div>
        <div className="grid grid-cols-2 gap-2">
          {[
            { key: "custom", label: "自定义规则(分组)" },
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
                onClick={() =>
                  setForm((prev) => ({
                    ...prev,
                    mode: entry.key as FormState["mode"],
                  }))
                }
              >
                {entry.label}
              </button>
            );
          })}
        </div>
      </div>

      {form.mode === "custom" ? groupEditor : templateEditor}

      {editing && (
        <div className="text-xs text-default-400">
          当前地址:{subUrl(editing.token)}(编辑不改地址;要换地址用「重置
          token」)
        </div>
      )}

      <div className="flex items-stretch gap-2">
        <Button
          className="flex-1"
          color="primary"
          isLoading={saving}
          onPress={submit}
        >
          {editing ? "保存订阅" : "生成订阅文件"}
        </Button>
        <Button
          variant="bordered"
          onPress={() =>
            setForm({
              ...EMPTY_FORM,
              templateId: defaultTemplateId,
              groups: [
                {
                  name: "分组1",
                  nodeIds: [],
                  rulesetIds: presetRulesetIDs(rulesets, "balanced"),
                },
              ],
            })
          }
        >
          清空
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
            onValueChange={(value) =>
              setAggregate({ ...aggregate, name: value })
            }
          />
          <Input
            label="说明(可选)"
            value={aggregate.remark}
            onValueChange={(value) =>
              setAggregate({ ...aggregate, remark: value })
            }
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
                  <span className="text-[11px] text-default-400">
                    {nodeCount(item)} 节点
                  </span>
                </label>
              ))}
            </div>
          )}
        </div>
        <Select
          label="绑定模板(可选)"
          selectedKeys={aggregate.templateId ? [aggregate.templateId] : []}
          onSelectionChange={(keys) =>
            setAggregate({
              ...aggregate,
              templateId: String(Array.from(keys)[0] ?? ""),
            })
          }
        >
          {templates
            .filter((tpl) => tpl.client === "clash")
            .map((tpl) => (
              <SelectItem key={String(tpl.id)}>{tpl.name}</SelectItem>
            ))}
        </Select>
        <div className="flex items-center justify-end gap-2">
          <Button
            variant="light"
            onPress={() =>
              setAggregate({
                name: "",
                remark: "",
                templateId: "",
                sources: [],
              })
            }
          >
            取消
          </Button>
          <Button
            color="primary"
            isLoading={aggregating}
            onPress={createAggregate}
          >
            创建聚合订阅
          </Button>
        </div>
      </CardBody>
    </Card>
  );

  return (
    <PageWrapper
      title="订阅链接"
      description="一条链接 = 一批节点 + 规则。自定义规则时按分组配置:每个分组一批节点走自己的规则,客户端里就是一个个策略组。"
    >
      <Card className="mb-4">
        <CardBody className="py-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="mr-auto whitespace-nowrap text-sm text-default-500">
              共 {links.length} 条订阅
            </div>
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
              <Button
                color="primary"
                size="sm"
                startContent={<PlusIcon size={18} />}
                onPress={openCreate}
              >
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
              <div className="px-6 py-16 text-center text-default-500">
                还没有订阅链接。
              </div>
            ) : (
              <Table aria-label="订阅列表" removeWrapper>
                <TableHeader>
                  <TableColumn>名称</TableColumn>
                  <TableColumn width={110}>格式</TableColumn>
                  <TableColumn width={110}>节点 / 分组</TableColumn>
                  <TableColumn>规则</TableColumn>
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
                          <Snippet
                            classNames={{
                              base: "bg-default-100/60",
                              pre: "text-[11px]",
                            }}
                            hideSymbol
                            size="sm"
                          >
                            {subUrl(item.token)}
                          </Snippet>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Chip size="sm" variant="flat">
                          {FORMAT_LABEL[item.format] || item.format}
                        </Chip>
                      </TableCell>
                      <TableCell className="text-sm">
                        {groupCount(item) > 0
                          ? `${groupCount(item)} 组`
                          : nodeCount(item)}
                      </TableCell>
                      <TableCell className="text-xs text-default-500">
                        {groupCount(item) > 0
                          ? "分组规则"
                          : item.shared === 1
                            ? "全局规则"
                            : templateName(item)}
                      </TableCell>
                      <TableCell className="text-sm">
                        {item.visitCount}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="light"
                            onPress={() => copyUrl(item.token)}
                          >
                            复制
                          </Button>
                          <Button
                            size="sm"
                            variant="light"
                            onPress={() => openEdit(item)}
                          >
                            编辑
                          </Button>
                          <Button
                            color="danger"
                            size="sm"
                            variant="light"
                            onPress={() => handleDelete(item)}
                          >
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
            还没有订阅,点「新建订阅」创建一个;可以按分组配规则,也可以套模板。
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
                      <span className="text-base font-semibold">
                        {item.name}
                      </span>
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
                          : groupCount(item) > 0
                            ? `${groupCount(item)} 个分组`
                            : item.shared === 1
                              ? "全体用户(全局规则)"
                              : `${nodeCount(item)} 个节点`}
                      </span>
                      <span>
                        {groupCount(item) > 0
                          ? "每组独立规则"
                          : templateName(item)}
                      </span>
                      <span>访问 {item.visitCount}</span>
                    </div>
                  </div>
                  <Button
                    isIconOnly
                    size="sm"
                    title="复制订阅地址"
                    variant="light"
                    onPress={() => copyUrl(item.token)}
                  >
                    <CopyIcon size={16} />
                  </Button>
                </div>
                {item.shared === 1 ? (
                  <div className="rounded-medium bg-secondary/5 px-3 py-2 text-xs text-default-500">
                    这是全局订阅集:所有用户的「我的订阅 → Clash
                    订阅」会自动套用这套模板和规则。
                  </div>
                ) : (
                  <Snippet
                    classNames={{
                      base: "bg-default-100/60",
                      pre: "text-[11px] break-all whitespace-pre-wrap",
                    }}
                    hideSymbol
                    size="sm"
                  >
                    {subUrl(item.token)}
                  </Snippet>
                )}
                <div className="flex items-center justify-end gap-1">
                  <Button
                    size="sm"
                    variant="light"
                    onPress={() => openEdit(item)}
                  >
                    编辑
                  </Button>
                  {item.shared !== 1 && (
                    <Button
                      color="warning"
                      size="sm"
                      variant="light"
                      onPress={() => handleResetToken(item)}
                    >
                      重置 token
                    </Button>
                  )}
                  <Button
                    color="danger"
                    size="sm"
                    variant="light"
                    onPress={() => handleDelete(item)}
                  >
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
