import { useEffect, useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import toast from "react-hot-toast";
import { createPaymentOrder, getCurrentSubscription, getSubscriptionPlans, redeemSubscriptionCode } from "@/api";

const formatBytes = (value: number) => { const amount = Number(value || 0); if (!amount) return "0 B"; if (amount >= 1073741824) return `${(amount / 1073741824).toFixed(2)} GB`; if (amount >= 1048576) return `${(amount / 1048576).toFixed(2)} MB`; return `${Math.round(amount / 1024)} KB`; };
const planTraffic = (value: number) => Number(value || 0) ? formatBytes(value) : "不限";

export default function SubscriptionPage() {
  const [plans, setPlans] = useState<any[]>([]); const [current, setCurrent] = useState<any>(null); const [code, setCode] = useState("");
  const load = async () => { const [p, c] = await Promise.all([getSubscriptionPlans(), getCurrentSubscription()]); if (p.code === 0) setPlans(p.data || []); if (c.code === 0) setCurrent(c.data); };
  useEffect(() => { load(); }, []);
  const redeem = async () => { const r = await redeemSubscriptionCode(code); if (r.code !== 0) toast.error(r.msg); else { toast.success("兑换成功"); setCode(""); load(); } };
  const buy = async (id: string) => { const r = await createPaymentOrder(id, "manual"); if (r.code !== 0) toast.error(r.msg); else toast.success(`订单 ${r.data.orderNo} 已创建，请联系管理员完成支付`); };
  return <div className="p-4 space-y-4 max-w-5xl"><div><h1 className="text-xl font-bold">套餐中心</h1><p className="text-sm text-default-500">查看账号级流量、到期时间和转发数量限制</p></div>
    {current && <Card><CardHeader><h2 className="font-semibold">当前套餐：{current.planName || "已开通"}</h2></CardHeader><CardBody><div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm"><div>规格：<b>{current.planValidityUnit === "permanent" ? "永久" : `${current.planValidityValue || "-"}${current.planValidityUnit === "year" ? "年" : "个月"}`}</b></div><div>流量：<b>{formatBytes(current.trafficUsedBytes)} / {planTraffic(current.trafficLimitBytes)}</b></div><div>到期：<b>{current.expiresAt === 0 ? "永久" : current.expiresAt ? new Date(current.expiresAt).toLocaleString() : "-"}</b></div><div>转发：<b>{current.usedForwards || 0} / {current.maxForwards || "不限"}</b></div></div></CardBody></Card>}
    <Card><CardHeader><h2 className="font-semibold">兑换码</h2></CardHeader><CardBody><div className="flex flex-col gap-2 sm:flex-row max-w-xl"><Input value={code} onChange={e => setCode(e.target.value)} placeholder="粘贴完整兑换码" /><Button className="w-full sm:w-auto" color="primary" onClick={redeem} isDisabled={!code}>兑换</Button></div></CardBody></Card>
    <div className="grid md:grid-cols-3 gap-4">{plans.map(plan => <Card key={String(plan.id)}><CardHeader className="flex-col items-start"><h2 className="font-semibold">{plan.name}</h2><span className="text-2xl font-bold">{plan.price} {plan.currency}</span></CardHeader><CardBody className="gap-3 text-sm"><p>{plan.description || "账号级订阅套餐"}</p><p>有效期：{plan.validityValue} {plan.validityUnit === "year" ? "年" : "个月"}</p><p>流量：{plan.trafficBytes ? `${Math.round(plan.trafficBytes / 1073741824)} GB` : "不限"}</p><Button color="primary" onClick={() => buy(String(plan.id))}>创建购买订单</Button></CardBody></Card>)}</div>
  </div>;
}
