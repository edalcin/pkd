import { CodeBlock } from '@tiptap/extension-code-block'
import mermaid from 'mermaid'

let renderCounter = 0

function getMermaidTheme() {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'default'
}

function initMermaid(theme) {
  mermaid.initialize({
    startOnLoad: false,
    theme,
    securityLevel: 'loose',
  })
}

// Detect mermaid syntax by content when language attr is absent/empty
const MERMAID_PATTERN = /^(graph\s+[A-Z]{1,3}|flowchart\s+|sequenceDiagram|classDiagram|stateDiagram|erDiagram|gantt|pie[\s\n]|gitGraph|mindmap|timeline|quadrantChart|xychart|block-beta|architecture-beta|requirementDiagram|journey|zenuml)/i

function isMermaid(node) {
  const lang = node.attrs.language
  if (lang && lang !== 'mermaid') return false
  if (lang === 'mermaid') return true
  return MERMAID_PATTERN.test(node.textContent.trim())
}

async function renderMermaid(source, container, theme) {
  initMermaid(theme)
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
    container.innerHTML = `<pre class="mermaid-error">Erro Mermaid: ${String(err.message).replace(/&/g, '&amp;').replace(/</g, '&lt;')}</pre>`
    console.error('[mermaid] render error:', err)
  }
}

export const MermaidCodeBlock = CodeBlock.extend({
  addNodeView() {
    return ({ node }) => {
      if (!isMermaid(node)) {
        const pre = document.createElement('pre')
        const code = document.createElement('code')
        if (node.attrs.language) code.className = `language-${node.attrs.language}`
        pre.appendChild(code)
        return { dom: pre, contentDOM: code }
      }

      // Mermaid block
      const wrapper = document.createElement('div')
      wrapper.className = 'mermaid-block'

      const pre = document.createElement('pre')
      pre.className = 'mermaid-source'
      const code = document.createElement('code')
      code.className = 'language-mermaid'
      pre.appendChild(code)
      wrapper.appendChild(pre)

      const diagram = document.createElement('div')
      diagram.className = 'mermaid-diagram'
      diagram.innerHTML = '<span class="mermaid-loading">Renderizando diagrama…</span>'
      wrapper.appendChild(diagram)

      let lastSource = null
      let lastTheme = null
      let pendingRender = 0

      const scheduleRender = (source, delay = 150) => {
        const token = ++pendingRender
        setTimeout(async () => {
          if (token !== pendingRender) return
          const theme = getMermaidTheme()
          if (source === lastSource && theme === lastTheme) return
          lastSource = source
          lastTheme = theme
          await renderMermaid(source, diagram, theme)
        }, delay)
      }

      scheduleRender(node.textContent, 0)

      // Re-render when user switches theme
      const themeObserver = new MutationObserver(() => {
        lastSource = null // force re-render even if source unchanged
        scheduleRender(code.textContent, 0)
      })
      themeObserver.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ['data-theme'],
      })

      return {
        dom: wrapper,
        contentDOM: code,

        ignoreMutation(mutation) {
          return diagram.contains(mutation.target) || mutation.target === diagram
        },

        update(updatedNode) {
          if (updatedNode.type.name !== 'codeBlock') return false
          if (!isMermaid(updatedNode)) return false
          scheduleRender(updatedNode.textContent, 150)
          return true
        },

        destroy() {
          themeObserver.disconnect()
        },
      }
    }
  },
})
