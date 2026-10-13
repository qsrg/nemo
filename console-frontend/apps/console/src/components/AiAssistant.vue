<template>
  <!-- AI 助手侧滑抽屉：全局 ⌘K / 顶栏入口唤起；不离开当前页面（保留上下文）。
       演示阶段回复为本地模拟流式；生产接 llm-proxy SSE（架构 §4.3）。 -->
  <el-drawer
    v-model="ai.visible"
    title="AI 助手"
    direction="rtl"
    size="460px"
    :modal="false"
    :append-to-body="true"
    class="ai-drawer"
  >
    <div class="chat">
      <el-scrollbar ref="scrollRef" class="msgs">
        <div v-if="!ai.messages.length" class="empty">
          <el-icon class="empty-icon"><MagicStick /></el-icon>
          <p class="empty-title">问我任何关于控制台的事</p>
          <p class="empty-sub">查数据 · 找页面 · 问操作</p>
          <div class="empty-tips">
            <button v-for="t in samples" :key="t" class="tip" type="button" @click="ai.send(t)">
              {{ t }}
            </button>
          </div>
        </div>

        <div v-for="(m, i) in ai.messages" :key="i" class="msg" :class="m.role">
          <div class="bubble">
            <div class="content">{{ m.content }}<span v-if="m.streaming" class="cursor" /></div>
            <!-- 页面跳转卡片：AI 即导航层 -->
            <div v-if="m.actions?.length" class="actions">
              <button
                v-for="a in m.actions"
                :key="a.path"
                class="action"
                type="button"
                @click="jump(a.path)"
              >
                <el-icon><Right /></el-icon>{{ a.title }}
              </button>
            </div>
          </div>
        </div>
      </el-scrollbar>

      <div class="composer">
        <el-input
          v-model="ai.draft"
          placeholder="问点什么，或让 AI 带你去页面…"
          @keyup.enter="ai.send(ai.draft)"
        />
        <el-button type="primary" @click="ai.send(ai.draft)">发送</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { MagicStick, Right } from '@element-plus/icons-vue'
import { useAiStore } from '../stores/ai'

const ai = useAiStore()
const router = useRouter()
const scrollRef = ref()

const samples = ['张三这个月有几笔退款？', '打开用户中心', '怎么给用户加权限？']

// 新消息自动滚到底
watch(
  () => ai.messages.length,
  async () => {
    await nextTick()
    scrollRef.value?.setScrollTop?.(99999)
  },
)

function jump(path: string) {
  ai.visible = false
  router.push(path)
}
</script>

<style scoped>
.chat { display: flex; flex-direction: column; height: 100%; }
.msgs { flex: 1; }
.empty { text-align: center; padding: 48px 16px; }
.empty-icon { font-size: 34px; color: var(--el-color-primary); }
.empty-title { margin: 14px 0 4px; font-size: 15px; font-weight: 600; }
.empty-sub { margin: 0 0 20px; font-size: 13px; color: var(--el-text-color-secondary); }
.empty-tips { display: flex; flex-direction: column; gap: 8px; align-items: center; }
.tip {
  padding: 8px 14px; border: 1px solid var(--el-border-color-lighter); border-radius: 999px;
  background: var(--el-bg-color); font-size: 13px; cursor: pointer; color: var(--el-text-color-regular);
}
.tip:hover { border-color: var(--el-color-primary-light-5); color: var(--el-color-primary); }

.msg { display: flex; margin-bottom: 12px; }
.msg.user { justify-content: flex-end; }
.msg.user .bubble {
  background: var(--el-color-primary); color: #fff; border-radius: 12px 12px 2px 12px;
}
.msg.assistant .bubble {
  background: var(--el-fill-color-light); color: var(--el-text-color-primary);
  border-radius: 12px 12px 12px 2px;
}
.bubble { max-width: 88%; padding: 10px 12px; font-size: 13px; line-height: 1.6; }
.content { white-space: pre-wrap; word-break: break-word; }
.cursor { display: inline-block; width: 2px; height: 14px; background: var(--el-text-color-secondary); margin-left: 2px; animation: blink 0.8s step-end infinite; vertical-align: -2px; }
@keyframes blink { 50% { opacity: 0; } }
.actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
.action {
  display: inline-flex; align-items: center; gap: 4px; padding: 6px 12px;
  border: 1px solid var(--el-color-primary-light-7); border-radius: 8px;
  background: var(--el-bg-color); color: var(--el-color-primary);
  font-size: 12px; cursor: pointer;
}
.action:hover { background: var(--el-color-primary-light-9); }

.composer { display: flex; gap: 8px; padding-top: 12px; border-top: 1px solid var(--el-border-color-lighter); }
.composer .el-input { flex: 1; }
</style>
