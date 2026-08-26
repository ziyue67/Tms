import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import toast from "react-hot-toast";
import { getAuthConfig, register, sendRegisterCode } from "@/api";
import DefaultLayout from "@/layouts/default";
import { title } from "@/components/primitives";

export default function RegisterPage() {
  const navigate = useNavigate();
  const [form, setForm] = useState({ email: "", username: "", password: "", code: "" });
  const [sending, setSending] = useState(false); const [cooldown, setCooldown] = useState(0); const [loading, setLoading] = useState(false);
  const [authConfig, setAuthConfig] = useState({ registrationEnabled: true, emailVerificationEnabled: true });
  useEffect(() => { getAuthConfig().then((r: any) => { if (r.code === 0) setAuthConfig((old) => ({ ...old, ...(r.data || {}) })); }); }, []);
  const change = (key: keyof typeof form, value: string) => setForm((old) => ({ ...old, [key]: value }));
  const send = async () => { if (!form.email || cooldown > 0) return; setSending(true); try { const r = await sendRegisterCode(form.email); if (r.code !== 0) throw new Error(r.msg); toast.success("验证码已发送"); setCooldown(60); const timer = window.setInterval(() => setCooldown((value) => { if (value <= 1) { window.clearInterval(timer); return 0; } return value - 1; }), 1000); } catch (e: any) { toast.error(e?.message || "验证码发送失败"); } finally { setSending(false); } };
  const submit = async () => { if (!form.email || !form.password || form.password.length < 6 || (authConfig.emailVerificationEnabled && !form.code)) { toast.error("请完整填写注册信息"); return; } setLoading(true); try { const r = await register(form); if (r.code !== 0) throw new Error(r.msg); localStorage.setItem("token", r.data.token); localStorage.setItem("name", r.data.name); localStorage.setItem("role_id", String(r.data.role_id)); localStorage.setItem("admin", "false"); toast.success("注册成功"); navigate("/my-sub", { replace: true }); } catch (e: any) { toast.error(e?.message || "注册失败"); } finally { setLoading(false); } };
  return <DefaultLayout><section className="flex justify-center py-8"><Card className="w-full max-w-md"><CardHeader className="flex-col items-center"><h1 className={title({ size: "sm" })}>注册账号</h1><p className="text-small text-default-500 mt-2">创建 TMS 账号</p></CardHeader><CardBody className="gap-4">{!authConfig.registrationEnabled ? <p className="text-center text-sm text-danger">管理员已关闭新用户注册</p> : <><Input label="邮箱" type="email" value={form.email} onChange={(e) => change("email", e.target.value)} /><Input label="用户名（可选）" value={form.username} onChange={(e) => change("username", e.target.value)} /><Input label="密码" type="password" value={form.password} onChange={(e) => change("password", e.target.value)} />{authConfig.emailVerificationEnabled && <div className="flex gap-2"><Input className="flex-1" label="邮箱验证码" value={form.code} onChange={(e) => change("code", e.target.value)} /><Button className="mt-2 shrink-0" variant="flat" onClick={send} isLoading={sending} isDisabled={cooldown > 0}>{cooldown > 0 ? `${cooldown}s` : "获取验证码"}</Button></div>}<Button color="primary" size="lg" onClick={submit} isLoading={loading}>注册</Button></>}<div className="text-center text-sm text-default-500">已有账号？<Link className="text-primary ml-1" to="/">返回登录</Link></div></CardBody></Card></section></DefaultLayout>;
}
