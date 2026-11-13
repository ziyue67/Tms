import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input } from "@heroui/input";
import { Chip } from "@heroui/chip";
import { Select, SelectItem } from "@heroui/select";
import { Spinner } from "@heroui/spinner";
import { Snippet } from "@heroui/snippet";
import toast from "react-hot-toast";
import PageWrapper from "@/components/page-wrapper";
import { CopyIcon, PlusIcon } from "@/components/icons";
import {
  createMyPanelLink,
  deleteMyPanelLink,
  getMyPanelLinks,
  getMyPanelOptions,
  resetMyPanelLinkToken,
  useMyPanelSet,
  updateMyPanelLink,
  type PanelLink,
  type PanelRuleset,
  type PanelTemplate,
} from "@/api";
import { RULE_PRESETS, categoryEmoji, presetRulesetIDs, type RulePreset } from "@/utils/panel-category";

/**
 * 我的订阅链接(用户侧)。
 *
 * 和管理端「订阅链接」的区别:这里不挑节点 —— 订阅里的节点永远是你账号下的线路
 * (后端按 user_id 渲染),用户只需要决定用哪个模板 / 勾哪些规则类别。
 */
const FORMAT_LABEL: Record<string, string> = { clash: "Clash/Mihomo", v2ray: "V2Ray", singbox: "sing-box" };
const FORMAT_CLIENTS: Record<string, string[]> = { clash: ["clash"], singbox: ["singbox"], v2ray: [] };

interface FormState {
  name: string;
  format: "clash" | "v2ray" | "singbox";
  mode: "template" | "custom";
  preset: RulePreset;
  templateId: string;
  rulesetIds: string[];
}

const EMPTY_FORM: FormState = {
  name: "",
  format: "clash",
  mode: "custom",
  preset: "balanced",
  templateId: "",
  rulesetIds: [],
};

export default function MyLinksPage() {
  const [links, setLinks] = useState<PanelLink[]>([]);
  const [templates, setTemplates] = useState<PanelTemplate[]>([]);
  const [rulesets, setRulesets] = useState<PanelRuleset[]>([]);
  // 管理员建的订阅集:只有模板 + 规则,套用后节点取自己的
  const [sets, setSets] = useState<PanelLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [view, setView] = useState<"list" | "create">("list");
  const [editing, setEditing] = useState<PanelLink | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [collapsed, setCollapsed] = useState(false);

  const load = useCallback(async () => {
    try {
      const [linksRes, optionsRes] = await Promise.all([getMyPanelLinks(), getMyPanelOptions()]);
      if (linksRes.code === 0) setLinks(linksRes.data || []);
      if (optionsRes.code === 0) {
        setTemplates(optionsRes.data?.templates || []);
        setRulesets(optionsRes.data?.rulesets || []);
        setSets(optionsRes.data?.sets || []);
      }
    } catch {
      toast.error("加载失败");
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const defaultTemplateId = useMemo(() => {
    const byDefault = templates.find((tpl) => tpl.isDefault === 1);
    return byDefault ? String(byDefault.id) : "";
  }, [templates]);

  const presetIDs = useCallback((preset: RulePreset) => presetRulesetIDs(rulesets, preset), [rulesets]);

  const openCreate = () => {
    setEditing(null);
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
    });
    setView("create");
  };

  const toggleRuleset = (id: string) => {
    setForm((prev) => ({
      ...prev,
      preset: "manual",
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
    const payload = {
      name: form.name.trim(),
      format: form.format,
      templateId: form.mode === "template" && form.templateId ? form.templateId : 0,
      rulesetIds: form.mode === "custom" ? form.rulesetIds.join(",") : "",
      expireAt: editing?.expireAt || 0,
    };
    setSaving(true);
    try {
      const res = editing ? await updateMyPanelLink(editing.id, payload) : await createMyPanelLink(payload);
      if (res.code === 0) {
        toast.success(editing ? "已保存" : "订阅已生成,地址在卡片里复制");
        setView("list");
        await load();
      } else {
        toast.error(res.msg || "保存失败");
      }
    } catch {
      toast.error("保存失败");
    }
    setSaving(false);
  };

  const handleUseSet = async (set: PanelLink) => {
    try {
      const res = await useMyPanelSet(set.id, set.name);
      if (res.code === 0) {
        toast.success(`已套用「${set.name}」,地址在下面的卡片里`);
        await load();
      } else {
        toast.error(res.msg || "套用失败");
      }
    } catch {
      toast.error("套用失败");
    }
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

  const handleReset = async (item: PanelLink) => {
    if (!window.confirm(`重置「${item.name}」的 token?已导入客户端的旧地址会立刻失效。`)) return;
    try {
      const res = await resetMyPanelLinkToken(item.id);
      if (res.code === 0) {
        toast.success("已重置,请用新地址重新导入");
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
      const res = await deleteMyPanelLink(item.id);
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

  const rulesetCount = (item: PanelLink) => (item.rulesetIds ? item.rulesetIds.split(",").filter(Boolean).length : 0);
  const templateName = (item: PanelLink) =>
    item.templateId ? templates.find((tpl) => tpl.id === item.templateId)?.name || "已绑定模板" : "默认模板";

  if (loading) {
    return (
      <PageWrapper title="我的订阅链接" description="订阅里的节点来自你自己的账号,只需要决定模板和规则。">
        <Card>
          <CardBody className="flex justify-center py-16">
            <Spinner />
          </CardBody>
        </Card>
      </PageWrapper>
    );
  }

  return (
    <PageWrapper
      title="我的订阅链接"
      description="订阅地址里的节点就是你账号下的线路;选个模板或勾几条规则,生成后直接导入客户端。"
    >
      <Card className="mb-4">
        <CardBody className="py-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="mr-auto whitespace-nowrap text-sm text-default-500">共 {links.length} 条订阅</div>
            {view === "create" ? (
              <Button size="sm" variant="light" onPress={() => setView("list")}>
                ← 返回列表
              </Button>
            ) : (
              <Button color="primary" size="sm" startContent={<PlusIcon size={18} />} onPress={openCreate}>
                新建订阅
              </Button>
            )}
          </div>
        </CardBody>
      </Card>

      {sets.length > 0 && view !== "create" && (
        <Card className="mb-4">
          <CardBody className="gap-3">
            <div>
              <div className="text-sm font-medium">管理员的订阅集</div>
              <div className="text-xs text-default-400">
                「我的订阅」里那条 Clash 链接已经自动按这套规则输出;这里套用是额外拿到一条独立地址
              </div>
            </div>
            <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
              {sets.map((set) => (
                <div
                  key={set.id}
                  className="flex items-center gap-3 rounded-medium border border-default-200 px-3 py-2"
                >
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{set.name}</div>
                    <div className="text-xs text-default-400">
                      {FORMAT_LABEL[set.format] || set.format}
                      {" · "}
                      {set.templateId
                        ? templates.find((tpl) => tpl.id === set.templateId)?.name || "已绑定模板"
                        : "默认模板"}
                      {set.rulesetIds ? ` · ${set.rulesetIds.split(",").filter(Boolean).length} 条规则集` : ""}
                    </div>
                  </div>
                  <Button color="primary" size="sm" variant="flat" onPress={() => handleUseSet(set)}>
                    使用
                  </Button>
                </div>
              ))}
            </div>
          </CardBody>
        </Card>
      )}

      {view === "create" ? (
        <div className="flex flex-col gap-4">
          <div>
            <div className="text-lg font-semibold">{editing ? `编辑订阅 · ${editing.name}` : "新建订阅"}</div>
            <div className="text-xs text-default-500">节点自动取你账号下的线路,不用也不允许挑别人的节点</div>
          </div>

          <div className="rounded-medium bg-primary/5 px-3 py-2 text-xs text-default-500">
            这条订阅会输出你账号下的全部线路(以及分配给你的节点);账号到期或流量用完后地址会返回不可用。
          </div>

          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <Input
              isRequired
              label="订阅名称"
              placeholder="例如 手机 Clash"
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
          </div>

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

          <div>
            <div className="mb-2 flex items-center justify-between">
              <span className="text-sm font-medium">规则选择</span>
              {form.mode === "custom" && (
                <span className="text-xs text-default-400">已选择 {form.rulesetIds.length} 个类别</span>
              )}
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
                管理员还没配置规则集。
              </div>
            ) : (
              <div className="flex flex-col gap-2">
                <Select
                  selectedKeys={[form.preset]}
                  onSelectionChange={(keys) => {
                    const preset = Array.from(keys)[0] as RulePreset;
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
                    onClick={() => setCollapsed((value) => !value)}
                  >
                    <span className="text-default-600">已选择 {form.rulesetIds.length} 个类别</span>
                    <span className="text-default-400">{collapsed ? "▾" : "▴"}</span>
                  </button>
                  {!collapsed && (
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
                            onChange={() => toggleRuleset(String(ruleset.id))}
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

          <div className="flex items-stretch gap-2">
            <Button className="flex-1" color="primary" isLoading={saving} onPress={submit}>
              {editing ? "保存订阅" : "生成订阅文件"}
            </Button>
            <Button
              variant="bordered"
              onPress={() =>
                setForm({ ...EMPTY_FORM, templateId: defaultTemplateId, rulesetIds: presetIDs("balanced") })
              }
            >
              清空
            </Button>
          </div>
        </div>
      ) : links.length === 0 ? (
        <Card>
          <CardBody className="py-16 text-center text-default-500">
            还没有订阅。点「新建订阅」生成一条 —— 里面的节点就是你账号下的线路。
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
                    <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-default-400">
                      <span>{templateName(item)}</span>
                      <span>{rulesetCount(item)} 条规则集</span>
                      <span>访问 {item.visitCount}</span>
                      <span>{item.expireAt > 0 ? `到期 ${new Date(item.expireAt).toLocaleDateString()}` : "长期有效"}</span>
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
                  <Button color="warning" size="sm" variant="light" onPress={() => handleReset(item)}>
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
