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
    securityLevel: 'strict',
  })
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
    container.innerHTML = `<pre class="mermaid-error">${String(err.message).replace(/&/g, '&amp;').replace(/</g, '&lt;')}</pre>`
  }
}

export const MermaidCodeBlock = CodeBlock.extend({
  addNodeView() {
    return ({ node }) => {
      initMermaid()

      const language = node.attrs.language

      // Non-mermaid: standard pre > code with contentDOM
      if (language !== 'mermaid') {
        const pre = document.createElement('pre')
        const code = document.createElement('code')
        if (language) code.className = `language-${language}`
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

      // Diagram area
      const diagram = document.createElement('div')
      diagram.className = 'mermaid-diagram'
      wrapper.appendChild(diagram)

      let lastSource = null
      let pendingRender = 0

      const scheduleRender = (source) => {
        const token = ++pendingRender
        setTimeout(async () => {
          if (token !== pendingRender) return
          if (source === lastSource) return
          lastSource = source
          await renderMermaid(source, diagram)
        }, 150)
      }

      scheduleRender(node.textContent)

      return {
        dom: wrapper,
        contentDOM: code,

        update(updatedNode) {
          if (updatedNode.type.name !== 'codeBlock') return false
          if (updatedNode.attrs.language !== 'mermaid') return false
          scheduleRender(updatedNode.textContent)
          return true
        },
      }
    }
  },
})
