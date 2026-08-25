import { useEffect, useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Select, SelectItem } from "@heroui/select";
import toast from "react-hot-toast";
import { createPaymentOrder, getPaymentOrder, getPaymentProviders, getSubscriptionPlans } from "@/api";
const traffic = (value: number) => value ? `${(value / 1073741824).toFixed(0)} GB` : "不限";

export default function PurchasePage() {
  const [plans, setPlans] = useState<any[]>([]); const [providers, setProviders] = useState<any[]>([]); const [provider, setProvider] = useState(""); const [order, setOrder] = useState<any>(null); const [busy, setBusy] = useState<number | null>(null);
  useEffect(() => { Promise.all([getSubscriptionPlans(), getPaymentProviders()]).then(([planRes, providerRes]) => { if (planRes.code === 0) setPlans(planRes.data || []); else toast.error(planRes.msg || "加载套餐失败"); if (providerRes.code === 0) { const list = providerRes.data || []; setProviders(list); setProvider(list[0]?.key || ""); } }); }, []);
  const buy = async (planId: number) => { setBusy(planId); const res = await createPaymentOrder(planId, provider); if (res.code === 0) { setOrder(res.data); toast.success("订单已创建"); } else toast.error(res.msg || "创建订单失败"); setBusy(null); };
  const refreshOrder = async () => { if (!order?.orderNo) return; const res = await getPaymentOrder(order.orderNo); if (res.code === 0) setOrder(res.data); else toast.error(res.msg || "查询订单失败"); };
  return <div className="p-4 space-y-4 max-w-6xl"><div><h1 className="text-xl font-bold">购买套餐</h1><p className="text-sm text-default-500">选择套餐和已启用的支付渠道创建订单</p></div>
    <div className="max-w-xs"><Select label="支付方式" selectedKeys={provider ? [provider] : []} isDisabled={!providers.length} onSelectionChange={(keys) => setProvider(String(Array.from(keys)[0] || ""))}>{providers.map((item) => <SelectItem key={item.key}>{item.label}</SelectItem>)}</Select>{!providers.length && <p className="text-xs text-danger mt-1">管理员尚未启用支付方式</p>}</div>
    {order && <Card><CardBody className="flex flex-wrap items-center gap-4 text-sm"><span>订单号：<b>{order.orderNo}</b></span><span>状态：<b>{order.status}</b></span><Button size="sm" variant="flat" onPress={refreshOrder}>刷新状态</Button>{order.status === "pending" && <span className="text-default-500">请按支付渠道返回的支付信息完成付款，人工支付请联系管理员。</span>}</CardBody></Card>}
    <div className="grid md:grid-cols-3 gap-4">{plans.map((plan) => <Card key={plan.id}><CardHeader className="flex-col items-start"><div className="font-semibold">{plan.name}</div><div className="text-2xl font-bold">{plan.price} {plan.currency}</div></CardHeader><CardBody className="space-y-2 text-sm"><p>{plan.description || "账号级套餐"}</p><p>有效期：{plan.validityValue} {plan.validityUnit === "year" ? "年" : "个月"}</p><p>流量：{traffic(plan.trafficBytes)}</p><p>转发：{plan.maxForwards || "不限"}</p><Button color="primary" isDisabled={!provider} isLoading={busy === plan.id} onPress={() => buy(plan.id)}>创建订单</Button></CardBody></Card>)}</div>
  </div>;
}
