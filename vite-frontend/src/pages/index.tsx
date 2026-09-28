import { Button } from "@heroui/button";
import { Input } from "@heroui/input";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { useState, useEffect } from "react";
import { Link, useNavigate } from "react-router-dom";
import toast from 'react-hot-toast';
import { isWebViewFunc } from '@/utils/panel';
import { siteConfig } from '@/config/site';
import { title } from "@/components/primitives";
import DefaultLayout from "@/layouts/default";
import { login, LoginData, checkCaptcha, generateCaptcha, verifyCaptcha } from "@/api";


interface LoginForm {
  username: string;
  password: string;
  captchaId: string;
}



export default function IndexPage() {
  const [form, setForm] = useState<LoginForm>({
    username: "",
    password: "",
    captchaId: "",
  });
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState<Partial<LoginForm>>({});
  const [showCaptcha, setShowCaptcha] = useState(false);
  const [captchaId, setCaptchaId] = useState("");
  const [captchaQuestion, setCaptchaQuestion] = useState("");
  const [captchaAnswer, setCaptchaAnswer] = useState("");
  const [captchaLoading, setCaptchaLoading] = useState(false);
  const navigate = useNavigate();
  const [isWebView, setIsWebView] = useState(false);
  // 检测是否在WebView中运行
  useEffect(() => {
    setIsWebView(isWebViewFunc());
  }, []);
  // 验证表单
  const validateForm = (): boolean => {
    const newErrors: Partial<LoginForm> = {};

    if (!form.username.trim()) {
      newErrors.username = '请输入用户名';
    }

    if (!form.password.trim()) {
      newErrors.password = '请输入密码';
    } else if (form.password.length < 6) {
      newErrors.password = '密码长度至少6位';
    }


    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // 处理输入变化
  const handleInputChange = (field: keyof LoginForm, value: string) => {
    setForm(prev => ({ ...prev, [field]: value }));
    // 清除该字段的错误
    if (errors[field]) {
      setErrors(prev => ({ ...prev, [field]: undefined }));
    }
  };

  const loadCaptcha = async () => {
    const response = await generateCaptcha();
    if (response.code !== 0) throw new Error(response.msg || "验证码生成失败");
    setCaptchaId(response.data.id);
    setCaptchaQuestion(response.data.question);
    setCaptchaAnswer("");
  };

  const submitCaptcha = async () => {
    if (!captchaAnswer.trim()) return;
    setCaptchaLoading(true);
    try {
      const response = await verifyCaptcha({ id: captchaId, answer: captchaAnswer.trim() });
      if (response.code !== 0) {
        toast.error(response.msg || "验证码错误");
        await loadCaptcha();
        return;
      }
      setForm(current => ({ ...current, captchaId: response.data.validToken }));
      setShowCaptcha(false);
      await performLogin(response.data.validToken);
    } catch {
      toast.error("验证码验证失败");
      await loadCaptcha().catch(() => undefined);
    } finally {
      setCaptchaLoading(false);
    }
  };

  // 执行登录请求
  const performLogin = async (captchaToken = form.captchaId) => {


    try {
      const loginData: LoginData = {
        username: form.username.trim(),
        password: form.password,
        captchaId: captchaToken,
      };

      const response = await login(loginData);
      
      if (response.code !== 0) {
        toast.error(response.msg || "登录失败");
        return;
      }

      // 检查是否需要强制修改密码
      if (response.data.requirePasswordChange) {
        localStorage.setItem('token', response.data.token);
        localStorage.setItem("role_id", response.data.role_id.toString());
        localStorage.setItem("name", response.data.name);
        localStorage.setItem("admin", (response.data.role_id === 0).toString());
        toast.success('检测到默认密码，即将跳转到修改密码页面');
        navigate("/change-password");
        return;
      }

      // 保存登录信息
      localStorage.setItem('token', response.data.token);
      localStorage.setItem("role_id", response.data.role_id.toString());
      localStorage.setItem("name", response.data.name);
      localStorage.setItem("admin", (response.data.role_id === 0).toString());

      // 登录成功:管理员进仪表板;车友进「我的订阅」
      toast.success('登录成功');
      navigate(response.data.role_id === 0 ? "/dashboard" : "/my-sub");

    } catch (error) {
      console.error('登录错误:', error);
      toast.error("网络错误，请稍后重试");
    } finally {
      setLoading(false);
    }
  };

  const handleLogin = async () => {
    if (!validateForm()) return;

    setLoading(true);

    try {
      // 先检查是否需要验证码
      const checkResponse = await checkCaptcha();
      
      if (checkResponse.code !== 0) {
        toast.error("检查验证码状态失败，请重试" + checkResponse.msg);
        setLoading(false);
        return;
      }

      // 根据返回值决定是否显示验证码
      if (checkResponse.data === 0) {
        // 不需要验证码，直接登录
        await performLogin();
      } else {
        // 需要验证码，显示验证码弹层
        await loadCaptcha();
        setShowCaptcha(true);
      }
    } catch (error) {
      console.error('检查验证码状态错误:', error);
      toast.error("网络错误，请稍后重试" + error);
      setLoading(false);
    }
  };


  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !loading) {
      handleLogin();
    }
  };

  return (
    <DefaultLayout>
      <section className="flex min-w-0 flex-col items-center justify-center gap-4 py-4 pb-20 sm:py-8 sm:pb-20 md:py-10 min-h-[calc(100dvh-120px)] sm:min-h-[calc(100dvh-200px)]">
        <div className="w-full min-w-0 max-w-md">
          <Card className="w-full min-w-0">
            <CardHeader className="flex-col items-center px-4 pb-0 pt-6 sm:px-6">
              <h1 className={title({ size: "sm" })}>登陆</h1>
              <p className="text-small text-default-500 mt-2">请输入您的账号信息</p>
            </CardHeader>
            <CardBody className="px-4 py-6 sm:px-6">
              <div className="flex flex-col gap-4">
                <Input
                  label="用户名"
                  placeholder="请输入用户名"
                  value={form.username}
                  onChange={(e) => handleInputChange('username', e.target.value)}
                  onKeyDown={handleKeyPress}
                  variant="bordered"
                  isDisabled={loading}
                  isInvalid={!!errors.username}
                  errorMessage={errors.username}
                />
                
                <Input
                  label="密码"
                  placeholder="请输入密码"
                  type="password"
                  value={form.password}
                  onChange={(e) => handleInputChange('password', e.target.value)}
                  onKeyDown={handleKeyPress}
                  variant="bordered"
                  isDisabled={loading}
                  isInvalid={!!errors.password}
                />

                
                <Button
                  color="primary"
                  size="lg"
                  onClick={handleLogin}
                  isLoading={loading}
                  disabled={loading}
                  className="mt-2"
                >
                  {loading ? (showCaptcha ? "验证中..." : "登录中...") : "登录"}
                </Button>
                <div className="flex flex-col items-center gap-2 text-center text-sm text-default-500 sm:flex-row sm:justify-between sm:text-left">
                  <span>没有账号？<Link className="text-primary ml-1" to="/register">注册账号</Link></span>
                  <Link className="text-primary" to="/forgot-password">忘记密码？</Link>
                </div>
              </div>
            </CardBody>
          </Card>

        </div>


      {/* 版权信息 - 固定在底部，不占据布局空间 */}
      
               <div className="fixed inset-x-0 bottom-4 text-center py-4">
               <p className="text-xs text-gray-400 dark:text-gray-500">
                 Powered by <span className="text-gray-500 dark:text-gray-400">TMS</span>
               </p>
               <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">
                 v{ isWebView ? siteConfig.app_version : siteConfig.version}
               </p>
             </div>
      
   

        {/* 验证码弹层 */}
        {showCaptcha && (
          <div className="fixed inset-0 z-50 flex items-center justify-center">
            <button aria-label="关闭验证码" className="absolute inset-0 bg-black/60 dark:bg-black/80 backdrop-blur-sm" onClick={() => { setShowCaptcha(false); setLoading(false); }} />
            <Card className="relative z-10 w-[min(92vw,360px)]">
              <CardHeader><h2 className="text-base font-semibold">安全验证</h2></CardHeader>
              <CardBody className="gap-4">
                <div className="rounded-md bg-default-100 px-4 py-5 text-center text-2xl font-semibold tracking-normal">{captchaQuestion}</div>
                <Input autoFocus label="答案" inputMode="numeric" value={captchaAnswer} onValueChange={setCaptchaAnswer} onKeyDown={event => { if (event.key === 'Enter') void submitCaptcha(); }} />
                <div className="flex justify-end gap-2">
                  <Button variant="flat" onPress={() => void loadCaptcha()}>换一题</Button>
                  <Button color="primary" isLoading={captchaLoading} onPress={() => void submitCaptcha()}>验证</Button>
                </div>
              </CardBody>
            </Card>
          </div>
        )}
      </section>
    </DefaultLayout>
  );
}
