import { useEffect, useState } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import toast from "react-hot-toast";
import { completeAdminTestOrder, getAdminPaymentOrders, retryAdminPaymentOrder } from "@/api";

const statusLabel: Record<string, string> = { pending: "待支付", paid: "已支付", failed: "失败", cancelled: "已取消" };

export default function AdminOrdersPage() {
  const [orders, setOrders] = useState<any[]>([]); const [loading, setLoading] = useState(false);
  const load = async () => { setLoading(true); const response = await getAdminPaymentOrders(); if (response.code === 0) setOrders(response.data || []); else toast.error(response.msg || "加载订单失败"); setLoading(false); };
  useEffect(() => { load(); }, []);
  const retry = async (orderNo: string) => { const response = await retryAdminPaymentOrder(orderNo); if (response.code === 0) toast.success("支付指令已重新生成"); else toast.error(response.msg || "重试失败"); };
  const completeTest = async (orderNo: string) => { const response = await completeAdminTestOrder(orderNo); if (response.code === 0) { toast.success("测试订单已完成"); load(); } else toast.error(response.msg || "测试支付未启用"); };
  return <div className="p-4 space-y-4 max-w-6xl"><div className="flex items-center justify-between"><div><h1 className="text-xl font-bold">订单管理</h1><p className="text-sm text-default-500">查看所有充值/订阅订单，处理失败订单和测试支付。</p></div><Button size="sm" variant="flat" onPress={load} isLoading={loading}>刷新</Button></div><Card><CardHeader><b>订单列表</b></CardHeader><CardBody><div className="overflow-x-auto"><table className="w-full min-w-[900px] text-sm"><thead><tr className="text-left text-default-500"><th className="py-2">订单号</th><th>用户</th><th>渠道</th><th>金额</th><th>状态</th><th>创建时间</th><th /></tr></thead><tbody>{orders.map((order) => <tr className="border-t border-divider" key={order.id || order.orderNo}><td className="py-3 font-mono">{order.orderNo}</td><td>{order.userId}</td><td>{order.provider}</td><td>{order.amount} {order.currency}</td><td>{statusLabel[order.status] || order.status}</td><td>{order.createdTime ? new Date(order.createdTime).toLocaleString() : "-"}</td><td>{order.status !== "paid" && <div className="flex gap-1"><Button size="sm" variant="light" onPress={() => retry(order.orderNo)}>重试</Button><Button size="sm" variant="light" onPress={() => completeTest(order.orderNo)}>测试完成</Button></div>}</td></tr>)}</tbody></table>{orders.length === 0 && <div className="py-8 text-center text-sm text-default-400">暂无订单</div>}</div></CardBody></Card></div>;
}
