import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input, Textarea } from "@heroui/input";
import { Chip } from "@heroui/chip";
import { Select, SelectItem } from "@heroui/select";
import { Switch } from "@heroui/switch";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
} from "@heroui/table";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from "@heroui/modal";
import { Spinner } from "@heroui/spinner";
import toast from "react-hot-toast";
import PageWrapper from "@/components/page-wrapper";
import {
  CopyIcon,
  DeleteIcon,
  EditIcon,
  EyeIcon,
  PlusIcon,
  RefreshIcon,
  StarIcon,
  UploadIcon,
} from "@/components/icons";
import {
  getPanelTemplates,
  getPanelTemplate,
  createPanelTemplate,
  updatePanelTemplate,
  deletePanelTemplate,
  setPanelTemplateDefault,
  resetPanelTemplate,
  seedPanelDefaults,
  type PanelTemplate,
  type PanelTemplateClient,
} from "@/api";

/**
 * 模板管理(对齐妙妙屋X 的「模板管理」面板)。
 *
 * 一个模板 = 订阅输出的配置骨架。clash 模板支持 YAML 文本(proxies: null /
 * __PROXY_NODES__ 占位)或 JSON 骨架,sing-box 模板是 JSON,Surge/Loon/Quan X
 * 是各自的纯文本配置。isDefault=1 的模板自动应用到没指定模板的订阅链接。
 */

const CLIENTS: { key: PanelTemplateClient | "all"; label: string }[] = [
  { key: "all", label: "全部" },
  { key: "clash", label: "Clash" },
  { key: "surge", label: "Surge" },
  { key: "loon", label: "Loon" },
  { key: "quanx", label: "Quan X" },
  { key: "singbox", label: "sing-box" },
];

const CLIENT_LABEL: Record<PanelTemplateClient, string> = {
  clash: "Clash",
  surge: "Surge",
  loon: "Loon",
  quanx: "Quan X",
  singbox: "sing-box",
};

const CLIENT_EXT: Record<PanelTemplateClient, string> = {
  clash: ".yaml",
  surge: ".conf",
  loon: ".lcf",
  quanx: ".qx",
  singbox: ".json",
};

// 新建模板时的空白骨架:每种客户端给一份能直接跑的最小配置。
const BLANK: Record<PanelTemplateClient, string> = {
  clash: [
    "mode: rule",
    "log-level: info",
    "mixed-port: 7890",
    "proxies: null",
    "proxy-groups:",
    "  - name: 节点选择",
    "    type: select",
    "    proxies:",
    "      - 自动选择",
    "      - __PROXY_NODES__",
    "  - name: 自动选择",
    "    type: url-test",
    "    proxies:",
    "      - __PROXY_NODES__",
    "    url: https://cp.cloudflare.com/generate_204",
    "    interval: 300",
    "rules:",
    "  - GEOIP,CN,DIRECT",
    "  - MATCH,节点选择",
    "",
  ].join("\n"),
  singbox: JSON.stringify(
    {
      log: { level: "info" },
      inbounds: [
        {
          type: "mixed",
          tag: "mixed-in",
          listen: "0.0.0.0",
          listen_port: 7890,
        },
      ],
      outbounds: [
        "__PROXY_OUTBOUNDS__",
        {
          type: "selector",
          tag: "节点选择",
          outbounds: ["__PROXY_TAGS__", "direct"],
          default: "direct",
        },
        { type: "direct", tag: "direct" },
      ],
      route: { final: "节点选择" },
    },
    null,
    2,
  ),
  surge: [
    "[General]",
    "loglevel = notify",
    "",
    "[Proxy Group]",
    "节点选择 = select, DIRECT",
    "",
    "[Rule]",
    "GEOIP,CN,DIRECT",
    "FINAL,节点选择",
    "",
  ].join("\n"),
  loon: [
    "[General]",
    "loglevel = notify",
    "",
    "[Proxy Group]",
    "节点选择 = select, DIRECT",
    "",
    "[Rule]",
    "GEOIP,CN,DIRECT",
    "FINAL,节点选择",
    "",
  ].join("\n"),
  quanx: [
    ";[general]",
    "server_check_url=http://cp.cloudflare.com/generate_204",
    "",
    ";[policy]",
    "static=节点选择, 直连",
    "",
    ";[filter_local]",
    "geoip, cn, 直连",
    "final, 节点选择",
    "",
  ].join("\n"),
};

type FormState = {
  name: string;
  client: PanelTemplateClient;
  remark: string;
  visible: boolean;
  isDefault: boolean;
};

const EMPTY_FORM: FormState = {
  name: "",
  client: "clash",
  remark: "",
  visible: true,
  isDefault: false,
};

export default function TemplatesPage() {
  const [items, setItems] = useState<PanelTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [client, setClient] = useState<PanelTemplateClient | "all">("all");
  const [visibleOnly, setVisibleOnly] = useState(false);

  const editor = useDisclosure();
  const preview = useDisclosure();
  const fileInput = useRef<HTMLInputElement | null>(null);

  const [editing, setEditing] = useState<PanelTemplate | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [content, setContent] = useState("");
  const [saving, setSaving] = useState(false);
  const [contentLoading, setContentLoading] = useState(false);

  const [previewItem, setPreviewItem] = useState<PanelTemplate | null>(null);
  const [previewContent, setPreviewContent] = useState("");
  const [previewError, setPreviewError] = useState("");

  const load = useCallback(async () => {
    try {
      const res = await getPanelTemplates();
      if (res.code === 0) setItems(res.data || []);
      else toast.error(res.msg || "加载模板失败");
    } catch {
      toast.error("加载模板失败");
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const visible = useMemo(() => {
    return items.filter((item) => {
      if (client !== "all" && item.client !== client) return false;
      if (visibleOnly && item.visible === 0) return false;
      return true;
    });
  }, [items, client, visibleOnly]);

  const openCreate = () => {
    setEditing(null);
    setForm(EMPTY_FORM);
    setContent(BLANK.clash);
    editor.onOpen();
  };

  const openEdit = async (item: PanelTemplate) => {
    setEditing(item);
    setForm({
      name: item.name,
      client: item.client,
      remark: item.remark,
      visible: item.visible !== 0,
      isDefault: item.isDefault === 1,
    });
    setContent("");
    setContentLoading(true);
    editor.onOpen();
    try {
      const res = await getPanelTemplate(item.id);
      if (res.code === 0 && res.data) setContent(res.data.content || "");
      else toast.error(res.msg || "读取模板内容失败");
    } catch {
      toast.error("读取模板内容失败");
    }
    setContentLoading(false);
  };

  const openPreview = async (item: PanelTemplate) => {
    setPreviewItem(item);
    setPreviewContent("");
    setPreviewError("");
    preview.onOpen();
    try {
      const res = await getPanelTemplate(item.id);
      if (res.code === 0 && res.data) setPreviewContent(res.data.content || "");
      else setPreviewError(res.msg || "读取模板内容失败");
    } catch {
      setPreviewError("读取模板内容失败,检查网络或面板版本");
    }
  };

  const changeClient = (next: PanelTemplateClient) => {
    const previous = form.client;
    setForm({ ...form, client: next });
    // 内容还是空白骨架(或还没动过)时就换成新客户端的骨架,免得给 sing-box
    // 塞一份 YAML。用户自己写过的内容一律不动。
    if (!editing && (content.trim() === "" || content === BLANK[previous])) {
      setContent(BLANK[next]);
    }
  };

  const handleSave = async () => {
    const name = form.name.trim();
    if (!name) {
      toast.error("请填模板名");
      return;
    }
    // 内容还没读回来就保存会把模板清空,直接拦住。
    if (editing && contentLoading) {
      toast.error("模板内容还在读取,稍等一下再保存");
      return;
    }
    if (form.client === "singbox" && content.trim()) {
      try {
        JSON.parse(content);
      } catch (e) {
        toast.error("sing-box 模板必须是合法 JSON: " + (e as Error).message);
        return;
      }
    }
    if (
      form.client === "clash" &&
      content.trim().startsWith("{") &&
      !jsonValid(content)
    ) {
      toast.error("这段内容看起来是 JSON,但解析不过,检查一下括号/逗号");
      return;
    }
    const payload = {
      name,
      client: form.client,
      version: (form.client === "surge" ||
      form.client === "loon" ||
      form.client === "quanx"
        ? "v2"
        : "v3") as "v2" | "v3",
      visible: form.visible ? 1 : 0,
      isDefault: form.isDefault ? 1 : 0,
      remark: form.remark.trim(),
      content,
    };
    setSaving(true);
    try {
      const res = editing
        ? await updatePanelTemplate(editing.id, payload)
        : await createPanelTemplate(payload);
      if (res.code === 0) {
        // 设为默认要单独走一次接口:后端会把其它模板的默认位取消。
        if (form.isDefault && res.data?.id)
          await setPanelTemplateDefault(res.data.id);
        toast.success(editing ? "已保存" : "已创建");
        editor.onClose();
        await load();
      } else {
        toast.error(res.msg || "保存失败");
      }
    } catch {
      toast.error("保存失败");
    }
    setSaving(false);
  };

  const handleSetDefault = async (item: PanelTemplate) => {
    try {
      const res = await setPanelTemplateDefault(item.id);
      if (res.code === 0) {
        toast.success(`「${item.name}」已设为默认模板`);
        await load();
      } else {
        toast.error(res.msg || "设置失败");
      }
    } catch {
      toast.error("设置失败");
    }
  };

  const handleDuplicate = async (item: PanelTemplate) => {
    try {
      const detail = await getPanelTemplate(item.id);
      if (detail.code !== 0) {
        toast.error(detail.msg || "读取模板失败");
        return;
      }
      const res = await createPanelTemplate({
        name: `${item.name.replace(/\.[^.]+$/, "")}-copy${CLIENT_EXT[item.client]}`,
        client: item.client,
        version: item.version,
        visible: item.visible,
        isDefault: 0,
        remark: item.remark,
        content: detail.data?.content || "",
      });
      if (res.code === 0) {
        toast.success("已复制");
        await load();
      } else {
        toast.error(res.msg || "复制失败");
      }
    } catch {
      toast.error("复制失败");
    }
  };

  const handleReset = async (item: PanelTemplate) => {
    if (
      !window.confirm(
        `把「${item.name}」的内容恢复成内置原版?你改过的内容会丢。`,
      )
    )
      return;
    try {
      const res = await resetPanelTemplate(item.id);
      if (res.code === 0) {
        toast.success("已恢复内置内容");
        await load();
      } else {
        toast.error(res.msg || "恢复失败");
      }
    } catch {
      toast.error("恢复失败");
    }
  };

  const handleDelete = async (item: PanelTemplate) => {
    if (
      !window.confirm(
        `删除模板「${item.name}」?引用它的订阅链接会退回默认模板。`,
      )
    )
      return;
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

  const handleUpload = async (file: File | undefined) => {
    if (!file) return;
    const text = await file.text();
    setContent(text);
    setForm((prev) => ({
      ...prev,
      name: prev.name || file.name,
      client: detectClient(file.name, text, prev.client),
    }));
    toast.success(`已读取 ${file.name}`);
  };

  const copyContent = async () => {
    try {
      await navigator.clipboard.writeText(previewContent);
      toast.success("内容已复制");
    } catch {
      toast.error("复制失败");
    }
  };

  const handleSeed = async () => {
    if (!window.confirm("导入内置模板(6 个)?同名的会跳过,不会覆盖你改过的内容。")) return;
    toast.loading("导入中…", { id: "seed" });
    try {
      const res = await seedPanelDefaults();
      if (res.code === 0) {
        toast.success(`已导入 ${res.data?.templatesAdded ?? 0} 个模板`, { id: "seed" });
        await load();
      } else {
        toast.error(res.msg || "导入失败", { id: "seed" });
      }
    } catch {
      toast.error("导入失败", { id: "seed" });
    }
  };

  const hiddenCount = items.filter((item) => item.visible === 0).length;

  return (
    <PageWrapper
      title="模板管理"
      description="订阅输出的配置骨架。Clash 支持 YAML/JSON 骨架(节点与规则集渲染时注入),sing-box 用 JSON;默认模板自动应用到未指定模板的订阅链接。"
    >
      <Card className="mb-4">
        {/* CardBody 自带 flex-col,想一行排列必须再包一层 row */}
        <CardBody className="py-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="mr-auto whitespace-nowrap text-sm text-default-500">
              共 {items.length} 个模板
              {hiddenCount > 0 && (
                <span className="text-default-400">
                  {" "}
                  · {hiddenCount} 个已隐藏
                </span>
              )}
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <div className="flex items-center overflow-hidden rounded-medium border border-default-200">
                {CLIENTS.map((entry) => {
                  const active = client === entry.key;
                  return (
                    <button
                      key={entry.key}
                      className={
                        "px-2.5 py-1.5 text-[13px] transition-colors " +
                        (active
                          ? "bg-default-100 font-medium text-foreground"
                          : "text-default-500 hover:bg-default-50 dark:hover:bg-default-100/10")
                      }
                      type="button"
                      onClick={() => setClient(entry.key)}
                    >
                      {entry.label}
                    </button>
                  );
                })}
              </div>
              <Button
                color={visibleOnly ? "primary" : "default"}
                size="sm"
                startContent={<EyeIcon size={16} />}
                variant={visibleOnly ? "flat" : "bordered"}
                onPress={() => setVisibleOnly((value) => !value)}
              >
                用户可见性
              </Button>
              <Button size="sm" variant="bordered" onPress={handleSeed}>
                导入内置模板
              </Button>
              <Button
                color="primary"
                size="sm"
                startContent={<PlusIcon size={18} />}
                onPress={openCreate}
              >
                新建模板
              </Button>
            </div>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardBody className="p-0">
          {loading ? (
            <div className="flex justify-center py-16">
              <Spinner />
            </div>
          ) : visible.length === 0 ? (
            <div className="px-6 py-16 text-center text-default-500">
              {items.length === 0
                ? "还没有模板。点右上角「导入内置模板」一键写入 6 个内置模板。"
                : "这个筛选下没有模板。"}
            </div>
          ) : (
            <Table aria-label="模板列表" removeWrapper>
              <TableHeader>
                <TableColumn>模板名称</TableColumn>
                <TableColumn width={180}>更新时间</TableColumn>
                <TableColumn align="end" width={260}>
                  操作
                </TableColumn>
              </TableHeader>
              <TableBody>
                {visible.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-mono text-sm font-medium">
                          {item.name}
                        </span>
                        <Chip
                          className="text-[11px]"
                          size="sm"
                          variant="bordered"
                        >
                          {CLIENT_LABEL[item.client] || item.client}
                        </Chip>
                        {item.isDefault === 1 && (
                          <Chip color="primary" size="sm" variant="flat">
                            默认
                          </Chip>
                        )}
                        {item.builtin === 1 && (
                          <Chip size="sm" variant="flat">
                            内置
                          </Chip>
                        )}
                        {item.visible === 0 && (
                          <Chip color="warning" size="sm" variant="flat">
                            已隐藏
                          </Chip>
                        )}
                      </div>
                      {item.remark && (
                        <div className="mt-1 text-xs text-default-400">
                          {item.remark}
                        </div>
                      )}
                    </TableCell>
                    <TableCell>
                      <span className="text-xs text-default-400">
                        {new Date(item.updatedTime).toLocaleString()}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end gap-0.5">
                        <Button
                          isIconOnly
                          className={
                            item.isDefault === 1
                              ? "text-warning"
                              : "text-default-500"
                          }
                          size="sm"
                          title={
                            item.isDefault === 1
                              ? "当前默认模板"
                              : "设为默认模板"
                          }
                          variant="light"
                          onPress={() => handleSetDefault(item)}
                        >
                          <StarIcon filled={item.isDefault === 1} size={17} />
                        </Button>
                        <Button
                          isIconOnly
                          className="text-default-500"
                          size="sm"
                          title="编辑"
                          variant="light"
                          onPress={() => openEdit(item)}
                        >
                          <EditIcon size={16} />
                        </Button>
                        <Button
                          isIconOnly
                          className="text-default-500"
                          size="sm"
                          title="复制一份"
                          variant="light"
                          onPress={() => handleDuplicate(item)}
                        >
                          <CopyIcon size={16} />
                        </Button>
                        <Button
                          isIconOnly
                          className="text-default-500"
                          isDisabled={item.builtin !== 1}
                          size="sm"
                          title={
                            item.builtin === 1
                              ? "恢复内置内容"
                              : "自建模板没有出厂内容"
                          }
                          variant="light"
                          onPress={() => handleReset(item)}
                        >
                          <RefreshIcon size={16} />
                        </Button>
                        <Button
                          isIconOnly
                          className="text-default-500"
                          size="sm"
                          title="预览"
                          variant="light"
                          onPress={() => openPreview(item)}
                        >
                          <EyeIcon size={16} />
                        </Button>
                        <Button
                          isIconOnly
                          className="text-danger"
                          size="sm"
                          title="删除"
                          variant="light"
                          onPress={() => handleDelete(item)}
                        >
                          <DeleteIcon size={16} />
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

      {/* 预览 */}
      <Modal
        isOpen={preview.isOpen}
        scrollBehavior="inside"
        size="5xl"
        onClose={preview.onClose}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            <span className="font-mono text-base">{previewItem?.name}</span>
            <span className="text-xs font-normal text-default-400">
              {previewItem && CLIENT_LABEL[previewItem.client]} ·{" "}
              {(previewContent.length / 1024).toFixed(1)} KB
            </span>
          </ModalHeader>
          <ModalBody>
            {previewError ? (
              <div className="rounded-medium bg-danger-50 px-3 py-6 text-center text-sm text-danger dark:bg-danger/10">
                {previewError}
              </div>
            ) : previewContent ? (
              <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap break-all rounded-medium bg-default-100/60 p-3 font-mono text-xs leading-relaxed dark:bg-default-100/10">
                {previewContent}
              </pre>
            ) : (
              <div className="flex justify-center py-10">
                <Spinner />
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button
              size="sm"
              startContent={<CopyIcon size={16} />}
              variant="flat"
              onPress={copyContent}
            >
              复制内容
            </Button>
            <Button size="sm" variant="light" onPress={preview.onClose}>
              关闭
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 新建 / 编辑 */}
      <Modal
        isOpen={editor.isOpen}
        scrollBehavior="inside"
        size="5xl"
        onClose={editor.onClose}
      >
        <ModalContent>
          <ModalHeader>
            {editing ? `编辑模板 · ${editing.name}` : "新建模板"}
          </ModalHeader>
          <ModalBody className="gap-4">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <Input
                isRequired
                label="模板名称"
                placeholder={`例如 my${CLIENT_EXT[form.client]}`}
                value={form.name}
                onValueChange={(value) => setForm({ ...form, name: value })}
              />
              <Select
                label="客户端"
                selectedKeys={[form.client]}
                onSelectionChange={(keys) =>
                  changeClient(Array.from(keys)[0] as PanelTemplateClient)
                }
              >
                {CLIENTS.filter((entry) => entry.key !== "all").map((entry) => (
                  <SelectItem
                    key={entry.key}
                  >{`${entry.label} (${CLIENT_EXT[entry.key as PanelTemplateClient]})`}</SelectItem>
                ))}
              </Select>
              <Input
                label="备注"
                placeholder="可选,一句话说明用途"
                value={form.remark}
                onValueChange={(value) => setForm({ ...form, remark: value })}
              />
              <div className="flex items-end gap-6 pb-1">
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-default-500">用户可见</span>
                  <Switch
                    isSelected={form.visible}
                    size="sm"
                    onValueChange={(value) =>
                      setForm({ ...form, visible: value })
                    }
                  />
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-default-500">设为默认</span>
                  <Switch
                    isSelected={form.isDefault}
                    size="sm"
                    onValueChange={(value) =>
                      setForm({ ...form, isDefault: value })
                    }
                  />
                </div>
              </div>
            </div>

            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-xs text-default-400">
                {form.client === "singbox"
                  ? "JSON。用 __PROXY_OUTBOUNDS__ / __PROXY_TAGS__ 占位,渲染时替换成真实节点。"
                  : form.client === "clash"
                    ? "YAML 或 JSON。YAML 用 proxies: null 与 - __PROXY_NODES__ 占位。"
                    : "纯文本配置,节点由客户端侧的订阅导入。"}
              </span>
              <div className="flex items-center gap-2">
                <input
                  ref={fileInput}
                  accept=".yaml,.yml,.json,.conf,.lcf,.qx,.txt"
                  className="hidden"
                  type="file"
                  onChange={(event) => {
                    void handleUpload(event.target.files?.[0]);
                    event.target.value = "";
                  }}
                />
                <Button
                  size="sm"
                  startContent={<UploadIcon size={15} />}
                  variant="bordered"
                  onPress={() => fileInput.current?.click()}
                >
                  上传文件
                </Button>
              </div>
            </div>

            {contentLoading ? (
              <div className="flex justify-center py-10">
                <Spinner />
              </div>
            ) : (
              <Textarea
                classNames={{ input: "font-mono text-xs leading-relaxed" }}
                minRows={20}
                placeholder="粘贴模板内容…"
                value={content}
                onValueChange={setContent}
              />
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={editor.onClose}>
              取消
            </Button>
            <Button color="primary" isLoading={saving} onPress={handleSave}>
              {editing ? "保存" : "创建"}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </PageWrapper>
  );
}

function jsonValid(text: string) {
  try {
    JSON.parse(text);
    return true;
  } catch {
    return false;
  }
}

function detectClient(
  filename: string,
  text: string,
  fallback: PanelTemplateClient,
): PanelTemplateClient {
  const lower = filename.toLowerCase();
  if (lower.endsWith(".json"))
    return text.includes("__PROXY_OUTBOUNDS__") ? "singbox" : "clash";
  if (lower.endsWith(".conf")) return "surge";
  if (lower.endsWith(".lcf")) return "loon";
  if (lower.endsWith(".qx")) return "quanx";
  if (lower.endsWith(".yaml") || lower.endsWith(".yml")) return "clash";
  return fallback;
}
