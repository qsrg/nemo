<template>
  <div class="page">
    <h3 class="page-title">资源共享</h3>

    <!-- 共享授权表单：仅本菜单 authz-shares:manage 可操作（按钮级 fail-closed）。
         后端 /v1/authz/relations 只有幂等写、无删除元组端点——撤销能力后端缺失，
         本页只提供「授权」，不做假撤销（与 Roles 存量分配不回显同类后端缺口，
         如需要可后续加删除端点回填）。 -->
    <h4 class="section-title">共享授权</h4>
    <el-form v-if="canManage" inline label-width="80px" class="grant-form" @submit.prevent="onGrant">
      <el-form-item label="资源类型">
        <el-select v-model="grantType" placeholder="选择已注册类型" class="type-select" v-loading="loadingTypes">
          <el-option v-for="t in types" :key="t.name" :label="t.name" :value="t.name" />
        </el-select>
      </el-form-item>
      <el-form-item label="资源 ID">
        <el-input v-model="grantId" placeholder="如 1" class="id-input" />
      </el-form-item>
      <el-form-item label="关系">
        <el-select v-model="grantRelation" placeholder="选择关系" class="relation-select">
          <el-option v-for="r in grantRelationOptions" :key="r.value" :label="r.label" :value="r.value" />
        </el-select>
      </el-form-item>
      <el-form-item label="目标用户">
        <UserSelect v-model="grantUserId" placeholder="搜索并选择用户" class="user-select" />
      </el-form-item>
      <el-form-item>
        <el-button
          type="primary"
          :disabled="!grantType || !grantId.trim() || !grantRelation || !grantUserId"
          :loading="granting"
          @click="onGrant"
        >
          授权
        </el-button>
      </el-form-item>
    </el-form>
    <p v-else class="readonly-note">当前账号无本菜单管理权限，仅可使用下方检查器。</p>
    <p class="page-note">
      授权后对方下次访问立即可见；可授权的关系由资源类型的配置决定。
      <span class="note-warn">撤销能力后端暂未提供，授权前请确认。</span>
    </p>

    <!-- 我的资源检查器：自助功能（判定面 /v1/authz/check 以当前登录用户为主语），
         不做按钮级 authz-shares:manage 门控 -->
    <h4 class="section-title">我的资源检查器</h4>
    <el-form inline label-width="80px" class="check-form" @submit.prevent="onCheck">
      <el-form-item label="资源类型">
        <el-select v-model="checkType" placeholder="选择已注册类型" class="type-select">
          <el-option v-for="t in types" :key="t.name" :label="t.name" :value="t.name" />
        </el-select>
      </el-form-item>
      <el-form-item label="资源 ID">
        <el-input v-model="checkId" placeholder="如 1" class="id-input" />
      </el-form-item>
      <el-form-item label="关系">
        <el-select v-model="checkRelation" placeholder="选择能力" class="relation-select">
          <el-option v-for="r in checkRelationOptions" :key="r.value" :label="r.label" :value="r.value" />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button
          type="primary"
          plain
          :disabled="!checkType || !checkId.trim() || !checkRelation"
          :loading="checking"
          @click="onCheck"
        >
          检查
        </el-button>
      </el-form-item>
    </el-form>
    <el-alert
      v-if="checkResult !== null"
      :title="checkResult ? '允许（allowed）' : '拒绝（无权或资源不存在判定为 false）'"
      :type="checkResult ? 'success' : 'warning'"
      :closable="false"
      class="check-result"
    />
    <p class="page-note">以当前登录用户为主语：无权限时返回“无权限”，不作为错误。</p>

    <!-- 我的可访问对象：list-objects 自助盘点（同为自助，不做按钮级门控） -->
    <h4 class="section-title">我的可访问对象</h4>
    <el-form inline label-width="80px" class="check-form" @submit.prevent="onListMine">
      <el-form-item label="资源类型">
        <el-select v-model="mineType" placeholder="选择已注册类型" class="type-select">
          <el-option v-for="t in types" :key="t.name" :label="t.name" :value="t.name" />
        </el-select>
      </el-form-item>
      <el-form-item label="基础关系">
        <el-select v-model="mineRelation" placeholder="选择关系" class="relation-select">
          <el-option label="owner（所有）" value="owner" />
          <el-option label="editor（可编辑）" value="editor" />
          <el-option label="viewer（可查看）" value="viewer" />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button
          type="primary"
          plain
          :disabled="!mineType || !mineRelation"
          :loading="listingMine"
          @click="onListMine"
        >
          查询
        </el-button>
      </el-form-item>
    </el-form>
    <div v-if="mineObjects !== null" class="mine-objects">
      <el-tag v-for="obj in mineObjects" :key="obj" size="small" class="mine-tag">{{ obj }}</el-tag>
      <span v-if="mineObjects.length === 0" class="muted">无符合条件的对象</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { hasPermission, UserSelect, sessionReady } from '@nemo/shared'
import { baseProps, showApiError } from '../api/client'
import { checkPermission, listObjects, writeRelations } from '../api/shares'
import { listResourceTypes, type ResourceType } from '../api/types'

/** 按钮级权限：共享授权仅本菜单 authz-shares:manage 可操作（fail-closed）；
 *  检查器/我的对象为自助功能，不门控。 */
const permissions = computed(() => baseProps()?.permissions ?? [])
const canManage = computed(() => hasPermission('authz-shares:manage', permissions.value))

/** 写白名单关系（writerelationslogic.go writableRelations 同源）——页面只提供
 *  合法组合；漏网组合（如类型未勾 can_share 的 sharer）403
 *  AUTHZ_RELATION_FORBIDDEN 由 showApiError 映射「不允许写该关系」。 */
const GRANT_RELATIONS = [
  { value: 'owner', label: 'owner（所有者）' },
  { value: 'editor', label: 'editor（编辑者）' },
  { value: 'viewer', label: 'viewer（查看者）' },
  { value: 'sharer', label: 'sharer（共享者，需类型勾选 can_share）' },
]

/** 检查器能力关系（modelgen can_* 出口固定集同源）。 */
const CHECK_RELATIONS = [
  { value: 'can_view', label: 'can_view（查看）' },
  { value: 'can_edit', label: 'can_edit（编辑）' },
  { value: 'can_delete', label: 'can_delete（删除）' },
  { value: 'can_share', label: 'can_share（共享）' },
]

/** 选中类型的勾选 actions：sharer 只有勾选 can_share 的类型才有；未注册
 *  can_* 出口的关系判定时引擎报错 503 fail-closed——下拉按已注册出口收敛，
 *  避免构造必然失败的组合（类型未加载时展示全集兜底）。 */
function actionsOf(typeName: string): string[] {
  return types.value.find((t) => t.name === typeName)?.actions ?? []
}

// ---------------------------------------------------------------------------
// 响应式状态(集中声明):下方 computed/watch 注册时会立即读取这些 ref,
// 必须先于它们声明,否则 TDZ——"Cannot access 'x' before initialization"
// (本次页面报错根因:重排代码后声明落在 watch 之后)
// ---------------------------------------------------------------------------
const types = ref<ResourceType[]>([])
const loadingTypes = ref(false)
const grantType = ref('')
const grantId = ref('')
const grantRelation = ref('')
const grantUserId = ref('')
const granting = ref(false)
const checkType = ref('')
const checkId = ref('')
const checkRelation = ref('')
const checking = ref(false)
const checkResult = ref<boolean | null>(null)
const mineType = ref('')
const mineRelation = ref('')
const listingMine = ref(false)
const mineObjects = ref<string[] | null>(null)
const grantRelationOptions = computed(() => {
  if (!grantType.value) return GRANT_RELATIONS
  return actionsOf(grantType.value).includes('can_share')
    ? GRANT_RELATIONS
    : GRANT_RELATIONS.filter((r) => r.value !== 'sharer')
})
const checkRelationOptions = computed(() => {
  if (!checkType.value) return CHECK_RELATIONS
  const acts = actionsOf(checkType.value)
  return CHECK_RELATIONS.filter((r) => acts.includes(r.value))
})

// 类型切换后原选中关系可能不在收敛后的选项内：重置避免携带非法组合提交。
watch(grantType, () => {
  grantRelation.value = ''
})
watch(checkType, () => {
  checkRelation.value = ''
  checkResult.value = null
})

// ---------------------------------------------------------------------------
// 已注册类型（共享/检查的对象类型下拉共用）
// ---------------------------------------------------------------------------
async function loadTypes() {
  loadingTypes.value = true
  try {
    const resp = await listResourceTypes()
    types.value = resp.items
  } catch (err) {
    showApiError(err)
  } finally {
    loadingTypes.value = false
  }
}

// ---------------------------------------------------------------------------
// 共享授权（元组 user 字段为 user:{uid}，引擎约定与后端各 logic 同源）
// ---------------------------------------------------------------------------
async function onGrant() {
  if (!grantType.value || !grantId.value.trim() || !grantRelation.value || !grantUserId.value) return
  granting.value = true
  try {
    const resp = await writeRelations([
      {
        user: `user:${grantUserId.value}`,
        relation: grantRelation.value,
        object: `${grantType.value}:${grantId.value.trim()}`,
      },
    ])
    ElMessage.success(`已授权（写入 ${resp.written} 条元组）`)
    grantUserId.value = ''
  } catch (err) {
    // 403 AUTHZ_RELATION_FORBIDDEN → 「不允许写该关系」（关系非白名单/类型未注册/
    // 保留类型）；其余透出后端 message
    showApiError(err)
  } finally {
    granting.value = false
  }
}

// ---------------------------------------------------------------------------
// 我的资源检查器（自助：无权也是 200 {allowed:false}，非错误）
// ---------------------------------------------------------------------------
async function onCheck() {
  if (!checkType.value || !checkId.value.trim() || !checkRelation.value) return
  checking.value = true
  try {
    const resp = await checkPermission({
      object: `${checkType.value}:${checkId.value.trim()}`,
      relation: checkRelation.value,
    })
    checkResult.value = resp.allowed
  } catch (err) {
    // 引擎故障/类型未注册 503 fail-closed（AUTHZ_ENGINE_ERROR）→ 透出
    showApiError(err)
  } finally {
    checking.value = false
  }
}

// ---------------------------------------------------------------------------
// 我的可访问对象（list-objects 自助盘点）
// ---------------------------------------------------------------------------
async function onListMine() {
  if (!mineType.value || !mineRelation.value) return
  listingMine.value = true
  try {
    const resp = await listObjects({ type: mineType.value, relation: mineRelation.value })
    mineObjects.value = resp.objects
  } catch (err) {
    showApiError(err)
  } finally {
    listingMine.value = false
  }
}

onMounted(async () => {
  await sessionReady() // iframe 握手竞态门控
  void loadTypes()
})
</script>

<style scoped>
.page {
  padding: 16px;
}
.page-title {
  margin: 0 0 12px;
  font-size: 16px;
  font-weight: 600;
}
.section-title {
  margin: 16px 0 12px;
  font-size: 14px;
  font-weight: 600;
}
.grant-form .type-select,
.check-form .type-select {
  width: 180px;
}
.grant-form .id-input,
.check-form .id-input {
  width: 140px;
}
.grant-form .relation-select,
.check-form .relation-select {
  width: 240px;
}
.grant-form .user-select {
  width: 240px;
}
.check-result {
  margin-bottom: 8px;
}
.mine-objects {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.mine-tag {
  font-family: monospace;
}
.readonly-note,
.page-note,
.muted {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.note-warn {
  color: var(--el-color-warning);
}
</style>
