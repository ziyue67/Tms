import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input, Textarea } from "@heroui/input";
import { Chip } from "@heroui/chip";
import { Select, SelectItem } from "@heroui/select";
import { Table, TableBody, TableCell, TableColumn, TableHeader, TableRow } from "@heroui/table";
import { Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, useDisclosure } from "@heroui/modal";
import { Spinner } from "@heroui/spinner";
import toast from "react-hot-toast";
import PageWrapper from "@/components/page-wrapper";
import { PlusIcon } from "@/components/icons";
import {
  deleteCustomNode,
  deleteCustomNodes,
  disableCustomNode,
  enableCustomNode,
  getCustomNodes,
  syncPanelNodes,
  importCustomNodes,
  updateCustomNodesStatus,
} from "@/api";

/**
 * 节点管理(对齐妙妙屋X):面板里自己存的机场/个人节点。
 * 「新建订阅」就是从这里挑节点 —— 订阅链接自带 node_ids,不再借用某个车友的 token。
 *
 * 注意 id 是 64 位雪花值,全流程当字符串,别转 Number。
 */
interface PanelNodeItem {
  id: string;
  name: string;
  protocol: string;
  visibility: string;
  status: number;
  created_time?: number;
  userIds?: number[];
}

const VISIBILITY_LABEL: Record<string, string> = {
  global: "全局可见",
  subscribers: "订阅用户可见",
  users: "指定用户",
};

export default function NodesPage() {
  const [items, setItems] = useState<PanelNodeItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<string[]>([]);
  const [importing, setImporting] = useState(false);

  const importer = useDisclosure();
  const [links, setLinks] = useState("");
  const [name, setName] = useState("");
  const [visibility, setVisibility] = useState<"global" | "subscribers">("subscribers");

  const load = useCallback(async () => {
    try {
      const res = await getCustomNodes();
      if (res.code === 0) setItems((res.data || []) as PanelNodeItem[]);
      else toast.error(res.msg || "加载节点失败");
    } catch {
      toast.error("加载节点失败");
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const toggle = (id: string) => {
    setSelected((prev) => (prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]));
  };

  const allSelected = items.length > 0 && selected.length === items.length;

  const handleImport = async () => {
    const raw = links
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    if (raw.length === 0) {
      toast.error("请粘贴至少一条节点链接");
      return;
    }
    setImporting(true);
    try {
      const res = await importCustomNodes(name.trim(), raw, visibility);
      if (res.code === 0) {
        const failed = res.data?.failures?.length || 0;
        toast.success(failed > 0 ? `导入完成,${failed} 条失败` : `已导入 ${raw.length} 条节点`);
        importer.onClose();
        setLinks("");
        setName("");
        await load();
      } else {
        toast.error(res.msg || "导入失败");
      }
    } catch {
      toast.error("导入失败");
    }
    setImporting(false);
  };

  const handleStatus = async (item: PanelNodeItem, enable: boolean) => {
    try {
      const res = enable ? await enableCustomNode(item.id) : await disableCustomNode(item.id);
      if (res.code === 0) {
        toast.success(enable ? "已启用" : "已停用");
        await load();
      } else {
        toast.error(res.msg || "操作失败");
      }
    } catch {
      toast.error("操作失败");
    }
  };

  const handleDelete = async (item: PanelNodeItem) => {
    if (!window.confirm(`删除节点「${item.name}」?引用它的订阅会少一个节点。`)) return;
    try {
      const res = await deleteCustomNode(item.id);
      if (res.code === 0) {
        toast.success("已删除");
        setSelected((prev) => prev.filter((id) => id !== item.id));
        await load();
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch {
      toast.error("删除失败");
    }
  };

  const handleSyncPanel = async () => {
    if (!window.confirm("把「协议管理 / 中转」里管理员名下的线路一键生成为公共节点?同名节点会更新链接,不会重复。")) return;
    toast.loading("同步中…", { id: "sync" });
    try {
      const res = await syncPanelNodes();
      if (res.code === 0) {
        toast.success(`新增 ${res.data?.created ?? 0} 个、更新 ${res.data?.updated ?? 0} 个(共 ${res.data?.total ?? 0} 条线路)`, { id: "sync" });
        await load();
      } else {
        toast.error(res.msg || "同步失败", { id: "sync" });
      }
    } catch {
      toast.error("同步失败", { id: "sync" });
    }
  };

  const handleBatch = async (action: "enable" | "disable" | "delete") => {
    if (selected.length === 0) return;
    if (action === "delete" && !window.confirm(`删除选中的 ${selected.length} 个节点?`)) return;
    try {
      const res =
        action === "delete"
          ? await deleteCustomNodes(selected)
          : await updateCustomNodesStatus(selected, action === "enable" ? 1 : 0);
      if (res.code === 0) {
        toast.success("已处理");
        setSelected([]);
        await load();
      } else {
        toast.error(res.msg || "批量操作失败");
      }
    } catch {
      toast.error("批量操作失败");
    }
  };

  const stats = useMemo(() => {
    const enabled = items.filter((item) => item.status === 1).length;
    return { enabled, disabled: items.length - enabled };
  }, [items]);

  return (
    <PageWrapper
      title="节点管理"
      description="面板自己保存的机场 / 个人节点。「新建订阅」从这里挑节点,挑中的节点会写进订阅链接,不依赖任何车友账号。"
    >
      <Card className="mb-4">
        <CardBody className="py-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="mr-auto whitespace-nowrap text-sm text-default-500">
              共 {items.length} 个节点
              {items.length > 0 && (
                <span className="text-default-400">
                  {" "}
                  · 启用 {stats.enabled} · 停用 {stats.disabled}
                </span>
              )}
            </div>
            {selected.length > 0 && (
              <>
                <span className="text-xs text-default-400">已选 {selected.length}</span>
                <Button size="sm" variant="flat" onPress={() => handleBatch("enable")}>
                  批量启用
                </Button>
                <Button size="sm" variant="flat" onPress={() => handleBatch("disable")}>
                  批量停用
                </Button>
                <Button color="danger" size="sm" variant="flat" onPress={() => handleBatch("delete")}>
                  批量删除
                </Button>
              </>
            )}
            <Button size="sm" variant="bordered" onPress={handleSyncPanel}>
              同步面板线路
            </Button>
            <Button color="primary" size="sm" startContent={<PlusIcon size={18} />} onPress={importer.onOpen}>
              导入节点
            </Button>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardBody className="p-0">
          {loading ? (
            <div className="flex justify-center py-16">
              <Spinner />
            </div>
          ) : items.length === 0 ? (
            <div className="px-6 py-16 text-center text-default-500">
              还没有节点。点「导入节点」把机场或个人节点的分享链接粘进来(支持 ss / vmess / vless / trojan / hysteria2 / tuic / anytls)。
            </div>
          ) : (
            <Table aria-label="节点列表" removeWrapper>
              <TableHeader>
                <TableColumn width={48}>
                  <input
                    aria-label="全选"
                    checked={allSelected}
                    className="h-4 w-4 accent-primary"
                    type="checkbox"
                    onChange={(event) => setSelected(event.target.checked ? items.map((item) => item.id) : [])}
                  />
                </TableColumn>
                <TableColumn>节点</TableColumn>
                <TableColumn width={120}>协议</TableColumn>
                <TableColumn width={160}>可见性</TableColumn>
                <TableColumn width={100}>状态</TableColumn>
                <TableColumn align="end" width={160}>
                  操作
                </TableColumn>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>
                      <input
                        aria-label={`选择 ${item.name}`}
                        checked={selected.includes(item.id)}
                        className="h-4 w-4 accent-primary"
                        type="checkbox"
                        onChange={() => toggle(item.id)}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">{item.name}</div>
                      <div className="font-mono text-[11px] text-default-400">#{item.id}</div>
                    </TableCell>
                    <TableCell>
                      <Chip size="sm" variant="flat">
                        {item.protocol}
                      </Chip>
                    </TableCell>
                    <TableCell>
                      <span className="text-xs text-default-500">{VISIBILITY_LABEL[item.visibility] || item.visibility}</span>
                    </TableCell>
                    <TableCell>
                      <Chip color={item.status === 1 ? "success" : "default"} size="sm" variant="flat">
                        {item.status === 1 ? "启用" : "停用"}
                      </Chip>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          size="sm"
                          variant="light"
                          onPress={() => handleStatus(item, item.status !== 1)}
                        >
                          {item.status === 1 ? "停用" : "启用"}
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

      <Modal isOpen={importer.isOpen} scrollBehavior="inside" size="2xl" onClose={importer.onClose}>
        <ModalContent>
          <ModalHeader>导入节点</ModalHeader>
          <ModalBody className="gap-4">
            <Textarea
              isRequired
              classNames={{ input: "font-mono text-xs" }}
              label="节点链接"
              minRows={8}
              placeholder={"每行一条,例如:\nss://YWVzLTI1Ni1nY206cGFzc0AxLjIuMy40OjQ0Mw\nvmess://eyJ2IjoiMiIsInBzIjoi..."}
              value={links}
              onValueChange={setLinks}
            />
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <Input
                label="名称前缀(可选)"
                placeholder="留空则用链接里的节点名"
                value={name}
                onValueChange={setName}
              />
              <Select
                label="可见性"
                selectedKeys={[visibility]}
                onSelectionChange={(keys) => setVisibility(Array.from(keys)[0] as "global" | "subscribers")}
              >
                <SelectItem key="subscribers">订阅用户可见</SelectItem>
                <SelectItem key="global">全局可见</SelectItem>
              </Select>
            </div>
            <p className="text-xs text-default-400">
              导入只解析链接,不会立刻探测连通性;要在订阅里用,还需要到「订阅链接 → 新建订阅」里选中这些节点。
            </p>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={importer.onClose}>
              取消
            </Button>
            <Button color="primary" isLoading={importing} onPress={handleImport}>
              导入
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </PageWrapper>
  );
}
