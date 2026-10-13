// AI 助手状态：会话与入口开关。
// 演示阶段回复为本地模拟（流式打字 + 页面跳转卡片）；
// 生产接 llm-proxy（架构 §4.3）SSE 流式，openapi 契约转 function calling 工具。
import { defineStore } from 'pinia'
import { SERVICES } from '../menus'

export interface JumpAction {
  title: string
  path: string
}

export interface ChatMsg {
  role: 'user' | 'assistant'
  content: string
  /** 回答附带的页面跳转卡片（AI 作为导航层） */
  actions?: JumpAction[]
  streaming?: boolean
}

export const useAiStore = defineStore('ai', {
  state: () => ({
    visible: false,
    messages: [] as ChatMsg[],
    draft: '',
  }),
  actions: {
    open() {
      this.visible = true
    },
    toggle() {
      this.visible = !this.visible
    },
    /** 从首页 hero 或其他入口带着问题打开 */
    openWith(question: string) {
      this.visible = true
      this.send(question)
    },
    send(text: string) {
      const q = text.trim()
      if (!q) return
      this.messages.push({ role: 'user', content: q })
      this.draft = ''

      const reply = this.demoReply(q)
      const msg: ChatMsg = { role: 'assistant', content: '', streaming: true }
      this.messages.push(msg)
      // 演示流式：逐字吐出；生产为 llm-proxy SSE 增量
      let i = 0
      const timer = window.setInterval(() => {
        i += 2
        msg.content = reply.text.slice(0, i)
        if (i >= reply.text.length) {
          window.clearInterval(timer)
          msg.streaming = false
          if (reply.actions?.length) msg.actions = reply.actions
        }
      }, 25)
    },
    /** 演示回复规则：命中最常见三类意图（跳页面 / 跨服务查数 / 操作指南） */
    demoReply(q: string): { text: string; actions?: JumpAction[] } {
      // 1. 让 AI 打开某服务：命中服务名
      const svc = SERVICES.find((s) => q.includes(s.title) || q.includes(s.key))
      if (svc && /打开|去|进入|跳/.test(q)) {
        return {
          text: `好的，「${svc.title}」在 ${svc.domain}，页面给你准备好了：`,
          actions: [{ title: '打开 ' + svc.title, path: '/' + svc.key }],
        }
      }
      // 2. 查数据：常见业务关键词 → 给直达页 + 参数建议
      if (/退款/.test(q)) {
        return {
          text: '退款数据在退款审核页（订单中心 · 退款域），支持按申请人和状态过滤。你要查的这个人我先帮你定位到页面：',
          actions: [{ title: '打开退款审核', path: '/order/refunds' }],
        }
      }
      if (/订单/.test(q)) {
        return {
          text: '订单数据在订单中心，可按用户过滤。也可以告诉我订单号，我直接帮你查详情：',
          actions: [{ title: '打开订单列表', path: '/order' }],
        }
      }
      if (/用户|权限|实名/.test(q)) {
        return {
          text: '用户资料在用户中心，实名审核有专门的页面；权限规则在权限服务。先看哪个：',
          actions: [
            { title: '用户列表', path: '/user' },
            { title: '实名审核', path: '/user/kyc' },
          ],
        }
      }
      // 3. 兜底：演示模式说明
      return {
        text: '演示模式：这条回复是本地模拟的。接入 llm-proxy 后，我可以跨 20+ 服务查数（openapi 契约会变成我的工具）、带上下文回答当前页面的问题、并直接把你要的页面送到面前。',
      }
    },
  },
})
