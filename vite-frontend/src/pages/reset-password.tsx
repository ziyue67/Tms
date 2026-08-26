import { useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import toast from "react-hot-toast";
import { resetPassword } from "@/api";
import DefaultLayout from "@/layouts/default";
import { title } from "@/components/primitives";

export default function ResetPasswordPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const email = useMemo(() => params.get("email") || "", [params]);
  const token = useMemo(() => params.get("token") || "", [params]);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [loading, setLoading] = useState(false);
  const submit = async () => {
    if (!email || !token) { toast.error("重置链接无效或不完整"); return; }
    if (password.length < 6) { toast.error("密码长度至少 6 位"); return; }
    if (password !== confirm) { toast.error("两次输入的密码不一致"); return; }
    setLoading(true);
    try {
      const response = await resetPassword({ email, token, newPassword: password });
      if (response.code !== 0) throw new Error(response.msg);
      toast.success("密码已重置，请重新登录");
      navigate("/", { replace: true });
    } catch (error: any) { toast.error(error?.message || "密码重置失败"); }
    finally { setLoading(false); }
  };
  return <DefaultLayout><section className="flex justify-center py-8"><Card className="w-full max-w-md"><CardHeader className="flex-col items-center"><h1 className={title({ size: "sm" })}>设置新密码</h1><p className="text-small text-default-500 mt-2">重置链接只能使用一次</p></CardHeader><CardBody className="gap-4"><Input label="新密码" type="password" value={password} onChange={(e) => setPassword(e.target.value)} /><Input label="确认新密码" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} /><Button color="primary" size="lg" onClick={submit} isLoading={loading}>重置密码</Button><div className="text-center text-sm text-default-500"><Link className="text-primary" to="/">返回登录</Link></div></CardBody></Card></section></DefaultLayout>;
}
