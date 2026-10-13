/**
 * 模拟用户视角弹窗（评审问题 4）：
 * 管理员选一个用户，预览其「登录后看到什么」——菜单树逐行标注
 * 可查看/可管理级别与来源（直接授予 / 经部门继承），覆盖通配授权提示。
 * 数据：GET /admin/permissions/overview?userId=（直接 + 经部门 + 过渡期角色）
 * + GET /admin/menus（菜单目录），判定语义与前端 hasPermission 一致。
 * 仅展示不改动：最小权限核查（没给多/没给少）用，非授权入口。
 */
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { UserSelect, hasPermission } from '@nemo/shared'
import { showApiError } from '../../api/client'
import { permissionOverview } from '../../api/permissions'
import { listMenus } from '../../api/menus'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

const userId = ref('')
const loading = ref(false)
const loadedUser = ref('')

/** 权限总览（当前选中的用户）。 */
interface Overview {
  direct: string[]
  viaOrgs: { orgId: string; orgName: string; codes: string[] }[]
  roles: string[]
}
const overview = ref<Overview>({ direct: [], viaOrgs: [], roles: [] })

/** 菜单目录行（与 Grants 页同构：base = readCode 的 base 段）。 */
interface MenuRow {
  base: string
  service: string
  title: string
  path: string
  sort: number
  readCode: string
  manageCode: string
}
const rows = ref<MenuRow[]>([])

/** 服务 → 菜单两级树（与 Grants 页同构，仅展示）。 */
const tree = computed(() => {
  const bySvc = new Map<string, { title: string; count: number; children: MenuRow[] }>()
  for (const r of rows.value) {
    if (!bySvc.has(r.service)) bySvc.set(r.service, { title: r.service, count: 0, children: [] })
    const n = bySvc.get(r.service)!
    n.count++
    n.children.push(r)
  }
  return [...bySvc.values()].map((n) => {
    n.children.sort((a, b) => a.sort - b.sort)
    return n
  })
})

/** 有效权限集合 = 直接 ∪ 各组织继承（与判定面口径一致，通配由 hasPermission 承担）。 */
const effectivePerms = computed(() => {
  const set = new Set<string>(overview.value.direct)
  for (const o of overview.value.viaOrgs) for (const c of o.codes) set.add(c)
  return [...set]
})

/** 某菜单的来源标注：direct 含码 → 直接；否则查各组织。 */
function sourceOf(code: string): string {
  if (overview.value.direct.includes(code)) {
    return '直接授予'
  }
  for (const o of overview.value.viaOrgs) {
    if (o.codes.includes(code)) return `经部门「${o.orgName}」`
  }
  return ''
}

/** 通配授权提示（可查看/可管理各一条）。 */
const wildRead = computed(() => hasPermission('*:read', effectivePerms.value))
const wildManage = computed(() => hasPermission('*:manage', effectivePerms.value))

async function loadCatalog() {
  try {
    const resp = await listMenus()
    // 仅启用且有权限码的菜单行（空码 = 全员可见，无授予语义；按钮行非菜单）；
    // service = 路由首段（/kafka/topics → kafka，与 Grants 页归组口径一致）。
    rows.value = resp.items
      .filter((m) => m.enabled && m.type !== 'button' && m.permissionCode)
      .map((m) => {
        const base = m.permissionCode.split(':')[0]
        return {
          base,
          service: m.path.split('/')[1] || '',
          title: m.title,
          path: m.path,
          sort: m.sort,
          readCode: m.permissionCode,
          manageCode: base + ':manage',
        }
      })
  } catch (err) {
    showApiError(err)
  }
}

async function loadOverview() {
  if (!userId.value) return
  loading.value = true
  try {
    overview.value = await permissionOverview(userId.value)
    loadedUser.value = userId.value
  } catch (err) {
    showApiError(err)
  } finally {
    loading.value = false
  }
}

watch(userId, () => void loadOverview())
watch(
  () => props.visible,
  (v) => {
    if (v) {
      if (!rows.value.length) void loadCatalog()
      if (userId.value) void loadOverview()
    }
  },
)
onMounted(() => {
  void loadCatalog()
})
</script>

<template>
  <el-dialog
    :model-value="visible"
    title="模拟用户视角"
    width="640px"
    append-to-body
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <div class="pick-row">
      <span class="pick-label">选择用户</span>
      <UserSelect v-model="userId" placeholder="搜索并选择用户" class="pick-input" />
    </div>
    <div v-loading="loading" class="preview-body">
      <template v-if="userId && loadedUser === userId">
        <p v-if="wildRead || wildManage" class="wild-note">
          <el-tag type="warning" size="small" class="wild-tag">通配授权</el-tag>
          该用户拥有全部菜单{{ wildManage ? '的可管理权限（含可查看）' : '的可查看权限' }}
        </p>
        <div class="tree-wrap">
          <div v-for="svc in tree" :key="svc.title" class="svc-block">
            <div class="svc-head">
              <span class="svc-name">{{ svc.title }}</span>
              <span class="svc-count">{{ svc.count }} 个菜单</span>
            </div>
            <div v-for="m in svc.children" :key="m.base" class="menu-row">
              <span class="menu-title" :title="m.title">{{ m.title }}</span>
              <span class="menu-path">{{ m.path }}</span>
              <div class="right">
                <el-tag
                  v-if="hasPermission(m.readCode, effectivePerms)"
                  size="small"
                  type="primary"
                  effect="light"
                >可查看</el-tag>
                <el-tag
                  v-if="hasPermission(m.manageCode, effectivePerms)"
                  size="small"
                  type="warning"
                  effect="light"
                >可管理</el-tag>
                <span v-if="!hasPermission(m.readCode, effectivePerms)" class="no-perm">不可见</span>
                <span v-if="sourceOf(m.readCode)" class="src">{{ sourceOf(m.readCode) }}</span>
              </div>
            </div>
          </div>
        </div>
        <p class="hint">该用户登录后将看到上表中「可查看/可管理」的菜单；「不可见」菜单不会出现在其侧边栏。</p>
      </template>
      <div v-else class="empty">先在上方选择用户，查看其有效权限</div>
    </div>
    <template #footer>
      <el-button @click="emit('update:visible', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.pick-row { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.pick-label { flex: none; font-size: 13px; color: var(--el-text-color-secondary); }
.pick-input { width: 280px; }
.preview-body { min-height: 160px; }
.wild-note { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--el-color-warning); margin: 0 0 10px; }
.wild-tag { flex: none; }
.tree-wrap { border: 1px solid var(--el-border-color-lighter); border-radius: 6px; padding: 4px 8px; max-height: 420px; overflow: auto; }
.svc-block { padding: 4px 0; }
.svc-head { display: flex; align-items: center; gap: 10px; font-size: 13px; font-weight: 600; padding: 6px 4px; }
.svc-count { font-size: 12px; color: var(--el-text-color-secondary); font-weight: 400; }
.menu-row { display: flex; align-items: center; gap: 10px; padding: 5px 4px 5px 16px; font-size: 13px; }
.menu-title { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.menu-path { font-size: 12px; color: var(--el-text-color-secondary); font-family: ui-monospace, 'SF Mono', Menlo, monospace; }
.right { margin-left: auto; display: flex; align-items: center; gap: 8px; flex: none; }
.no-perm { font-size: 12px; color: var(--el-text-color-placeholder); }
.src { font-size: 12px; color: var(--el-text-color-secondary); }
.empty { font-size: 13px; color: var(--el-text-color-secondary); padding: 20px 0; text-align: center; }
.hint { font-size: 12px; color: var(--el-text-color-secondary); margin: 8px 0 0; }
</style>
