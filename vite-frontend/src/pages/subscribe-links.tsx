import { useState, useEffect, useCallback } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input } from "@heroui/input";
import { Chip } from "@heroui/chip";
import { Select, SelectItem } from "@heroui/select";
import { Table, TableHeader, TableColumn, TableBody, TableRow, TableCell } from "@heroui/table";
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from "@heroui/modal";
import { Spinner } from "@heroui/spinner";
import { Snippet } from "@heroui/snippet";
import toast from "react-hot-toast";
import PageWrapper from "@/components/page-wrapper";
import {
  getPanelLinks,
  createPanelLink,
  updatePanelLink,
  resetPanelLinkToken,
  deletePanelLink,
  getPanelTemplates,
  getPanelRulesets,
  type PanelLink,
  type PanelTemplate,
  type PanelRuleset,
} from "@/api";

/**
 * 订阅链接(对应妙妙屋X 的「订阅链接」面板)。
 * 一条链接 = token + 输出格式(clash/v2ray) + 绑定模板 + 引用的规则集。
 * 公开地址 /api/v1/open_api/panel/{token},重置 token 后旧链接立即失效。
 */
const FORMAT_LABEL: Record<string, string> = {
  clash: "Clash/Mihomo (YAML)",
  v2ray: "V2Ray (base64)",
  singbox: "sing-box",
};

export default function SubscribeLinksPage() {
  const [links, setLinks] = useState<PanelLink[]>([]);
  const [templates, setTemplates] = useState<PanelTemplate[]>([]);
  const [rulesets, setRulesets] = useState<PanelRuleset[]>([]);
  const [loading, setLoading] = useState(true);

  const { isOpen, onOpen, onClose } = useDisclosure();
  const [editing, setEditing] = useState<PanelLink | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({ name: "", format: "clash", templateId: "", rulesetIds: [] as string[], expireAt: "" });

  const load = useCallback(async () => {
    try {
      const [lr, tr, rr] = await Promise.all([getPanelLinks(), getPanelTemplates(), getPanelRulesets()]);
      if (lr.code === 0) setLinks(lr.data || []);
      if (tr.code === 0) setTemplates(tr.data || []);
      if (rr.code === 0) setRulesets(rr.data || []);
    } catch {
      toast.error("加载失败");
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setEditing(null);
    setForm({ name: "", format: "clash", templateId: "", rulesetIds: [], expireAt: "" });
    onOpen();
  };

  const openEdit = (item: PanelLink) => {
    setEditing(item);
    setForm({
      name: item.name,
      format: item.format,
      templateId: item.templateId ? String(item.templateId) : "",
      rulesetIds: item.rulesetIds ? item.rulesetIds.split(",").filter(Boolean) : [],
      expireAt: item.expireAt > 0 ? new Date(item.expireAt).toISOString().slice(0, 10) : "",
    });
    onOpen();
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const payload = {
        name: form.name,
        format: form.format,
        templateId: form.templateId ? Number(form.templateId) : 0,
        rulesetIds: form.rulesetIds.join(","),
        expireAt: form.expireAt ? new Date(form.expireAt).getTime() : 0,
      };
      const res = editing ? await updatePanelLink(editing.id, payload) : await createPanelLink(payload);
      if (res.code === 0) {
        toast.success(editing ? "已更新" : "已创建");
        onClose();
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
    if (!window.confirm(`重置「${item.name}」的 token?所有已导入客户端的旧链接会立即失效。`)) return;
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
    if (!window.confirm(`删除订阅链接「${item.name}」?`)) return;
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

  const subUrl = (token: string) => `${window.location.origin}/api/v1/open_api/panel/${token}`;

  const copyUrl = async (token: string) => {
    try {
      await navigator.clipboard.writeText(subUrl(token));
      toast.success("订阅地址已复制");
    } catch {
      toast.error("复制失败");
    }
  };

  return (
    <PageWrapper title="订阅链接" description="生成对外的订阅直链:绑定模板与规则集,客户端直接导入。重置 token 可让泄露的链接立即失效。">
      <div className="flex justify-between items-center mb-4">
        <div className="text-sm text-default-500">共 {links.length} 条订阅链接</div>
        <Button color="primary" onPress={openCreate}>新建订阅链接</Button>
      </div>

      <Card>
        <CardBody className="p-0">
          {loading ? (
            <div className="flex justify-center py-16"><Spinner /></div>
          ) : links.length === 0 ? (
            <div className="text-center text-default-500 py-16">还没有订阅链接。新建一条,把地址导入 Clash / V2Ray 客户端。</div>
          ) : (
            <Table aria-label="订阅链接列表" removeWrapper>
              <TableHeader>
                <TableColumn>名称</TableColumn>
                <TableColumn>格式</TableColumn>
                <TableColumn>模板</TableColumn>
                <TableColumn>规则集</TableColumn>
                <TableColumn>访问</TableColumn>
                <TableColumn>到期</TableColumn>
                <TableColumn>操作</TableColumn>
              </TableHeader>
              <TableBody>
                {links.map((item) => {
                  const expired = item.expireAt > 0 && item.expireAt <= Date.now();
                  return (
                    <TableRow key={item.id}>
                      <TableCell>
                        <div className="font-medium">{item.name}</div>
                        <div className="mt-1 max-w-md">
                          <Snippet size="sm" hideSymbol classNames={{ base: "bg-default-100", pre: "text-xs" }}>{subUrl(item.token)}</Snippet>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Chip size="sm" variant="flat" color={item.format === "clash" ? "secondary" : "default"}>
                          {item.format}
                        </Chip>
                      </TableCell>
                      <TableCell className="text-sm">
                        {item.templateId ? templates.find((t) => t.id === item.templateId)?.name || `#${item.templateId}` : "默认"}
                      </TableCell>
                      <TableCell className="text-sm">
                        {item.rulesetIds
                          ? item.rulesetIds.split(",").map((id) => rulesets.find((r) => String(r.id) === id)?.name || `#${id}`).join(", ")
                          : "-"}
                      </TableCell>
                      <TableCell className="text-sm">{item.visitCount}</TableCell>
                      <TableCell className="text-xs">
                        {item.expireAt > 0 ? (
                          <span className={expired ? "text-danger" : ""}>
                            {new Date(item.expireAt).toLocaleDateString()}
                            {expired && " 已过期"}
                          </span>
                        ) : (
                          "永久"
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex gap-1">
                          <Button size="sm" variant="light" onPress={() => copyUrl(item.token)}>复制</Button>
                          <Button size="sm" variant="light" onPress={() => openEdit(item)}>编辑</Button>
                          <Button size="sm" variant="light" color="warning" onPress={() => handleResetToken(item)}>重置</Button>
                          <Button size="sm" variant="light" color="danger" onPress={() => handleDelete(item)}>删除</Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardBody>
      </Card>

      <Modal isOpen={isOpen} onClose={onClose} size="2xl">
        <ModalContent>
          <ModalHeader>{editing ? "编辑订阅链接" : "新建订阅链接"}</ModalHeader>
          <ModalBody>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Input label="名称" value={form.name} onValueChange={(v) => setForm({ ...form, name: v })} placeholder="例如 我的Clash" />
              <Select label="输出格式" selectedKeys={[form.format]} onSelectionChange={(keys) => setForm({ ...form, format: Array.from(keys)[0] as string })}>
                {Object.entries(FORMAT_LABEL).map(([k, label]) => (
                  <SelectItem key={k}>{label}</SelectItem>
                ))}
              </Select>
              <Select
                label="订阅模板"
                selectedKeys={form.templateId ? [form.templateId] : []}
                onSelectionChange={(keys) => setForm({ ...form, templateId: String(Array.from(keys)[0] ?? "") })}
                description="不选则用默认模板或内置配置"
              >
                {templates.map((t) => (
                  <SelectItem key={String(t.id)}>{t.name}({t.version})</SelectItem>
                ))}
              </Select>
              <Select
                label="引用规则集"
                selectionMode="multiple"
                selectedKeys={form.rulesetIds}
                onSelectionChange={(keys) => setForm({ ...form, rulesetIds: Array.from(keys).map(String) })}
                description="规则按引用顺序插入 MATCH 兜底行之前"
              >
                {rulesets.map((r) => (
                  <SelectItem key={String(r.id)}>{r.displayName || r.name}</SelectItem>
                ))}
              </Select>
              <Input label="到期时间(可选)" type="date" value={form.expireAt} onValueChange={(v) => setForm({ ...form, expireAt: v })} />
            </div>
            {editing && (
              <div className="text-xs text-default-400">当前地址:{subUrl(editing.token)}(编辑后地址不变,重置 token 才会换新)</div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onClose}>取消</Button>
            <Button color="primary" onPress={handleSave} isLoading={saving}>保存</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </PageWrapper>
  );
}
