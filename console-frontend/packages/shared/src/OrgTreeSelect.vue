<template>
  <!-- 部门树选择器：跨微应用复用的业务组件（基础能力层）。
       数据来自 authz 的组织目录 GET /v1/me/orgs（登录即可读；CAS 权威源
       改造后组织数据唯一事实源 = authz 组织快照），构建树形结构供
       el-tree-select 使用——收编 Grants.vue / TopicManage.vue 各一份的
       orgTree 构建样板。
       option value = 部门 id，label = 部门名（树形层级）。
       组件同时暴露扁平 orgs（含 parentId/name），父组件可经 ref 反查部门名。 -->
  <el-tree-select
    :model-value="modelValue"
    :data="tree"
    :props="{ label: 'name', children: 'children' }"
    node-key="id"
    check-strictly
    default-expand-all
    :placeholder="placeholder"
    clearable
    :loading="loading"
    style="width: 100%"
    @update:model-value="(v: string | number | undefined) => $emit('update:modelValue', String(v ?? ''))"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { createMicroHttp } from './microHttp'
import { useSession } from './useSession'
import type { PagedResult } from './types'

/** 组织目录条目（GET /v1/me/orgs 响应项）。 */
export interface OrgOption {
  id: string
  name: string
  parentId: string
}

/** 树节点（扁平列表挂树后的形态）。 */
export interface OrgNode extends OrgOption {
  children?: OrgNode[]
}

defineProps<{ modelValue?: string; placeholder?: string }>()
defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const loading = ref(false)
const orgs = ref<OrgOption[]>([])

/** 扁平列表 → 树（parentId 挂父节点；无父或父缺失为根，避免孤儿节点丢失）。 */
const tree = computed<OrgNode[]>(() => {
  const byId = new Map<string, OrgNode>()
  for (const o of orgs.value) byId.set(o.id, { ...o })
  const roots: OrgNode[] = []
  for (const o of byId.values()) {
    const p = o.parentId ? byId.get(o.parentId) : undefined
    if (p) (p.children ??= []).push(o)
    else roots.push(o)
  }
  return roots
})

/** 部门名反查：父组件展示授予目标名用（避免各自再拉一次组织列表）。 */
function orgName(id: string): string {
  return orgs.value.find((o) => o.id === id)?.name ?? id
}

defineExpose({ orgs, orgName })

const http = createMicroHttp()
const { ready } = useSession()

async function load() {
  if (orgs.value.length) return
  loading.value = true
  try {
    // /me/orgs（登录即可读）：部门选择器在资源创建/授权场景对普通用户开放，
    // 管理面 /admin/orgs 的超管门槛不适用。
    const resp = await http.get<PagedResult<OrgOption>>('/me/orgs')
    orgs.value = resp.items
  } catch (err) {
    ElMessage.error(err instanceof Error ? `加载部门失败：${err.message}` : '加载部门失败')
  } finally {
    loading.value = false
  }
}

// 首载挂 sessionReady 门控：iframe 形态下 postMessage 会话未到前 token 为空，
// 立刻请求会 401（与各页面同款握手竞态修复，见 micro.ts sessionReady）
onMounted(async () => {
  await ready()
  void load()
})
</script>
