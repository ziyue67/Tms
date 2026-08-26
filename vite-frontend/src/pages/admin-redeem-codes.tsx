import { useEffect, useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import { Select, SelectItem } from "@heroui/select";
import toast from "react-hot-toast";
import { generateRedeemCodes, getAdminPlans, getAdminRedeemCodes, revokeRedeemCode } from "@/api";

export default function AdminRedeemCodesPage() {
  const [plans, setPlans] = useState<any[]>([]); const [codes, setCodes] = useState<any[]>([]); const [planId, setPlanId] = useState(""); const [count, setCount] = useState("10"); const [generated, setGenerated] = useState(""); const [loading, setLoading] = useState(false);
  const load = async () => { const [plansResponse, codesResponse] = await Promise.all([getAdminPlans(), getAdminRedeemCodes()]); if (plansResponse.code === 0) setPlans(plansResponse.data || []); if (codesResponse.code === 0) setCodes(codesResponse.data || []); };
  useEffect(() => { load(); }, []);
  const makeCodes = async () => { if (!planId) return toast.error("请选择套餐"); const amount = Math.min(1000, Math.max(1, Number(count) || 1)); setLoading(true); const response = await generateRedeemCodes(planId, amount); if (response.code === 0) { setGenerated((response.data || []).join("\n")); toast.success(`已生成 ${response.data?.length || amount} 张兑换码`); load(); } else toast.error(response.msg || "生成失败"); setLoading(false); };
  const revoke = async (id: string) => { const response = await revokeRedeemCode(id); if (response.code === 0) { toast.success("兑换码已作废"); load(); } else toast.error(response.msg || "作废失败"); };
  return <div className="p-4 space-y-4 max-w-6xl">
    <div><h1 className="text-xl font-bold">兑换码</h1><p className="text-sm text-default-500">批量生成、查询和作废套餐兑换码。数据库只保存兑换码哈希。</p></div>
    <Card><CardHeader><b>批量生成</b></CardHeader><CardBody className="grid md:grid-cols-3 gap-3"><Select label="套餐" selectedKeys={planId ? [planId] : []} onSelectionChange={(keys) => setPlanId(String(Array.from(keys)[0] || ""))}>{plans.filter((plan) => Number(plan.status) === 1 && Number(plan.redeemable) === 1).map((plan) => <SelectItem key={String(plan.id)}>{plan.name}</SelectItem>)}</Select><Input label="数量（1-1000）" type="number" min={1} max={1000} value={count} onChange={(event) => setCount(event.target.value)} /><Button className="self-end" color="primary" onPress={makeCodes} isLoading={loading}>生成兑换码</Button>{generated && <pre className="md:col-span-3 max-h-48 overflow-auto rounded bg-default-100 p-3 text-sm whitespace-pre-wrap">{generated}</pre>}</CardBody></Card>
    <Card><CardHeader><b>兑换码记录</b></CardHeader><CardBody><div className="overflow-x-auto"><table className="w-full min-w-[640px] text-sm"><thead><tr className="text-left text-default-500"><th className="py-2">预览</th><th>套餐</th><th>批次</th><th>状态</th><th /></tr></thead><tbody>{codes.map((code) => <tr className="border-t border-divider" key={String(code.id)}><td className="py-3 font-mono">{code.codePreview}</td><td>{plans.find((plan) => String(plan.id) === String(code.planId))?.name || code.planId}</td><td>{code.batchId || "-"}</td><td>{code.status === 1 ? "可用" : code.status === 0 ? "已使用" : "已作废"}</td><td>{code.status === 1 && <Button size="sm" color="danger" variant="light" onPress={() => revoke(String(code.id))}>作废</Button>}</td></tr>)}</tbody></table>{codes.length === 0 && <div className="py-8 text-center text-sm text-default-400">暂无兑换码</div>}</div></CardBody></Card>
  </div>;
}
