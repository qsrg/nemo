// 工作台偏好：最近访问与菜单展开偏好，localStorage 持久化
// （菜单栏为固定全量模式；各服务菜单默认展开/收起在系统设置页维护）
import { defineStore } from 'pinia'
import { SERVICES, type ServiceDef } from '../menus'

const RECENT_KEY = 'nemo_recent_services'
const MENU_COLLAPSED_KEY = 'nemo_menu_collapsed'

function load(key: string): string[] {
  try {
    return JSON.parse(localStorage.getItem(key) ?? '[]')
  } catch {
    return []
  }
}

function save(key: string, ids: string[]) {
  localStorage.setItem(key, JSON.stringify(ids))
}

export const usePrefsStore = defineStore('prefs', {
  state: () => ({
    recents: load(RECENT_KEY) as string[],
    /** 左侧面板中处于收起状态的服务 key 列表 */
    menuCollapsed: load(MENU_COLLAPSED_KEY) as string[],
  }),
  getters: {
    recentServices(): ServiceDef[] {
      return this.recents
        .map((k: string) => SERVICES.find((s) => s.key === k))
        .filter(Boolean)
        .slice(0, 6)
    },
  },
  actions: {
    // 每次进入服务记录一次；新访问置顶，最多留 6 个
    touchRecent(key: string) {
      this.recents = [key, ...this.recents.filter((k: string) => k !== key)].slice(0, 6)
      save(RECENT_KEY, this.recents)
    },
    // 设置某服务菜单在左侧面板中的展开/收起偏好（collapsed=true 收起）
    setMenuCollapsed(key: string, collapsed: boolean) {
      this.menuCollapsed = collapsed
        ? [...new Set([...this.menuCollapsed, key])]
        : this.menuCollapsed.filter((k: string) => k !== key)
      save(MENU_COLLAPSED_KEY, this.menuCollapsed)
    },
  },
})
