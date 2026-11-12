import { useState, useEffect, useCallback } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input, Textarea } from "@heroui/input";
import { Chip } from "@heroui/chip";
import { Select, SelectItem } from "@heroui/select";
import { Table, TableHeader, TableColumn, TableBody, TableRow, TableCell } from "@heroui/table";
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from "@heroui/modal";
import { Spinner } from "@heroui/spinner";
import toast from "react-hot-toast";
import PageWrapper from "@/components/page-wrapper";
import {
  getPanelRulesets,
  getPanelRuleset,
  createPanelRuleset,
  updatePanelRuleset,
  deletePanelRuleset,
  refreshPanelRuleset,
  type PanelRuleset,
} from "@/api";

/**
 * 规则集(对应妙妙屋X 的「规则集」面板)。
 * 托管 clash rule-providers 规则文件:手动编辑,或填远程地址由主控定时抓取。
 * 公开下载路径:/api/v1/open_api/panel-ruleset/{name} —— 订阅模板按 rulesetIds 引用。
 */
export default function RulesetsPage() {
  const [items, setItems] = useState<PanelRuleset[]>([]);
  const [loading, setLoading] = useState(true);

  const { isOpen, onOpen, onClose } = useDisclosure();
  const [editing, setEditing] = useState<PanelRuleset | null>(null);
  const [content, setContent] = useState("");
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({ name: "", displayName: "", source: "manual" as "manual" | "remote", remoteUrl: "", refreshMinutes: 30 });

  const load = useCallback(async () => {
    try {
      const res = await getPanelRulesets();
      if (res.code === 0) setItems(res.data || []);
    } catch {
      toast.error("加载规则集失败");
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setEditing(null);
    setContent("");
    setForm({ name: "", displayName: "", source: "manual", remoteUrl: "", refreshMinutes: 30 });
    onOpen();
  };

  const openEdit = async (item: PanelRuleset) => {
    setEditing(item);
    setForm({ name: item.name, displayName: item.displayName, source: item.source, remoteUrl: item.remoteUrl, refreshMinutes: item.refreshMinutes });
    setContent("");
    onOpen();
    try {
      const res = await getPanelRuleset(item.id);
      if (res.code === 0) setContent(res.data?.content || "");
    } catch {
      toast.error("读取规则集内容失败");
    }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const payload = { ...form };
      const res = editing ? await updatePanelRuleset(editing.id, payload) : await createPanelRuleset(payload);
      if (res.code === 0) {
        // 内容只在创建时提交;编辑走单独保存,避免大文本反复上传
        if (!editing && payload.source === "manual" && content) {
          await updatePanelRuleset(res.data.id, { ...payload });
        }
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

  const handleSaveContent = async () => {
    if (!editing) return;
    setSaving(true);
    try {
      const res = await updatePanelRuleset(editing.id, { ...form, content });
      if (res.code === 0) {
        toast.success("内容已保存");
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

  const handleDelete = async (item: PanelRuleset) => {
    if (!window.confirm(`删除规则集「${item.displayName || item.name}」?引用它的订阅会拿不到该文件。`)) return;
    try {
      const res = await deletePanelRuleset(item.id);
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

  const handleRefresh = async (item: PanelRuleset) => {
    toast.loading("抓取中…", { id: "ruleset-refresh" });
    try {
      const res = await refreshPanelRuleset(item.id);
      if (res.code === 0) {
        toast.success("抓取成功", { id: "ruleset-refresh" });
      } else {
        toast.error(res.msg || "抓取失败", { id: "ruleset-refresh" });
      }
      await load();
    } catch {
      toast.error("抓取失败", { id: "ruleset-refresh" });
    }
  };

  const publicUrl = (name: string) => `${window.location.origin}/api/v1/open_api/panel-ruleset/${name}`;

  const copyUrl = async (name: string) => {
    try {
      await navigator.clipboard.writeText(publicUrl(name));
      toast.success("下载地址已复制");
    } catch {
      toast.error("复制失败");
    }
  };

  return (
    <PageWrapper title="规则集" description="托管 clash rule-providers 规则文件,远程来源由主控定时抓取,订阅模板按规则集引用。">
      <div className="flex justify-between items-center mb-4">
        <div className="text-sm text-default-500">共 {items.length} 个规则集</div>
        <Button color="primary" onPress={openCreate}>新建规则集</Button>
      </div>

      <Card>
        <CardBody className="p-0">
          {loading ? (
            <div className="flex justify-center py-16"><Spinner /></div>
          ) : items.length === 0 ? (
            <div className="text-center text-default-500 py-16">还没有规则集。新建一个,然后在订阅模板里引用。</div>
          ) : (
            <Table aria-label="规则集列表" removeWrapper>
              <TableHeader>
                <TableColumn>名称</TableColumn>
                <TableColumn>来源</TableColumn>
                <TableColumn>大小</TableColumn>
                <TableColumn>最近抓取</TableColumn>
                <TableColumn>操作</TableColumn>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>
                      <div className="font-medium">{item.displayName || item.name}</div>
                      <div className="text-xs text-default-400 font-mono">{item.name}</div>
                    </TableCell>
                    <TableCell>
                      <Chip size="sm" variant="flat" color={item.source === "remote" ? "secondary" : "default"}>
                        {item.source === "remote" ? `远程/${item.refreshMinutes}分` : "手动"}
                      </Chip>
                    </TableCell>
                    <TableCell className="text-sm">{(item.size / 1024).toFixed(1)} KB</TableCell>
                    <TableCell className="text-xs">
                      {item.source === "remote" ? (
                        item.lastFetchError ? (
                          <span className="text-danger">{item.lastFetchError}</span>
                        ) : item.lastFetchAt ? (
                          new Date(item.lastFetchAt).toLocaleString()
                        ) : (
                          "未抓取"
                        )
                      ) : (
                        new Date(item.updatedTime).toLocaleString()
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <Button size="sm" variant="light" onPress={() => openEdit(item)}>编辑</Button>
                        <Button size="sm" variant="light" onPress={() => copyUrl(item.name)}>复制地址</Button>
                        {item.source === "remote" && (
                          <Button size="sm" variant="light" color="secondary" onPress={() => handleRefresh(item)}>刷新</Button>
                        )}
                        <Button size="sm" variant="light" color="danger" onPress={() => handleDelete(item)}>删除</Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardBody>
      </Card>

      <Modal isOpen={isOpen} onClose={onClose} size="3xl" scrollBehavior="inside">
        <ModalContent>
          <ModalHeader>{editing ? "编辑规则集" : "新建规则集"}</ModalHeader>
          <ModalBody>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Input
                label="文件名(公开路径)"
                placeholder="例如 acl4ssr.yaml"
                value={form.name}
                onValueChange={(v) => setForm({ ...form, name: v })}
                description="字母数字._-,以字母数字开头;名字公开可下载,建议带随机后缀防猜"
              />
              <Input label="显示名" placeholder="可选" value={form.displayName} onValueChange={(v) => setForm({ ...form, displayName: v })} />
              <Select
                label="来源"
                selectedKeys={[form.source]}
                onSelectionChange={(keys) => setForm({ ...form, source: Array.from(keys)[0] as "manual" | "remote" })}
              >
                <SelectItem key="manual">手动(面板编辑)</SelectItem>
                <SelectItem key="remote">远程(定时抓取)</SelectItem>
              </Select>
              {form.source === "remote" ? (
                <div className="grid grid-cols-3 gap-3 md:col-span-1">
                  <Input className="col-span-2" label="远程地址" value={form.remoteUrl} onValueChange={(v) => setForm({ ...form, remoteUrl: v })} placeholder="https://..." />
                  <Input label="间隔(分)" type="number" value={String(form.refreshMinutes)} onValueChange={(v) => setForm({ ...form, refreshMinutes: Number(v) || 0 })} />
                </div>
              ) : (
                <Textarea
                  label="规则内容"
                  minRows={10}
                  value={content}
                  onValueChange={setContent}
                  placeholder="DOMAIN-SUFFIX,google.com,PROXY&#10;GEOIP,CN,DIRECT&#10;MATCH,漏网之鱼"
                  className="md:col-span-2"
                />
              )}
            </div>
            {editing && (
              <Textarea label="规则内容(编辑已有)" minRows={12} value={content} onValueChange={setContent} className="md:col-span-2" />
            )}
            {editing && (
              <div className="text-xs text-default-400">公开下载地址:{publicUrl(editing.name)}</div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onClose}>取消</Button>
            {editing ? (
              <>
                <Button color="primary" variant="flat" onPress={handleSave} isLoading={saving}>保存信息</Button>
                <Button color="primary" onPress={handleSaveContent} isLoading={saving}>保存内容</Button>
              </>
            ) : (
              <Button color="primary" onPress={handleSave} isLoading={saving}>创建</Button>
            )}
          </ModalFooter>
        </ModalContent>
      </Modal>
    </PageWrapper>
  );
}
