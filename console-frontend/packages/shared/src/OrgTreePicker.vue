<template>
  <!-- 单选组织架构树选择器（人员授权等"定位组织→选人"场景）：
       部门 + 用户混合节点，单击部门/用户 = 选中该对象视角（右栏联动）。
       与 ResourceGrantDialog 的多选勾选树不同——本组件是单选高亮定位。
       数据：部门 + 全部成员一次拉全（用户规模小，如 500 人，全量加载；
       搜索对所有节点生效，无需逐层展开）。节点展示 displayName（CAS 权威，
       未同步回退 userId）。
       交互：点行 = 选中（不展开收起），展开/收起用行首箭头——避免"点部门
       既展开又选中"的双重行为；默认展开部门层（成员收起，浏览清爽）。 -->
  <div class="org-picker">
    <el-input
      v-model="kw"
      size="small"
      clearable
      class="org-picker-search"
      :placeholder="placeholder"
      @input="onFilter"
    />
    <el-tree
      v-if="!initialLoading"
      ref="treeRef"
      :data="treeData"
      node-key="key"
      highlight-current
      :current-node-key="modelValue || undefined"
      :default-expanded-keys="expandedKeys"
      :expand-on-click-node="false"
      :props="{ label: 'name', children: 'children' }"
      :filter-node-method="filterNode"
      class="org-picker-tree"
      empty-text="无部门数据"
      @node-click="onNodeClick"
    >
      <template #default="{ data }">
        <span class="org-node" :title="data.name">
          <span class="org-node-name">{{ data.name }}</span>
          <span v-if="data.kind === 'org'" class="org-node-tag">部门</span>
          <span v-else class="org-node-tag org-node-tag-user">用户</span>
        </span>
      </template>
    </el-tree>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { ElTree } from 'element-plus'
import { createMicroHttp } from './microHttp'
import { useSession } from './useSession'
import type { PagedResult } from './types'

/** 组织目录条目（GET /v1/admin/orgs 响应项）。 */
interface OrgOption {
  id: string
  name: string
  parentId: string
}

/** 组织成员条目（GET /v1/admin/orgs/:id/members 响应项；displayName 为
 *  orgsync 同步的 CAS 权威显示名，未同步为空串）。 */
interface MemberItem {
  orgId: string
  userId: string
  displayName?: string
}

/** 树节点：部门（kind=org，children 含子部门与用户）或用户叶子（kind=user）。 */
interface TreeNode {
  key: string
  id: string
  name: string
  kind: 'org' | 'user'
  /** 所属部门 id（仅用户节点，供上级组件反查/回显） */
  orgId?: string
  children?: TreeNode[]
}

const props = withDefaults(
  defineProps<{
    /** 当前选中节点的 key（org:{id} / user:{uid}），用于回显高亮 */
    modelValue?: string
    placeholder?: string
  }>(),
  { modelValue: '', placeholder: '搜索部门/用户' },
)
const emit = defineEmits<{
  (e: 'select', payload: { type: 'user' | 'org'; id: string }): void
  (e: 'update:modelValue', key: string): void
}>()

const http = createMicroHttp()
const { ready } = useSession()
const treeRef = ref<InstanceType<typeof ElTree>>()
const kw = ref('')

// ---------- 全量数据（部门 + 全部成员，用户规模小一次拉全） ----------
const orgs = ref<OrgOption[]>([])
const membersByOrg = ref<Map<string, MemberItem[]>>(new Map())
const loading = ref(false)
/** 首载是否就绪：数据拉取完成前不挂载树（保证 default-expanded-keys 生效）。 */
const initialLoading = ref(true)

/** 全量树：部门树 + 各部门直接成员（子部门在前、成员在后，挂部门节点下）。 */
const treeData = computed<TreeNode[]>(() => {
  const byId = new Map<string, OrgOption>()
  for (const o of orgs.value) byId.set(o.id, { ...o })
  const nodeOf = new Map<string, TreeNode>()
  const ensureNode = (org: OrgOption): TreeNode => {
    let n = nodeOf.get(org.id)
    if (!n) {
      n = { key: `org:${org.id}`, id: org.id, name: org.name, kind: 'org', children: [] }
      nodeOf.set(org.id, n)
    }
    return n
  }
  const roots: TreeNode[] = []
  for (const o of orgs.value) {
    const node = ensureNode(o)
    const members = membersByOrg.value.get(o.id) ?? []
    for (const m of members) {
      node.children!.push({
        key: `user:${m.userId}`,
        id: m.userId,
        name: m.displayName ? `${m.displayName}（${m.userId}）` : m.userId,
        kind: 'user',
        orgId: m.orgId,
      })
    }
    const p = o.parentId ? byId.get(o.parentId) : undefined
    if (p) ensureNode(p).children!.push(node)
    else roots.push(node)
  }
  return roots
})

/** 默认展开部门层：根部门（无父或父缺失）展开，成员收起。 */
const expandedKeys = computed(() =>
  orgs.value.filter((o) => !o.parentId || !orgs.value.some((x) => x.id === o.parentId)).map((o) => `org:${o.id}`),
)

/** 用户显示名缓存（全量填充，供外部反查用户展示名）。 */
const memberNames = ref<Map<string, string>>(new Map())

async function load() {
  if (orgs.value.length) return
  loading.value = true
  try {
    const resp = await http.get<PagedResult<OrgOption>>('/admin/orgs')
    orgs.value = resp.items
    // 并行拉各部门直接成员（部门数量少；单部门成员失败不阻断整体）
    const ids = resp.items.map((o) => o.id)
    const results = await Promise.all(
      ids.map((id) =>
        http
          .get<{ members: MemberItem[] }>(`/admin/orgs/${encodeURIComponent(id)}/members`)
          .then((r) => r.members ?? [])
          .catch(() => [] as MemberItem[]),
      ),
    )
    const map = new Map<string, MemberItem[]>()
    ids.forEach((id, i) => map.set(id, results[i]))
    membersByOrg.value = map
    const names = new Map<string, string>()
    for (const ms of map.values()) {
      for (const m of ms) {
        if (m.displayName) names.set(m.userId, m.displayName)
      }
    }
    memberNames.value = names
  } catch (err) {
    ElMessage.error(err instanceof Error ? `加载组织架构失败：${err.message}` : '加载组织架构失败')
  } finally {
    initialLoading.value = false
    loading.value = false
  }
}

// ---------- 选择（单选高亮） ----------
/** 点击节点 = 选中该对象（部门/用户视角），不展开收起（展开用行首箭头）。 */
function onNodeClick(data: TreeNode) {
  const type = data.kind === 'org' ? 'org' : 'user'
  emit('select', { type, id: data.id })
  emit('update:modelValue', `${type}:${data.id}`)
}

// ---------- 本地过滤（全量节点均生效） ----------
function filterNode(value: string, data: TreeNode): boolean {
  if (!value) return true
  const kwLower = value.toLowerCase()
  return data.name.toLowerCase().includes(kwLower) || data.id.toLowerCase().includes(kwLower)
}
function onFilter() {
  treeRef.value?.filter(kw.value)
}

// 首载挂 sessionReady 门控：iframe 形态下 postMessage 会话未到前 token 为空
//（与各页面同款握手竞态修复）
void ready().then(load)

// 组件常驻挂载场景下外部要求重载：暴露 reload 供父组件（如打开对话框后）调用
/** 按用户名反查显示名（全量加载后所有成员可查；未命中返回空串，
 *  调用方回退展示 userId）。 */
function getMemberName(uid: string): string {
  return memberNames.value.get(uid) ?? ''
}
/** 按部门 id 反查部门名（未加载返回空串）。 */
function getOrgName(id: string): string {
  return orgs.value.find((o) => o.id === id)?.name ?? ''
}

/** 外部回滚高亮（确认对话框拒绝后恢复原选中；传空清空高亮）。 */
function setCurrentKey(key: string) {
  treeRef.value?.setCurrentKey(key || undefined)
}

defineExpose({ reload: load, orgs, getMemberName, getOrgName, setCurrentKey })
</script>

<style scoped>
.org-picker {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 0;
}
.org-picker-search {
  flex: none;
}
.org-picker-tree {
  flex: 1;
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 4px;
}
.org-node {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 13px;
}
.org-node-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.org-node-tag {
  flex: none;
  font-size: 11px;
  line-height: 16px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
  border-radius: 4px;
  padding: 0 4px;
}
.org-node-tag-user {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
</style>
