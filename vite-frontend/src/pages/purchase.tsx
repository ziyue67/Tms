import { useEffect, useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Select, SelectItem } from "@heroui/select";
import { QRCodeSVG } from "qrcode.react";
import toast from "react-hot-toast";
import { createPaymentOrder, getCurrentSubscription, getPaymentOrder, getPaymentProviders, getSubscriptionPlans } from "@/api";
const traffic = (value: number) => value ? `${(value / 1073741824).toFixed(0)} GB` : "不限";
const usedTraffic = (value: number) => value ? traffic(value) : "0 GB";
const validity = (plan: any) => plan.validityUnit === "permanent" ? "永久" : `${plan.validityValue} ${plan.validityUnit === "year" ? "年" : "个月"}`;
const quotaMode = (plan: any) => plan.resetQuota === 0 ? "有效期内总量，不按月恢复" : "每月恢复完整流量";

export default function PurchasePage() {
  const [plans, setPlans] = useState<any[]>([]); const [providers, setProviders] = useState<any[]>([]); const [provider, setProvider] = useState(""); const [order, setOrder] = useState<any>(null); const [checkout, setCheckout] = useState<any>(null); const [current, setCurrent] = useState<any>(null); const [busy, setBusy] = useState<string | null>(null);
  useEffect(() => { Promise.all([getSubscriptionPlans(), getPaymentProviders(), getCurrentSubscription()]).then(([planRes, providerRes, currentRes]) => { if (planRes.code === 0) setPlans(Array.isArray(planRes.data) ? planRes.data : []); else toast.error(planRes.msg || "加载套餐失败"); if (providerRes.code === 0) { const list = Array.isArray(providerRes.data) ? providerRes.data : []; setProviders(list); setProvider(list[0]?.key || ""); } if (currentRes.code === 0) setCurrent(currentRes.data || null); }); }, []);
  const buy = async (planId: string) => { setBusy(planId); const res = await createPaymentOrder(planId, provider); if (res.code === 0) { setOrder(res.data?.order || res.data); setCheckout(res.data?.checkout || null); toast.success("订单已创建"); } else toast.error(res.msg || "创建订单失败"); setBusy(null); };
  const refreshOrder = async () => { if (!order?.orderNo) return; const res = await getPaymentOrder(order.orderNo); if (res.code === 0) setOrder(res.data); else toast.error(res.msg || "查询订单失败"); };
  const continuePayment = () => {
    if (!checkout?.url) return;
    if (checkout.type === "redirect") { window.location.assign(checkout.url); return; }
    if (checkout.type === "form") {
      const form = document.createElement("form"); form.method = "post"; form.action = checkout.url;
      Object.entries(checkout.fields || {}).forEach(([name, value]) => { const input = document.createElement("input"); input.type = "hidden"; input.name = name; input.value = String(value); form.appendChild(input); });
      document.body.appendChild(form); form.submit();
    }
  };
  return <div className="p-4 space-y-4 max-w-6xl"><div><h1 className="text-xl font-bold">充值/订阅</h1><p className="text-sm text-default-500">选择套餐和已启用的支付渠道创建订单</p></div>
    {current?.planName && <Card><CardHeader><b>当前订阅：{current.planName}</b></CardHeader><CardBody className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm"><div>有效期：<b>{(current.planValidityUnit || current.validityUnit) === "permanent" ? "永久" : `${current.planValidityValue || current.validityValue || "-"}${(current.planValidityUnit || current.validityUnit) === "year" ? "年" : "个月"}`}</b></div><div>流量：<b>{traffic(current.trafficLimitBytes)}（已用 {usedTraffic(current.trafficUsedBytes)}）</b></div><div>到期：<b>{current.expiresAt === 0 ? "永久" : current.expiresAt ? new Date(current.expiresAt).toLocaleString() : "-"}</b></div><div>转发：<b>{current.usedForwards || 0} / {current.maxForwards || "不限"}</b></div></CardBody></Card>}
    <div className="max-w-xs"><Select label="支付方式" selectedKeys={provider ? [provider] : []} isDisabled={!providers.length} onSelectionChange={(keys) => setProvider(String(Array.from(keys)[0] || ""))}>{providers.map((item) => <SelectItem key={item.key}>{item.label}</SelectItem>)}</Select>{!providers.length && <p className="text-xs text-danger mt-1">支付系统已关闭或管理员尚未启用支付方式</p>}</div>
    {order && <Card><CardBody className="flex flex-wrap items-center gap-4 text-sm"><span>订单号：<b>{order.orderNo}</b></span><span>状态：<b>{order.status}</b></span><Button size="sm" variant="flat" onPress={refreshOrder}>刷新状态</Button>{order.status === "pending" && checkout?.type === "qr" && <div className="flex items-center gap-3"><QRCodeSVG value={checkout.url} size={128} level="M" includeMargin /><span className="text-default-500">{checkout.message || "请扫码完成付款"}</span></div>}{order.status === "pending" && (checkout?.type === "redirect" || checkout?.type === "form") && <Button size="sm" color="primary" onPress={continuePayment}>前往支付</Button>}{order.status === "pending" && checkout?.type === "manual" && <span className="text-default-500">{checkout.message}</span>}</CardBody></Card>}
    {plans.length === 0 ? <Card><CardBody className="py-8 text-center text-sm text-default-500">暂无可购买套餐，请联系管理员在套餐管理中启用“允许购买”和“启用”。</CardBody></Card> : <div className="grid md:grid-cols-3 gap-4">{plans.map((plan) => <Card key={String(plan.id)}><CardHeader className="flex-col items-start"><div className="font-semibold">{plan.name}</div><div className="text-2xl font-bold">{plan.price} {plan.currency}</div></CardHeader><CardBody className="space-y-2 text-sm"><p>{plan.description || "账号级套餐"}</p><p>有效期：{validity(plan)}</p><p>流量：{traffic(plan.trafficBytes)}</p><p>额度模式：{quotaMode(plan)}</p><p>转发：{plan.maxForwards || "不限"}</p><Button color="primary" isDisabled={!provider} isLoading={busy === String(plan.id)} onPress={() => buy(String(plan.id))}>创建订单</Button></CardBody></Card>)}</div>}
  </div>;
}
