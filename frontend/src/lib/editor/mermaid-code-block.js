import { CodeBlock } from '@tiptap/extension-code-block'
import mermaid from 'mermaid'

let mermaidReady = false
let renderCounter = 0

function initMermaid() {
  if (mermaidReady) return
  mermaidReady = true
  mermaid.initialize({
    startOnLoad: false,
    theme: 'default',
    securityLevel: 'loose',
  })
}

// Detect mermaid syntax by content when language attr is absent/empty
const MERMAID_PATTERN = /^(graph\s+[A-Z]{1,3}|flowchart\s+|sequenceDiagram|classDiagram|stateDiagram|erDiagram|gantt|pie[\s\n]|gitGraph|mindmap|timeline|quadrantChart|xychart|block-beta|architecture-beta|requirementDiagram|journey|zenuml)/i

function isMermaid(node) {
  const lang = node.attrs.language
  // Truthy lang that is not 'mermaid' → regular code block
  if (lang && lang !== 'mermaid') return false
  if (lang === 'mermaid') return true
  // No language set: detect by content
  return MERMAID_PATTERN.test(node.textContent.trim())
}

async function renderMermaid(source, container) {
  const trimmed = source.trim()
  if (!trimmed) {
    container.innerHTML = '<span class="mermaid-empty">Diagrama vazio</span>'
    return
  }
  const id = `mermaid-${Date.now()}-${renderCounter++}`
  try {
    const { svg } = await mermaid.render(id, trimmed)
    container.innerHTML = svg
  } catch (err) {
    // Show error so user knows rendering failed
    container.innerHTML = `<pre class="mermaid-error">Erro Mermaid: ${String(err.message).replace(/&/g, '&amp;').replace(/</g, '&lt;')}</pre>`
    console.error('[mermaid] render error:', err)
  }
}

export const MermaidCodeBlock = CodeBlock.extend({
  addNodeView() {
    return ({ node }) => {
      initMermaid()

      if (!isMermaid(node)) {
        // Standard pre > code with contentDOM
        const pre = document.createElement('pre')
        const code = document.createElement('code')
        if (node.attrs.language) code.className = `language-${node.attrs.language}`
        pre.appendChild(code)
        return { dom: pre, contentDOM: code }
      }

      // Mermaid block
      const wrapper = document.createElement('div')
      wrapper.className = 'mermaid-block'

      // Code area (shown in edit mode, hidden when locked via CSS)
      const pre = document.createElement('pre')
      pre.className = 'mermaid-source'
      const code = document.createElement('code')
      code.className = 'language-mermaid'
      pre.appendChild(code)
      wrapper.appendChild(pre)

      // Diagram area — starts with loading indicator
      const diagram = document.createElement('div')
      diagram.className = 'mermaid-diagram'
      diagram.innerHTML = '<span class="mermaid-loading">Renderizando diagrama…</span>'
      wrapper.appendChild(diagram)

      let lastSource = null
      let pendingRender = 0

      const scheduleRender = (source, delay = 150) => {
        const token = ++pendingRender
        setTimeout(async () => {
          if (token !== pendingRender) return
          if (source === lastSource) return
          lastSource = source
          await renderMermaid(source, diagram)
        }, delay)
      }

      // Render immediately on first load, debounced on updates
      scheduleRender(node.textContent, 0)

      return {
        dom: wrapper,
        contentDOM: code,

        update(updatedNode) {
          if (updatedNode.type.name !== 'codeBlock') return false
          if (!isMermaid(updatedNode)) return false
          scheduleRender(updatedNode.textContent, 150)
          return true
        },
      }
    }
  },
})
