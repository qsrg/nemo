/**
 * 我的权限弹窗（评审问题 7）：
 * 当前登录用户查看「为什么侧边栏有这些菜单」——按服务归组的菜单树，
 * 每行标注可查看/可管理级别；数据直接拉取 authz me 接口（me/menus 带
 * permissionCode + me/permissions），不依赖 menus store 的降级结构。
 * 仅展示：权限来源（服务端菜单过滤 + 权限码）即用户登录后的真实可见范围。
 */
<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { createHttp } from '@nemo/http'
import { hasPermission } from '@nemo/shared'
import { useAuthStore } from '../stores/auth'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

/** me/menus 行（服务端已按调用者权限过滤）。 */
interface MeMenuItem {
  id: number
  title: string
  icon: string
  path: string
  sort: number
  permissionCode: string
}

/** 服务组视图：service → menus（同基座 rebuildServices 归组口径）。 */
interface SvcGroup {
  key: string
  title: string
  menus: MeMenuItem[]
}

const groups = ref<SvcGroup[]>([])
const perms = ref<string[]>([])
const loading = ref(false)
const error = ref('')

const http = createHttp({
  baseURL: '/api/v1',
  getToken: () => useAuthStore().token,
})

function rebuild(rows: MeMenuItem[]): SvcGroup[] {
  const bySvc = new Map<string, SvcGroup>()
  for (const r of rows) {
    // 按路由首段归组（/kafka/topics → kafka，与基座 rebuildServices 同口径）。
    const key = r.path.split('/')[1] || ''
    if (!bySvc.has(key)) bySvc.set(key, { key, title: key, menus: [] })
    const g = bySvc.get(key)!
    g.title = key // 服务展示名以注册表为准；此处用 key 段兜底
    g.menus.push(r)
  }
  for (const g of bySvc.values()) g.menus.sort((a, b) => a.sort - b.sort || a.id - b.id)
  return [...bySvc.values()]
}

/** 菜单 manage 码 = read 码服务段 + :manage（与授权面同构）；空码菜单无 manage 码。 */
function manageCodeOf(m: MeMenuItem): string {
  if (!m.permissionCode) return ''
  return m.permissionCode.split(':')[0] + ':manage'
}

async function load() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    const [menus, perm] = await Promise.all([
      http.get<{ items: MeMenuItem[] }>('/me/menus'),
      http.get<{ perms: string[] }>('/me/permissions'),
    ])
    groups.value = rebuild(menus.items ?? [])
    perms.value = perm.perms ?? []
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (props.visible) void load()
})
// 弹窗每次打开时拉取（组件常驻 Layout，仅 visible 变化）
watch(
  () => props.visible,
  (v) => {
    if (v) void load()
  },
)
</script>

<template>
  <el-dialog
    :model-value="visible"
    title="我的权限"
    width="600px"
    append-to-body
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <div v-loading="loading" class="my-perm-body">
      <p v-if="error" class="err">{{ error }}</p>
      <template v-else-if="groups.length">
        <div v-for="g in groups" :key="g.key" class="svc-block">
          <div class="svc-head">
            <span class="svc-name">{{ g.title }}</span>
            <span class="svc-count">{{ g.menus.length }} 个菜单</span>
          </div>
          <div v-for="m in g.menus" :key="m.id" class="menu-row">
            <span class="menu-title" :title="m.title">{{ m.title }}</span>
            <span class="menu-path">{{ m.path }}</span>
            <div class="right">
              <!-- 空码菜单 = 全员可见（不过滤）；非空码 = 准入门槛，未持有者
                   经资源驱动通道（资源关系）解锁时菜单仍下发。 -->
              <el-tag v-if="!m.permissionCode" size="small" type="success" effect="light">公开</el-tag>
              <el-tag v-else-if="hasPermission(m.permissionCode, perms)" size="small" type="primary" effect="light">可查看</el-tag>
              <el-tag v-if="m.permissionCode && hasPermission(manageCodeOf(m), perms)" size="small" type="warning" effect="light">可管理</el-tag>
              <span v-if="m.permissionCode && !hasPermission(m.permissionCode, perms)" class="no-perm">资源授权</span>
            </div>
          </div>
        </div>
        <p class="hint">「可查看」= 可进入该页面查看；「可管理」= 可在页面内执行写操作（如新建、删除）。权限由管理员在「人员授权」中分配，直接授予或经所在部门继承。</p>
      </template>
      <div v-else-if="!loading" class="empty">当前账号暂无可见菜单</div>
    </div>
    <template #footer>
      <el-button @click="emit('update:visible', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.my-perm-body { min-height: 140px; }
.err { font-size: 13px; color: var(--el-color-danger); }
.svc-block { padding: 6px 0; }
.svc-head { display: flex; align-items: center; gap: 10px; font-size: 13px; font-weight: 600; padding: 4px 0; }
.svc-count { font-size: 12px; color: var(--el-text-color-secondary); font-weight: 400; }
.menu-row { display: flex; align-items: center; gap: 10px; padding: 5px 4px 5px 14px; font-size: 13px; }
.menu-title { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.menu-path { font-size: 12px; color: var(--el-text-color-secondary); font-family: ui-monospace, 'SF Mono', Menlo, monospace; }
.right { margin-left: auto; display: flex; align-items: center; gap: 8px; flex: none; }
.no-perm { font-size: 12px; color: var(--el-text-color-placeholder); }
.empty { font-size: 13px; color: var(--el-text-color-secondary); padding: 24px 0; text-align: center; }
.hint { font-size: 12px; color: var(--el-text-color-secondary); margin: 10px 0 0; }
</style>
