// 服务图标与域色彩注册表：菜单注册表只存名字（字符串），视觉映射集中在这里。
// 新增服务：登记图标；新业务域：在 DOMAIN_COLORS 加一个色对（未登记的域按序循环取色）。
import type { Component } from 'vue'
import {
  Menu,
  User,
  Tickets,
  Setting,
  Monitor,
  Bell,
  Document,
  Files,
  DataLine,
  Coin,
  Goods,
  Warning,
  Lock,
  Ticket,
  Box,
  Collection,
  Connection,
  Money,
  Present,
  ChatDotRound,
  Upload,
  MagicStick,
  Search,
  Service,
  Van,
  Stamp,
  Wallet,
  List,
  Odometer,
  Notebook,
  Ship,
  DataAnalysis,
  Share,
} from '@element-plus/icons-vue'

export const serviceIcons: Record<string, Component> = {
  User,
  Tickets,
  Menu,
  Setting,
  Monitor,
  Bell,
  Document,
  Files,
  DataLine,
  Coin,
  Goods,
  Warning,
  Lock,
  Ticket,
  Box,
  Collection,
  Connection,
  Money,
  Present,
  ChatDotRound,
  Upload,
  MagicStick,
  Search,
  Stamp,
  Wallet,
  List,
  Odometer,
  Notebook,
  Ship,
  DataAnalysis,
  Share,
}

/** 域 → 色彩（前景 + 浅底），用于图标底色，让不同域的服务一眼可辨 */
export const DOMAIN_COLORS: Record<string, { fg: string; bg: string }> = {
  存储域: { fg: '#0e6e7e', bg: '#e3f1f4' },
  注册中心域: { fg: '#8a5a2b', bg: '#f5ece2' },
  分库分表域: { fg: '#476581', bg: '#e9edf3' },
  消息域: { fg: '#b25e09', bg: '#fdf0e2' },
  调度域: { fg: '#5b5bd6', bg: '#ececfb' },
  监控运维域: { fg: '#2f7d32', bg: '#e5f3e6' },
  安全合规域: { fg: '#ab3563', bg: '#fbe7ee' },
}

const COLOR_CYCLE = [
  { fg: '#0e6e7e', bg: '#e3f1f4' },
  { fg: '#b25e09', bg: '#fdf0e2' },
  { fg: '#5b5bd6', bg: '#ececfb' },
  { fg: '#2f7d32', bg: '#e5f3e6' },
  { fg: '#ab3563', bg: '#fbe7ee' },
  { fg: '#8a5a2b', bg: '#f5ece2' },
]

const domainIndex = new Map<string, number>()

/** 取域色彩：已登记用登记值，未登记按域名顺序稳定取色 */
export function domainColor(domain: string): { fg: string; bg: string } {
  if (DOMAIN_COLORS[domain]) return DOMAIN_COLORS[domain]
  if (!domainIndex.has(domain)) domainIndex.set(domain, domainIndex.size)
  return COLOR_CYCLE[(domainIndex.get(domain) ?? 0) % COLOR_CYCLE.length]
}

/** 域 → 图标（左侧图标栏用），未登记的域回退 Menu */
export const DOMAIN_ICONS: Record<string, Component> = {
  存储域: Coin,
  注册中心域: Connection,
  消息域: Bell,
  分库分表域: DataAnalysis,
  调度域: Odometer,
  监控运维域: Monitor,
  安全合规域: Lock,
  用户域: User,
  账号安全域: Lock,
  交易域: Money,
  售后域: Service,
  物流域: Van,
  平台域: Setting,
  内容域: Collection,
  数据域: DataLine,
  客服域: ChatDotRound,
}

/** 取域图标：未登记的域回退 Menu */
export function domainIcon(domain: string): Component {
  return DOMAIN_ICONS[domain] ?? Menu
}
