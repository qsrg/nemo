<template>
  <div class="page">
    <h3 class="page-title">部门与成员</h3>

    <!-- 组织事实源在 CAS：本页为只读视图，手工维护入口已下线，避免与
         CAS 同步对账冲突（CAS 已移除的组织会被清理或告警） -->
    <p class="page-note">
      组织架构与成员归属由 CAS 统一维护，本页为只读视图；如需调整请在 CAS 侧操作，同步后生效。
      <el-button v-if="canManage" link type="primary" :loading="syncing" @click="onSyncFromCas">
        立即同步
      </el-button>
    </p>

    <!-- 组织架构树：部门节点下直接挂直接成员（用户叶子），与权限配置（ResourceGrantDialog）
         同款"部门+用户"混合树——层级与归属一眼可见，无需点击抽屉。 -->
    <div class="tree-wrap" v-loading="loading">
      <el-tree
        :data="treeData"
        node-key="key"
        :props="{ label: 'name', children: 'children' }"
        highlight-current
        default-expand-all
        :expand-on-click-node="false"
        empty-text="暂无组织，请确认 CAS 同步已执行"
      >
        <template #default="{ data }">
          <!-- 类型靠前缀图标区分：部门=实心方块（加粗），用户=小圆点（常规），不挂文字标签 -->
          <span class="org-node" :class="data.kind === 'org' ? 'org-node--org' : 'org-node--user'">
            <span class="org-node-glyph"></span>
            <span class="org-node-name" :title="data.name">{{ data.name }}</span>
          </span>
        </template>
      </el-tree>
    </div>

    <p class="tree-note">
      树中用户为对应部门的直接成员；授权视角下，给某个部门授权时，该部门及其全部子部门的人员均获得权限。
      成员归属由 CAS 维护，如需调整请在 CAS 侧操作。
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { hasPermission, sessionReady } from '@nemo/shared'
import { baseProps, showApiError } from '../api/client'
import { listOrgMembers, listOrgs, syncOrgs, type Org } from '../api/orgs'
import { fetchUserNames, fetchOrgNames } from '../api/directory'

/** 树节点：部门节点（kind=org，children 含子部门与用户叶子）或用户叶子（kind=user）。
 *  key 为复合唯一键（org:{id} / user:{userId}），与 ResourceGrantDialog 同构。 */
interface TreeNode {
  key: string
  name: string
  kind: 'org' | 'user'
  orgId?: string
  children?: TreeNode[]
}

/** 按钮级权限：手动同步是写路径（审计 + 引擎写），仅 authz-orgs:manage 可见（fail-closed）。 */
const permissions = computed(() => baseProps()?.permissions ?? [])
const canManage = computed(() => hasPermission('authz-orgs:manage', permissions.value))

const loading = ref(false)
/** 扁平组织列表（后端 id 升序全量）。 */
const orgs = ref<Org[]>([])
/** 各部门直接成员 userId 列表（orgId → userId[]），构建混合树用。 */
const membersByOrg = ref<Map<string, string[]>>(new Map())

/**
 * 组织名映射（orgId → 可读名）：CAS 目录组织架构信息聚合而来，树节点
 * 优先用它，回退后端 name（组织名称权威展示源在目录侧）。
 */
const orgNameMap = ref<Record<string, string>>({})

/** 裸 userId → 展示名（用户节点渲染用，fetchUserNames 带会话级缓存）。 */
const nameMap = ref<Record<string, string>>({})

/** 组织可读名：后端 name（注册表，含管理面改名后的新名）优先；仅当后端
 * name 是裸 ID（引擎可见未注册组织的 ID 回退）时，用 CAS 目录组织名兜底。 */
function orgDisplayName(o: { id: string; name: string } | null | undefined): string {
  if (!o) return ''
  if (o.name && o.name !== o.id) return o.name
  return orgNameMap.value[o.id] ?? o.name
}

/** 裸 userId 的展示名（反查缓存命中显示姓名，否则回退裸 ID）。 */
function memberName(userId: string): string {
  return nameMap.value[userId] ?? userId
}

/** 扁平组织列表 + 各部门直接成员 → "部门→用户"混合树。
 *  成员取自引擎 org:xxx#member 元组（CAS 同步的权威组织归属）；
 *  parentId 空或父组织不在列表（防孤儿挂死）一律作根渲染。 */
const treeData = computed<TreeNode[]>(() => {
  const byId = new Map<string, Org>()
  for (const o of orgs.value) byId.set(o.id, { ...o })
  const nodeOf = new Map<string, TreeNode>()
  const ensureNode = (o: Org): TreeNode => {
    let n = nodeOf.get(o.id)
    if (!n) {
      n = { key: `org:${o.id}`, name: orgDisplayName(o), kind: 'org', children: [] }
      nodeOf.set(o.id, n)
    }
    return n
  }
  const roots: TreeNode[] = []
  for (const o of orgs.value) {
    const node = ensureNode(o)
    const members = membersByOrg.value.get(o.id) ?? []
    for (const uid of members) {
      node.children!.push({ key: `user:${uid}`, name: memberName(uid), kind: 'user', orgId: o.id })
    }
    const p = o.parentId ? byId.get(o.parentId) : undefined
    if (p) ensureNode(p).children!.push(node)
    else roots.push(node)
  }
  return roots
})

async function loadOrgs() {
  loading.value = true
  try {
    const resp = await listOrgs()
    orgs.value = resp.items
    // 并行拉各部门直接成员（部门数量少，直连最简——与 ResourceGrantDialog 同模式）。
    const ids = orgs.value.map((o) => o.id)
    const results = await Promise.all(
      ids.map((id) =>
        listOrgMembers(id)
          .then((r) => r.members.map((m) => m.userId))
          .catch(() => [] as string[]),
      ),
    )
    const map = new Map<string, string[]>()
    ids.forEach((id, i) => map.set(id, results[i]))
    membersByOrg.value = map
    // 批量反查所有用户名（展示增强：失败静默，用户节点回退裸 ID）。
    const allUserIds = [...new Set(results.flat())]
    if (allUserIds.length) {
      try {
        const names = await fetchUserNames(allUserIds)
        nameMap.value = { ...nameMap.value, ...names }
      } catch {
        /* 姓名不可得时保留裸 userId，不阻断页面 */
      }
    }
  } catch (err) {
    showApiError(err)
  } finally {
    loading.value = false
  }
  // 组织名映射独立异步加载（展示增强：失败静默，页面回退后端 name）。
  fetchOrgNames()
    .then((m) => {
      orgNameMap.value = m
    })
    .catch(() => {
      /* 组织名不可得时保留后端 name（可能为裸 id），不阻断页面 */
    })
}

// ---------------------------------------------------------------------------
// CAS 组织同步（手动触发）：组织事实源在 CAS，本页只读展示；同步为全量
// 对账（幂等），与后端定时任务同入口，同步后刷新树以呈现收敛结果。
// ---------------------------------------------------------------------------
const syncing = ref(false)

async function onSyncFromCas() {
  syncing.value = true
  try {
    const r = await syncOrgs()
    const changed =
      r.parentsWritten + r.parentsDeleted + r.membersWritten + r.membersDeleted + r.orgsDeleted
    if (r.orgsSkipped > 0 || r.danglingUsers > 0) {
      // 有需人工处理项（待删组织仍有授权引用 / 用户归属悬空）：
      // 后端已跳过并告警，明细见服务日志。
      ElMessage.warning(
        `同步完成（${changed} 项写入）；另有 ${r.orgsSkipped} 个待删组织因仍有授权被跳过、${r.danglingUsers} 个用户因归属悬空被跳过，详见服务日志`,
      )
    } else if (changed === 0) {
      ElMessage.success('组织数据已与 CAS 一致')
    } else {
      ElMessage.success(
        `同步完成：层级关系 写${r.parentsWritten}/删${r.parentsDeleted}，成员关系 写${r.membersWritten}/删${r.membersDeleted}，删除组织 ${r.orgsDeleted} 个`,
      )
    }
    await loadOrgs()
  } catch (err) {
    showApiError(err)
  } finally {
    syncing.value = false
  }
}

onMounted(async () => {
  await sessionReady() // iframe 握手竞态门控:token 到位后再发首个请求
  void loadOrgs()
})
</script>

<style scoped>
.page {
  padding: 16px;
}
.page-title {
  margin: 0 0 8px;
  font-size: 16px;
  font-weight: 600;
}
.page-note {
  margin: 0 0 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.8;
}
/* 树面板：带边框、限高内部滚动，避免全展开后撑长页面 */
.tree-wrap {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 8px;
  max-height: calc(100vh - 240px);
  overflow: auto;
}
.org-node {
  display: flex;
  align-items: center;
  gap: 6px;
}
/* 前缀图标：部门实心方块（主色浅底，像文件夹缩略）；用户小圆点（灰） */
.org-node-glyph {
  flex-shrink: 0;
  width: 12px;
  height: 12px;
  border-radius: 2px;
  background: var(--el-color-primary-light-5);
}
.org-node--user .org-node-glyph {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--el-text-color-placeholder);
  margin: 0 2.5px;
}
.org-node-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 部门名加粗，用户名常规——叠加图标进一步强化层级 */
.org-node--org .org-node-name {
  font-weight: 600;
}
.tree-note {
  margin-top: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
</style>
