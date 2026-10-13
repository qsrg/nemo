<template>
  <div class="page">
    <header class="page-head">
      <div class="head-left">
        <h3 class="page-title">节点浏览</h3>
        <!-- 可见范围说明：集群授权驱动——看到集群才能浏览其下节点 -->
        <el-tooltip content="可见范围 = 您被授权（owner/被授予可查看）的 ZK 集群；持管理码时全部可见。集群授权即全量节点只读浏览" placement="top">
          <span class="demo-mark">集群授权</span>
        </el-tooltip>
      </div>
      <!-- 权限身份胶囊（右对齐）：当前登录者在本页的能力档位，与按钮显隐同一判定源 -->
      <span v-if="canManage" class="perm-pill grant">可管理 · 集群增删/授权名单</span>
      <span v-else class="perm-pill readonly">可查看</span>
    </header>

    <!-- 集群级联：先选集群再浏览节点。集群进引擎（zk_cluster）：列表按引擎
         can_view 过滤（manage 码全量），新建/删除挂 zookeeper:manage 码，
         「集群权限」入口 = owner 或 manage 码（行级徽标驱动）。 -->
    <div class="cluster-bar">
      <span class="cluster-label">集群</span>
      <el-select
        v-model="currentClusterId"
        class="cluster-select"
        placeholder="选择集群"
        :loading="clustersLoading"
        @change="onClusterChange"
      >
        <el-option v-for="c in clusters" :key="c.id" :value="c.id" :label="clusterLabel(c)" />
      </el-select>
      <template v-if="canManage">
        <el-button @click="openCreateCluster">新建集群</el-button>
        <el-button v-if="currentClusterId" type="danger" plain @click="onDeleteCluster">删除集群</el-button>
      </template>
      <el-button v-if="currentClusterCanManage" link type="primary" @click="clusterGrantVisible = true">集群权限</el-button>
      <span class="cluster-hint">集群授权即全量节点只读浏览（阶段一：演示数据）</span>
    </div>

    <section class="browse-card">
      <!-- 空态分两态：无集群（引导先建集群）与正常浏览 -->
      <template v-if="!clusters.length">
        <el-empty :image-size="72" description="暂无集群">
          <p class="empty-note">{{ canManage ? '先创建集群，授权后即可浏览该集群下的节点' : '请先联系管理员创建集群' }}</p>
        </el-empty>
      </template>
      <template v-else>
        <div class="browse-layout">
          <!-- 左：节点树（全树一次拉取，mock 演示结构；阶段二接真 ZK 不改交互） -->
          <div class="tree-pane" v-loading="loading">
            <div class="pane-head">节点树</div>
            <el-tree
              v-if="treeData.length"
              :data="treeData"
              node-key="path"
              default-expand-all
              :props="{ label: 'name', children: 'children' }"
              class="znode-tree"
              @node-click="onNodeClick"
            >
              <template #default="{ data }">
                <span class="znode-node" :class="{ selected: selected?.path === data.path }">
                  <span class="znode-icon">{{ data.children?.length ? '📁' : '📄' }}</span>
                  <span class="znode-name">{{ data.name }}</span>
                  <span v-if="data.children?.length" class="znode-count">{{ data.children.length }}</span>
                </span>
              </template>
            </el-tree>
            <el-empty v-else-if="!loading" :image-size="64" description="该集群暂无节点" />
          </div>
          <!-- 右：节点详情（path/数据/子节点数） -->
          <div class="detail-pane">
            <div class="pane-head">节点详情</div>
            <template v-if="selected">
              <div class="detail-row"><span class="detail-label">路径</span><code class="detail-value">{{ selected.path }}</code></div>
              <div class="detail-row"><span class="detail-label">子节点</span><span class="detail-value">{{ selected.children?.length ?? 0 }}</span></div>
              <div class="detail-row detail-data">
                <span class="detail-label">数据</span>
                <pre class="detail-pre">{{ selected.data || '（空）' }}</pre>
              </div>
            </template>
            <el-empty v-else :image-size="64" description="点击左侧节点查看详情" />
          </div>
        </div>
      </template>
    </section>

    <!-- 新建集群对话框（zookeeper:manage 持有者）：code 唯一且创建后不可改。 -->
    <el-dialog v-model="clusterDialogVisible" title="新建集群" width="480px">
      <el-form label-width="104px" @submit.prevent="onSubmitCluster">
        <el-form-item label="集群编码" required>
          <el-input v-model="clusterForm.code" maxlength="64" placeholder="如 zk-prod（唯一，创建后不可改）" />
        </el-form-item>
        <el-form-item label="连接串">
          <el-input v-model="clusterForm.hosts" maxlength="512" placeholder="host:port 逗号分隔，如 10.0.0.1:2181,10.0.0.2:2181" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="clusterForm.remark" maxlength="512" placeholder="用途 / 机房等说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="clusterDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="clusterSubmitting" @click="onSubmitCluster">创建</el-button>
      </template>
    </el-dialog>

    <!-- 集群权限对话框：通用资源授权组件——资源类型 zk_cluster + viewer 单关系
         （节点浏览只读，viewer 即全量浏览权）。manage 码持有者或集群 owner
         可配置；后端 relations 按注册表 manage_code + owner 关系校验。 -->
    <ResourceGrantDialog
      v-model="clusterGrantVisible"
      resource-type="zk_cluster"
      :resource-id="currentClusterId"
      :resource-name="currentClusterCode"
      manageable
      :title="`集群权限：${currentClusterCode}`"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ResourceGrantDialog, useApiError, usePermission, useSession } from '@nemo/shared'
import { createCluster, deleteCluster, listClusters, listNodes, type ZkCluster, type Znode } from '../api/zookeeper'

/** 菜单管理码 base：根行 permissionCode=zookeeper:read 控菜单可见（页面能进来
 *  即已授），manage（zookeeper:manage）控"管理员全权"（建删集群/授权名单）。 */
const SERVICE = 'zookeeper'

/** 当前用户是否持有 manage 码（基座 session.permissions 同步，实时刷新）。
 *  usePermission.canManage 是 (service)=>boolean 函数，须包 computed 才能供
 *  模板 v-if 使用（裸函数恒 truthy）。 */
const { canManage: hasManageCode } = usePermission()
const canManage = computed(() => hasManageCode(SERVICE))

/** 统一错误提示（403 → 无权限提示，其余透出后端 message）。 */
const handleApiError = useApiError()

// ---------------------------------------------------------------------------
// 集群级联（先选集群再浏览节点）：集群进引擎（zk_cluster），列表按引擎
// can_view 过滤（manage 码全量）；节点树接口按同一口径守卫（不可见 403）。
// ---------------------------------------------------------------------------
const clusters = ref<ZkCluster[]>([])
const clustersLoading = ref(false)
/** 当前选中集群（空串 = 未选：集群列表为空时，空态引导建集群）。 */
const currentClusterId = ref('')
/** 当前集群展示 label（code + 备注），选择器与集群权限对话框共用。 */
function clusterLabel(c: ZkCluster): string {
  return c.remark ? `${c.code} · ${c.remark}` : c.code
}
const currentClusterCode = computed(() =>
  clusters.value.find((c) => c.id === currentClusterId.value)?.code ?? '',
)
/** 当前集群是否可配置授权名单（manage 码或集群 owner——行级徽标直接消费）。 */
const currentClusterCanManage = computed(() =>
  clusters.value.find((c) => c.id === currentClusterId.value)?.canManageGrants ?? false,
)

async function loadClusters() {
  clustersLoading.value = true
  try {
    const resp = await listClusters()
    clusters.value = resp.items
    // 选中态自愈：默认选第一个；已选集群被删（或不存在）时回落到第一个。
    if (!clusters.value.some((c) => c.id === currentClusterId.value)) {
      currentClusterId.value = clusters.value[0]?.id ?? ''
    }
  } catch (err) {
    handleApiError(err)
  } finally {
    clustersLoading.value = false
  }
}

/** 切换集群：重新拉取该集群的节点树并清空选中态。 */
function onClusterChange() {
  selected.value = null
  void loadNodes()
}

// ---------------------------------------------------------------------------
// 节点树（只读浏览）：全树一次拉取（阶段一 mock 演示树；阶段二接真 ZK）。
// ---------------------------------------------------------------------------
const loading = ref(false)
const treeData = ref<Znode[]>([])
const selected = ref<Znode | null>(null)

async function loadNodes() {
  if (!currentClusterId.value) {
    treeData.value = []
    return
  }
  loading.value = true
  try {
    const resp = await listNodes(currentClusterId.value)
    treeData.value = [resp.root]
    selected.value = null
  } catch (err) {
    treeData.value = []
    handleApiError(err)
  } finally {
    loading.value = false
  }
}

/** 选中节点：右侧详情面板展示 path/数据/子节点数。 */
function onNodeClick(data: Znode) {
  selected.value = data
}

// ---------------------------------------------------------------------------
// 集群管理（新建/删除，zookeeper:manage 持有者）：code 唯一且创建后不可改。
// ---------------------------------------------------------------------------
const clusterDialogVisible = ref(false)
const clusterSubmitting = ref(false)
const clusterForm = reactive({ code: '', hosts: '', remark: '' })

function openCreateCluster() {
  Object.assign(clusterForm, { code: '', hosts: '', remark: '' })
  clusterDialogVisible.value = true
}

async function onSubmitCluster() {
  const code = clusterForm.code.trim()
  if (!code) {
    ElMessage.warning('请输入集群编码')
    return
  }
  clusterSubmitting.value = true
  try {
    const created = await createCluster({ code, hosts: clusterForm.hosts.trim(), remark: clusterForm.remark.trim() })
    ElMessage.success(`集群「${code}」已创建，授权后即可浏览节点`)
    clusterDialogVisible.value = false
    await loadClusters()
    // 建完即切到新集群（创建人即 owner，天然可见）。
    currentClusterId.value = created.id
    onClusterChange()
  } catch (err) {
    handleApiError(err)
  } finally {
    clusterSubmitting.value = false
  }
}

async function onDeleteCluster() {
  const target = clusters.value.find((c) => c.id === currentClusterId.value)
  if (!target) return
  try {
    await ElMessageBox.confirm(
      `确定删除集群「${target.code}」？其授权关系将一并移除。`,
      '删除集群',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消
  }
  try {
    await deleteCluster(target.id)
    ElMessage.success('集群已删除')
    await loadClusters() // 选中态在 loadClusters 内回落到第一个
    onClusterChange()
  } catch (err) {
    handleApiError(err)
  }
}

/** 集群权限对话框显隐（单资源模式：zk_cluster + viewer 单关系）。 */
const clusterGrantVisible = ref(false)

const { ready } = useSession()

onMounted(async () => {
  await ready() // iframe 握手竞态门控:token 到位后再发首个请求
  // 先加载集群并默认选中第一个，再拉该集群的节点树（浏览依赖集群上下文）。
  await loadClusters()
  void loadNodes()
})
</script>

<style scoped>
/* =====================================================================
   视觉层：全部使用 Element Plus 语义变量着色（明暗主题自动适配），
   页面骨架与 apps/redis 同款（页头/工具条/卡片/空态）。
   ===================================================================== */
.page {
  padding: 16px 18px 20px;
}

/* 进场编排：标题 → 工具条 → 卡片 依次上浮（尊重减弱动效偏好）。 */
@media (prefers-reduced-motion: no-preference) {
  .page-head,
  .cluster-bar,
  .browse-card {
    animation: rise 0.38s cubic-bezier(0.22, 0.61, 0.36, 1) both;
  }
  .cluster-bar { animation-delay: 0.05s; }
  .browse-card { animation-delay: 0.1s; }
}
@keyframes rise {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: none; }
}

/* ---- 页头：左（标题 + 授权标记）右（权限身份胶囊）---- */
.page-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}
.head-left {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-right: auto;
}
.page-title {
  position: relative;
  margin: 0;
  padding-left: 12px;
  font-size: 17px;
  font-weight: 600;
}
.page-title::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 4px;
  height: 16px;
  border-radius: 2px;
  background: var(--el-color-primary);
}
.demo-mark {
  cursor: help;
  font-size: 12px;
  line-height: 1;
  padding: 4px 8px;
  border-radius: 4px;
  color: var(--el-color-info);
  border: 1px solid var(--el-border-color);
  background: var(--el-fill-color-light);
}
.perm-pill {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 999px;
  border: 1px solid transparent;
}
.perm-pill.grant {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary-light-7);
  background: var(--el-color-primary-light-9);
}
.perm-pill.readonly {
  color: var(--el-text-color-secondary);
  border-color: var(--el-border-color);
  background: var(--el-fill-color-light);
}

/* ---- 集群工具条（先选集群再浏览节点的级联入口） ---- */
.cluster-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.cluster-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  flex-shrink: 0;
}
.cluster-select {
  width: 260px;
}
.cluster-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* ---- 浏览卡片：左树右详情双栏 ---- */
.browse-card {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  overflow: hidden;
  background: var(--el-bg-color);
}
.browse-layout {
  display: grid;
  grid-template-columns: minmax(260px, 5fr) minmax(280px, 4fr);
  min-height: 420px;
}
.tree-pane,
.detail-pane {
  padding: 12px;
}
.tree-pane {
  border-right: 1px solid var(--el-border-color-lighter);
}
.pane-head {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin-bottom: 8px;
}
.znode-tree {
  max-height: 520px;
  overflow-y: auto;
}
.znode-node {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.znode-icon {
  font-size: 12px;
}
.znode-name {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 13px;
}
.znode-count {
  font-size: 11px;
  line-height: 16px;
  padding: 0 6px;
  border-radius: 999px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color);
}
.detail-row {
  display: flex;
  gap: 10px;
  margin-bottom: 10px;
  align-items: baseline;
}
.detail-label {
  flex-shrink: 0;
  width: 48px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.detail-value {
  font-size: 13px;
  color: var(--el-text-color-regular);
  word-break: break-all;
}
.detail-data {
  flex-direction: column;
  gap: 6px;
}
.detail-pre {
  margin: 0;
  padding: 10px;
  font-size: 12px;
  line-height: 1.6;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-lighter);
  white-space: pre-wrap;
  word-break: break-all;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  max-height: 320px;
  overflow-y: auto;
}
.empty-note {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
