import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input } from "@heroui/input";
import toast from "react-hot-toast";
import { requestPasswordReset } from "@/api";
import DefaultLayout from "@/layouts/default";
import { title } from "@/components/primitives";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const submit = async () => {
    if (!email) { toast.error("请输入注册邮箱"); return; }
    setLoading(true);
    try {
      const response = await requestPasswordReset(email);
      if (response.code !== 0) throw new Error(response.msg);
      toast.success("如果邮箱已注册，重置链接将发送到该邮箱");
    } catch (error: any) { toast.error(error?.message || "请求失败，请稍后重试"); }
    finally { setLoading(false); }
  };
  return <DefaultLayout><section className="flex min-w-0 justify-center py-8"><Card className="w-full min-w-0 max-w-md"><CardHeader className="flex-col items-center"><h1 className={title({ size: "sm" })}>忘记密码</h1><p className="text-small text-default-500 mt-2">输入注册邮箱获取一次性重置链接</p></CardHeader><CardBody className="gap-4"><Input label="注册邮箱" type="email" value={email} onChange={(e) => setEmail(e.target.value)} /><Button color="primary" size="lg" onClick={submit} isLoading={loading}>发送重置链接</Button><div className="text-center text-sm text-default-500"><Link className="text-primary" to="/">返回登录</Link></div></CardBody></Card></section></DefaultLayout>;
}
