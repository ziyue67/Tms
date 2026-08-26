import { useState, useEffect } from "react";
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input } from "@heroui/input";
import { Select, SelectItem } from "@heroui/select";
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter } from "@heroui/modal";
import { Chip } from "@heroui/chip";
import { Autocomplete, AutocompleteItem } from "@heroui/autocomplete";
import { DatePicker } from "@heroui/date-picker";
import { parseDate } from "@internationalized/date";
import toast from "react-hot-toast";
import {
  getInboundList,
  createInbound,
  oneClickInbound,
  deleteInboundsByNode,
  assignAllToUser,
  provisionSubscribedUsers,
  assignSelf,
  getNodeList,
  getAllUsers,
  getSpeedLimitList,
  getCustomNodes,
  importCustomNode,
  deleteCustomNode,
  disableCustomNode,
  buildMeteredCustomRelay,
} from "@/api";
import { copyTextToClipboard } from "@/utils/clipboard";
import { SNI_PRESETS, DEFAULT_SNI, cleanSni } from "@/config/sni";
import { SubQr } from "@/components/sub-qr";

/**
 * 协议管理(合体面板)· 机器卡模式。
 * 一台机器 = 一张卡(卡上折叠着这台机器的全套协议)。
 * 卡上「分配用户」→ 把这台机器所有协议一次分给车友 → 出一条订阅链接。
 * 车友加这一条订阅,机器上全部协议自动到手,以后加新协议自动更新。
 */
export default function InboundPage() {
  const [inbounds, setInbounds] = useState<any[]>([]);
  const [nodes, setNodes] = useState<any[]>([]);
  const [users, setUsers] = useState<any[]>([]);
  const [speedRules, setSpeedRules] = useState<any[]>([]);
  const [customNodes, setCustomNodes] = useState<any[]>([]);
  const [customOpen, setCustomOpen] = useState(false);
  const [customForm, setCustomForm] = useState<{ name: string; link: string; visibility: "global" | "users"; userIds: string[]; ingressNodeId: number | null; sni: string }>({ name: "", link: "", visibility: "global", userIds: [], ingressNodeId: null, sni: DEFAULT_SNI });
  const [customLoading, setCustomLoading] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState<any>({ nodeId: null, protocol: "vless", sni: DEFAULT_SNI, dest: "", remark: "" });
  const [createLoading, setCreateLoading] = useState(false);

  const [oneClickOpen, setOneClickOpen] = useState(false);
  const [oneClickNodeId, setOneClickNodeId] = useState<number | null>(null);
  const [oneClickSni, setOneClickSni] = useState<string>(DEFAULT_SNI);
  const [oneClickScope, setOneClickScope] = useState<'manual' | 'global'>('manual');
  const [oneClickLoading, setOneClickLoading] = useState(false);
  const [meteredRelayOpen, setMeteredRelayOpen] = useState(false);
  const [meteredRelayForm, setMeteredRelayForm] = useState<{ customNode: any | null; ingressNodeId: number | null; sni: string }>({ customNode: null, ingressNodeId: null, sni: DEFAULT_SNI });
  const [meteredRelayLoading, setMeteredRelayLoading] = useState(false);

  // 机器卡「分配用户」:把整台机器的协议分给车友(只分配,链接去「用户管理」拿)
  const [assignOpen, setAssignOpen] = useState(false);
  const [assignForm, setAssignForm] = useState<any>({ nodeId: null, nodeName: "", protocolCount: 0, userId: null, speedId: null, expDate: null, flowGb: null });
  const [assignLoading, setAssignLoading] = useState(false);

  // 「我自己用」:一键开给当前管理员自己,完事直接把订阅链接弹出来
  const [selfLoading, setSelfLoading] = useState<number | null>(null);
  const [globalProvisioning, setGlobalProvisioning] = useState<number | null>(null);
  const [selfSubUrl, setSelfSubUrl] = useState<string>("");
  const [selfOpen, setSelfOpen] = useState(false);
  // 订阅链接的域名部分永远是【面板地址】,几台机器点出来长得几乎一样,
  // 只有末尾 token 不同 —— 不写清楚是哪台机器,很容易以为"点第二台弹的还是第一台"
  const [selfNodeName, setSelfNodeName] = useState<string>("");

  const handleAssignSelf = async (nodeId: number, nodeName?: string) => {
    setSelfLoading(nodeId);
    setSelfNodeName(nodeName || "");
    try {
      const res = await assignSelf({ nodeId });
      if (res.code === 0 && res.data?.subToken) {
        setSelfSubUrl(`${window.location.origin}/api/v1/open_api/sub?token=${res.data.subToken}`);
        setSelfOpen(true);
        loadAll();
      } else {
        toast.error(res.msg || "开通失败");
      }
    } catch (e) {
      toast.error("开通失败");
    }
    setSelfLoading(null);
  };

  const loadAll = async () => {
    try {
      const [ib, nd, us, sp, cn] = await Promise.all([
        getInboundList(),
        getNodeList(),
        getAllUsers(),
        getSpeedLimitList(),
        getCustomNodes(),
      ]);
      if (ib.code === 0) setInbounds(ib.data || []);
      if (nd.code === 0) setNodes(nd.data || []);
      if (us.code === 0) {
        const d: any = us.data;
        setUsers(Array.isArray(d) ? d : (d && d.records ? d.records : []));
      }
      if (sp.code === 0) setSpeedRules(sp.data || []);
      if (cn.code === 0) setCustomNodes(cn.data || []);
    } catch (e) {
      toast.error("加载失败");
    }
  };

  useEffect(() => {
    loadAll();
  }, []);

  const protoLabel = (p: string) =>
    (({ vless: "VLESS-Reality", trojan: "Trojan-Reality", vmess: "VMess", shadowsocks: "Shadowsocks-2022", hysteria2: "Hysteria2", tuic: "TUIC", anytls: "AnyTLS" } as any)[p] || p);
  const isReality = (p: string) => p === "vless" || p === "trojan";

  const handleCreate = async () => {
    if (!createForm.nodeId) return toast.error("请选择节点");
    if (isReality(createForm.protocol) && !createForm.sni) return toast.error("Reality 协议需要填 SNI");
    setCreateLoading(true);
    try {
      const payload: any = {
        nodeId: createForm.nodeId,
        protocol: createForm.protocol,
        remark: createForm.remark,
      };
      if (isReality(createForm.protocol)) {
        payload.sni = cleanSni(createForm.sni);
        payload.dest = createForm.dest;
      }
      const res = await createInbound(payload);
      if (res.code === 0) {
        toast.success("入站已创建");
        setCreateOpen(false);
        loadAll();
      } else {
        toast.error(res.msg || "创建失败");
      }
    } catch (e) {
      toast.error("创建失败");
    }
    setCreateLoading(false);
  };

  const handleOneClick = async () => {
    if (!oneClickNodeId) return toast.error("请选择节点");
    setOneClickLoading(true);
    try {
      const res = await oneClickInbound(oneClickNodeId, cleanSni(oneClickSni));
      if (res.code === 0) {
        if (oneClickScope === 'global') {
          const provisioned = await provisionSubscribedUsers(oneClickNodeId);
          if (provisioned.code !== 0) {
            toast.error(`协议已创建，但全局分配失败：${provisioned.msg || '请在机器卡重试'}`);
          } else {
            const errors = Array.isArray(provisioned.data?.errors) ? provisioned.data.errors : [];
            if (errors.length) toast.error(`协议已创建；${provisioned.data?.provisionedUsers || 0} 个用户已分配，${errors.length} 个失败`);
            else toast.success(`整机协议已创建，并已分配给 ${provisioned.data?.provisionedUsers || 0} 个套餐用户`);
          }
        } else {
          toast.success("一键添加完成:整机全套协议已建好");
        }
        setOneClickOpen(false);
        loadAll();
      } else {
        toast.error(res.msg || "一键添加失败");
      }
    } catch (e) {
      toast.error("一键添加失败");
    }
    setOneClickLoading(false);
  };

  const openMeteredRelay = (customNode: any) => {
    setMeteredRelayForm({ customNode, ingressNodeId: null, sni: DEFAULT_SNI });
    setMeteredRelayOpen(true);
  };

  const handleBuildMeteredRelay = async () => {
    const { customNode, ingressNodeId, sni } = meteredRelayForm;
    if (!customNode || !ingressNodeId) return toast.error('请选择承载中转的 TMS 节点');
    setMeteredRelayLoading(true);
    try {
      const response = await buildMeteredCustomRelay(customNode.id, ingressNodeId, cleanSni(sni));
      if (response.code !== 0) return toast.error(response.msg || '创建计费中转失败');
      const provisioned = response.data?.provision?.provisionedUsers || 0;
      toast.success(`已将「${customNode.name}」设为计费中转，并分配给 ${provisioned} 个套餐用户`);
      setMeteredRelayOpen(false);
      loadAll();
    } catch (error) {
      toast.error('创建计费中转失败');
    } finally {
      setMeteredRelayLoading(false);
    }
  };

  const openNodeAssign = (n: any, count: number) => {
    setAssignForm({ nodeId: n.id, nodeName: n.name, protocolCount: count, userId: null, speedId: null, expDate: null, flowGb: null });
    setAssignOpen(true);
  };

  const handleNodeAssign = async () => {
    if (!assignForm.userId) return toast.error("请选择车友");
    setAssignLoading(true);
    try {
      const payload: any = { userId: assignForm.userId, nodeId: assignForm.nodeId };
      if (assignForm.speedId) payload.speedId = assignForm.speedId;
      // 到期直接选日期(当天 23:59:59 截止),比填"多少天"直观,续费也只是把日期往后改
      if (assignForm.expDate) payload.expTime = new Date(`${assignForm.expDate}T23:59:59`).getTime();
      if (assignForm.flowGb) payload.flow = Math.round(assignForm.flowGb); // 单位 GB(线路配额按 GB 存)
      const res = await assignAllToUser(payload);
      if (res.code === 0) {
        {
          const a = res.data?.assigned ?? 0, u = res.data?.updated ?? 0;
          toast.success(
            a > 0
              ? `已分配 ${a} 个协议` + (u ? `,更新 ${u} 个` : "") + " · 订阅链接去「用户管理」拿"
              : u > 0
              ? `已更新这条线路的限速/到期/流量(${u} 个协议)`
              : "配额和到期已更新"
          );
        }
        setAssignOpen(false);
        loadAll();
      } else {
        toast.error(res.msg || "分配失败");
      }
    } catch (e) {
      toast.error("分配失败");
    }
    setAssignLoading(false);
  };

  const handleClearNode = async (nodeId: number, nodeName: string) => {
    if (!window.confirm(`确定清空「${nodeName}」上的直连协议?(连带其转发/用户;中转协议不受影响)`)) return;
    const res = await deleteInboundsByNode(nodeId, false);
    if (res.code === 0) {
      toast.success("已清空该机协议");
      loadAll();
    } else {
      toast.error(res.msg || "清空失败");
    }
  };

  const handleProvisionSubscribedUsers = async (nodeId: number, nodeName: string) => {
    if (!window.confirm(`将「${nodeName}」的原生协议分配给所有已开通套餐的普通用户？每个用户会获得独立凭据，流量会计入其套餐。`)) return;
    setGlobalProvisioning(nodeId);
    try {
      const response = await provisionSubscribedUsers(nodeId);
      if (response.code !== 0) return toast.error(response.msg || '全局分配失败');
      const data = response.data || {};
      const errors = Array.isArray(data.errors) ? data.errors : [];
      if (errors.length) toast.error(`已开通 ${data.provisionedUsers || 0} 个用户，${errors.length} 个失败`);
      else toast.success(`已开通 ${data.provisionedUsers || 0} 个套餐用户`);
      loadAll();
    } catch (error) {
      toast.error('全局分配失败');
    } finally {
      setGlobalProvisioning(null);
    }
  };

  const handleImportCustomNode = async () => {
    if (!customForm.link.trim()) return toast.error("请输入协议分享链接");
    if (customForm.visibility === "users" && customForm.userIds.length === 0) return toast.error("按用户订阅时至少选择一个用户");
    setCustomLoading(true);
    try {
      const imported = await importCustomNode(customForm.name, customForm.link, customForm.visibility, customForm.userIds.map(Number), customForm.ingressNodeId, cleanSni(customForm.sni));
      if (imported.code !== 0) return toast.error(imported.msg || "导入失败");
      toast.success(customForm.ingressNodeId ? "已创建可计费中转并分配给套餐用户" : "自定义节点已导入");
      setCustomOpen(false); setCustomForm({ name: "", link: "", visibility: "global", userIds: [], ingressNodeId: null, sni: DEFAULT_SNI }); loadAll();
    } catch (e) { toast.error("导入失败"); }
    finally { setCustomLoading(false); }
  };

  // 协议管理只管【直连】协议(landingId 为空);中转的协议在「中转」页管
  const machineNodes = nodes.filter((n) => inbounds.some((ib) => ib.nodeId === n.id && !ib.landingId));

  return (
    <div className="p-4 space-y-4">
      <div className="flex justify-between items-center">
        <h1 className="text-xl font-bold">协议管理</h1>
        <div className="flex gap-2">
          <Button
            color="secondary"
            onPress={() => {
              setOneClickNodeId(null);
              setOneClickScope('manual');
              setOneClickOpen(true);
            }}
          >
            ⚡ 一键搭建整机协议
          </Button>
          <Button
            color="primary"
            variant="flat"
            onPress={() => {
              setCreateForm({ nodeId: null, protocol: "vless", sni: DEFAULT_SNI, dest: "", remark: "" });
              setCreateOpen(true);
            }}
          >
            单独加一个协议
          </Button>
          <Button variant="bordered" onPress={() => setCustomOpen(true)}>导入自定义协议</Button>
        </div>
      </div>

      {/* 一机一卡:每台机器的全套协议折叠成一条记录,卡上直接分配用户 */}
      <div className="grid gap-3 md:grid-cols-2">
        {machineNodes.map((n) => {
          const nodeInbounds = inbounds.filter((ib) => ib.nodeId === n.id && !ib.landingId);
          const online = n.status === 1;
          const firstIp = n.ip ? String(n.ip).split(",")[0].trim() : (n.serverIp || "");
          return (
            <Card key={n.id}>
              <CardBody className="space-y-3">
                <div className="flex items-center gap-2">
                  <span className="text-lg font-semibold truncate">🖥️ {n.name}</span>
                  <Chip size="sm" variant="flat" color={online ? "success" : "default"}>{online ? "在线" : "离线"}</Chip>
                  <Chip size="sm" variant="flat" color="primary" className="ml-auto">{nodeInbounds.length} 协议</Chip>
                </div>
                {firstIp && <div className="text-xs text-default-500 font-mono">{firstIp}</div>}

                {/* 节点在线 ≠ 协议可用:gost 和 sing-box 是两个服务,sing-box 挂了
                    这里照样显示「在线」,但这台机上所有协议全都连不上。必须单独标出来 —— 
                    不然只会以为是协议参数配错了,往那个方向查很久都查不出来 */}
                {online && n.singboxRunning === false && nodeInbounds.length > 0 && (
                  n.singboxInstalling ? (
                    <div className="rounded-lg border border-default-300 bg-default-100 px-3 py-2 space-y-1">
                      <div className="text-sm font-medium text-default-600">⏳ sing-box 正在安装,请稍候…</div>
                      <div className="text-xs text-default-500">
                        首次建协议时会现下 sing-box(约 57MB),一般 1-2 分钟。装好后这里自动恢复正常,不用管。
                      </div>
                    </div>
                  ) : n.singboxInstallErr ? (
                    <div className="rounded-lg border border-danger/40 bg-danger/10 px-3 py-2 space-y-1">
                      <div className="text-sm font-semibold text-danger">⚠️ sing-box 安装失败,这台机的协议全部不可用</div>
                      <div className="text-xs text-default-500 break-all">
                        节点报的原因:<code className="font-mono">{n.singboxInstallErr}</code>
                      </div>
                      <div className="text-xs text-default-500">
                        多半是这台机下载 GitHub 失败。国内机器改用镜像版命令重跑节点安装脚本(见 README)。
                      </div>
                    </div>
                  ) : (
                    <div className="rounded-lg border border-danger/40 bg-danger/10 px-3 py-2 space-y-1">
                      <div className="text-sm font-semibold text-danger">⚠️ sing-box 未运行,这台机的协议全部不可用</div>
                      {n.singboxInstalled === false ? (
                        <div className="text-xs text-default-500">
                          这台机上<span className="text-danger font-medium">根本没装 sing-box</span> —— 装节点时从 GitHub
                          下载失败了(国内机常见)。到这台机上重跑一次节点安装脚本即可,装好后面板会自动把协议配置推下去,
                          不用重新分配。
                        </div>
                      ) : (
                        <div className="text-xs text-default-500">
                          节点本身在线(gost 正常),但跑协议的 sing-box 没起来。到这台机上执行:
                          <code className="font-mono bg-default-200 px-1 rounded ml-1">systemctl enable --now sing-box</code>
                          <div className="mt-1">
                            若报 <code className="font-mono">Unit file sing-box.service does not exist</code>,说明根本没装上
                            (下载 GitHub 失败),重跑一次节点安装脚本即可。
                          </div>
                        </div>
                      )}
                    </div>
                  )
                )}
                <div className="flex flex-wrap gap-1">
                  {nodeInbounds.map((ib) => (
                    <Chip key={ib.id} size="sm" variant="flat" color="secondary">{protoLabel(ib.protocol)}</Chip>
                  ))}
                </div>
                <div className="text-xs text-default-400">
                  整机一条订阅:分配给车友后,一条订阅链接导入客户端即拿到上面全部协议,以后加新协议自动更新。
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" color="primary" className="flex-1" onPress={() => openNodeAssign(n, nodeInbounds.length)}>
                    👤 分配用户
                  </Button>
                  <Button
                    size="sm"
                    color="secondary"
                    variant="flat"
                    isLoading={globalProvisioning === n.id}
                    onPress={() => handleProvisionSubscribedUsers(n.id, n.name)}
                  >
                    全局分配套餐用户
                  </Button>
                  {/* 自己用不必先建车友再分配:一键开给当前管理员,不限速不限量不到期 */}
                  <Button
                    size="sm"
                    color="success"
                    variant="flat"
                    isLoading={selfLoading === n.id}
                    onPress={() => handleAssignSelf(n.id, n.name)}
                  >
                    🔑 我自己用
                  </Button>
                  <Button size="sm" color="danger" variant="flat" onPress={() => handleClearNode(n.id, n.name)}>
                    清空该机
                  </Button>
                </div>
              </CardBody>
            </Card>
          );
        })}
      </div>
      {machineNodes.length === 0 && (
        <div className="text-center text-default-400 py-8">还没有协议,点右上角「⚡ 一键搭建整机协议」在某台机器上把全套协议建出来</div>
      )}

      {customNodes.length > 0 && <Card>
        <CardBody className="space-y-3">
          <div className="font-semibold">自定义订阅节点</div>
            <div className="text-xs text-default-500">外部订阅默认直连，不能识别具体用户流量。点击“设为计费中转”后，用户连接 TMS 原生协议，流量才会计入套餐。</div>
          {customNodes.map((node) => <div key={node.id} className="flex flex-wrap items-center gap-2 border-t border-divider pt-3">
            <Chip size="sm" color={node.status === 1 ? "success" : "default"}>{({ vless: "VLESS-Reality", trojan: "Trojan-Reality", vmess: "VMess", hysteria2: "Hysteria2", tuic: "TUIC", anytls: "AnyTLS" } as any)[node.protocol] || node.protocol}</Chip>
            <span className="font-medium">{node.name}</span>
            <Chip size="sm" variant="flat" color="primary">{node.visibility === "users" ? `按用户 (${(node.userIds || []).length})` : "全局聚合"}</Chip>
            {node.visibility === "users" && (node.userIds || []).map((id: number) => {
              const user = users.find((u) => Number(u.id) === Number(id));
              return <Chip key={id} size="sm" variant="flat">{user?.user || `用户 #${id}`}</Chip>;
            })}
            {node.status === 1 && <Button size="sm" color="secondary" variant="flat" onPress={() => openMeteredRelay(node)}>设为计费中转</Button>}
            {node.status === 1 && <Button size="sm" color="warning" variant="flat" onPress={async () => { if (window.confirm(`停用「${node.name}」？`)) { const r = await disableCustomNode(node.id); if (r.code === 0) { toast.success("已停用"); loadAll(); } else toast.error(r.msg || "停用失败"); } }}>停用</Button>}
            <Button size="sm" color="danger" variant="flat" onPress={async () => { if (window.confirm(`永久删除「${node.name}」？此操作不可恢复`)) { const r = await deleteCustomNode(node.id); if (r.code === 0) { toast.success("已删除"); loadAll(); } else toast.error(r.msg || "删除失败"); } }}>删除</Button>
          </div>)}
        </CardBody>
      </Card>}

      {/* 「我自己用」结果:直接把订阅链接给出来,不用再去用户管理找 */}
      <Modal isOpen={selfOpen} onClose={() => setSelfOpen(false)} size="2xl">
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            <span>🔑 已开给你自己(不限速 · 不限流量 · 不限到期)</span>
            {selfNodeName && (
              <span className="text-sm font-normal text-default-500">
                机器:<b className="text-foreground">{selfNodeName}</b>
              </span>
            )}
          </ModalHeader>
          <ModalBody className="space-y-2">
            <div className="text-sm text-default-500">
              这条订阅是给你自己用的,复制到 v2rayN / 小火箭 里就能用。以后随时在「我的订阅」页也能找到。
            </div>
            <div className="text-xs text-default-400 bg-default-100 rounded-lg px-3 py-2">
              💡 链接前半段是<b>面板地址</b>,所以每台机器点出来都一样 —— 真正区分线路的是末尾的
              <b> token</b>。拉下来的节点才是这台机器的。
            </div>
            <Input
              readOnly
              value={selfSubUrl}
              onClick={(e: any) => { if (e.target?.select) e.target.select(); }}
            />
            <SubQr url={selfSubUrl} />
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setSelfOpen(false)}>关闭</Button>
            <Button
              color="primary"
              onPress={async () => {
                (await copyTextToClipboard(selfSubUrl))
                  ? toast.success("已复制订阅链接")
                  : toast.error("复制失败,点框内已全选,按 Ctrl+C");
              }}
            >
              复制订阅链接
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal isOpen={customOpen} onClose={() => setCustomOpen(false)} size="2xl">
        <ModalContent>
          <ModalHeader>导入自定义协议节点</ModalHeader>
          <ModalBody className="space-y-3">
            <Input label="协议分享链接" value={customForm.link} onChange={(e) => setCustomForm({ ...customForm, link: e.target.value })} placeholder="vless://、trojan://、vmess://、hysteria2://、tuic://、anytls://" />
            <Input label="显示名称（可空）" value={customForm.name} onChange={(e) => setCustomForm({ ...customForm, name: e.target.value })} />
            <Select
              label="订阅范围"
              selectedKeys={[customForm.visibility]}
              onSelectionChange={(keys) => setCustomForm({ ...customForm, visibility: String(Array.from(keys)[0] || "global") as "global" | "users", userIds: String(Array.from(keys)[0] || "global") === "users" ? customForm.userIds : [] })}
            >
              <SelectItem key="global">全局聚合（所有用户）</SelectItem>
              <SelectItem key="users">按用户订阅</SelectItem>
            </Select>
            {customForm.visibility === "users" && <Select
              label="选择用户"
              placeholder="选择一个或多个用户"
              selectionMode="multiple"
              selectedKeys={new Set(customForm.userIds)}
              onSelectionChange={(keys) => setCustomForm({ ...customForm, userIds: Array.from(keys).map(String) })}
            >
              {users.map((u) => <SelectItem key={String(u.id)}>{u.user}</SelectItem>)}
            </Select>}
            <Select
              label="中转出站（转发机）"
              placeholder="不选：仅导入外部订阅，不能按套餐计费"
              selectedKeys={customForm.ingressNodeId ? [String(customForm.ingressNodeId)] : []}
              onSelectionChange={(keys) => {
                const id = Array.from(keys)[0];
                setCustomForm({ ...customForm, ingressNodeId: id ? Number(id) : null });
              }}
            >
              {nodes.filter((node) => node.status === 1).map((node) => <SelectItem key={String(node.id)}>{node.name}</SelectItem>)}
            </Select>
            {customForm.ingressNodeId && <Autocomplete
              label="伪装域名（Reality）"
              allowsCustomValue
              defaultItems={SNI_PRESETS}
              inputValue={customForm.sni}
              onInputChange={(value) => setCustomForm({ ...customForm, sni: value })}
              onSelectionChange={(key) => { if (key) setCustomForm({ ...customForm, sni: String(key) }); }}
            >
              {(item: any) => <AutocompleteItem key={item.value}>{item.label}</AutocompleteItem>}
            </Autocomplete>}
            <p className="text-xs text-default-500">选择转发机后会自动创建“转发机 → 此节点”的 TMS 中转，原始外部链接不再下发，全部用户按各自套餐总流量和到期时间计费。</p>
          </ModalBody>
          <ModalFooter><Button variant="light" onPress={() => setCustomOpen(false)}>取消</Button><Button color="primary" isLoading={customLoading} onPress={handleImportCustomNode}>导入</Button></ModalFooter>
        </ModalContent>
      </Modal>

      {/* 机器卡「分配用户」:整机协议一次分给车友,出一条订阅链接 */}
      <Modal isOpen={assignOpen} onClose={() => setAssignOpen(false)}>
        <ModalContent>
          <ModalHeader>👤 给车友分配「{assignForm.nodeName}」</ModalHeader>
          <ModalBody className="space-y-3">
            <div className="text-sm text-default-500">
              把这台机器上的 <b>{assignForm.protocolCount} 个协议</b> 一次分给车友。分配完到「用户管理」页,点该车友的「🔗 订阅链接」拿链接发给他。
            </div>
            <Select
              label="子账号(车友)"
              placeholder="选一个车友"
              selectedKeys={assignForm.userId ? [String(assignForm.userId)] : []}
              onSelectionChange={(k) => setAssignForm({ ...assignForm, userId: Number(Array.from(k)[0]) })}
            >
              {users.map((u) => (<SelectItem key={u.id}>{u.user}</SelectItem>))}
            </Select>
            <Select
              label="限速规则(可空)"
              placeholder="不限速"
              selectedKeys={assignForm.speedId ? [String(assignForm.speedId)] : []}
              onSelectionChange={(k) => setAssignForm({ ...assignForm, speedId: Number(Array.from(k)[0]) })}
            >
              {speedRules.map((s) => (<SelectItem key={s.id}>{s.name}</SelectItem>))}
            </Select>
            <DatePicker
              label="到期日期(留空=永久)"
              value={assignForm.expDate ? parseDate(assignForm.expDate) as any : null}
              onChange={(d: any) => setAssignForm({
                ...assignForm,
                expDate: d ? `${d.year}-${String(d.month).padStart(2, "0")}-${String(d.day).padStart(2, "0")}` : null,
              })}
              showMonthAndYearPickers
              className="cursor-pointer"
              description="到这天 23:59 自动停;续费直接把日期往后改再点一次分配"
            />
            <Input
              type="number"
              label="这条线路的流量配额(GB,留空=不单独限)"
              value={assignForm.flowGb ?? ""}
              onChange={(e) => setAssignForm({ ...assignForm, flowGb: e.target.value ? Number(e.target.value) : null })}
              description="只算这条线路的用量,超了只停这条,车友其它线路照用;留空则只受账号总流量约束"
            />
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setAssignOpen(false)}>关闭</Button>
            <Button color="primary" isLoading={assignLoading} onPress={handleNodeAssign}>分配</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 一键搭建整机协议:选机器,把所有支持的协议一键全建出来 */}
      <Modal isOpen={oneClickOpen} onClose={() => setOneClickOpen(false)}>
        <ModalContent>
          <ModalHeader>⚡ 一键搭建整机协议</ModalHeader>
          <ModalBody className="space-y-3">
            <div className="text-sm text-default-500">
              在选中的机器上一键建好全部协议:<b>VLESS-Reality、Trojan-Reality、VMess、Hysteria2、TUIC、AnyTLS</b>(端口、密钥、自签证书全自动;端口被占自动上移)。建好后就是一张机器卡,点「分配用户」出订阅即可。
            </div>
            <Select
              label="机器"
              placeholder="选一台机器(需在线)"
              selectedKeys={oneClickNodeId ? [String(oneClickNodeId)] : []}
              onSelectionChange={(k) => setOneClickNodeId(Number(Array.from(k)[0]))}
            >
              {nodes.map((n) => (
                <SelectItem key={n.id}>{n.name}</SelectItem>
              ))}
            </Select>
            <Select
              label="订阅范围"
              selectedKeys={[oneClickScope]}
              onSelectionChange={(keys) => setOneClickScope(String(Array.from(keys)[0] || 'manual') as 'manual' | 'global')}
              description={oneClickScope === 'global' ? '创建后立即给所有有效套餐用户生成独立凭据，流量按用户套餐统计。' : '只创建协议，之后可在机器卡手动分配用户。'}
            >
              <SelectItem key="manual">手动分配用户</SelectItem>
              <SelectItem key="global">全局聚合（所有有效套餐用户）</SelectItem>
            </Select>
            {/* Reality 借壳域名:给个常用列表,也允许自己输 */}
            <Autocomplete
              label="伪装域名(Reality 借壳)"
              allowsCustomValue
              defaultItems={SNI_PRESETS}
              inputValue={oneClickSni}
              onInputChange={(v) => setOneClickSni(v)}
              onSelectionChange={(k) => { if (k) setOneClickSni(String(k)); }}
              description="只影响 VLESS / Trojan 这两个 Reality 协议。可以直接输入别的域名;别用 www.microsoft.com(它上了后量子,握不上手)"
            >
              {(item: any) => <AutocompleteItem key={item.value} description={item.desc || undefined}>{item.label}</AutocompleteItem>}
            </Autocomplete>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setOneClickOpen(false)}>取消</Button>
            <Button color="secondary" isLoading={oneClickLoading} onPress={handleOneClick}>一键全建</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal isOpen={meteredRelayOpen} onClose={() => setMeteredRelayOpen(false)}>
        <ModalContent>
          <ModalHeader>设为计费中转</ModalHeader>
          <ModalBody className="space-y-3">
            <p className="text-sm text-default-500">“{meteredRelayForm.customNode?.name}”将作为出站。用户订阅会拿到 TMS 的独立原生协议，流量经该外部节点出网并计入各自套餐。</p>
            <Select
              label="承载中转的 TMS 节点"
              placeholder="选择在线节点"
              selectedKeys={meteredRelayForm.ingressNodeId ? [String(meteredRelayForm.ingressNodeId)] : []}
              onSelectionChange={(keys) => setMeteredRelayForm({ ...meteredRelayForm, ingressNodeId: Number(Array.from(keys)[0]) })}
            >
              {nodes.filter((node) => node.status === 1).map((node) => <SelectItem key={String(node.id)}>{node.name}</SelectItem>)}
            </Select>
            <Autocomplete
              label="伪装域名（Reality）"
              allowsCustomValue
              defaultItems={SNI_PRESETS}
              inputValue={meteredRelayForm.sni}
              onInputChange={(value) => setMeteredRelayForm({ ...meteredRelayForm, sni: value })}
              onSelectionChange={(key) => { if (key) setMeteredRelayForm({ ...meteredRelayForm, sni: String(key) }); }}
            >
              {(item: any) => <AutocompleteItem key={item.value}>{item.label}</AutocompleteItem>}
            </Autocomplete>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setMeteredRelayOpen(false)}>取消</Button>
            <Button color="secondary" isLoading={meteredRelayLoading} onPress={handleBuildMeteredRelay}>创建并全局分配</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 单独加一个协议(补充用) */}
      <Modal isOpen={createOpen} onClose={() => setCreateOpen(false)}>
        <ModalContent>
          <ModalHeader>单独加一个协议</ModalHeader>
          <ModalBody className="space-y-3">
            <Select
              label="协议"
              selectedKeys={[createForm.protocol]}
              onSelectionChange={(k) => setCreateForm({ ...createForm, protocol: String(Array.from(k)[0]) })}
              description={
                isReality(createForm.protocol)
                  ? "无域名借 Reality(SNI 借壳),抗封锁强(推荐)"
                  : createForm.protocol === "vmess"
                  ? "VMess:TCP 无 TLS,无域名,兼容各种老客户端"
                  : ["hysteria2", "tuic", "anytls"].includes(createForm.protocol)
                  ? "自签证书(无域名);客户端需勾选\"允许不安全/insecure\"。Hy2/TUIC 是 QUIC,快"
                  : "Shadowsocks-2022:无 TLS、任何客户端都通,简单稳"
              }
            >
              <SelectItem key="vless">VLESS-Reality(无域名,推荐)</SelectItem>
              <SelectItem key="trojan">Trojan-Reality(无域名)</SelectItem>
              <SelectItem key="vmess">VMess(无域名,兼容老客户端)</SelectItem>
              <SelectItem key="hysteria2">Hysteria2(QUIC,快,自签证书)</SelectItem>
              <SelectItem key="tuic">TUIC(QUIC,自签证书)</SelectItem>
              <SelectItem key="anytls">AnyTLS(自签证书)</SelectItem>
            </Select>
            <Select
              label="机器"
              placeholder="选一台机器"
              selectedKeys={createForm.nodeId ? [String(createForm.nodeId)] : []}
              onSelectionChange={(k) => setCreateForm({ ...createForm, nodeId: Number(Array.from(k)[0]) })}
            >
              {nodes.map((n) => (
                <SelectItem key={n.id}>{n.name}</SelectItem>
              ))}
            </Select>
            {isReality(createForm.protocol) && (
              <>
                <Autocomplete
                  label="伪装域名(Reality 借壳)"
                  allowsCustomValue
                  defaultItems={SNI_PRESETS}
                  inputValue={createForm.sni}
                  onInputChange={(v) => setCreateForm({ ...createForm, sni: v })}
                  onSelectionChange={(k) => { if (k) setCreateForm({ ...createForm, sni: String(k) }); }}
                  description="可以直接输入别的域名;别用 www.microsoft.com(它上了后量子,Reality 握不上手)"
                >
                  {(item: any) => <AutocompleteItem key={item.value} description={item.desc || undefined}>{item.label}</AutocompleteItem>}
                </Autocomplete>
                <Input
                  label="Reality 目标(留空=同 SNI)"
                  value={createForm.dest}
                  onChange={(e) => setCreateForm({ ...createForm, dest: e.target.value })}
                />
              </>
            )}
            <Input
              label="备注"
              value={createForm.remark}
              onChange={(e) => setCreateForm({ ...createForm, remark: e.target.value })}
            />
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setCreateOpen(false)}>取消</Button>
            <Button color="primary" isLoading={createLoading} onPress={handleCreate}>创建</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
