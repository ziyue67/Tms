import React, { useState, useEffect } from 'react';
import { Card, CardBody } from "@heroui/card";
import { Button } from "@heroui/button";
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from "@heroui/modal";
import { Input } from "@heroui/input";
import { toast } from 'react-hot-toast';
import { useNavigate } from 'react-router-dom';
import { isWebViewFunc } from '@/utils/panel';
import { siteConfig } from '@/config/site';
import { updatePassword, deleteCurrentAccount, getCurrentSubscription, getUserPackageInfo } from '@/api';
import { safeLogout } from '@/utils/logout';
interface PasswordForm {
  newUsername: string;
  currentPassword: string;
  newPassword: string;
  confirmPassword: string;
}


interface MenuItem {
  path: string;
  label: string;
  icon: React.ReactNode;
  color: string;
  description: string;
}

export default function ProfilePage() {
  const navigate = useNavigate();
  const { isOpen, onOpen, onOpenChange } = useDisclosure();
  const accountDeleteModal = useDisclosure();
  const [username, setUsername] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const [account, setAccount] = useState<any>(null);
  const [subscription, setSubscription] = useState<any>(null);
  const [deleting, setDeleting] = useState(false);
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [passwordForm, setPasswordForm] = useState<PasswordForm>({
    newUsername: '',
    currentPassword: '',
    newPassword: '',
    confirmPassword: ''
  });

  useEffect(() => {
    // 获取用户信息
    const name = localStorage.getItem('name') || 'Admin';
    
    // 兼容处理：如果没有admin字段，根据role_id判断（0为管理员）
    let adminFlag = localStorage.getItem('admin') === 'true';
    if (localStorage.getItem('admin') === null) {
      const roleId = parseInt(localStorage.getItem('role_id') || '1', 10);
      adminFlag = roleId === 0;
      // 补充设置admin字段，避免下次再次判断
      localStorage.setItem('admin', adminFlag.toString());
    }
    
    setUsername(name);
    setIsAdmin(adminFlag);
    Promise.all([getUserPackageInfo(), getCurrentSubscription()]).then(([pkg, sub]) => {
      if (pkg.code === 0) setAccount(pkg.data?.userInfo || null);
      if (sub.code === 0) setSubscription(sub.data || null);
    }).catch(() => {});
  }, []);

  // 管理员菜单项
  const adminMenuItems: MenuItem[] = [
    {
      path: '/limit',
      label: '限速管理',
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z" clipRule="evenodd" />
        </svg>
      ),
      color: 'bg-orange-100 dark:bg-orange-500/20 text-orange-600 dark:text-orange-400',
      description: '管理用户限速策略'
    },
    {
      path: '/user',
      label: '用户管理',
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z" />
        </svg>
      ),
      color: 'bg-blue-100 dark:bg-blue-500/20 text-blue-600 dark:text-blue-400',
      description: '管理系统用户'
    },
    {
      path: '/config',
      label: '网站配置',
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path fillRule="evenodd" d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z" clipRule="evenodd" />
        </svg>
      ),
      color: 'bg-purple-100 dark:bg-purple-500/20 text-purple-600 dark:text-purple-400',
      description: '配置网站设置'
    }
  ];

  // 退出登录
  const handleLogout = () => {
    safeLogout();
    navigate('/', { replace: true });
  };

  const handleDeleteAccount = async () => {
    setDeleting(true);
    try {
      const response = await deleteCurrentAccount();
      if (response.code !== 0) {
        toast.error(response.msg || '注销失败');
        return;
      }
      accountDeleteModal.onClose();
      safeLogout();
      toast.success('账户已注销');
      navigate('/', { replace: true });
    } catch {
      toast.error('注销失败，请稍后重试');
    } finally {
      setDeleting(false);
    }
  };

  const formatBytes = (value: any) => {
    const bytes = Number(value || 0);
    if (!bytes) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const index = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
    return `${(bytes / Math.pow(1024, index)).toFixed(index === 0 ? 0 : 2)} ${units[index]}`;
  };

  const subscriptionExpiry = subscription?.expiresAt ?? account?.subscriptionExpiresAt;
  const subscriptionLimit = subscription?.totalTrafficBytes ?? account?.subscriptionTrafficLimitBytes;
  const subscriptionUsed = subscription?.usedTrafficBytes ?? account?.subscriptionTrafficUsedBytes;

  // 密码表单验证
  const validatePasswordForm = (): boolean => {
    if (!passwordForm.newUsername.trim()) {
      toast.error('请输入新用户名');
      return false;
    }
    if (passwordForm.newUsername.length < 3) {
      toast.error('用户名长度至少3位');
      return false;
    }
    if (!passwordForm.currentPassword) {
      toast.error('请输入当前密码');
      return false;
    }
    if (!passwordForm.newPassword) {
      toast.error('请输入新密码');
      return false;
    }
    if (passwordForm.newPassword.length < 6) {
      toast.error('新密码长度不能少于6位');
      return false;
    }
    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      toast.error('两次输入密码不一致');
      return false;
    }
    return true;
  };

  // 提交密码修改
  const handlePasswordSubmit = async () => {
    if (!validatePasswordForm()) return;

    setPasswordLoading(true);
    try {
      const response = await updatePassword(passwordForm);
      if (response.code === 0) {
        toast.success('密码修改成功，请重新登录');
        onOpenChange();
        handleLogout();
      } else {
        toast.error(response.msg || '密码修改失败');
      }
    } catch (error) {
      toast.error('修改密码时发生错误');
      console.error('修改密码错误:', error);
    } finally {
      setPasswordLoading(false);
    }
  };

  // 重置密码表单
  const resetPasswordForm = () => {
    setPasswordForm({
      newUsername: '',
      currentPassword: '',
      newPassword: '',
      confirmPassword: ''
    });
  };

  return (
    <div className="p-4 space-y-4 max-w-4xl">

      <div>
        <h1 className="text-xl font-bold">我的账户</h1>
        <p className="text-sm text-default-500">查看账户信息、当前订阅和账户安全设置。</p>
      </div>

      <div className="space-y-4">
        {/* 用户信息卡片 */}
        <Card className="border border-gray-200 dark:border-default-200 shadow-md hover:shadow-lg transition-shadow">
          <CardBody className="p-4">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
              <div className="w-12 h-12 bg-primary-100 dark:bg-primary-900/30 rounded-full flex items-center justify-center">
                <svg className="w-6 h-6 text-primary" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 9a3 3 0 100-6 3 3 0 000 6zm-7 9a7 7 0 1114 0H3z" clipRule="evenodd" />
                </svg>
              </div>
              <div className="min-w-0 flex-1">
                <h3 className="text-base font-medium text-foreground">{username}</h3>
                <div className="flex items-center space-x-2 mt-1">
                  <span className={`px-2 py-1 rounded-md text-xs font-medium ${
                    isAdmin 
                      ? 'bg-primary-100 dark:bg-primary-500/20 text-primary-700 dark:text-primary-300' 
                      : 'bg-blue-100 dark:bg-blue-500/20 text-blue-700 dark:text-blue-300'
                  }`}>
                    {isAdmin ? '管理员' : '普通用户'}
                  </span>
                </div>
                <div className="mt-2 break-all text-sm text-default-500">邮箱：{account?.email || '未绑定邮箱'}</div>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="border border-gray-200 dark:border-default-200 shadow-md">
          <CardBody className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
            <div><div className="text-xs text-default-500">当前套餐</div><b>{subscription?.planName || account?.subscriptionPlanName || '未开通套餐'}</b></div>
            <div><div className="text-xs text-default-500">套餐流量</div><b>{Number(subscriptionLimit || 0) > 0 ? formatBytes(subscriptionLimit) : '不限'}</b></div>
            <div><div className="text-xs text-default-500">已用流量</div><b>{formatBytes(subscriptionUsed)}</b></div>
            <div><div className="text-xs text-default-500">到期时间</div><b>{subscriptionExpiry === 0 ? '永久' : subscriptionExpiry ? new Date(subscriptionExpiry).toLocaleString() : '-'}</b></div>
          </CardBody>
        </Card>

        {/* 功能网格 */}
        <Card className="border border-gray-200 dark:border-default-200 shadow-md hover:shadow-lg transition-shadow">
          <CardBody className="p-4">
            <div className="grid grid-cols-3 gap-3">
              {/* 管理员功能 */}
              {isAdmin && adminMenuItems.map((item) => (
                <button
                  key={item.path}
                  onClick={() => navigate(item.path)}
                  className="flex flex-col items-center p-3 rounded-2xl bg-gray-50 dark:bg-default-100 hover:bg-gray-100 dark:hover:bg-default-200 transition-colors duration-200"
                >
                  <div className={`w-10 h-10 ${item.color} rounded-full flex items-center justify-center mb-2`}>
                    {item.icon}
                  </div>
                  <span className="text-xs text-foreground text-center">{item.label}</span>
                </button>
              ))}
              
              {/* 修改密码 */}
              <button
                onClick={onOpen}
                className="flex flex-col items-center p-3 rounded-2xl bg-gray-50 dark:bg-default-100 hover:bg-gray-100 dark:hover:bg-default-200 transition-colors duration-200"
              >
                <div className="w-10 h-10 bg-blue-100 dark:bg-blue-500/20 text-blue-600 dark:text-blue-400 rounded-full flex items-center justify-center mb-2">
                  <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-1 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z" clipRule="evenodd" />
                  </svg>
                </div>
                <span className="text-xs text-foreground text-center">修改密码</span>
              </button>
              
              {/* 注销账户 */}
              <button
                onClick={accountDeleteModal.onOpen}
                className="flex flex-col items-center p-3 rounded-2xl bg-gray-50 dark:bg-default-100 hover:bg-danger-50 dark:hover:bg-danger-500/10 transition-colors duration-200"
              >
                <div className="w-10 h-10 bg-danger-100 dark:bg-danger-500/20 text-danger-600 dark:text-danger-400 rounded-full flex items-center justify-center mb-2">
                  <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M9 2a1 1 0 01.894.553l.724 1.447H16a1 1 0 011 1v11a1 1 0 01-1 1H4a1 1 0 01-1-1V5a1 1 0 011-1h5.382l.724-1.447A1 1 0 019 2zm-3 6a1 1 0 012 0v5a1 1 0 11-2 0V8zm6 0a1 1 0 10-2 0v5a1 1 0 102 0V8z" clipRule="evenodd" /></svg>
                </div>
                <span className="text-xs text-danger-600 text-center">注销账户</span>
              </button>

              {/* 退出登录 */}
              <button
                onClick={handleLogout}
                className="flex flex-col items-center p-3 rounded-2xl bg-gray-50 dark:bg-default-100 hover:bg-gray-100 dark:hover:bg-default-200 transition-colors duration-200"
              >
                <div className="w-10 h-10 bg-red-100 dark:bg-red-500/20 text-red-600 dark:text-red-400 rounded-full flex items-center justify-center mb-2">
                  <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M3 3a1 1 0 00-1 1v12a1 1 0 102 0V4a1 1 0 00-1-1zm10.293 9.293a1 1 0 001.414 1.414l3-3a1 1 0 000-1.414l-3-3a1 1 0 10-1.414 1.414L14.586 9H7a1 1 0 100 2h7.586l-1.293 1.293z" clipRule="evenodd" />
                  </svg>
                </div>
                <span className="text-xs text-foreground text-center">退出登录</span>
              </button>
            </div>
          </CardBody>
        </Card>

        <div className="relative mt-8 text-center py-4 md:fixed md:inset-x-0 md:bottom-20">
               <p className="text-xs text-gray-400 dark:text-gray-500">
                 Powered by <span className="text-gray-500 dark:text-gray-400">TMS</span>
               </p>
               <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">
                 v{ isWebViewFunc() ? siteConfig.app_version : siteConfig.version}
               </p>
             </div>

      </div>


      


      {/* 修改密码弹窗 */}
      <Modal 
        isOpen={isOpen} 
        onOpenChange={() => {
          onOpenChange();
          resetPasswordForm();
        }}
        size="2xl"
      scrollBehavior="outside"
      backdrop="blur"
      placement="center"
      >
        <ModalContent>
          {(onClose: () => void) => (
            <>
              <ModalHeader className="flex flex-col gap-1">修改密码</ModalHeader>
              <ModalBody>
                <div className="space-y-4">
                  <Input
                    label="新用户名"
                    placeholder="请输入新用户名（至少3位）"
                    value={passwordForm.newUsername}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setPasswordForm(prev => ({ ...prev, newUsername: e.target.value }))}
                    variant="bordered"
                  />
                  <Input
                    label="当前密码"
                    type="password"
                    placeholder="请输入当前密码"
                    value={passwordForm.currentPassword}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setPasswordForm(prev => ({ ...prev, currentPassword: e.target.value }))}
                    variant="bordered"
                  />
                  <Input
                    label="新密码"
                    type="password"
                    placeholder="请输入新密码（至少6位）"
                    value={passwordForm.newPassword}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setPasswordForm(prev => ({ ...prev, newPassword: e.target.value }))}
                    variant="bordered"
                  />
                  <Input
                    label="确认密码"
                    type="password"
                    placeholder="请再次输入新密码"
                    value={passwordForm.confirmPassword}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setPasswordForm(prev => ({ ...prev, confirmPassword: e.target.value }))}
                    variant="bordered"
                  />
                </div>
              </ModalBody>
              <ModalFooter>
                <Button color="default" variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button 
                  color="primary" 
                  onPress={handlePasswordSubmit}
                  isLoading={passwordLoading}
                >
                  确定
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal isOpen={accountDeleteModal.isOpen} onOpenChange={accountDeleteModal.onOpenChange} size="md" backdrop="blur">
        <ModalContent>
          {(onClose) => <>
            <ModalHeader>确认删除账户</ModalHeader>
            <ModalBody>
              <p>确定要删除用户 <strong>“{username || '用户账户'}”</strong> 吗？此操作不可撤销，用户的所有数据将被永久删除。</p>
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={onClose}>取消</Button>
              <Button color="danger" onPress={handleDeleteAccount} isLoading={deleting}>确认删除</Button>
            </ModalFooter>
          </>}
        </ModalContent>
      </Modal>
    </div>
  );
}
