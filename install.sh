#!/bin/bash

# 获取系统架构
get_architecture() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            echo "amd64"  # 默认使用 amd64
            ;;
    esac
}

# 构建下载地址
build_download_url() {
    local ARCH=$(get_architecture)
    echo "https://github.com/ziyue67/Tms/releases/latest/download/gost-${ARCH}"
}

INSTALL_DIR="/etc/gost"
FORCE_CN=0                                       # -c 强制走国内 GitHub 镜像(国内机器 ipinfo 常超时/失败)
GH_MIRROR="${GH_MIRROR:-https://ghfast.top/}"    # 国内 GitHub 加速镜像,可用环境变量覆盖
PUBLIC_PORT_RANGE="20000:39999"                  # 旧行为:整段放行;现在只在 -r 兜底时使用
PROTOCOL_PORTS="${PROTOCOL_PORTS:-}"             # -P 指定要放行的端口(逗号分隔),不传就自动探测 gost 监听端口
FIREWALL_ONLY=0                                  # -f 只同步防火墙,不重装
ALLOW_RANGE_FALLBACK=0                           # -r 探测不到端口时退回整段放行(老行为)
PORT_RANGE_GIVEN=0                               # 面板传了 -p(端口段)就记一下,只用来提示已忽略
RELEASE_INSTALL_URL="https://github.com/ziyue67/Tms/releases/latest/download/install.sh"



# 显示菜单
show_menu() {
  echo "==============================================="
  echo "              管理脚本"
  echo "==============================================="
  echo "请选择操作："
  echo "1. 安装"
  echo "2. 更新"  
  echo "3. 卸载"
  echo "4. 同步防火墙端口(建完入站后跑)"
  echo "5. 退出"
  echo "==============================================="
}

# 删除脚本自身
delete_self() {
  echo ""
  echo "🗑️ 操作已完成，正在清理脚本文件..."
  SCRIPT_PATH="$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")"
  sleep 1
  rm -f "$SCRIPT_PATH" && echo "✅ 脚本文件已删除" || echo "❌ 删除脚本文件失败"
}

# 检查并安装 tcpkill
check_and_install_tcpkill() {
  # 检查 tcpkill 是否已安装
  if command -v tcpkill &> /dev/null; then
    return 0
  fi
  
  # 检测操作系统类型
  OS_TYPE=$(uname -s)
  
  # 检查是否需要 sudo
  if [[ $EUID -ne 0 ]]; then
    SUDO_CMD="sudo"
  else
    SUDO_CMD=""
  fi
  
  if [[ "$OS_TYPE" == "Darwin" ]]; then
    if command -v brew &> /dev/null; then
      brew install dsniff &> /dev/null
    fi
    return 0
  fi
  
  # 检测 Linux 发行版并安装对应的包
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    DISTRO=$ID
  elif [ -f /etc/redhat-release ]; then
    DISTRO="rhel"
  elif [ -f /etc/debian_version ]; then
    DISTRO="debian"
  else
    return 0
  fi
  
  case $DISTRO in
    ubuntu|debian)
      $SUDO_CMD apt update &> /dev/null
      $SUDO_CMD apt install -y dsniff &> /dev/null
      ;;
    centos|rhel|fedora)
      if command -v dnf &> /dev/null; then
        $SUDO_CMD dnf install -y dsniff &> /dev/null
      elif command -v yum &> /dev/null; then
        $SUDO_CMD yum install -y dsniff &> /dev/null
      fi
      ;;
    alpine)
      $SUDO_CMD apk add --no-cache dsniff &> /dev/null
      ;;
    arch|manjaro)
      $SUDO_CMD pacman -S --noconfirm dsniff &> /dev/null
      ;;
    opensuse*|sles)
      $SUDO_CMD zypper install -y dsniff &> /dev/null
      ;;
    gentoo)
      $SUDO_CMD emerge --ask=n net-analyzer/dsniff &> /dev/null
      ;;
    void)
      $SUDO_CMD xbps-install -Sy dsniff &> /dev/null
      ;;
  esac
  
  return 0
}


# 获取用户输入的配置参数
get_config_params() {
  if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
    echo "请输入配置参数："
    
    if [[ -z "$SERVER_ADDR" ]]; then
      read -p "服务器地址: " SERVER_ADDR
    fi
    
    if [[ -z "$SECRET" ]]; then
      read -p "密钥: " SECRET
    fi
    
    if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
      echo "❌ 参数不完整，操作取消。"
      exit 1
    fi
  fi
}

# 解析命令行参数
while getopts "a:s:cp:P:fr" opt; do
  case $opt in
    a) SERVER_ADDR="$OPTARG" ;;
    s) SECRET="$OPTARG" ;;
    c) FORCE_CN=1 ;;
    p) PUBLIC_PORT_RANGE="$OPTARG"; PORT_RANGE_GIVEN=1 ;;
    P) PROTOCOL_PORTS="$OPTARG" ;;
    f) FIREWALL_ONLY=1 ;;
    r) ALLOW_RANGE_FALLBACK=1 ;;
    *) echo "❌ 无效参数"; exit 1 ;;
  esac
done

# 只接受合法端口范围，避免将任意字符串交给防火墙命令。
validate_public_port_range() {
  if [[ ! "$PUBLIC_PORT_RANGE" =~ ^([0-9]{1,5}):([0-9]{1,5})$ ]]; then
    echo "❌ 公网端口范围格式错误: $PUBLIC_PORT_RANGE（应为 起始:结束，例如 20000:39999）"
    exit 1
  fi
  local start="${BASH_REMATCH[1]}"
  local end="${BASH_REMATCH[2]}"
  if (( start < 1 || end > 65535 || start > end )); then
    echo "❌ 公网端口范围无效: $PUBLIC_PORT_RANGE"
    exit 1
  fi
}

# 需要 root:ufw / firewall-cmd 都要提权(手工跑 -f 时通常不是 root)。
run_priv() {
  if [[ $EUID -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

# 逗号/空格分隔的端口清单 → 去重排序,非法项丢掉
normalize_ports() {
  tr ', ' '\n\n' <<<"$1" \
    | grep -E '^[0-9]{1,5}$' \
    | awk '$1 >= 1 && $1 <= 65535' \
    | sort -n -u | paste -sd, -
}

# 从运行中的 gost 反查它对外监听的端口。
# 127.0.0.1 / ::1 上的(sing-box 的 41000+ 内部端口)不用放行,直接剔除。
# 注意:非 root 跑 ss 看不到进程名,探测会为空。
detect_gost_ports() {
  command -v ss >/dev/null 2>&1 || return 0
  ss -lntup 2>/dev/null | grep -i 'gost' | awk '{print $5}' \
    | grep -vE '^(127\.|\[::1\])' \
    | sed -E 's/^.*:([0-9]+)$/\1/' \
    | grep -E '^[0-9]+$' \
    | sort -n -u | paste -sd, -
}

# 只放行给定端口(TCP + UDP):gost 的每个转发服务都是 tcp/udp 各起一个监听。
apply_firewall_rules() {
  local ports="$1"
  local -a list=() rules=()
  local p
  IFS=',' read -r -a list <<<"$ports"
  for p in "${list[@]}"; do
    rules+=("${p}/tcp" "${p}/udp")
  done

  echo ""
  echo "🔐 放行协议端口(共 ${#list[@]} 个): ${ports}（TCP + UDP）"

  if command -v ufw >/dev/null 2>&1; then
    for p in "${rules[@]}"; do run_priv ufw allow "$p" >/dev/null 2>&1 || true; done
    run_priv ufw reload >/dev/null 2>&1 || true
    echo "✅ UFW 已放行: ${rules[*]}"
  elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    for p in "${rules[@]}"; do run_priv firewall-cmd --permanent --add-port="$p" >/dev/null 2>&1 || true; done
    run_priv firewall-cmd --reload >/dev/null 2>&1 || true
    echo "✅ firewalld 已放行: ${rules[*]}"
  else
    echo "ℹ️ 没检测到 ufw/firewalld,用下面的命令自己加(iptables 不持久化,重启要重来):"
    for p in "${rules[@]}"; do
      echo "  iptables -C INPUT -p ${p#*/} --dport ${p%/*} -j ACCEPT || iptables -I INPUT -p ${p#*/} --dport ${p%/*} -j ACCEPT"
    done
  fi

  echo "云安全组照着这一行放就行(不用再开 20000-39999 整段):"
  echo "  TCP ${ports}"
  echo "  UDP ${ports}"
}

# 协议端口由 gost 监听在公网,sing-box 的 41000+ 仅监听 127.0.0.1,
# 所以这里只放行 gost 真正在用的那几个端口,不再整段放行。
configure_public_firewall() {
  local ports="$PROTOCOL_PORTS"

  if [[ -n "$ports" ]]; then
    ports=$(normalize_ports "$ports")
    if [[ -z "$ports" ]]; then
      echo "❌ -P 里没有合法端口: $PROTOCOL_PORTS"
      exit 1
    fi
    echo "🔧 使用 -P 指定的协议端口: ${ports}"
  else
    ports=$(detect_gost_ports)
    if [[ -n "$ports" ]]; then
      echo "🔎 探测到 gost 正在监听的端口: ${ports}"
    fi
  fi

  if [[ -z "$ports" ]]; then
    if [[ "$ALLOW_RANGE_FALLBACK" = "1" ]]; then
      validate_public_port_range
      echo "⚠️ 没有可用端口,按 -r 退回整段放行: ${PUBLIC_PORT_RANGE}（TCP + UDP）"
      command -v ufw >/dev/null 2>&1 && {
        run_priv ufw allow "${PUBLIC_PORT_RANGE}/tcp" >/dev/null 2>&1 || true
        run_priv ufw allow "${PUBLIC_PORT_RANGE}/udp" >/dev/null 2>&1 || true
        run_priv ufw reload >/dev/null 2>&1 || true
      }
      return
    fi
    local rerun="bash ./install.sh -f"
    [[ $EUID -ne 0 ]] && rerun="sudo bash ./install.sh -f"
    echo ""
    echo "ℹ️ 本机还没有 gost 在监听的公网端口(新装机,或入站/转发还没建),本次不放行任何端口。"
    echo "   等面板里把入站建好后再同步一次:"
    echo "   curl -fsSL ${RELEASE_INSTALL_URL} -o ./install.sh && ${rerun}"
    echo "   也知道端口的话可以直接指定: ${rerun} -P 20000,20001,20002"
    echo "   想恢复老的整段放行: 加 -r"
    return
  fi

  if [[ "$PORT_RANGE_GIVEN" = "1" && "$ALLOW_RANGE_FALLBACK" != "1" ]]; then
    echo "ℹ️ 面板传的端口段 ${PUBLIC_PORT_RANGE} 已忽略(只放行实际在用的端口;要整段放行加 -r)"
  fi

  apply_firewall_rules "$ports"
}

# 计算 gost 下载地址(国内或 -c 时走镜像;ipinfo 检测加超时,避免无网时卡死)
# -f 只同步防火墙,不下载东西,跳过这段网络探测
if [[ "$FIREWALL_ONLY" != "1" ]]; then
  DOWNLOAD_URL=$(build_download_url)
  if [ "$FORCE_CN" = "1" ]; then
    COUNTRY="CN"
  else
    COUNTRY=$(curl -s --max-time 5 https://ipinfo.io/country 2>/dev/null || echo "")
  fi
  if [ "$COUNTRY" = "CN" ]; then
    DOWNLOAD_URL="${GH_MIRROR}${DOWNLOAD_URL}"
    echo "🌏 使用国内镜像: ${GH_MIRROR}"
  fi
fi

# 安装功能
install_gost() {
  echo "🚀 开始安装 GOST..."
  get_config_params
  configure_public_firewall

    # 检查并安装 tcpkill
  check_and_install_tcpkill
  

  mkdir -p "$INSTALL_DIR"

  # 停止并禁用已有服务
  if systemctl list-units --full -all | grep -Fq "gost.service"; then
    echo "🔍 检测到已存在的gost服务"
    systemctl stop gost 2>/dev/null && echo "🛑 停止服务"
    systemctl disable gost 2>/dev/null && echo "🚫 禁用自启"
  fi

  # 删除旧文件
  [[ -f "$INSTALL_DIR/gost" ]] && echo "🧹 删除旧文件 gost" && rm -f "$INSTALL_DIR/gost"

  # 下载 gost
  echo "⬇️ 下载 gost 中..."
  curl -L "$DOWNLOAD_URL" -o "$INSTALL_DIR/gost"
  if [[ ! -f "$INSTALL_DIR/gost" || ! -s "$INSTALL_DIR/gost" ]]; then
    echo "❌ 下载失败，请检查网络或下载链接。"
    exit 1
  fi
  chmod +x "$INSTALL_DIR/gost"
  echo "✅ 下载完成"

  # 打印版本
  echo "🔎 gost 版本：$($INSTALL_DIR/gost -V)"

  # 写入 config.json (安装时总是创建新的)
  CONFIG_FILE="$INSTALL_DIR/config.json"
  echo "📄 创建新配置: config.json"
  cat > "$CONFIG_FILE" <<EOF
{
  "addr": "$SERVER_ADDR",
  "secret": "$SECRET"
}
EOF

  # 写入 gost.json
  GOST_CONFIG="$INSTALL_DIR/gost.json"
  if [[ -f "$GOST_CONFIG" ]]; then
    echo "⏭️ 跳过配置文件: gost.json (已存在)"
  else
    echo "📄 创建新配置: gost.json"
    cat > "$GOST_CONFIG" <<EOF
{}
EOF
  fi

  # 加强权限
  chmod 600 "$INSTALL_DIR"/*.json

  # 创建 systemd 服务
  SERVICE_FILE="/etc/systemd/system/gost.service"
  cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Gost Proxy Service
After=network.target

[Service]
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/gost
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

  # 启动服务
  systemctl daemon-reload
  systemctl enable gost
  systemctl start gost

  # 检查状态
  echo "🔄 检查服务状态..."
  if systemctl is-active --quiet gost; then
    echo "✅ 安装完成，gost服务已启动并设置为开机启动。"
    echo "📁 配置目录: $INSTALL_DIR"
    echo "🔧 服务状态: $(systemctl is-active gost)"
  else
    echo "❌ gost服务启动失败，请执行以下命令查看日志："
    echo "journalctl -u gost -f"
  fi

  print_firewall_summary
}

# 安装完成后最后一块提示:云厂商安全组到底要放行哪几个端口
print_firewall_summary() {
  local ports="$PROTOCOL_PORTS"
  [[ -n "$ports" ]] && ports=$(normalize_ports "$ports")
  [[ -z "$ports" ]] && ports=$(detect_gost_ports)

  echo ""
  echo "================= 云安全组需要手动放行 ================="
  if [[ -n "$ports" ]]; then
    echo "  TCP ${ports}"
    echo "  UDP ${ports}"
  else
    echo "  现在还没有端口可放行(入站/转发还没建)。"
    echo "  这个节点在面板里的端口段是 ${PUBLIC_PORT_RANGE}，每建一个入站/转发"
    echo "  就从段首开始占用一个空闲端口(通常就是 20000 起连续几个)。"
    echo "  建好后执行下面这行，会打印要放行的准确清单:"
    echo "   curl -fsSL ${RELEASE_INSTALL_URL} -o ./install.sh && sudo bash ./install.sh -f"
  fi
  echo "  ⚠️ 不要放行 20000-39999 整段,也不要放 41000+"
  echo "     (41000+ 是 sing-box 内部端口,只监听 127.0.0.1,放出去等于裸奔)"
  echo "======================================================="
}

# 更新功能
update_gost() {
  echo "🔄 开始更新 GOST..."
  
  if [[ ! -d "$INSTALL_DIR" ]]; then
    echo "❌ GOST 未安装，请先选择安装。"
    return 1
  fi
  
  echo "📥 使用下载地址: $DOWNLOAD_URL"
  
  # 检查并安装 tcpkill
  check_and_install_tcpkill
  configure_public_firewall
  
  # 先下载新版本
  echo "⬇️ 下载最新版本..."
  curl -L "$DOWNLOAD_URL" -o "$INSTALL_DIR/gost.new"
  if [[ ! -f "$INSTALL_DIR/gost.new" || ! -s "$INSTALL_DIR/gost.new" ]]; then
    echo "❌ 下载失败。"
    return 1
  fi

  # 停止服务
  if systemctl list-units --full -all | grep -Fq "gost.service"; then
    echo "🛑 停止 gost 服务..."
    systemctl stop gost
  fi

  # 替换文件
  mv "$INSTALL_DIR/gost.new" "$INSTALL_DIR/gost"
  chmod +x "$INSTALL_DIR/gost"
  
  # 打印版本
  echo "🔎 新版本：$($INSTALL_DIR/gost -V)"

  # 重启服务
  echo "🔄 重启服务..."
  systemctl start gost
  
  echo "✅ 更新完成，服务已重新启动。"
}

# 卸载功能
uninstall_gost() {
  echo "🗑️ 开始卸载 GOST..."
  
  read -p "确认卸载 GOST 吗？此操作将删除所有相关文件 (y/N): " confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "❌ 取消卸载"
    return 0
  fi

  # 停止并禁用服务
  if systemctl list-units --full -all | grep -Fq "gost.service"; then
    echo "🛑 停止并禁用服务..."
    systemctl stop gost 2>/dev/null
    systemctl disable gost 2>/dev/null
  fi

  # 协议功能会在本机装 sing-box(服务文件在 /etc/systemd/system,不在安装目录里),
  # 不一起清掉的话:二进制被删、服务还注册着 → systemd 会一直重启失败刷日志
  if systemctl list-units --full -all | grep -Fq "sing-box.service"; then
    echo "🛑 停止并禁用 sing-box 服务..."
    systemctl stop sing-box 2>/dev/null
    systemctl disable sing-box 2>/dev/null
  fi

  # 删除服务文件
  if [[ -f "/etc/systemd/system/gost.service" ]]; then
    rm -f "/etc/systemd/system/gost.service"
    echo "🧹 删除服务文件"
  fi
  if [[ -f "/etc/systemd/system/sing-box.service" ]]; then
    rm -f "/etc/systemd/system/sing-box.service"
    echo "🧹 删除 sing-box 服务文件"
  fi
  # sing-box 的 systemd 覆盖配置(排查重启限流时可能加过)
  rm -rf /etc/systemd/system/sing-box.service.d 2>/dev/null

  # target 的 .wants 里残留的软链接。正常情况 systemctl disable 会删掉,
  # 但服务本身已经异常、或当初是手工 enable 的话就会留下来 ——
  # 结果是 systemctl list-units --all 里一直挂着一条 not-found,看着像没卸干净
  find /etc/systemd /run/systemd \( -name 'gost.service' -o -name 'sing-box.service' \) -delete 2>/dev/null

  # 删除安装目录(gost 二进制、sing-box 二进制、配置、自签证书都在这里)
  if [[ -d "$INSTALL_DIR" ]]; then
    rm -rf "$INSTALL_DIR"
    echo "🧹 删除安装目录: $INSTALL_DIR"
  fi

  # 重载 systemd 并清掉 failed 记录
  systemctl daemon-reload
  systemctl reset-failed 2>/dev/null

  echo "✅ 卸载完成(gost + sing-box + 配置 + 证书 已全部清除)"
}

# 主逻辑
main() {
  # -f 只同步防火墙:入站建好后不用重装,跑一次把新端口补上就行
  if [[ "$FIREWALL_ONLY" = "1" ]]; then
    configure_public_firewall
    exit 0
  fi

  # 如果提供了命令行参数，直接执行安装
  if [[ -n "$SERVER_ADDR" && -n "$SECRET" ]]; then
    install_gost
    delete_self
    exit 0
  fi

  # 显示交互式菜单
  while true; do
    show_menu
    read -p "请输入选项 (1-5): " choice
    
    case $choice in
      1)
        install_gost
        delete_self
        exit 0
        ;;
      2)
        update_gost
        delete_self
        exit 0
        ;;
      3)
        uninstall_gost
        delete_self
        exit 0
        ;;
      4)
        # 这里以前调的是不存在的 block_protocol(),选 4 会报 command not found 还把脚本自己删了
        configure_public_firewall
        echo ""
        read -p "按回车返回菜单..." _
        ;;
      5)
        echo "👋 退出脚本"
        delete_self
        exit 0
        ;;
      *)
        echo "❌ 无效选项，请输入 1-5"
        echo ""
        ;;
    esac
  done
}

# 执行主函数
main
