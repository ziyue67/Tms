import { useEffect, useMemo, useState } from "react";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Button } from "@heroui/button";
import { Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import toast from "react-hot-toast";
import { getSubscriptionDashboard } from "@/api";

const bytes = (value?: number) => {
  const amount = Number(value || 0);
  if (amount === 0) return "不限";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let unit = 0; let current = amount;
  while (current >= 1024 && unit < units.length - 1) { current /= 1024; unit++; }
  return `${current.toFixed(unit === 0 ? 0 : 2)} ${units[unit]}`;
};

export default function UserSubscriptionDashboardPage() {
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const load = async () => {
    setLoading(true);
    const response = await getSubscriptionDashboard();
    if (response.code === 0) setData(response.data || {});
    else toast.error(response.msg || "加载账户套餐失败");
    setLoading(false);
  };
  useEffect(() => { load(); }, []);
  const chart = useMemo(() => (data?.last24Hours || []).map((item: any) => ({
    time: item.time || (item.createdTime ? new Date(item.createdTime).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "-"),
    flow: Number(item.flow || 0),
  })), [data]);
  const total = Number(data?.totalTrafficBytes || 0);
  const used = Number(data?.usedTrafficBytes || 0);

  return <div className="p-4 space-y-4 max-w-6xl">
    <div className="flex items-center justify-between"><div><h1 className="text-xl font-bold">仪表盘</h1><p className="text-sm text-default-500">账号级套餐额度与近 24 小时用量</p></div><Button size="sm" variant="flat" onPress={load} isLoading={loading}>刷新</Button></div>
    <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
      <Card><CardBody><div className="text-xs text-default-500">套餐总流量</div><div className="text-lg font-semibold">{bytes(total)}</div></CardBody></Card>
      <Card><CardBody><div className="text-xs text-default-500">已使用</div><div className="text-lg font-semibold">{total ? bytes(used) : bytes(data?.accountUsedTrafficBytes)}</div></CardBody></Card>
      <Card><CardBody><div className="text-xs text-default-500">到期时间</div><div className="text-sm font-semibold">{data?.expiresAt ? new Date(data.expiresAt).toLocaleString() : "未开通套餐"}</div></CardBody></Card>
    </div>
    <Card><CardHeader><div><div className="font-semibold">近 24 小时流量</div><div className="text-xs text-default-500">按统计任务生成的小时流量</div></div></CardHeader><CardBody><div className="h-64">{chart.length ? <ResponsiveContainer width="100%" height="100%"><LineChart data={chart}><XAxis dataKey="time" minTickGap={24} /><YAxis tickFormatter={(value) => bytes(Number(value))} width={72} /><Tooltip formatter={(value) => bytes(Number(value))} /><Line type="monotone" dataKey="flow" stroke="#2563eb" strokeWidth={2} dot={false} /></LineChart></ResponsiveContainer> : <div className="h-full grid place-items-center text-sm text-default-400">暂无最近 24 小时流量记录</div>}</div></CardBody></Card>
  </div>;
}
