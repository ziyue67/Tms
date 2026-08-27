import { Route, Routes, useNavigate, Navigate } from "react-router-dom";
import { useEffect } from "react";

import IndexPage from "@/pages/index";
import ChangePasswordPage from "@/pages/change-password";
import DashboardPage from "@/pages/dashboard";
import ForwardPage from "@/pages/forward";
import TunnelPage from "@/pages/tunnel";
import NodePage from "@/pages/node";
import UserPage from "@/pages/user";
import ProfilePage from "@/pages/profile";
import LimitPage from "@/pages/limit";
import InboundPage from "@/pages/inbound";
import RelayPage from "@/pages/relay";
import GuidePage from "@/pages/guide";
import MySubPage from "@/pages/my-sub";
import ConfigPage from "@/pages/config";
import RegisterPage from "@/pages/register";
import ForgotPasswordPage from "@/pages/forgot-password";
import ResetPasswordPage from "@/pages/reset-password";
import AdminSubscriptionPage from "@/pages/admin-subscription";
import UserSubscriptionDashboardPage from "@/pages/user-subscription-dashboard";
import PurchasePage from "@/pages/purchase";
import RedeemPage from "@/pages/redeem";
import MyOrdersPage from "@/pages/my-orders";
import AdminRedeemCodesPage from "@/pages/admin-redeem-codes";
import AdminOrdersPage from "@/pages/admin-orders";
import { SettingsPage } from "@/pages/settings";

import AdminLayout from "@/layouts/admin";

import { isLoggedIn, isAdmin } from "@/utils/auth";
import { siteConfig } from "@/config/site";

// 简化的路由保护组件 - 使用 React Router 导航避免循环
const ProtectedRoute = ({ children, skipLayout = false }: { children: React.ReactNode, skipLayout?: boolean }) => {
  const authenticated = isLoggedIn();
  const navigate = useNavigate();
  
  useEffect(() => {
    if (!authenticated) {
      // 使用 React Router 导航，避免无限跳转
      navigate('/', { replace: true });
    }
  }, [authenticated, navigate]);

  if (!authenticated) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-white dark:bg-black">
        <div className="text-lg text-gray-700 dark:text-gray-200"></div>
      </div>
    );
  }

  // 如果跳过布局，直接返回子组件
  if (skipLayout) {
    return <>{children}</>;
  }

  // Desktop and mobile share the same information architecture. AdminLayout
  // switches its sidebar to an accessible drawer on smaller screens.
  return <AdminLayout>{children}</AdminLayout>;
};


// 登录页面路由组件 - 已登录则重定向到dashboard
const LoginRoute = () => {
  const authenticated = isLoggedIn();
  const navigate = useNavigate();
  
  useEffect(() => {
    if (authenticated) {
      // 使用 React Router 导航，避免无限跳转
      navigate('/dashboard', { replace: true });
    }
  }, [authenticated, navigate]);
  
  if (authenticated) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-gray-100 dark:bg-black">
        <div className="text-lg text-gray-700 dark:text-gray-200"></div>
      </div>
    );
  }
  
  return <IndexPage />;
};

function App() {
  // 立即设置页面标题（使用已从缓存读取的配置）
  useEffect(() => {
    document.title = siteConfig.name;
    
    // 异步检查是否有配置更新
    const checkTitleUpdate = async () => {
      try {
        // 引入必要的函数
        const { getCachedConfig } = await import('@/config/site');
        const cachedAppName = await getCachedConfig('app_name');
        if (cachedAppName && cachedAppName !== document.title) {
          document.title = cachedAppName;
        }
      } catch (error) {
        console.warn('检查标题更新失败:', error);
      }
    };

    // 延迟检查，避免阻塞初始渲染
    const timer = setTimeout(checkTitleUpdate, 100);

    return () => clearTimeout(timer);
  }, []);

  return (
    <Routes>
      <Route path="/" element={<LoginRoute />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/forgot-password" element={<ForgotPasswordPage />} />
      <Route path="/reset-password" element={<ResetPasswordPage />} />
      <Route path="/subscription" element={<ProtectedRoute><Navigate to="/purchase" replace /></ProtectedRoute>} />
      <Route path="/purchase" element={<ProtectedRoute><PurchasePage /></ProtectedRoute>} />
      <Route path="/redeem" element={<ProtectedRoute><RedeemPage /></ProtectedRoute>} />
      <Route path="/my-orders" element={<ProtectedRoute><MyOrdersPage /></ProtectedRoute>} />
      <Route path="/admin/subscription" element={<ProtectedRoute><AdminSubscriptionPage /></ProtectedRoute>} />
      <Route path="/admin/redeem-codes" element={<ProtectedRoute><AdminRedeemCodesPage /></ProtectedRoute>} />
      <Route path="/admin/orders" element={<ProtectedRoute><AdminOrdersPage /></ProtectedRoute>} />
      <Route 
        path="/change-password" 
        element={
          <ProtectedRoute skipLayout={true}>
            <ChangePasswordPage />
          </ProtectedRoute>
        } 
      />
      <Route
        path="/dashboard"
        element={
          <ProtectedRoute>
            {isAdmin() ? <DashboardPage /> : <UserSubscriptionDashboardPage />}
          </ProtectedRoute>
        }
      />
      <Route
        path="/forward"
        element={
          <ProtectedRoute>
            <ForwardPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/inbound"
        element={
          <ProtectedRoute>
            <InboundPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/relay"
        element={
          <ProtectedRoute>
            <RelayPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/guide"
        element={
          <ProtectedRoute>
            <GuidePage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/my-sub"
        element={
          <ProtectedRoute>
            <MySubPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/tunnel"
        element={
          <ProtectedRoute>
            <TunnelPage />
          </ProtectedRoute>
        } 
      />
      <Route 
        path="/node" 
        element={
          <ProtectedRoute>
            <NodePage />
          </ProtectedRoute>
        } 
      />
      <Route 
        path="/user" 
        element={
          <ProtectedRoute>
            <UserPage />
          </ProtectedRoute>
        } 
      />
      <Route 
        path="/profile" 
        element={
          <ProtectedRoute>
            <ProfilePage />
          </ProtectedRoute>
        } 
      />
      <Route 
        path="/limit" 
        element={
          <ProtectedRoute>
            <LimitPage />
          </ProtectedRoute>
        } 
      />
      <Route 
        path="/config" 
        element={
          <ProtectedRoute>
            <ConfigPage />
          </ProtectedRoute>
        } 
      />
      <Route 
        path="/settings" 
        element={<SettingsPage />}
      />
    </Routes>
  );
}

export default App;
