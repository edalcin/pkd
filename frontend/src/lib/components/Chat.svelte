<script>
  import { apiFetch } from '../api.js'
  import { createDoc, saveDoc } from '../stores/documents.js'

  // Conversa efêmera: vive aqui e morre ao trocar de rota ou recarregar
  // (ADR-006 D8). O que sobrevive é o que o usuário salvar como documento —
  // por isso os links de fonte abrem em nova aba.
  let messages = $state([]) // { role: 'user' | 'model', text, sources? }
  let question = $state('')
  let streaming = $state(false)
  let errorMsg = $state('')
  let saved = $state(null)
  let controller = null
  let scroller = null

  const canSend = $derived(question.trim() !== '' && !streaming)

  function scrollToEnd() {
    // Deixa o layout assentar antes de rolar; sem isso a última linha do
    // chunk recém-chegado fica cortada.
    requestAnimationFrame(() => {
      if (scroller) scroller.scrollTop = scroller.scrollHeight
    })
  }

  async function send() {
    if (!canSend) return
    const text = question.trim()
    question = ''
    errorMsg = ''
    saved = null
    messages = [...messages, { role: 'user', text }]
    const idx = messages.length
    messages = [...messages, { role: 'model', text: '', sources: [] }]
    streaming = true
    scrollToEnd()

    // fetch + ReadableStream, não EventSource: EventSource não manda o header
    // X-CSRF-Token, e isentar a rota do CSRF global não é uma troca aceitável
    // (ADR-006 D7). O AbortController mata a geração no servidor.
    controller = new AbortController()
    try {
      const res = await apiFetch('/api/chat', {
        method: 'POST',
        body: JSON.stringify({
          messages: messages.slice(0, idx).map(m => ({ role: m.role, text: m.text })),
        }),
        signal: controller.signal,
      })
      if (!res.ok) {
        errorMsg = res.status === 503
          ? 'GEMINI_API_KEY não configurada no servidor.'
          : 'Falha ao consultar os documentos.'
        messages = messages.slice(0, idx)
        return
      }
      await readStream(res.body, idx)
    } catch (e) {
      if (e.name !== 'AbortError') errorMsg = 'Conexão interrompida.'
    } finally {
      streaming = false
      controller = null
      // Erro sem nenhum texto gerado: remove a bolha vazia em vez de deixar
      // um retângulo cinza sem conteúdo abaixo da pergunta.
      const last = messages[messages.length - 1]
      if (last?.role === 'model' && last.text === '') messages = messages.slice(0, -1)
    }
  }

  // readStream consome o SSE do backend: eventos `sources`, `text`, `error`,
  // `done`, separados por linha em branco.
  async function readStream(body, idx) {
    const reader = body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let sep
      while ((sep = buffer.indexOf('\n\n')) !== -1) {
        const frame = buffer.slice(0, sep)
        buffer = buffer.slice(sep + 2)
        let event = ''
        let data = ''
        for (const line of frame.split('\n')) {
          if (line.startsWith('event: ')) event = line.slice(7).trim()
          else if (line.startsWith('data: ')) data += line.slice(6)
        }
        if (!event || !data) continue
        let payload
        try { payload = JSON.parse(data) } catch { continue }
        if (event === 'sources') {
          messages[idx].sources = payload || []
        } else if (event === 'text') {
          messages[idx].text += payload
          scrollToEnd()
        } else if (event === 'error') {
          errorMsg = payload
        }
      }
    }
  }

  function stop() {
    controller?.abort()
  }

  // Salvar como documento: a conversa vira conhecimento indexado, versionado e
  // linkável, reusando o DocumentStore em vez de um segundo armazenamento de
  // texto ao lado dele (ADR-006 D8).
  async function saveAsDocument() {
    const first = messages.find(m => m.role === 'user')
    if (!first) return
    const title = first.text.length > 80 ? first.text.slice(0, 80) + '…' : first.text
    const html = messages.map(m => {
      const who = m.role === 'user' ? 'Pergunta' : 'Resposta'
      const sources = (m.sources || []).length
        ? '<p><em>Documentos consultados: ' +
          m.sources.map(s => escapeHtml(s.title)).join(', ') + '</em></p>'
        : ''
      return `<h3>${who}</h3><p>${escapeHtml(m.text).replace(/\n/g, '<br>')}</p>${sources}`
    }).join('')
    const text = messages.map(m => m.text).join('\n\n')
    try {
      const doc = await createDoc(null, title)
      await saveDoc(doc.id, {
        version: doc.version,
        title,
        body_html: html,
        body_text: text,
        icon: '',
      })
      saved = doc.id
    } catch {
      errorMsg = 'Não foi possível salvar a conversa como documento.'
    }
  }

  function escapeHtml(s) {
    return s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]))
  }

  function onKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }
</script>

<div class="chat">
  <div class="chat-log" bind:this={scroller}>
    {#if messages.length === 0}
      <div class="chat-empty">
        <p><strong>Converse com os seus documentos.</strong></p>
        <p class="muted">
          As respostas vêm apenas do conteúdo do seu PKD. Quando não houver base
          nos documentos, a resposta dirá isso em vez de inventar.
        </p>
      </div>
    {/if}

    {#each messages as m, i (i)}
      <div class="msg msg-{m.role}">
        <div class="msg-text">{m.text}{#if streaming && i === messages.length - 1 && m.role === 'model'}<span class="cursor">▍</span>{/if}</div>
        {#if m.role === 'model' && m.sources?.length}
          <div class="sources">
            <span class="sources-label">Documentos consultados:</span>
            {#each m.sources as s (s.id)}
              <a href={`#/doc/${s.id}`} target="_blank" rel="noreferrer" class="source-link">{s.title}</a>
            {/each}
          </div>
        {/if}
      </div>
    {/each}

    {#if errorMsg}
      <div class="chat-error">{errorMsg}</div>
    {/if}
    {#if saved}
      <div class="chat-saved">
        Conversa salva como documento.
        <a href={`#/doc/${saved}`}>Abrir</a>
      </div>
    {/if}
  </div>

  <div class="chat-input">
    <textarea
      bind:value={question}
      onkeydown={onKeydown}
      rows="2"
      placeholder="Pergunte algo sobre os seus documentos…"
      disabled={streaming}
    ></textarea>
    {#if streaming}
      <button class="btn" onclick={stop} title="Interromper a geração">Parar</button>
    {:else}
      <button class="btn btn-primary" onclick={send} disabled={!canSend}>Perguntar</button>
    {/if}
    {#if messages.length > 0 && !streaming}
      <button class="btn" onclick={saveAsDocument} title="Salvar a conversa como um documento do PKD">
        Salvar como documento
      </button>
    {/if}
  </div>
</div>

<style>
  .chat {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  .chat-log {
    flex: 1;
    overflow-y: auto;
    padding: 1rem 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .chat-empty {
    margin: auto;
    max-width: 34rem;
    text-align: center;
  }
  .chat-empty .muted {
    color: var(--text-muted, #888);
    font-size: 0.9rem;
  }
  .msg {
    max-width: 46rem;
    padding: 0.6rem 0.85rem;
    border-radius: 0.6rem;
    line-height: 1.5;
  }
  .msg-user {
    align-self: flex-end;
    background: var(--accent-soft, rgba(80, 130, 255, 0.12));
  }
  .msg-model {
    align-self: flex-start;
    background: var(--bg-subtle, rgba(127, 127, 127, 0.1));
  }
  .msg-text {
    white-space: pre-wrap;
    word-break: break-word;
  }
  .cursor {
    opacity: 0.6;
  }
  .sources {
    margin-top: 0.5rem;
    padding-top: 0.4rem;
    border-top: 1px solid var(--border, rgba(127, 127, 127, 0.25));
    font-size: 0.82rem;
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    align-items: baseline;
  }
  .sources-label {
    color: var(--text-muted, #888);
  }
  .source-link {
    text-decoration: none;
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--border, rgba(127, 127, 127, 0.35));
    border-radius: 0.35rem;
  }
  .chat-error {
    align-self: center;
    color: #c0392b;
    font-size: 0.9rem;
  }
  .chat-saved {
    align-self: center;
    font-size: 0.9rem;
  }
  .chat-input {
    display: flex;
    gap: 0.5rem;
    align-items: flex-end;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border, rgba(127, 127, 127, 0.25));
  }
  .chat-input textarea {
    flex: 1;
    resize: vertical;
    font: inherit;
    padding: 0.5rem;
    border-radius: 0.4rem;
    border: 1px solid var(--border, rgba(127, 127, 127, 0.35));
    background: var(--bg, transparent);
    color: inherit;
  }
</style>
