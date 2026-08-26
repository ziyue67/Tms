import { useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import toast from "react-hot-toast";
import { redeemSubscriptionCode } from "@/api";

export default function RedeemPage() {
  const [code, setCode] = useState(""); const [result, setResult] = useState<any>(null); const [busy, setBusy] = useState(false);
  const redeem = async () => { if (!code.trim()) return toast.error("请输入兑换码"); setBusy(true); const response = await redeemSubscriptionCode(code); if (response.code === 0) { setResult(response.data); setCode(""); toast.success("兑换成功，套餐已生效"); } else toast.error(response.msg || "兑换失败"); setBusy(false); };
  return <div className="p-4 max-w-xl space-y-4"><div><h1 className="text-xl font-bold">兑换码</h1><p className="text-sm text-default-500">兑换套餐后，账号级流量和转发额度会切换为新套餐配置。</p></div><Card><CardHeader><div className="font-semibold">兑换套餐</div></CardHeader><CardBody className="space-y-3"><Input label="兑换码" value={code} onChange={(event) => setCode(event.target.value.toUpperCase())} placeholder="ABCDE-FGHIJ-KLMNO-PQRST" /><Button color="primary" isLoading={busy} onPress={redeem}>立即兑换</Button></CardBody></Card>{result && <Card><CardBody className="text-sm space-y-1"><div>套餐已生效。</div><div>到期时间：{result.expiresAt === 0 ? "永久" : result.expiresAt ? new Date(result.expiresAt).toLocaleString() : "-"}</div><div>流量额度：{result.trafficLimitBytes ? `${(Number(result.trafficLimitBytes) / 1073741824).toFixed(2)} GB` : "不限"}</div></CardBody></Card>}</div>;
}
