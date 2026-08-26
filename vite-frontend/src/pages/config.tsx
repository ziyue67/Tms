import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Input, Textarea } from "@heroui/input";
import { Spinner } from "@heroui/spinner";
import { Divider } from "@heroui/divider";
import { Switch } from "@heroui/switch";
import { Select, SelectItem } from "@heroui/select";
import toast from 'react-hot-toast';
import { getAdminConfigs, getAdminEmailHealth, testAdminEmail, updateConfigs } from '@/api';
import { SettingsIcon } from '@/components/icons';

import { isAdmin } from '@/utils/auth';
import { clearConfigCache, updateSiteConfig } from '@/config/site';

// 简单的保存图标组件
const SaveIcon = ({ className }: { className?: string }) => (
  <svg
    className={className}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z" />
    <polyline points="17,21 17,13 7,13 7,21" />
    <polyline points="7,3 7,8 15,8" />
  </svg>
);

interface ConfigItem {
  key: string;
  label: string;
  placeholder?: string;
  description?: string;
  type: 'input' | 'textarea' | 'switch' | 'select';
  inputType?: 'text' | 'password';
  options?: { label: string; value: string; description?: string }[];
  dependsOn?: string; // 依赖的配置项key
  dependsValue?: string; // 依赖的配置项值
}

type ConfigSection = 'general' | 'security' | 'payment' | 'email';

const CONFIG_SECTIONS: { key: ConfigSection; label: string; description: string }[] = [
  { key: 'general', label: '通用设置', description: '面板名称、对接地址与订阅格式' },
  { key: 'security', label: '安全与认证', description: '登录验证码与认证服务状态' },
  { key: 'payment', label: '支付设置', description: '支付系统、渠道和支付回调配置' },
  { key: 'email', label: '邮件设置', description: 'SMTP、验证码时效与邮件模板' },
];

const SECTION_KEYS: Record<ConfigSection, string[]> = {
  general: ['ip', 'app_name'],
  security: ['captcha_enabled', 'captcha_type'],
  payment: [],
  email: [],
};

const PAYMENT_GROUP_TITLES: Record<string, string> = {
  payment_enabled: '基础支付设置',
  payment_manual_enabled: '可用支付方式',
  payment_alipay_enabled: '支付宝',
  payment_wechat_enabled: '微信支付',
  payment_easypay_enabled: '易支付',
  payment_stripe_enabled: 'Stripe',
};

const DEFAULT_REGISTER_TEMPLATE = '<!DOCTYPE html><html><body style="margin:0;padding:20px;background:#f5f5f5;font-family:Arial,sans-serif"><div style="max-width:600px;margin:auto;background:#fff;border-radius:8px;overflow:hidden"><div style="padding:30px;text-align:center;background:#667eea;color:#fff"><h1>{{app_name}}</h1></div><div style="padding:40px 30px;text-align:center"><h2>邮箱验证码</h2><p>请使用下面的验证码完成注册：</p><div style="padding:16px;background:#f3f4f6;border-radius:6px;font-size:32px;font-weight:bold;letter-spacing:8px">{{code}}</div><p>验证码将在 <strong>{{expires_minutes}} 分钟</strong> 后失效。</p></div><div style="padding:20px;text-align:center;background:#f8f9fa;color:#999;font-size:12px">这是系统自动发送的邮件，请勿直接回复。</div></div></body></html>';
const DEFAULT_RESET_TEMPLATE = '<!DOCTYPE html><html><body style="margin:0;padding:20px;background:#f5f5f5;font-family:Arial,sans-serif"><div style="max-width:600px;margin:auto;background:#fff;border-radius:8px;overflow:hidden"><div style="padding:30px;text-align:center;background:#667eea;color:#fff"><h1>{{app_name}}</h1></div><div style="padding:40px 30px;text-align:center"><h2>密码重置请求</h2><p>请点击下方按钮设置新密码：</p><p><a href="{{reset_url}}" style="display:inline-block;padding:12px 24px;background:#667eea;color:#fff;text-decoration:none;border-radius:5px">重置密码</a></p><p>该链接将在 <strong>{{expires_minutes}} 分钟</strong> 后失效且只能使用一次。</p></div><div style="padding:20px;text-align:center;background:#f8f9fa;color:#999;font-size:12px">这是系统自动发送的邮件，请勿直接回复。</div></div></body></html>';

// 网站配置项定义
const CONFIG_ITEMS: ConfigItem[] = [
  {
    key: 'ip',
    label: '面板后端地址',
    placeholder: '请输入面板后端IP:PORT',
    description: '格式“ip:port”,用于对接转发机时使用,ip是你安装面板服务器的公网ip,端口是安装脚本内输入的后端端口。不要套CDN,不支持https,通讯数据有加密',
    type: 'input'
  },
  {
    key: 'app_name',
    label: '应用名称',
    placeholder: '请输入应用名称',
    description: '在浏览器标签页和导航栏显示的应用名称',
    type: 'input'
  },
  {
    key: 'captcha_enabled',
    label: '启用验证码',
    description: '开启后，用户登录时需要完成验证码验证',
    type: 'switch'
  },
  {
    key: 'captcha_type',
    label: '验证码类型',
    description: '选择验证码的显示类型，不同类型有不同的安全级别',
    type: 'select',
    dependsOn: 'captcha_enabled',
    dependsValue: 'true',
    options: [
      { 
        label: '随机类型', 
        value: 'RANDOM', 
        description: '系统随机选择验证码类型' 
      },
      { 
        label: '滑块验证码', 
        value: 'SLIDER', 
        description: '拖动滑块完成拼图验证' 
      },
      { 
        label: '文字点选验证码', 
        value: 'WORD_IMAGE_CLICK', 
        description: '按顺序点击指定文字' 
      },
      { 
        label: '旋转验证码', 
        value: 'ROTATE', 
        description: '旋转图片到正确角度' 
      },
      { 
        label: '拼图验证码', 
        value: 'CONCAT', 
        description: '拖动滑块完成图片拼接' 
      }
    ]
  },
  { key: 'registration_enabled', label: '开放注册', description: '允许新用户自行注册账号。', type: 'switch' },
  { key: 'registration_email_verification', label: '邮箱验证', description: '新用户注册时必须完成邮箱验证码验证。', type: 'switch' },
  { key: 'registration_email_whitelist', label: '邮箱域名白名单', placeholder: '@qq.com, @gmail.com, *.edu.cn', description: '留空不限。填写后仅白名单域名可无限注册；可使用逗号、空格或换行分隔。', type: 'input' },
  { key: 'registration_non_whitelist_domain_limit', label: '非白名单域名限量注册', description: '白名单存在时，允许每个非白名单域名注册一个账号；关闭则直接拒绝。', type: 'switch' },
  { key: 'smtp_host', label: 'SMTP 服务器', placeholder: 'smtp.example.com', description: '用于发送注册验证码；留空时不能发送邮箱验证码。', type: 'input' },
  { key: 'smtp_port', label: 'SMTP 端口', placeholder: '587', type: 'input' },
  { key: 'smtp_username', label: 'SMTP 用户名', placeholder: '发件邮箱账号', type: 'input' },
  { key: 'smtp_password', label: 'SMTP 密码', placeholder: 'SMTP 密码或授权码', type: 'input', inputType: 'password' },
  { key: 'smtp_from', label: '发件人地址', placeholder: 'no-reply@example.com', type: 'input' },
  { key: 'smtp_from_name', label: '发件人名称', placeholder: 'TMS', type: 'input' },
  { key: 'smtp_starttls', label: '启用 STARTTLS', description: '常见 587 端口开启；465 端口通常使用 SSL。', type: 'switch' },
  { key: 'smtp_ssl', label: '启用 SMTP SSL', description: '通常用于 465 端口；不要与 STARTTLS 同时启用。', type: 'switch' },
  { key: 'email_code_expire_seconds', label: '邮箱验证码有效期（秒）', placeholder: '600', type: 'input' },
  { key: 'email_code_cooldown_seconds', label: '邮箱验证码发送间隔（秒）', placeholder: '60', type: 'input' },
  { key: 'email_register_subject', label: '注册验证码邮件主题', placeholder: 'TMS 注册验证码', type: 'input' },
  { key: 'email_register_template', label: '注册验证码邮件模板', placeholder: '你的验证码是 {{code}}，有效期 {{expires_minutes}} 分钟。', description: '可用变量：{{code}}、{{expires_minutes}}、{{app_name}}。', type: 'textarea' },
  { key: 'email_reset_subject', label: '密码重置邮件主题', placeholder: 'TMS 密码重置', type: 'input' },
  { key: 'email_reset_template', label: '密码重置邮件模板', placeholder: '请点击链接：{{reset_url}}，有效期 {{expires_minutes}} 分钟。', description: '可用变量：{{reset_url}}、{{expires_minutes}}、{{app_name}}。', type: 'textarea' },
  { key: 'payment_enabled', label: '启用支付系统', description: '关闭后用户不能创建新的购买订单，已存在订单和支付回调仍保留。', type: 'switch' },
  { key: 'payment_test_mode', label: '支付测试模式', description: '仅测试环境启用，管理员可手动完成待支付订单。', type: 'switch' },
  { key: 'payment_manual_enabled', label: '启用人工支付', type: 'switch' },
  { key: 'payment_alipay_enabled', label: '启用支付宝', type: 'switch' },
  { key: 'payment_alipay_app_id', label: '支付宝 App ID', type: 'input' },
  { key: 'payment_alipay_private_key', label: '支付宝应用私钥（PKCS#8 PEM）', type: 'input', inputType: 'password' },
  { key: 'payment_alipay_public_key', label: '支付宝公钥（PEM）', type: 'input', inputType: 'password' },
  { key: 'payment_alipay_gateway', label: '支付宝网关', placeholder: 'https://openapi.alipay.com/gateway.do', type: 'input' },
  { key: 'payment_alipay_notify_url', label: '支付宝异步通知地址', type: 'input' },
  { key: 'payment_alipay_return_url', label: '支付宝同步返回地址', type: 'input' },
  { key: 'payment_wechat_enabled', label: '启用微信支付', type: 'switch' },
  { key: 'payment_wechat_app_id', label: '微信支付 AppID', type: 'input' },
  { key: 'payment_wechat_mchid', label: '微信支付商户号', type: 'input' },
  { key: 'payment_wechat_serial_no', label: '微信商户证书序列号', type: 'input' },
  { key: 'payment_wechat_private_key', label: '微信商户私钥（PKCS#8 PEM）', type: 'input', inputType: 'password' },
  { key: 'payment_wechat_api_v3_key', label: '微信 API v3 密钥', type: 'input', inputType: 'password' },
  { key: 'payment_wechat_platform_certificate', label: '微信平台证书（PEM）', type: 'input', inputType: 'password' },
  { key: 'payment_wechat_gateway', label: '微信支付网关', placeholder: 'https://api.mch.weixin.qq.com', type: 'input' },
  { key: 'payment_wechat_notify_url', label: '微信异步通知地址', type: 'input' },
  { key: 'payment_easypay_enabled', label: '启用易支付', type: 'switch' },
  { key: 'payment_easypay_gateway', label: '易支付网关', placeholder: 'https://example.com/api.php', type: 'input' },
  { key: 'payment_easypay_pid', label: '易支付商户 ID', type: 'input' },
  { key: 'payment_easypay_key', label: '易支付商户密钥', type: 'input', inputType: 'password' },
  { key: 'payment_easypay_type', label: '易支付支付类型', placeholder: 'alipay / wxpay / qqpay', type: 'input' },
  { key: 'payment_easypay_notify_url', label: '易支付异步通知地址', type: 'input' },
  { key: 'payment_easypay_return_url', label: '易支付同步返回地址', type: 'input' },
  { key: 'payment_stripe_enabled', label: '启用 Stripe', type: 'switch' },
  { key: 'payment_stripe_secret_key', label: 'Stripe Secret Key', type: 'input', inputType: 'password' },
  { key: 'payment_stripe_webhook_secret', label: 'Stripe Webhook Secret', type: 'input', inputType: 'password' },
  { key: 'payment_stripe_success_url', label: 'Stripe 成功返回地址', type: 'input' },
  { key: 'payment_stripe_cancel_url', label: 'Stripe 取消返回地址', type: 'input' }
];

// 初始化时从缓存读取配置，避免闪烁
const getInitialConfigs = (): Record<string, string> => {
  if (typeof window === 'undefined') return {};
  
  const configKeys = ['app_name', 'captcha_enabled', 'captcha_type'];
  const initialConfigs: Record<string, string> = {};
  
  try {
    configKeys.forEach(key => {
      const cachedValue = localStorage.getItem('vite_config_' + key);
      if (cachedValue) {
        initialConfigs[key] = cachedValue;
      }
    });
  } catch (error) {
  }
  
  return initialConfigs;
};

export default function ConfigPage() {
  const navigate = useNavigate();
  const initialConfigs = getInitialConfigs();
  const [configs, setConfigs] = useState<Record<string, string>>(initialConfigs);
  const [loading, setLoading] = useState(Object.keys(initialConfigs).length === 0); // 如果有缓存数据，不显示loading
  const [saving, setSaving] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [originalConfigs, setOriginalConfigs] = useState<Record<string, string>>(initialConfigs);
  const [activeSection, setActiveSection] = useState<ConfigSection>('general');
  const [testRecipient, setTestRecipient] = useState('');
  const [emailTemplateEvent, setEmailTemplateEvent] = useState<'register' | 'reset'>('register');

  // 权限检查
  useEffect(() => {
    if (!isAdmin()) {
      toast.error('权限不足，只有管理员可以访问此页面');
      navigate('/dashboard', { replace: true });
      return;
    }
  }, [navigate]);

  // 加载配置数据（优先从缓存）
  const loadConfigs = async (currentConfigs?: Record<string, string>) => {
    const configsToCompare = currentConfigs || configs;
    const hasInitialData = Object.keys(configsToCompare).length > 0;
    
    // 如果已有缓存数据，不显示loading，静默更新
    if (!hasInitialData) {
      setLoading(true);
    }
    
    try {
      const response = await getAdminConfigs();
      if (response.code !== 0) {
        throw new Error(response.msg || '加载配置失败');
      }
      const configData = { payment_enabled: 'true', email_register_template: DEFAULT_REGISTER_TEMPLATE, email_reset_template: DEFAULT_RESET_TEMPLATE, ...(response.data || {}) };
      
      // 只有在数据有变化时才更新
      const hasDataChanged = JSON.stringify(configData) !== JSON.stringify(configsToCompare);
      if (hasDataChanged) {
        setConfigs(configData);
        setOriginalConfigs({ ...configData });
        setHasChanges(false);
      } else {
      }
    } catch (error) {
      // 只有在没有缓存数据时才显示错误
      if (!hasInitialData) {
        toast.error('加载配置出错，请重试');
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    // 延迟加载，避免阻塞初始渲染
    const timer = setTimeout(() => {
      loadConfigs(initialConfigs);
    }, 100);

    return () => clearTimeout(timer);
  }, []); // 只在组件挂载时执行一次

  // 处理配置项变更
  const handleConfigChange = (key: string, value: string) => {
    let newConfigs = { ...configs, [key]: value };
    
    // 特殊处理：启用验证码时，如果验证码类型未设置，默认为随机
    if (key === 'captcha_enabled' && value === 'true') {
      if (!newConfigs.captcha_type) {
        newConfigs.captcha_type = 'RANDOM';
      }
    }
    
    setConfigs(newConfigs);
    
    // 检查是否有变更
    const hasChangesNow = Object.keys(newConfigs).some(
      k => newConfigs[k] !== originalConfigs[k]
    ) || Object.keys(originalConfigs).some(
      k => originalConfigs[k] !== newConfigs[k]
    );
    setHasChanges(hasChangesNow);
  };

  // 保存配置
  const handleSave = async () => {
    setSaving(true);
    try {
      const response = await updateConfigs(configs);
      if (response.code === 0) {
        toast.success('配置保存成功');
        
        // 清除所有配置缓存，强制下次重新获取
        clearConfigCache();
        
        // 获取变更的配置项
        const changedKeys = Object.keys(configs).filter(
          key => configs[key] !== originalConfigs[key]
        );
        
        setOriginalConfigs({ ...configs });
        setHasChanges(false);
        
        // 如果应用名称发生变化，立即更新网站配置
        if (changedKeys.includes('app_name')) {
          await updateSiteConfig();
        }
        
        // 触发配置更新事件，通知其他组件
        window.dispatchEvent(new CustomEvent('configUpdated', { 
          detail: { changedKeys } 
        }));
      } else {
        toast.error('保存配置失败: ' + response.msg);
      }
    } catch (error) {
      toast.error('保存配置出错，请重试');
    } finally {
      setSaving(false);
    }
  };

  const handleSmtpTest = async () => {
    const email = testRecipient.trim();
    if (!email) return toast.error('请输入接收测试邮件的地址');
    const response = await testAdminEmail(email);
    if (response.code === 0) toast.success('测试邮件已发送');
    else toast.error(response.msg || 'SMTP 测试失败');
  };

  const handleRedisHealth = async () => {
    const response = await getAdminEmailHealth();
    if (response.code !== 0) return toast.error(response.msg || 'Redis 状态检查失败');
    if (response.data?.redisAvailable) toast.success('Redis 连接正常');
    else toast.error(response.data?.message || 'Redis 不可用，请检查 REDIS_PASSWORD');
  };



  // 检查配置项是否应该显示（依赖检查）
  const shouldShowItem = (item: ConfigItem): boolean => {
    if (!item.dependsOn || !item.dependsValue) {
      return true;
    }
    return configs[item.dependsOn] === item.dependsValue;
  };

  const sectionFor = (key: string): ConfigSection => {
    if (SECTION_KEYS.general.includes(key)) return 'general';
    if (SECTION_KEYS.security.includes(key)) return 'security';
    return key.startsWith('payment_') ? 'payment' : 'email';
  };

  const emailTemplate = emailTemplateEvent === 'register'
    ? {
        label: '邮箱验证码',
        description: '用于注册验证码邮件。',
        subjectKey: 'email_register_subject',
        contentKey: 'email_register_template',
        defaultSubject: 'TMS 注册验证码',
        defaultContent: DEFAULT_REGISTER_TEMPLATE,
      }
    : {
        label: '密码重置',
        description: '用于密码重置的一次性链接邮件。',
        subjectKey: 'email_reset_subject',
        contentKey: 'email_reset_template',
        defaultSubject: 'TMS 密码重置',
        defaultContent: DEFAULT_RESET_TEMPLATE,
      };

  const emailPreview = (configs[emailTemplate.contentKey] || emailTemplate.defaultContent)
    .replace(/{{app_name}}/g, configs.app_name || 'TMS')
    .replace(/{{code}}/g, '123456')
    .replace(/{{expires_minutes}}/g, String(Math.max(1, Math.round(Number(configs.email_code_expire_seconds || 600) / 60))))
    .replace(/{{reset_url}}/g, 'https://example.com/reset-password?token=example');

  const insertTemplateVariable = (variable: string) => {
    handleConfigChange(emailTemplate.contentKey, `${configs[emailTemplate.contentKey] || emailTemplate.defaultContent}${variable}`);
  };

  // 渲染不同类型的配置项
  const renderConfigItem = (item: ConfigItem) => {
    const isChanged = hasChanges && configs[item.key] !== originalConfigs[item.key];
    
    switch (item.type) {
      case 'input':
        return (
          <Input
            type={item.inputType || 'text'}
            value={configs[item.key] || ''}
            onChange={(e) => handleConfigChange(item.key, e.target.value)}
            placeholder={item.placeholder}
            variant="bordered"
            size="md"
            classNames={{
              input: "text-sm",
              inputWrapper: isChanged 
                ? "border-warning-300 data-[hover=true]:border-warning-400" 
                : ""
            }}
          />
        );

      case 'textarea':
        return (
          <Textarea
            minRows={4}
            value={configs[item.key] || ''}
            onChange={(e) => handleConfigChange(item.key, e.target.value)}
            placeholder={item.placeholder}
            variant="bordered"
            classNames={{
              input: "text-sm",
              inputWrapper: isChanged
                ? "border-warning-300 data-[hover=true]:border-warning-400"
                : ""
            }}
          />
        );

      case 'switch':
        return (
          <Switch
            isSelected={configs[item.key] === 'true'}
            onValueChange={(checked) => handleConfigChange(item.key, checked ? 'true' : 'false')}
            color="primary"
            size="md"
            classNames={{
              wrapper: isChanged ? "border-warning-300" : ""
            }}
          >
            <span className="text-sm text-gray-700 dark:text-gray-300">
              {configs[item.key] === 'true' ? '已启用' : '已禁用'}
            </span>
          </Switch>
        );

      case 'select':
        return (
          <Select
            selectedKeys={configs[item.key] ? [configs[item.key]] : []}
            onSelectionChange={(keys) => {
              const selectedKey = Array.from(keys)[0] as string;
              if (selectedKey) {
                handleConfigChange(item.key, selectedKey);
              }
            }}
            placeholder="请选择验证码类型"
            variant="bordered"
            size="md"
            classNames={{
              trigger: isChanged 
                ? "border-warning-300 data-[hover=true]:border-warning-400" 
                : ""
            }}
          >
            {item.options?.map((option) => (
              <SelectItem 
                key={option.value}
                description={option.description}
              >
                {option.label}
              </SelectItem>
            )) || []}
          </Select>
        );

      default:
        return null;
    }
  };

  if (loading) {
    return (
      
        <div className="flex items-center justify-center min-h-[400px]">
          <Spinner size="lg" label="加载配置中..." />
        </div>
      
    );
  }

  const activeConfigItems = CONFIG_ITEMS.filter((item) =>
    sectionFor(item.key) === activeSection
      && shouldShowItem(item)
      && !(activeSection === 'email' && ['email_register_subject', 'email_register_template', 'email_reset_subject', 'email_reset_template'].includes(item.key))
  );
  const activeSectionInfo = CONFIG_SECTIONS.find((section) => section.key === activeSection)!;

  return (
    
      <div className="p-4 md:p-6 max-w-6xl mx-auto">
        {/* 页面标题 */}
        <div className="flex items-center gap-3 mb-6">
          <SettingsIcon className="w-8 h-8 text-primary" />
          <div>
            <h1 className="text-2xl font-bold">网站配置</h1>
            <p className="text-gray-600 dark:text-gray-400">
              管理网站的基本信息和显示设置
            </p>
          </div>
        </div>

        <div className="mb-5 overflow-x-auto rounded-lg border border-divider bg-content1 p-1 shadow-sm" role="tablist" aria-label="网站配置分类">
          <div className="flex min-w-max gap-1">
          {CONFIG_SECTIONS.map((section) => (
            <Button
              key={section.key}
              size="sm"
              variant={activeSection === section.key ? 'flat' : 'light'}
              color={activeSection === section.key ? 'primary' : 'default'}
              className={activeSection === section.key ? 'font-semibold' : 'text-default-600'}
              onPress={() => setActiveSection(section.key)}
              role="tab"
              aria-selected={activeSection === section.key}
            >
              {section.label}
            </Button>
          ))}
          </div>
        </div>

        {activeSection === 'general' && <Card className="mb-4 shadow-md">
          <CardHeader><div><h2 className="text-lg font-semibold">订阅格式</h2><p className="text-sm text-gray-600 dark:text-gray-400">同一 API token 同时提供两种客户端格式。</p></div></CardHeader>
          <CardBody className="text-sm space-y-1 text-gray-600 dark:text-gray-300">
            <div><b>v2rayN / Base64：</b><code>/api/v1/open_api/sub?token=...</code></div>
            <div><b>Clash Verge / ClashMeta / Mihomo：</b><code>/api/v1/open_api/clash?token=...</code></div>
            <div className="text-xs text-gray-500">两种链接使用同一个 token，格式不同，不能互相替代。</div>
          </CardBody>
        </Card>}

        <Card className="shadow-md">
          <CardHeader className="pb-4">
            <div className="flex justify-between items-center w-full">
              <div>
                <h2 className="text-xl font-semibold">{activeSectionInfo.label}</h2>
                <p className="text-sm text-gray-600 dark:text-gray-400">
                  {activeSectionInfo.description}
                </p>
              </div>
              <div className="flex gap-2">
                <Button variant="flat" onPress={handleRedisHealth}>检查 Redis</Button>
                <Button variant="flat" onPress={handleSmtpTest}>测试 SMTP</Button>
                <Button
                  color="primary"
                  startContent={<SaveIcon className="w-4 h-4" />}
                  onClick={handleSave}
                  isLoading={saving}
                  disabled={!hasChanges}
                >
                  {saving ? '保存中...' : '保存所有变更'}
                </Button>
              </div>
            </div>
          </CardHeader>

          <Divider />

          <CardBody className="space-y-6 pt-6">
            {activeSection === 'security' && (
              <div className="rounded-md border border-divider bg-default-50 p-3 text-sm text-default-600">
                邮箱验证码依赖 Redis 与 SMTP。保存后可用右上角“检查 Redis”和“测试 SMTP”验证认证服务。
              </div>
            )}
            {activeSection === 'payment' && (
              <div className="rounded-md border border-divider bg-default-50 p-3 text-sm text-default-600">
                先开启支付系统，再启用至少一种支付渠道。密钥只通过管理员私有配置接口读取和保存。
              </div>
            )}
            {activeConfigItems.map((item, index) => {
              const isLastItem = index === activeConfigItems.length - 1;
              const paymentGroupTitle = activeSection === 'payment' ? PAYMENT_GROUP_TITLES[item.key] : undefined;

              const compactSwitch = item.type === 'switch';
              return (
                <div key={item.key} className="space-y-3">
                  {paymentGroupTitle && (
                    <div className={index === 0 ? '' : 'pt-4 border-t border-divider'}>
                      <h3 className="text-base font-semibold">{paymentGroupTitle}</h3>
                    </div>
                  )}
                  <div className={compactSwitch ? 'flex items-center justify-between gap-5' : 'flex flex-col gap-1'}>
                    <div className="min-w-0">
                      <label className="text-sm font-medium text-gray-700 dark:text-gray-300">
                        {item.label}
                      </label>
                      {item.description && (
                        <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                          {item.description}
                        </p>
                      )}
                    </div>
                    {compactSwitch && <div className="shrink-0">{renderConfigItem(item)}</div>}
                  </div>
                  {!compactSwitch && renderConfigItem(item)}
                  
                  {/* 分隔线 */}
                  {!isLastItem && (
                    <Divider className="mt-6" />
                  )}
                </div>
              );
            })}
            {activeSection === 'email' && (
              <div className="border-t border-divider pt-5 space-y-4">
                <div className="space-y-1">
                  <h3 className="text-base font-semibold">邮件模板</h3>
                  <p className="text-sm text-default-500">选择事件后编辑主题与 HTML 内容，预览不会发送邮件。</p>
                </div>
                <Select
                  label="邮件事件"
                  selectedKeys={[emailTemplateEvent]}
                  onSelectionChange={(keys) => setEmailTemplateEvent(Array.from(keys)[0] as 'register' | 'reset')}
                >
                  <SelectItem key="register">邮箱验证码</SelectItem>
                  <SelectItem key="reset">密码重置</SelectItem>
                </Select>
                <div className="rounded-md border border-primary/20 bg-primary/5 p-3 text-sm text-default-700">
                  <span className="font-medium">{emailTemplate.label}</span>
                  <span className="ml-2 text-default-500">{emailTemplate.description}</span>
                </div>
                <div className="grid gap-4 lg:grid-cols-2">
                  <div className="space-y-3">
                    <Input
                      label="邮件主题"
                      value={configs[emailTemplate.subjectKey] || emailTemplate.defaultSubject}
                      onChange={(event) => handleConfigChange(emailTemplate.subjectKey, event.target.value)}
                      variant="bordered"
                    />
                    <Textarea
                      label="HTML 模板"
                      minRows={14}
                      value={configs[emailTemplate.contentKey] || emailTemplate.defaultContent}
                      onChange={(event) => handleConfigChange(emailTemplate.contentKey, event.target.value)}
                      variant="bordered"
                      classNames={{ input: 'font-mono text-xs' }}
                    />
                    <div className="rounded-md border border-divider p-3">
                      <p className="mb-2 text-xs text-default-500">可插入变量</p>
                      <div className="flex flex-wrap gap-2">
                        {['{{app_name}}', '{{code}}', '{{expires_minutes}}', '{{reset_url}}'].map((variable) => (
                          <Button key={variable} size="sm" variant="flat" onPress={() => insertTemplateVariable(variable)}>{variable}</Button>
                        ))}
                      </div>
                    </div>
                  </div>
                  <div className="overflow-hidden rounded-md border border-divider bg-default-100 p-3">
                    <p className="mb-2 text-sm font-medium">实时预览</p>
                    <iframe title="邮件模板预览" sandbox="" srcDoc={emailPreview} className="h-[520px] w-full rounded border border-divider bg-white" />
                  </div>
                </div>
                <div>
                  <h3 className="text-base font-semibold">发送测试邮件</h3>
                  <p className="text-sm text-default-500">保存 SMTP 配置后，向指定邮箱发送一封验证邮件。</p>
                </div>
                <div className="flex flex-col sm:flex-row gap-2">
                  <Input
                    type="email"
                    label="收件人邮箱"
                    placeholder="test@example.com"
                    value={testRecipient}
                    onChange={(event) => setTestRecipient(event.target.value)}
                  />
                  <Button className="sm:self-end" variant="flat" onPress={handleSmtpTest}>发送测试邮件</Button>
                </div>
              </div>
            )}
          </CardBody>
        </Card>

        {/* 操作提示 */}
        {hasChanges && (
          <Card className="mt-4 bg-warning-50 dark:bg-warning-900/20 border-warning-200 dark:border-warning-800">
            <CardBody className="py-3">
              <div className="flex items-center gap-2 text-warning-700 dark:text-warning-300">
                <div className="w-2 h-2 bg-warning-500 rounded-full animate-pulse" />
                <span className="text-sm">
                  检测到配置变更，请记得保存您的修改
                </span>
              </div>
            </CardBody>
          </Card>
        )}
      </div>
    
  );
} 
