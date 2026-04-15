# Feature Specification: Troca do Editor para TipTap

**Feature**: `002-switch-editor-tiptap`
**Date**: 2026-04-15
**Status**: Approved for Implementation
**Author**: Eduardo Dalcin

---

## Summary

Substituir o CKEditor 5 (bundle GPL v2+, 1.3 MB) pelo **TipTap v2** (MIT, ~200 KB) como editor de texto rico do PKD. A mudança mantém todas as funcionalidades editoriais existentes e melhora em três eixos dos princípios do projeto: **simplicidade** (API mais limpa), **modernidade** (TypeScript-first, headless) e **tamanho do Docker** (bundle 4–5x menor).

A decisão é fundamentada na pesquisa original (`research.md §4`) que documentou TipTap como runner-up aceito:

> "TipTap (ProseMirror wrapper, MIT): smaller bundle, excellent developer ergonomics, image-resize extension exists. Runner-up. Rejected only because the user explicitly referenced Trilium/CKEditor 5."

O usuário removeu explicitamente a restrição de aderência ao Trilium/CKEditor.

---

## Decision Analysis

### Por que TipTap vence nos princípios do projeto

| Critério | CKEditor 5 | TipTap v2 | Vencedor |
|---|---|---|---|
| Bundle size (min) | ~1.3 MB | ~200 KB | **TipTap** (~6x menor) |
| Bundle size (gzip) | ~400 KB | ~70 KB | **TipTap** |
| Licença | GPL v2+ | MIT | **TipTap** |
| API | Plugin verboso | Extension fluente | **TipTap** |
| TypeScript | Parcial | Nativo | **TipTap** |
| Headless | Não | Sim | **TipTap** |
| Image resize | ✓ | ✓ (extension) | Empate |
| Tabelas | ✓ | ✓ (extension) | Empate |
| Upload de imagem | ✓ (SimpleUpload) | ✓ (custom handler) | Empate |
| Maturidade | Alta | Alta (v2 estável) | Empate |

**Veredicto**: Trocar para TipTap. Menor, mais livre, mais moderno, sem perda funcional.

---

## User Scenarios & Testing

### Cenário 1 — Edição rica de documentos (mantido)
O usuário abre um documento existente, formata texto (negrito, itálico, headings, listas, código), insere imagem inline, redimensiona pela alça, insere tabela. Salva. Recarrega. Tudo persiste idêntico.

**Independent Test**: O HTML gerado pelo TipTap deve ser compatível com o sanitizador bluemonday existente e renderizar corretamente na view pública.

### Cenário 2 — Upload de imagem inline
O usuário cola ou arrasta uma imagem no editor. A imagem é enviada via POST para `/api/documents/{id}/attachments`, retorna URL `/api/attachments/{id}`, e é inserida inline no documento com suporte a redimensionamento.

### Cenário 3 — Migração de conteúdo existente
Documentos criados com CKEditor 5 (HTML) continuam renderizando corretamente. O TipTap usa o mesmo formato HTML de saída; bluemonday sanitiza ambos.

---

## Functional Requirements

### Funcionalidades mantidas (sem regressão)

- FR-T01: Editor suporta: headings (H1–H4), parágrafo, negrito, itálico, sublinhado, riscado, lista ordenada, lista não-ordenada, bloco de código, citação, link, tabela.
- FR-T02: Imagens inline com redimensionamento por alça de arrastar.
- FR-T03: Upload de imagem via paste/drag integrado ao endpoint `/api/documents/{id}/attachments`.
- FR-T04: Auto-save 2 segundos após parar de digitar.
- FR-T05: Detecção de conflito de versão (409) com diálogo overwrite/reload (FR-010a existente).
- FR-T06: Modo somente-leitura ao detectar offline (`x-pkd-offline: read-only`).
- FR-T07: HTML gerado é sanitizável pelo bluemonday (EditorPolicy existente) sem perda de conteúdo legítimo.

### Funcionalidades novas / melhoradas

- FR-T08: Bundle do editor servido de `/vendor/tiptap/tiptap.umd.js` a partir de arquivo commitado no repositório (sem CDN externo em runtime).
- FR-T09: O bundle total do editor será ≤ 300 KB minificado antes de gzip.
- FR-T10: A imagem Docker final não aumentará (idealmente diminuirá) em relação à versão com CKEditor.

---

## Success Criteria

- **SC-T01**: Todas as funcionalidades do FR-T01 ao FR-T07 funcionam identicamente à versão com CKEditor.
- **SC-T02**: O bundle do TipTap ≤ 300 KB minificado (vs 1.3 MB do CKEditor 5).
- **SC-T03**: A imagem Docker final é igual ou menor que a versão anterior.
- **SC-T04**: Testes existentes (`go test ./...`) continuam passando sem modificação nos testes Go.
- **SC-T05**: Conteúdo HTML gerado pelo TipTap passa pelo sanitizador bluemonday sem truncamento de formatação legítima.
- **SC-T06**: Zero dependência de rede externa em runtime (bundle servido localmente).

---

## Assumptions

- O HTML emitido pelo TipTap é semanticamente equivalente ao do CKEditor 5 para os elementos suportados — ambos usam HTML5 padrão.
- A política bluemonday existente (`EditorPolicy`) é suficiente para o HTML do TipTap sem alterações.
- Documentos criados com CKEditor 5 continuarão renderizando corretamente (o HTML é o formato de armazenamento, não o formato do editor).
- O bundle será gerado com `npm` localmente e commitado — sem Node.js no Docker build.

## Out of Scope

- Migração automática de formatação de documentos existentes (HTML é HTML).
- Funcionalidades novas não previstas no spec original do PKD.
- Collaborative editing / CRDT.
