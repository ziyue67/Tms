import { useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import toast from "react-hot-toast";
import { redeemSubscriptionCode } from "@/api";

export default function RedeemPage() {
  const [code, setCode] = useState(""); const [result, setResult] = useState<any>(null); const [busy, setBusy] = useState(false);
  const redeem = async () => { if (!code.trim()) return toast.error("请输入兑换码"); setBusy(true); const response = await redeemSubscriptionCode(code); if (response.code === 0) { setResult(response.data); setCode(""); toast.success("兑换成功，套餐已生效"); } else toast.error(response.msg || "兑换失败"); setBusy(false); };
  return <div className="p-4 max-w-xl space-y-4"><div><h1 className="text-xl font-bold">兑换码</h1><p className="text-sm text-default-500">兑换套餐后，账号级流量和转发额度会切换为新套餐配置。</p></div><Card><CardHeader><div className="font-semibold">兑换套餐</div></CardHeader><CardBody className="space-y-3"><Input label="兑换码" value={code} onChange={(event) => setCode(event.target.value.toUpperCase())} placeholder="ABCDE-FGHIJ-KLMNO-PQRST" /><Button color="primary" isLoading={busy} onPress={redeem}>立即兑换</Button></CardBody></Card>{result && <Card><CardHeader><b>套餐已生效</b></CardHeader><CardBody className="grid grid-cols-2 gap-3 text-sm"><div>套餐：<b>{result.planName || "套餐"}</b></div><div>有效期：<b>{result.validityUnit === "permanent" ? "永久" : `${result.planValidityValue || result.validityValue || "-"}${result.planValidityUnit === "year" || result.validityUnit === "year" ? "年" : "个月"}`}</b></div><div>流量额度：<b>{result.trafficLimitBytes ? `${(Number(result.trafficLimitBytes) / 1073741824).toFixed(2)} GB` : "不限"}</b></div><div>转发上限：<b>{result.maxForwards ? `${result.maxForwards} 个` : "不限"}</b></div><div className="col-span-2">到期时间：<b>{result.expiresAt === 0 ? "永久" : result.expiresAt ? new Date(result.expiresAt).toLocaleString() : "-"}</b></div></CardBody></Card>}</div>;
}
