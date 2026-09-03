import { Image } from '@tiptap/extension-image'

export const ResizableImage = Image.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      width: {
        default: null,
        parseHTML: el => el.getAttribute('width') ? parseInt(el.getAttribute('width'), 10) : null,
        renderHTML: attrs => attrs.width ? { width: attrs.width } : {},
      },
    }
  },

  addNodeView() {
    return ({ node, editor, getPos }) => {
      const wrapper = document.createElement('span')
      wrapper.setAttribute('contenteditable', 'false')
      wrapper.className = 'resizable-image-wrapper'

      const img = document.createElement('img')
      img.src = node.attrs.src || ''
      if (node.attrs.alt) img.alt = node.attrs.alt
      if (node.attrs.title) img.title = node.attrs.title

      const handle = document.createElement('span')
      handle.className = 'resizable-image-handle'
      handle.setAttribute('title', 'Redimensionar imagem')

      if (node.attrs.width) {
        wrapper.style.width = node.attrs.width + 'px'
        img.style.width = '100%'
      }

      handle.addEventListener('mousedown', (e) => {
        e.preventDefault()
        e.stopPropagation()
        const startX = e.clientX
        const startWidth = wrapper.offsetWidth || img.naturalWidth || 200

        const onMove = (ev) => {
          const newWidth = Math.max(50, startWidth + ev.clientX - startX)
          wrapper.style.width = newWidth + 'px'
          img.style.width = '100%'
        }

        const onUp = () => {
          document.removeEventListener('mousemove', onMove)
          document.removeEventListener('mouseup', onUp)
          // v3: getPos() may return undefined when the node view is detached
          const pos = typeof getPos === 'function' ? getPos() : undefined
          if (pos !== undefined) {
            editor.view.dispatch(
              editor.view.state.tr.setNodeMarkup(pos, undefined, {
                ...node.attrs,
                width: wrapper.offsetWidth,
              })
            )
          }
        }

        document.addEventListener('mousemove', onMove)
        document.addEventListener('mouseup', onUp)
      })

      wrapper.appendChild(img)
      wrapper.appendChild(handle)

      return {
        dom: wrapper,
        update(updatedNode) {
          if (updatedNode.type.name !== 'image') return false
          img.src = updatedNode.attrs.src || ''
          img.alt = updatedNode.attrs.alt || ''
          img.title = updatedNode.attrs.title || ''
          if (updatedNode.attrs.width) {
            wrapper.style.width = updatedNode.attrs.width + 'px'
            img.style.width = '100%'
          } else {
            wrapper.style.width = ''
            img.style.width = ''
          }
          return true
        },
      }
    }
  },
})
