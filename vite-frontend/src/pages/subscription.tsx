import { useEffect, useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import toast from "react-hot-toast";
import { createPaymentOrder, getCurrentSubscription, getSubscriptionPlans, redeemSubscriptionCode } from "@/api";

export default function SubscriptionPage() {
  const [plans, setPlans] = useState<any[]>([]); const [current, setCurrent] = useState<any>(null); const [code, setCode] = useState("");
  const load = async () => { const [p, c] = await Promise.all([getSubscriptionPlans(), getCurrentSubscription()]); if (p.code === 0) setPlans(p.data || []); if (c.code === 0) setCurrent(c.data); };
  useEffect(() => { load(); }, []);
  const redeem = async () => { const r = await redeemSubscriptionCode(code); if (r.code !== 0) toast.error(r.msg); else { toast.success("兑换成功"); setCode(""); load(); } };
  const buy = async (id: number) => { const r = await createPaymentOrder(id, "manual"); if (r.code !== 0) toast.error(r.msg); else toast.success(`订单 ${r.data.orderNo} 已创建，请联系管理员完成支付`); };
  return <div className="p-4 space-y-4 max-w-5xl"><div><h1 className="text-xl font-bold">套餐中心</h1><p className="text-sm text-default-500">查看账号级流量、到期时间和转发数量限制</p></div>
    {current && <Card><CardHeader><h2 className="font-semibold">当前套餐</h2></CardHeader><CardBody><div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm"><div>到期：<b>{current.expiresAt ? new Date(current.expiresAt).toLocaleString() : "永久"}</b></div><div>流量：<b>{current.trafficUsedBytes || 0} / {current.trafficLimitBytes || "不限"}</b></div><div>重置：<b>{current.nextResetAt ? new Date(current.nextResetAt).toLocaleDateString() : "-"}</b></div><div>转发：<b>{current.usedForwards || 0} / {current.maxForwards || "不限"}</b></div></div></CardBody></Card>}
    <Card><CardHeader><h2 className="font-semibold">兑换码</h2></CardHeader><CardBody><div className="flex gap-2 max-w-xl"><Input value={code} onChange={e => setCode(e.target.value)} placeholder="XXXX-XXXX-XXXX-XXXX" /><Button color="primary" onClick={redeem} isDisabled={!code}>兑换</Button></div></CardBody></Card>
    <div className="grid md:grid-cols-3 gap-4">{plans.map(plan => <Card key={plan.id}><CardHeader className="flex-col items-start"><h2 className="font-semibold">{plan.name}</h2><span className="text-2xl font-bold">{plan.price} {plan.currency}</span></CardHeader><CardBody className="gap-3 text-sm"><p>{plan.description || "账号级订阅套餐"}</p><p>有效期：{plan.validityValue} {plan.validityUnit === "year" ? "年" : "个月"}</p><p>流量：{plan.trafficBytes ? `${Math.round(plan.trafficBytes / 1073741824)} GB` : "不限"}</p><Button color="primary" onClick={() => buy(plan.id)}>创建购买订单</Button></CardBody></Card>)}</div>
  </div>;
}
