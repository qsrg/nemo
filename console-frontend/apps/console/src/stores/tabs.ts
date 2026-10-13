// 打开的页面页签：多开常驻，localStorage 持久化（刷新后恢复）
import { defineStore } from 'pinia'

export interface PageTab {
  /** 完整路径，如 /order/refunds */
  path: string
  /** 页签名 = 叶子菜单名 */
  title: string
  /** 所属服务 key */
  serviceKey: string
}

const TABS_KEY = 'nemo_open_tabs'

function load(): PageTab[] {
  try {
    return JSON.parse(localStorage.getItem(TABS_KEY) ?? '[]')
  } catch {
    return []
  }
}

export const useTabsStore = defineStore('tabs', {
  state: () => ({
    tabs: load(),
    active: '',
  }),
  getters: {
    /** 打开中的服务（去重，保持首次打开顺序）——每个服务一个常驻容器 */
    serviceKeys(): string[] {
      return [...new Set(this.tabs.map((t: PageTab) => t.serviceKey))]
    },
  },
  actions: {
    save() {
      localStorage.setItem(TABS_KEY, JSON.stringify(this.tabs))
    },
    /** 路由进入某页面时调用：不存在则新开页签，并置为激活（幂等） */
    open(path: string, title: string, serviceKey: string) {
      if (!this.tabs.some((t: PageTab) => t.path === path)) {
        this.tabs.push({ path, title, serviceKey })
      }
      this.active = path
      this.save()
    },
    /** 关闭页签；若关闭的是激活页签，返回应激活的相邻页签（无则 null → 回工作台） */
    close(path: string): PageTab | null {
      const i = this.tabs.findIndex((t: PageTab) => t.path === path)
      if (i < 0) return null
      this.tabs.splice(i, 1)
      this.save()
      if (this.active !== path) return null
      const next = this.tabs[i] ?? this.tabs[i - 1] ?? null
      this.active = next?.path ?? ''
      this.save()
      return next
    },
    /** 关闭全部页签，返回关闭前的服务 key 集合（基座据此销毁沙箱） */
    closeAll(): string[] {
      const keys = this.serviceKeys
      this.tabs = []
      this.active = ''
      this.save()
      return keys
    },
    /** 关闭除 keep 外的全部页签并把 keep 置为激活，返回关闭前的服务 key 集合 */
    closeOthers(keep: string): string[] {
      const keys = this.serviceKeys
      this.tabs = this.tabs.filter((t: PageTab) => t.path === keep)
      this.active = keep
      this.save()
      return keys
    },
  },
})
