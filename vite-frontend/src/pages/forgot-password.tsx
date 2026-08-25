import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import toast from "react-hot-toast";
import { resetPassword, sendResetCode } from "@/api";
import DefaultLayout from "@/layouts/default";
import { title } from "@/components/primitives";

export default function ForgotPasswordPage() {
  const navigate = useNavigate();
  const [form, setForm] = useState({ email: "", code: "", newPassword: "", confirmPassword: "" });
  const [sending, setSending] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [loading, setLoading] = useState(false);
  const [sent, setSent] = useState(false);
  const change = (key: keyof typeof form, value: string) => setForm((old) => ({ ...old, [key]: value }));
  const send = async () => {
    if (!form.email || cooldown > 0) return;
    setSending(true);
    try {
      const response = await sendResetCode(form.email);
      if (response.code !== 0) throw new Error(response.msg);
      setSent(true);
      toast.success("如果邮箱已注册，验证码将发送到该邮箱");
      setCooldown(60);
      const timer = window.setInterval(() => setCooldown((value) => {
        if (value <= 1) { window.clearInterval(timer); return 0; }
        return value - 1;
      }), 1000);
    } catch (error: any) {
      toast.error(error?.message || "验证码发送失败");
    } finally { setSending(false); }
  };
  const submit = async () => {
    if (!form.email || !form.code || form.newPassword.length < 6) { toast.error("请完整填写重置密码信息"); return; }
    if (form.newPassword !== form.confirmPassword) { toast.error("两次输入的密码不一致"); return; }
    setLoading(true);
    try {
      const response = await resetPassword({ email: form.email, code: form.code, newPassword: form.newPassword });
      if (response.code !== 0) throw new Error(response.msg);
      toast.success("密码已重置，请重新登录");
      navigate("/", { replace: true });
    } catch (error: any) {
      toast.error(error?.message || "密码重置失败");
    } finally { setLoading(false); }
  };
  return <DefaultLayout><section className="flex justify-center py-8"><Card className="w-full max-w-md"><CardHeader className="flex-col items-center"><h1 className={title({ size: "sm" })}>忘记密码</h1><p className="text-small text-default-500 mt-2">使用注册邮箱验证码设置新密码</p></CardHeader><CardBody className="gap-4"><Input label="注册邮箱" type="email" value={form.email} onChange={(e) => change("email", e.target.value)} /><div className="flex gap-2"><Input className="flex-1" label="邮箱验证码" value={form.code} onChange={(e) => change("code", e.target.value)} /><Button className="mt-2 shrink-0" variant="flat" onClick={send} isLoading={sending} isDisabled={cooldown > 0 || !form.email}>{cooldown > 0 ? `${cooldown}s` : "获取验证码"}</Button></div>{sent && <p className="text-xs text-default-500">未注册邮箱不会提示具体原因，请检查邮箱或重新发送。</p>}<Input label="新密码" type="password" value={form.newPassword} onChange={(e) => change("newPassword", e.target.value)} /><Input label="确认新密码" type="password" value={form.confirmPassword} onChange={(e) => change("confirmPassword", e.target.value)} /><Button color="primary" size="lg" onClick={submit} isLoading={loading}>重置密码</Button><div className="text-center text-sm text-default-500"><Link className="text-primary" to="/">返回登录</Link></div></CardBody></Card></section></DefaultLayout>;
}
