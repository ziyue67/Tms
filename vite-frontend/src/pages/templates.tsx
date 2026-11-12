import { useState, useEffect, useCallback } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input, Textarea } from "@heroui/input";
import { Chip } from "@heroui/chip";
import { Select, SelectItem } from "@heroui/select";
import { Switch } from "@heroui/switch";
import { Table, TableHeader, TableColumn, TableBody, TableRow, TableCell } from "@heroui/table";
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from "@heroui/modal";
import { Spinner } from "@heroui/spinner";
import toast from "react-hot-toast";
import PageWrapper from "@/components/page-wrapper";
import {
  getPanelTemplates,
  getPanelTemplate,
  createPanelTemplate,
  updatePanelTemplate,
  deletePanelTemplate,
  type PanelTemplate,
} from "@/api";

/**
 * 模板管理(对应妙妙屋X 的「模板管理」面板)。
 * v2 = 传统 ACL4SSR 文本模板;v3 = mihomo 风格 JSON 模板(proxies/proxy-groups/rules 骨架,
 * 渲染时节点与规则集注入)。isDefault=1 的模板作为新订阅链接的默认选择。
 */
const SAMPLE_V3 = JSON.stringify(
  {
    "mixed-port": 7890,
    "allow-lan": false,
    mode: "rule",
    "log-level": "info",
    dns: { enable: true, "enhanced-mode": "fake-ip", nameserver: ["223.5.5.5"] },
    rules: ["GEOIP,CN,DIRECT", "MATCH,漏网之鱼"],
  },
  null,
  2
);

export default function TemplatesPage() {
  const [items, setItems] = useState<PanelTemplate[]>([]);
  const [loading, setLoading] = useState(true);

  const { isOpen, onOpen, onClose } = useDisclosure();
  const [editing, setEditing] = useState<PanelTemplate | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({ name: "", version: "v3" as "v2" | "v3", isDefault: false, remark: "" });
  const [content, setContent] = useState("");

  const load = useCallback(async () => {
    try {
      const res = await getPanelTemplates();
      if (res.code === 0) setItems(res.data || []);
    } catch {
      toast.error("加载模板失败");
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setEditing(null);
    setForm({ name: "", version: "v3", isDefault: false, remark: "" });
    setContent(SAMPLE_V3);
    onOpen();
  };

  const openEdit = async (item: PanelTemplate) => {
    setEditing(item);
    setForm({ name: item.name, version: item.version, isDefault: item.isDefault === 1, remark: item.remark });
    onOpen();
    try {
      const res = await getPanelTemplate(item.id);
      if (res.code === 0) setContent(res.data?.content || "");
    } catch {
      toast.error("读取模板内容失败");
    }
  };

  const handleSave = async () => {
    if (form.version === "v3" && content.trim()) {
      try {
        JSON.parse(content);
      } catch (e) {
        toast.error("v3 模板必须是合法 JSON: " + (e as Error).message);
        return;
      }
    }
    setSaving(true);
    try {
      const payload = { ...form, isDefault: form.isDefault ? 1 : 0, content };
      const res = editing ? await updatePanelTemplate(editing.id, payload) : await createPanelTemplate(payload);
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

  const handleDelete = async (item: PanelTemplate) => {
    if (!window.confirm(`删除模板「${item.name}」?使用它的订阅链接会退回内置默认配置。`)) return;
    try {
      const res = await deletePanelTemplate(item.id);
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

  return (
    <PageWrapper title="模板管理" description="订阅输出的 clash 配置骨架。v3 = JSON 模板(推荐),v2 = 传统文本模板;默认模板自动应用到未指定模板的订阅链接。">
      <div className="flex justify-between items-center mb-4">
        <div className="text-sm text-default-500">共 {items.length} 个模板</div>
        <Button color="primary" onPress={openCreate}>新建模板</Button>
      </div>

      <Card>
        <CardBody className="p-0">
          {loading ? (
            <div className="flex justify-center py-16"><Spinner /></div>
          ) : items.length === 0 ? (
            <div className="text-center text-default-500 py-16">还没有模板。订阅链接未选模板时使用内置默认配置。</div>
          ) : (
            <Table aria-label="模板列表" removeWrapper>
              <TableHeader>
                <TableColumn>名称</TableColumn>
                <TableColumn>版本</TableColumn>
                <TableColumn>备注</TableColumn>
                <TableColumn>更新时间</TableColumn>
                <TableColumn>操作</TableColumn>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>
                      <div className="flex items-center gap-2 font-medium">
                        {item.name}
                        {item.isDefault === 1 && <Chip size="sm" color="primary" variant="flat">默认</Chip>}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Chip size="sm" variant="flat" color={item.version === "v3" ? "secondary" : "default"}>
                        {item.version.toUpperCase()}
                      </Chip>
                    </TableCell>
                    <TableCell className="text-sm text-default-500">{item.remark || "-"}</TableCell>
                    <TableCell className="text-xs">{new Date(item.updatedTime).toLocaleString()}</TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <Button size="sm" variant="light" onPress={() => openEdit(item)}>编辑</Button>
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

      <Modal isOpen={isOpen} onClose={onClose} size="4xl" scrollBehavior="inside">
        <ModalContent>
          <ModalHeader>{editing ? "编辑模板" : "新建模板"}</ModalHeader>
          <ModalBody>
            <div className="grid grid-cols-1 md:grid-cols-4 gap-3 items-end">
              <Input label="模板名" value={form.name} onValueChange={(v) => setForm({ ...form, name: v })} />
              <Select label="版本" selectedKeys={[form.version]} onSelectionChange={(keys) => setForm({ ...form, version: Array.from(keys)[0] as "v2" | "v3" })}>
                <SelectItem key="v3">v3(JSON,推荐)</SelectItem>
                <SelectItem key="v2">v2(文本)</SelectItem>
              </Select>
              <Input label="备注" value={form.remark} onValueChange={(v) => setForm({ ...form, remark: v })} />
              <div className="flex flex-col gap-1 pb-2">
                <span className="text-sm text-default-500">设为默认模板</span>
                <Switch isSelected={form.isDefault} onValueChange={(v) => setForm({ ...form, isDefault: v })} />
              </div>
            </div>
            <Textarea
              label={form.version === "v3" ? "模板内容(JSON)" : "模板内容(文本)"}
              minRows={18}
              value={content}
              onValueChange={setContent}
              className="font-mono text-xs"
            />
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
