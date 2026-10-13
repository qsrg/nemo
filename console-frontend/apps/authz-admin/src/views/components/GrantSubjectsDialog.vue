/**
 * 菜单授权对象弹窗（反向视图，评审问题 5）：
 * 按菜单（readCode/manageCode）反向查询直接授权主体——用户或组织。
 * 数据：GET /admin/permissions/subjects?permission=（管理面审计接口）。
 * 展示：可查看/可管理两组；用户经目录反查显示姓名；组织经注册表显示组织名。
 * 2026-09-21 优化：多主体时平铺一长条改为 tag 流式（用户/部门同排紧凑展示），
 * 部门成员改 popover 展开，超过 12 个主体默认折叠（+N 展开），去掉冗余 id 展示。
 * 仅展示直接授权主体（org#member 继承的成员不在此展开，点部门 tag 查看成员）。
 */
<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { showApiError } from '../../api/client'
import { listPermissionSubjects } from '../../api/permissions'
import { listOrgMembers } from '../../api/orgs'
import { fetchUserNames } from '../../api/directory'
import type { Org } from '../../api/orgs'

const props = defineProps<{
  visible: boolean
  /** 菜单展示名（对话框标题用）。 */
  menuTitle: string
  readCode: string
  manageCode: string
  /** 组织注册表（org id → 名翻译用）。 */
  orgs: Org[]
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

/** 授权主体（user|org），含展示名与成员（部门主体 popover 用）。 */
interface SubjectRow {
  kind: 'user' | 'org'
  id: string
  name: string
  /** org 主体展开成员后填充。 */
  members?: { userId: string; name: string }[]
  loading?: boolean
}

/** 每组默认展示上限，超出折叠为 +N。 */
const MAX_SHOWN = 12

const readRows = ref<SubjectRow[]>([])
const manageRows = ref<SubjectRow[]>([])
const loading = ref(false)
const readExpanded = ref(false)
const manageExpanded = ref(false)

/** 把 subjects 归一化为带展示名的行（用户反查姓名，组织查注册表名）。 */
async function normalize(
  subjects: { kind: 'user' | 'org'; id: string }[],
  rows: SubjectRow[],
) {
  rows.length = 0
  if (!subjects.length) return
  const users = subjects.filter((s) => s.kind === 'user').map((s) => s.id)
  const names = await fetchUserNames(users)
  const orgNames = new Map(props.orgs.map((o) => [o.id, o.name]))
  for (const s of subjects) {
    rows.push({
      kind: s.kind,
      id: s.id,
      name: s.kind === 'user' ? names[s.id] ?? s.id : orgNames.get(s.id) ?? s.id,
    })
  }
}

async function load() {
  loading.value = true
  readExpanded.value = false
  manageExpanded.value = false
  try {
    const [r, m] = await Promise.all([
      listPermissionSubjects(props.readCode),
      listPermissionSubjects(props.manageCode),
    ])
    await Promise.all([normalize(r.subjects, readRows.value), normalize(m.subjects, manageRows.value)])
  } catch (err) {
    showApiError(err)
  } finally {
    loading.value = false
  }
}

/** 部门主体：首次点击 popover 时加载成员并反查姓名。 */
async function ensureMembers(row: SubjectRow) {
  if (row.members) return
  row.loading = true
  try {
    const resp = await listOrgMembers(row.id)
    const ids = resp.members.map((m) => m.userId)
    const names = await fetchUserNames(ids)
    row.members = resp.members.map((m) => ({ userId: m.userId, name: names[m.userId] ?? m.userId }))
  } catch (err) {
    showApiError(err)
  } finally {
    row.loading = false
  }
}

/** 当前可见行：未折叠时全部，折叠时前 MAX_SHOWN 个。 */
function visibleRows(rows: SubjectRow[], expanded: boolean) {
  return expanded || rows.length <= MAX_SHOWN ? rows : rows.slice(0, MAX_SHOWN)
}

/** 折叠隐藏的数量（>0 才显示 +N）。 */
function hiddenCount(rows: SubjectRow[]) {
  return rows.length > MAX_SHOWN ? rows.length - MAX_SHOWN : 0
}

watch(
  () => props.visible,
  (v) => {
    if (v) void load()
  },
)

onMounted(() => {
  if (props.visible) void load()
})
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="`「${menuTitle}」的授权对象`"
    width="560px"
    append-to-body
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <div v-loading="loading" class="subj-body">
      <section class="grp">
        <h4 class="grp-title">可查看权限<span v-if="readRows.length" class="grp-count">{{ readRows.length }}</span></h4>
        <div v-if="!readRows.length" class="empty">无直接授权对象</div>
        <div v-else class="tag-wrap">
          <template v-for="row in visibleRows(readRows, readExpanded)" :key="'r' + row.kind + row.id">
            <!-- 用户主体：el-tooltip 包 el-tag（悬停显示 ID），单根元素 -->
            <el-tooltip v-if="row.kind === 'user'" :content="row.id" placement="top">
              <el-tag type="primary" size="small" class="subj-tag" effect="light">{{ row.name }}</el-tag>
            </el-tooltip>
            <!-- 部门主体：el-popover 包 el-tag（点击查看成员），单根元素 -->
            <el-popover
              v-else
              trigger="click"
              :width="260"
              placement="bottom"
              @show="ensureMembers(row)"
            >
              <template #reference>
                <el-tag type="warning" size="small" class="subj-tag" effect="plain">{{ row.name }}</el-tag>
              </template>
              <div v-loading="row.loading" class="member-pop">
                <div v-if="row.members?.length" v-for="m in row.members" :key="m.userId" class="member-item">
                  {{ m.name }}<span class="sub"> · {{ m.userId }}</span>
                </div>
                <div v-else class="empty">{{ row.loading ? '加载中…' : '该部门无成员' }}</div>
              </div>
            </el-popover>
          </template>
          <el-tag v-if="hiddenCount(readRows)" class="more-tag" size="small" @click="readExpanded = true">
            +{{ hiddenCount(readRows) }} 个，展开
          </el-tag>
          <el-button v-else-if="readExpanded && readRows.length > MAX_SHOWN" link type="primary" size="small" @click="readExpanded = false">
            收起
          </el-button>
        </div>
      </section>
      <section class="grp">
        <h4 class="grp-title">可管理权限<span v-if="manageRows.length" class="grp-count">{{ manageRows.length }}</span></h4>
        <div v-if="!manageRows.length" class="empty">无直接授权对象</div>
        <div v-else class="tag-wrap">
          <template v-for="row in visibleRows(manageRows, manageExpanded)" :key="'m' + row.kind + row.id">
            <!-- 用户主体：el-tooltip 包 el-tag（悬停显示 ID），单根元素 -->
            <el-tooltip v-if="row.kind === 'user'" :content="row.id" placement="top">
              <el-tag type="primary" size="small" class="subj-tag" effect="light">{{ row.name }}</el-tag>
            </el-tooltip>
            <!-- 部门主体：el-popover 包 el-tag（点击查看成员），单根元素 -->
            <el-popover
              v-else
              trigger="click"
              :width="260"
              placement="bottom"
              @show="ensureMembers(row)"
            >
              <template #reference>
                <el-tag type="warning" size="small" class="subj-tag" effect="plain">{{ row.name }}</el-tag>
              </template>
              <div v-loading="row.loading" class="member-pop">
                <div v-if="row.members?.length" v-for="m in row.members" :key="m.userId" class="member-item">
                  {{ m.name }}<span class="sub"> · {{ m.userId }}</span>
                </div>
                <div v-else class="empty">{{ row.loading ? '加载中…' : '该部门无成员' }}</div>
              </div>
            </el-popover>
          </template>
          <el-tag v-if="hiddenCount(manageRows)" class="more-tag" size="small" @click="manageExpanded = true">
            +{{ hiddenCount(manageRows) }} 个，展开
          </el-tag>
          <el-button v-else-if="manageExpanded && manageRows.length > MAX_SHOWN" link type="primary" size="small" @click="manageExpanded = false">
            收起
          </el-button>
        </div>
      </section>
      <p class="hint">蓝色为直接授权用户（悬停显示 ID）；黄色为部门，点击查看成员。仅列出直接授权对象；部门授权对其成员（含上级部门继承）即时生效。</p>
    </div>
    <template #footer>
      <el-button @click="emit('update:visible', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.subj-body { min-height: 120px; }
.grp { margin-bottom: 14px; }
.grp-title { margin: 0 0 8px; font-size: 13px; color: var(--el-text-color-secondary); }
.grp-count { margin-left: 6px; font-size: 12px; color: var(--el-text-color-placeholder); }
.empty { font-size: 13px; color: var(--el-text-color-placeholder); padding: 4px 0; }
.tag-wrap { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.subj-tag { cursor: default; }
.more-tag { cursor: pointer; color: var(--el-color-primary); background: var(--el-color-primary-light-9); border-color: var(--el-color-primary-light-5); }
.member-pop { max-height: 240px; overflow: auto; }
.member-item { padding: 3px 0; font-size: 13px; color: var(--el-text-color-regular); }
.sub { font-size: 12px; color: var(--el-text-color-secondary); font-family: ui-monospace, 'SF Mono', Menlo, monospace; }
.hint { font-size: 12px; color: var(--el-text-color-secondary); margin: 8px 0 0; }
</style>
