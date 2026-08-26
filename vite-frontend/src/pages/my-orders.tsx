import { useEffect, useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import toast from "react-hot-toast";
import { getMyPaymentOrders } from "@/api";

const statusLabel: Record<string, string> = {
  pending: "待支付",
  paid: "已支付",
  failed: "失败",
  cancelled: "已取消",
};

export default function MyOrdersPage() {
  const [orders, setOrders] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const load = async () => {
    setLoading(true);
    const response = await getMyPaymentOrders();
    if (response.code === 0) setOrders(response.data || []);
    else toast.error(response.msg || "加载订单失败");
    setLoading(false);
  };
  useEffect(() => { load(); }, []);

  return <div className="p-4 space-y-4 max-w-5xl">
    <div className="flex items-center justify-between">
      <div><h1 className="text-xl font-bold">我的订单</h1><p className="text-sm text-default-500">查看充值/订阅订单和支付状态。</p></div>
      <Button size="sm" variant="flat" onPress={load} isLoading={loading}>刷新</Button>
    </div>
    <Card><CardHeader><b>订单记录</b></CardHeader><CardBody>
      {orders.length === 0 ? <div className="py-10 text-center text-sm text-default-400">暂无订单</div> : <div className="overflow-x-auto"><table className="w-full min-w-[640px] text-sm"><thead><tr className="text-left text-default-500"><th className="py-2">订单号</th><th>金额</th><th>支付方式</th><th>状态</th><th>创建时间</th></tr></thead><tbody>{orders.map((order) => <tr className="border-t border-divider" key={order.id || order.orderNo}><td className="py-3 font-mono">{order.orderNo}</td><td>{order.amount} {order.currency}</td><td>{order.provider}</td><td>{statusLabel[order.status] || order.status}</td><td>{order.createdTime ? new Date(order.createdTime).toLocaleString() : "-"}</td></tr>)}</tbody></table></div>}
    </CardBody></Card>
  </div>;
}
