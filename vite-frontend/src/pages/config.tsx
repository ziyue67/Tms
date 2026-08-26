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
import { getAdminConfigs, updateConfigs } from '@/api';
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
      const configData = response.data || {};
      
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



  // 检查配置项是否应该显示（依赖检查）
  const shouldShowItem = (item: ConfigItem): boolean => {
    if (!item.dependsOn || !item.dependsValue) {
      return true;
    }
    return configs[item.dependsOn] === item.dependsValue;
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

  return (
    
      <div className="p-6 max-w-4xl mx-auto">
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

        <Card className="mb-4 shadow-md">
          <CardHeader><div><h2 className="text-lg font-semibold">订阅格式</h2><p className="text-sm text-gray-600 dark:text-gray-400">同一 API token 同时提供两种客户端格式。</p></div></CardHeader>
          <CardBody className="text-sm space-y-1 text-gray-600 dark:text-gray-300">
            <div><b>v2rayN / Base64：</b><code>/api/v1/open_api/sub?token=...</code></div>
            <div><b>Clash Verge / ClashMeta / Mihomo：</b><code>/api/v1/open_api/clash?token=...</code></div>
            <div className="text-xs text-gray-500">两种链接使用同一个 token，格式不同，不能互相替代。</div>
          </CardBody>
        </Card>

        <Card className="shadow-md">
          <CardHeader className="pb-4">
            <div className="flex justify-between items-center w-full">
              <div>
                <h2 className="text-xl font-semibold">基本设置</h2>
                <p className="text-sm text-gray-600 dark:text-gray-400">
                  配置网站的基本信息，这些设置会影响网站的显示效果
                </p>
              </div>
              <div className="flex gap-2">

                <Button
                  color="primary"
                  startContent={<SaveIcon className="w-4 h-4" />}
                  onClick={handleSave}
                  isLoading={saving}
                  disabled={!hasChanges}
                >
                  {saving ? '保存中...' : '保存配置'}
                </Button>
              </div>
            </div>
          </CardHeader>

          <Divider />

          <CardBody className="space-y-6 pt-6">
            {CONFIG_ITEMS.map((item, index) => {
              // 检查配置项是否应该显示
              if (!shouldShowItem(item)) {
                return null;
              }

              // 计算是否是最后一个显示的项目（用于决定是否显示分隔线）
              const remainingItems = CONFIG_ITEMS.slice(index + 1).filter(shouldShowItem);
              const isLastItem = remainingItems.length === 0;

              return (
                <div key={item.key} className="space-y-3">
                  <div className="flex flex-col gap-1">
                    <label className="text-sm font-medium text-gray-700 dark:text-gray-300">
                      {item.label}
                    </label>
                    {item.description && (
                      <p className="text-xs text-gray-500 dark:text-gray-400">
                        {item.description}
                      </p>
                    )}
                  </div>
                  
                  {/* 渲染配置项 */}
                  {renderConfigItem(item)}
                  
                  {/* 分隔线 */}
                  {!isLastItem && (
                    <Divider className="mt-6" />
                  )}
                </div>
              );
            })}
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
