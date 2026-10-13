<template>
  <div class="layout">
    <!-- 顶栏：汉堡开关（最左）+ 品牌 + AI 入口/用户；
           菜单面板在顶栏下方展开，不再有纵贯全高的图标栏 -->
    <header class="topbar">
      <button
        class="rail-btn"
        :class="{ active: menuOpen }"
        title="菜单"
        @click="menuOpen = !menuOpen"
      >
        <el-icon :size="18"><Menu /></el-icon>
      </button>
      <div class="brand" @click="router.push('/workbench')">
        <span class="brand-name">nemo 控制台</span>
      </div>

      <div class="spacer" />

      <!-- 告警中心入口：铃铛 + 未恢复告警计数（静态阶段取 alerts.ts） -->
      <button class="bell-entry" type="button" title="告警中心" @click="router.push('/alerts')">
        <el-badge :value="firingCount" :hidden="!firingCount" :offset="[0, 4]">
          <el-icon :size="17"><Bell /></el-icon>
        </el-badge>
      </button>

      <!-- AI 助手全局入口（⌘K / Ctrl+K）：侧滑抽屉，不离开当前页面 -->
      <button class="ai-entry" type="button" @click="ai.toggle()">
        <el-icon><MagicStick /></el-icon>
        <span>问 AI</span>
        <span class="kbd">⌘K</span>
      </button>

      <el-dropdown @command="onCommand">
        <span class="user">
          {{ auth.user?.name ?? '已登录' }}
          <el-icon><ArrowDown /></el-icon>
        </span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="my-permissions">我的权限</el-dropdown-item>
            <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </header>

    <div class="body">
      <!-- 菜单面板：固定全量菜单（服务平铺），展开时推挤内容区 -->
      <aside v-if="menuOpen" class="panel">
        <el-scrollbar class="panel-scroll">
          <!-- 零权限引导：登录后无任何可见菜单时，给出明确提示而非空白面板 -->
          <template v-if="visibleServices.length">
          <!-- 固定全量菜单：服务平铺（不按业务域分组），顺序与注册表一致；
               按服务级权限码过滤（permission-design §5.2），不可见即隐藏 -->
          <div v-for="s in visibleServices" :key="s.key" class="svc-block">
            <div
              class="svc-block-head"
              @click="s.menus.length ? toggleGroup(s.key) : router.push(s.defaultPath ?? '/' + s.key)"
            >
              <span
                class="svc-block-icon"
                :style="{ color: domainColor(s.domain).fg, background: domainColor(s.domain).bg }"
              >
                <el-icon><component :is="serviceIcons[s.icon ?? ''] ?? serviceIcons.Menu" /></el-icon>
              </span>
              <span class="svc-block-title">{{ s.title }}</span>
              <!-- 展开/收起箭头：仅服务有子菜单时展示；无子菜单（仅根行）时
                   服务标题即入口，点击直接进入服务默认页 -->
              <el-icon v-if="s.menus.length" class="chevron" :class="{ open: !collapsed.has(s.key) }">
                <CaretBottom />
              </el-icon>
            </div>
            <template v-if="!collapsed.has(s.key)">
              <template v-for="m in s.menus" :key="m.path">
                <!-- 有子菜单的父菜单：子菜单展开/收起由 subCollapsed 控制 -->
                <template v-if="m.children?.length">
                  <div
                    class="menu-item"
                    :class="{ active: activePathIn(s.key, [m, ...m.children]) === m.path }"
                    @click="toggleSub(s.key, m.path)"
                  >
                    <el-icon class="menu-item-icon">
                      <component :is="serviceIcons[m.icon ?? s.icon ?? ''] ?? serviceIcons.Menu" />
                    </el-icon>
                    <span class="menu-item-title">{{ m.title }}</span>
                    <el-icon class="sub-chevron" :class="{ open: !subCollapsed.has(s.key + m.path) }">
                      <CaretBottom />
                    </el-icon>
                  </div>
                  <div v-if="!subCollapsed.has(s.key + m.path)" class="sub-menu">
                    <div
                      v-for="c in m.children"
                      :key="c.path"
                      class="menu-item sub-item"
                      :class="{ active: c.path === activePathIn(s.key, [m, ...m.children]) }"
                      @click="router.push(c.path)"
                    >
                      <el-icon class="menu-item-icon">
                        <component :is="serviceIcons[c.icon ?? s.icon ?? ''] ?? serviceIcons.Menu" />
                      </el-icon>
                      {{ c.title }}
                    </div>
                  </div>
                </template>
                <!-- 无子菜单的普通菜单项 -->
                <div
                  v-else
                  class="menu-item"
                  :class="{ active: m.path === activePathIn(s.key, s.menus) }"
                  @click="router.push(m.path)"
                >
                  <el-icon class="menu-item-icon">
                    <component :is="serviceIcons[m.icon ?? s.icon ?? ''] ?? serviceIcons.Menu" />
                  </el-icon>
                  {{ m.title }}
                </div>
              </template>
            </template>
          </div>
          </template>
          <div v-else class="menu-empty">
            <el-icon class="menu-empty-icon"><InfoFilled /></el-icon>
            <p class="menu-empty-title">你当前没有可见菜单</p>
            <p class="menu-empty-sub">请联系管理员为你分配菜单权限或资源授权</p>
          </div>
        </el-scrollbar>
        <!-- 底部固定入口：系统设置（用户偏好维护） -->
        <div
          class="panel-footer"
          :class="{ active: route.path === '/settings' }"
          @click="router.push('/settings')"
        >
          <el-icon><Setting /></el-icon>
          <span>系统设置</span>
        </div>
      </aside>
      <main class="main">
        <router-view v-if="!current" />
        <template v-else>
          <!-- 页签栏：多开常驻，点击切换（路由驱动）；右键页签弹出批量关闭菜单。
               溢出时左右箭头滚动 + 尾部页签列表下拉（多页签导航三件套） -->
          <div class="tabbar-wrap">
            <button v-show="canScrollLeft" class="tab-nav-btn" type="button" @click="scrollTabs(-1)">
              <el-icon><ArrowLeft /></el-icon>
            </button>
            <div ref="tabbarRef" class="tabbar" @scroll="updateScrollState">
              <div
                v-for="t in visibleTabs"
                :key="t.path"
                class="tab"
                :class="{ active: tabs.active === t.path }"
                @click="router.push(t.path)"
                @contextmenu.prevent.stop="openCtxMenu($event, t.path)"
              >
                <span class="tab-title">{{ t.title }}</span>
                <el-icon class="tab-close" @click.stop="closeTab(t.path)"><Close /></el-icon>
              </div>
            </div>
            <button v-show="canScrollRight" class="tab-nav-btn" type="button" @click="scrollTabs(1)">
              <el-icon><ArrowRight /></el-icon>
            </button>
            <el-dropdown class="tab-more" trigger="click" @command="onTabPick">
              <button class="tab-nav-btn tab-more-btn" type="button" title="全部页签">
                <el-icon><CaretBottom /></el-icon>
                <span>{{ visibleTabs.length }}</span>
              </button>
              <template #dropdown>
                <el-dropdown-menu>
                  <template v-for="g in tabListGroups" :key="g.domain">
                    <el-dropdown-item disabled class="tab-list-group">{{ g.domain }}</el-dropdown-item>
                    <el-dropdown-item
                      v-for="t in g.tabs"
                      :key="t.path"
                      :command="t.path"
                      :class="{ 'tab-list-active': tabs.active === t.path }"
                    >
                      <span class="tab-list-title">{{ t.title }}</span>
                      <el-icon class="tab-list-close" @click.stop="closeTab(t.path)"><Close /></el-icon>
                    </el-dropdown-item>
                  </template>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
          <!-- 常驻容器：每个打开中的服务一个 wujie 容器，v-show 切换显隐。
               同时打开的微应用全部保持运行（沙箱与页面状态不销毁），
               关闭服务最后一个页签时销毁其沙箱释放内存。
               注意：多个 wujie 应用并发 startApp 存在竞态（脚本不执行），
               容器经 readyKeys 队列串行挂载——上一个 appMounted（或超时兜底）后挂下一个 -->
          <div class="stage">
            <div
              v-for="key in readyKeys"
              :key="key"
              v-show="key === current.key && canSee(key)"
              class="stage-item"
            >
              <MicroAppFrame :app-name="key" :url="urlOf(key)" @app-mounted="settleMount" />
            </div>
          </div>
        </template>
      </main>
    </div>
    <!-- 页签右键菜单：跟随鼠标定位，任意点击/再右键其他区域即关闭 -->
    <div
      v-if="ctxMenu"
      class="ctx-menu"
      :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
    >
      <button class="ctx-item" type="button" @click="onCtxClose">关闭当前</button>
      <button
        class="ctx-item"
        type="button"
        :disabled="tabs.tabs.length <= 1"
        @click="onCtxCloseOthers"
      >
        关闭其他
      </button>
      <button class="ctx-item" type="button" @click="onCtxCloseAll">关闭全部</button>
    </div>
    <AiAssistant />
    <MyPermissionsDialog v-model:visible="myPermsVisible" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowDown, ArrowLeft, ArrowRight, Close, MagicStick, Menu, CaretBottom, Setting, Bell, InfoFilled } from '@element-plus/icons-vue'
import { destroyApp } from 'wujie'
import MicroAppFrame from '../components/MicroAppFrame.vue'
import AiAssistant from '../components/AiAssistant.vue'
import MyPermissionsDialog from '../components/MyPermissionsDialog.vue'
import { useAuthStore } from '../stores/auth'
import { usePrefsStore } from '../stores/prefs'
import { useTabsStore } from '../stores/tabs'
import { useAiStore } from '../stores/ai'
import { leafTitle, type MenuItem } from '../menus'
import { usePermissionStore } from '../stores/permission'
import { useMenusStore } from '../stores/menus'
import { serviceIcons, domainColor } from '../serviceIcons'
import { firingAlerts } from '../alerts'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const prefs = usePrefsStore()
const tabs = useTabsStore()
/** 页签栏 DOM：激活页签滚入视野用。 */
const tabbarRef = ref<HTMLElement | null>(null)
const ai = useAiStore()
const perm = usePermissionStore()
// 菜单动态注册表（Phase C 改造①）：拉取成功后为库内行重建的可见服务，
// 失败回退静态 SERVICES；可见性仍过一道 hasPermission（双保险）
const menus = useMenusStore()

// 按路由查服务（serviceByPath 的动态版）：以 store.items（库内行重建的可见
// 服务集）为源，静态服务存在但库内不可见时返回 undefined → 不挂载容器
function serviceByPath(path: string) {
  return menus.items.find((s) => s.key === path.split('/')[1])
}

// —— 服务级可见性（菜单级权限码改造后）——
// me/menus 已由服务端按调用者菜单权限码过滤（仅下发 permGranted 的行），
// menus.items 重建的服务 = 有可见菜单的服务；此处仅做存在性判定（fail-closed：
// 库内无该服务行 = 服务不可见）。fallback（拉取失败回退静态）时导航降级全量
// 展示——菜单是导航数据而非安全边界，资源级安全由服务端 Check 把守。
function canSee(key: string): boolean {
  return !!menus.items.find((x) => x.key === key)
}
const visibleServices = computed(() => menus.items)
const visibleTabs = computed(() => tabs.tabs.filter((t) => canSee(t.serviceKey)))
/** 页签列表按业务域分组（与侧边栏分组同源），多页签时快速定位。 */
const tabListGroups = computed(() => {
  const groups = new Map<string, { path: string; title: string }[]>()
  for (const t of visibleTabs.value) {
    const domain = serviceByPath(t.path)?.domain ?? '其他'
    if (!groups.has(domain)) groups.set(domain, [])
    groups.get(domain)!.push({ path: t.path, title: t.title })
  }
  return [...groups.entries()].map(([domain, tabs]) => ({ domain, tabs }))
})

// 页签溢出导航：左右箭头可见性随滚动/页签变化更新
//（依赖 visibleTabs，须在其定义之后注册）
const canScrollLeft = ref(false)
const canScrollRight = ref(false)

function updateScrollState() {
  const el = tabbarRef.value
  if (!el) return
  canScrollLeft.value = el.scrollLeft > 1
  canScrollRight.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 1
}

function scrollTabs(dir: number) {
  tabbarRef.value?.scrollBy({ left: dir * 280, behavior: 'smooth' })
}

/** 页签列表下拉：点击切换到对应页签。 */
function onTabPick(path: string) {
  router.push(path)
}

watch([visibleTabs, () => tabs.active], () => nextTick(updateScrollState))

onMounted(() => {
  window.addEventListener('resize', updateScrollState)
  nextTick(updateScrollState)
})

onUnmounted(() => {
  window.removeEventListener('resize', updateScrollState)
})

// ⌘K / Ctrl+K 唤起 AI 助手；全局点击/右键关闭页签右键菜单
function onKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    ai.toggle()
  }
}
onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('click', closeCtxMenu)
  window.addEventListener('contextmenu', closeCtxMenu)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('click', closeCtxMenu)
  window.removeEventListener('contextmenu', closeCtxMenu)
})

// —— 左侧菜单（固定模式） ——
// 面板固定展示注册表全量菜单（服务平铺，不按域分组），不依赖用户收藏；
// 汉堡开关控制显隐，服务块可折叠，当前页按最长前缀高亮
const menuOpen = ref(false)
const current = computed(() => serviceByPath(route.path))
/** 问题 7：用户下拉「我的权限」弹窗显隐。 */
const myPermsVisible = ref(false)

// 未恢复告警数（顶栏铃铛徽标；静态阶段与首页待关注同源）
const firingCount = firingAlerts.length

// 组折叠状态：以 prefs store 为单一数据源（系统设置页维护），
// 点击服务名切换即持久化，设置页与面板两侧即时同步
const collapsed = computed(() => new Set(prefs.menuCollapsed))
function toggleGroup(key: string) {
  prefs.setMenuCollapsed(key, !collapsed.value.has(key))
}

// 子菜单（三级菜单第二层）展开状态：会话内存态，点击父菜单项切换
const subCollapsed = ref<Set<string>>(new Set())
function toggleSub(serviceKey: string, parentPath: string) {
  const key = serviceKey + parentPath
  const next = new Set(subCollapsed.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  subCollapsed.value = next
}

// 组内激活叶子：仅当前路由处于该服务时高亮，最长前缀优先（递归含二级子菜单）
function activePathIn(serviceKey: string, menus: MenuItem[]): string | undefined {
  if (!route.path.startsWith('/' + serviceKey)) return undefined
  const flat: MenuItem[] = []
  const walk = (list: MenuItem[]) => {
    for (const m of list) {
      flat.push(m)
      if (m.children?.length) walk(m.children)
    }
  }
  walk(menus)
  const hit = flat
    .filter((m) => route.path.startsWith(m.path))
    .sort((a, b) => b.path.length - a.path.length)[0]
  return hit?.path ?? flat[0]?.path
}

// 进入服务时记录最近访问；服务页面进页签（多开常驻）
// 从主页点进服务页面时自动展开左侧面板，保持位置感
watch(
  () => current.value?.key,
  (key) => {
    if (key) {
      prefs.touchRecent(key)
      tabs.open(route.path, leafTitle(current.value!, route.path), key)
    }
  },
  { immediate: true },
)
watch(
  () => route.path,
  (path, old) => {
    if (old === '/workbench' && serviceByPath(path)) menuOpen.value = true
  },
)

// 页签路由同步：路由变化即激活对应页签（含直达/刷新场景）
watch(
  () => route.path,
  (path) => {
    const s = serviceByPath(path)
    if (s) tabs.open(path, leafTitle(s, path), s.key)
    // 激活页签滚入视野：页签多时 tabbar 横向滚动，避免当前页签被滚出可视区
    nextTick(() => {
      tabbarRef.value
        ?.querySelector('.tab.active')
        ?.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' })
    })
  },
)

// 关闭页签：若关闭的是激活页签则跳相邻页签（无则回工作台）；
// 服务无页签时销毁其沙箱释放内存
function closeTab(path: string) {
  const wasActive = tabs.active === path
  const key = tabs.tabs.find((t) => t.path === path)?.serviceKey
  const next = tabs.close(path)
  if (wasActive) {
    router.push(next ? next.path : '/workbench')
  }
  if (key && !tabs.serviceKeys.includes(key)) {
    destroyService(key)
  }
}

// 销毁服务沙箱并同步移除其常驻容器，避免 stage 里留空壳
function destroyService(key: string) {
  // wujie 沙箱模式保留：当前直连模式未注册沙箱应用，destroyApp 静默无害
  try {
    destroyApp(key)
  } catch { /* 未注册的沙箱应用无需销毁 */ }
  readyKeys.value = readyKeys.value.filter((k) => k !== key)
}

// 关闭全部页签：回工作台，销毁所有打开中服务的沙箱
function closeAllTabs() {
  const keys = tabs.closeAll()
  router.push('/workbench')
  for (const key of keys) destroyService(key)
}

// 关闭其他页签：仅保留 keep（右键目标页签）并激活，销毁不再有页签服务的沙箱
function closeOtherTabs(keep: string) {
  const keys = tabs.closeOthers(keep)
  router.push(keep)
  for (const key of keys) {
    if (!tabs.serviceKeys.includes(key)) destroyService(key)
  }
}

// —— 页签右键菜单 ——
// 记录弹出位置与目标页签；左键点击或右键其他区域时关闭
const ctxMenu = ref<{ x: number; y: number; path: string } | null>(null)

function openCtxMenu(e: MouseEvent, path: string) {
  ctxMenu.value = { x: e.clientX, y: e.clientY, path }
}

function closeCtxMenu() {
  ctxMenu.value = null
}

function onCtxClose() {
  if (ctxMenu.value) closeTab(ctxMenu.value.path)
  closeCtxMenu()
}

function onCtxCloseOthers() {
  if (ctxMenu.value) closeOtherTabs(ctxMenu.value.path)
  closeCtxMenu()
}

function onCtxCloseAll() {
  closeAllTabs()
  closeCtxMenu()
}

// —— 微应用容器串行挂载队列 ——
// 恢复页签/批量打开时多个容器同时 startApp 会触发 wujie 竞态（脚本不执行），
// 这里保证同一时刻只有一个应用在启动：上一个 afterMount（或 8s 超时）后挂下一个。
const readyKeys = ref<string[]>([])
const waitQueue: string[] = []
let inflight: { key: string; timer: number } | null = null

watch(
  // 同时监听 perms：直达/刷新场景下权限码晚于路由到达（fetch 异步），
  // 需在权限就绪后重新泵队，否则已授权服务的容器不会被挂载
  () => [tabs.serviceKeys, perm.perms] as const,
  () => {
    for (const k of tabs.serviceKeys) {
      if (!readyKeys.value.includes(k) && !waitQueue.includes(k) && inflight?.key !== k) {
        waitQueue.push(k)
      }
    }
    pumpQueue()
  },
  { immediate: true },
)

function pumpQueue() {
  if (inflight) return
  while (waitQueue.length) {
    const key = waitQueue.shift()!
    if (!tabs.serviceKeys.includes(key)) continue // 页签已关，跳过
    if (!canSee(key)) continue // 服务已无权限，跳过（不挂载隐藏服务的沙箱）
    readyKeys.value.push(key)
    inflight = { key, timer: window.setTimeout(() => settleMount(key), 8000) }
    return
  }
}

function settleMount(key: string) {
  if (inflight?.key !== key) return
  clearTimeout(inflight.timer)
  inflight = null
  pumpQueue()
}

function urlOf(key: string) {
  return menus.items.find((s) => s.key === key)?.url ?? ''
}

// —— 会话恢复：刷新后 token 在 localStorage、user 为空，拉 /me 恢复 ——
onMounted(async () => {
  try {
    await auth.restoreUser()
  } catch {
    /* /me 失败时已在 store 内登出，交给路由守卫 */
  }
  if (!auth.token) router.push('/login')
})

function onCommand(cmd: string) {
  if (cmd === 'logout') {
    auth.logout()
    router.push('/login')
  } else if (cmd === 'my-permissions') {
    myPermsVisible.value = true
  }
}
</script>

<style scoped>
/* Grid 布局：顶栏整行 + 下方 body（菜单面板 + 内容区），
   不再为开关单留一列图标栏 */
.layout {
  height: 100%; display: grid;
  grid-template-rows: 52px minmax(0, 1fr);
}
.topbar {
  display: flex; align-items: center; gap: 12px; padding: 0 16px;
  background: var(--el-bg-color); border-bottom: 1px solid var(--el-border-color-light);
}
.brand { display: flex; align-items: center; cursor: pointer; white-space: nowrap; }
.brand-name { font-weight: 600; font-size: 15px; }
.spacer { flex: 1; }
/* AI 入口：与切换器同族但更醒目（主色描边） */
.ai-entry {
  display: inline-flex; align-items: center; gap: 6px; padding: 6px 10px;
  border: 1px solid var(--el-color-primary-light-7); border-radius: 6px;
  background: var(--el-color-primary-light-9); font-size: 13px; cursor: pointer;
  color: var(--el-color-primary); white-space: nowrap; flex: none;
}
.ai-entry:hover { background: var(--el-color-primary-light-8); }
.kbd {
  font-size: 11px; padding: 1px 5px; border-radius: 4px;
  border: 1px solid var(--el-color-primary-light-7); color: var(--el-color-primary);
  background: var(--el-bg-color);
}
.user { display: inline-flex; align-items: center; gap: 4px; cursor: pointer; font-size: 13px; white-space: nowrap; flex: none; }
/* 顶栏告警铃铛：与 AI 入口同族 */
.bell-entry {
  width: 34px; height: 34px; border: none; border-radius: 8px; background: transparent;
  display: inline-flex; align-items: center; justify-content: center;
  cursor: pointer; color: var(--el-text-color-regular); flex: none;
}
.bell-entry:hover { background: var(--el-fill-color); color: var(--el-color-primary); }
.body { display: flex; min-height: 0; min-width: 0; }

/* —— 汉堡开关（顶栏内）+ 菜单面板（顶栏下方，阿里云/方舟形态） —— */
.rail-btn {
  width: 32px; height: 32px; border: none; border-radius: 8px; background: transparent;
  display: inline-flex; align-items: center; justify-content: center;
  cursor: pointer; color: var(--el-text-color-regular); font-size: 17px; position: relative;
  flex: none;
}
.rail-btn:hover { background: var(--el-fill-color); }
.rail-btn.active { background: var(--el-color-primary-light-9); color: var(--el-color-primary); }
.panel {
  width: 232px; flex: none; display: flex; flex-direction: column;
  background: var(--el-bg-color); border-right: 1px solid var(--el-border-color-light);
}
.panel-scroll { flex: 1; padding-top: 8px; }

/* 零权限空态：无可见菜单时的引导提示 */
.menu-empty {
  padding: 48px 24px;
  text-align: center;
  color: var(--el-text-color-secondary);
}
.menu-empty-icon { font-size: 32px; margin-bottom: 12px; color: var(--el-color-info); }
.menu-empty-title { margin: 0 0 6px; font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); }
.menu-empty-sub { margin: 0; font-size: 12px; line-height: 1.6; }
/* 面板底部固定入口：系统设置 */
.panel-footer {
  display: flex; align-items: center; gap: 8px; flex: none;
  padding: 10px 14px; font-size: 13px; cursor: pointer;
  color: var(--el-text-color-regular);
  border-top: 1px solid var(--el-border-color-lighter);
}
.panel-footer:hover { background: var(--el-fill-color-light); color: var(--el-color-primary); }
.panel-footer.active {
  color: var(--el-color-primary); background: var(--el-color-primary-light-9);
  box-shadow: inset 2px 0 0 var(--el-color-primary);
}
.svc-block { padding-bottom: 6px; }
.svc-block-head {
  display: flex; align-items: center; gap: 4px;
  padding: 8px 14px 4px; cursor: pointer;
}
/* 应用名加大加重（区别于页面菜单项） */
.svc-block-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); }
.svc-block-icon {
  width: 24px; height: 24px; border-radius: 6px; flex: none;
  display: inline-flex; align-items: center; justify-content: center; font-size: 14px;
}
.svc-block-head:hover .svc-block-title { color: var(--el-color-primary); }
/* 展开/收起箭头：位于行尾右侧（margin-left:auto 推到行尾） */
.chevron { font-size: 12px; color: var(--el-text-color-secondary); transition: transform 0.2s; margin-left: auto; }
.chevron.open { transform: rotate(180deg); }
.menu-item {
  display: flex; align-items: center; gap: 8px;
  padding: 7px 14px 7px 20px; font-size: 13px; cursor: pointer;
  color: var(--el-text-color-regular);
}
.menu-item-icon { font-size: 14px; color: var(--el-text-color-secondary); flex: none; }
.menu-item:hover { background: var(--el-fill-color-light); }
.menu-item:hover .menu-item-icon { color: var(--el-color-primary); }
.menu-item.active {
  color: var(--el-color-primary); background: var(--el-color-primary-light-9);
  box-shadow: inset 2px 0 0 var(--el-color-primary);
}
.menu-item.active .menu-item-icon { color: var(--el-color-primary); }
/* 三级菜单：父菜单标题可点击展开/收起，子菜单缩进一层 */
.menu-item-title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sub-chevron { font-size: 12px; color: var(--el-text-color-secondary); transition: transform 0.2s; flex: none; }
.sub-chevron.open { transform: rotate(180deg); }
.sub-menu { padding-left: 14px; }
.sub-item { padding-left: 24px; }
.tabbar-wrap {
  display: flex; align-items: stretch; flex: none; min-width: 0;
  background: var(--el-bg-color); border-bottom: 1px solid var(--el-border-color-light);
}
.tabbar {
  display: flex; gap: 4px; padding: 0 8px; overflow-x: auto;
  flex: 1; flex-wrap: nowrap; min-width: 0;
  scrollbar-width: thin; scrollbar-color: var(--el-border-color) transparent;
}
/* 原生横滚动条粗且常显——细化为 6px、hover 才显色 */
.tabbar::-webkit-scrollbar { height: 6px; }
.tabbar::-webkit-scrollbar-track { background: transparent; }
.tabbar::-webkit-scrollbar-thumb { background: transparent; border-radius: 3px; }
.tabbar:hover::-webkit-scrollbar-thumb { background: var(--el-border-color); }
/* 溢出导航：左右箭头 + 页签列表按钮 */
.tab-nav-btn {
  flex: none; border: none; background: transparent; cursor: pointer;
  display: inline-flex; align-items: center; gap: 3px;
  padding: 0 10px; font-size: 13px; color: var(--el-text-color-secondary);
}
.tab-nav-btn:hover { color: var(--el-color-primary); background: var(--el-fill-color-light); }
.tab-more-btn { border-left: 1px solid var(--el-border-color-lighter); }
/* 页签列表下拉项：标题 + 关闭钮 */
.tab-list-title { margin-right: 16px; }
:deep(.tab-list-group) {
  font-size: 12px; color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light); cursor: default;
  margin-top: 2px;
}
:deep(.tab-list-group:hover) { background: var(--el-fill-color-light); color: var(--el-text-color-secondary); }
.tab-list-close { color: var(--el-text-color-placeholder); }
.tab-list-close:hover { color: var(--el-color-danger); }
:deep(.tab-list-active) { color: var(--el-color-primary); font-weight: 500; background: var(--el-color-primary-light-9); }
/* 页签右键菜单：fixed 跟随鼠标，盖在页面之上 */
.ctx-menu {
  position: fixed; z-index: 3000; min-width: 112px; padding: 4px;
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-light);
  border-radius: 6px; box-shadow: var(--el-box-shadow-light);
}
.ctx-item {
  display: block; width: 100%; border: none; background: none; text-align: left;
  padding: 6px 12px; font-size: 13px; border-radius: 4px; cursor: pointer;
  color: var(--el-text-color-regular);
}
.ctx-item:hover:not(:disabled) { background: var(--el-fill-color-light); color: var(--el-color-primary); }
.ctx-item:disabled { color: var(--el-text-color-placeholder); cursor: not-allowed; }
.tab {
  display: inline-flex; align-items: center; gap: 6px; padding: 10px 8px 8px;
  font-size: 13px; cursor: pointer; color: var(--el-text-color-regular);
  white-space: nowrap; border-bottom: 2px solid transparent; margin-bottom: -1px;
  /* 拥挤时先均匀收缩到最小宽（标题省略+悬浮全名），到底才横向滚动 */
  flex: 0 1 auto; min-width: 96px; max-width: 200px;
}
.tab-title {
  min-width: 0; overflow: hidden; text-overflow: ellipsis;
}
.tab:hover { color: var(--el-color-primary); }
.tab.active {
  color: var(--el-color-primary); font-weight: 500;
  border-bottom-color: var(--el-color-primary);
}
.tab-close { font-size: 12px; color: var(--el-text-color-placeholder); border-radius: 3px; }
.tab-close:hover { color: var(--el-text-color-primary); background: var(--el-fill-color); }
.stage { flex: 1; min-height: 0; position: relative; }
.stage-item { position: absolute; inset: 0; }
.main { flex: 1; min-width: 0; background: var(--el-fill-color-lighter); display: flex; flex-direction: column; }
/* 溢出兜底：微应用页签总宽超视口时裁剪在 layout 内，横滚收敛到页签栏自身 */
.layout { overflow-x: clip; }
</style>
