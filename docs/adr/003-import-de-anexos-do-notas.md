# ADR-003: Anexos no Import do Notas (`/api/import`)

**Status:** Aceito
**Data:** 2026-08-12

## Contexto

`POST /api/import` (autenticado por `PKD_IMPORT_TOKEN`, sem cookie de sessão) cria um documento a partir de `{title, content, tags}` enviado pelo Notas. Notas passou a precisar enviar também os arquivos anexados à nota (imagens e outros tipos) para que fiquem visíveis no documento criado.

O upload normal de anexos (`POST /api/documents/{id}/attachments`) exige sessão autenticada (`AuthRequired`) — o token de import não é aceito ali, e não há como referenciar um anexo dentro do `body_html` antes do documento existir (o anexo precisa de um `document_id`). Embutir os arquivos como `data:` URI no HTML também não é viável: a política de sanitização (`bluemonday`) só permite os esquemas `http`, `https`, `mailto` em atributos de URL.

## Decisões

### D1 — `/api/import` aceita anexos em base64, sem nova superfície de autenticação

`POST /api/import` passa a aceitar um campo opcional `attachments: [{filename, mime_type, data_base64}]` no mesmo corpo JSON. O handler decodifica cada item e chama `attachments.CreateFile(...)` (o mesmo método já usado por `handleCreateAttachmentFromURL`) logo após criar o documento, dentro da mesma requisição — nenhuma rota nova é exposta ao Bearer token de import.

**Alternativa descartada:** duas chamadas (criar doc vazio, depois multipart em `/api/documents/{id}/attachments`, depois `PUT` para referenciar as imagens no corpo) — rejeitada porque exigiria abrir a rota de anexos (hoje só sessão) para o token de import, ampliando a superfície de autenticação sem necessidade, e adicionaria round-trips e um estado intermediário (documento sem anexos) visível em caso de falha parcial.

**Custo aceito:** overhead de ~33% do base64 sobre o tamanho do arquivo. Irrelevante na escala de uso (notas pessoais, poucos anexos por nota); `PKD_MAX_ATTACHMENT_MB`/`PKD_MAX_IMAGE_MB` já limitam o tamanho aceito.

### D2 — Anexos são embutidos inline no `body_html`, não só associados ao documento

Cada anexo importado gera `<img src="/api/attachments/{id}">` (MIME `image/*`) ou `<a href="/api/attachments/{id}">{filename}</a>` (demais tipos), concatenados num bloco `<hr><p><strong>Anexos</strong></p>...` ao final do HTML convertido do Markdown, **antes** de `SanitizeEditorHTML` — preservando a mesma política de sanitização já aplicada ao restante do corpo.

### D3 — Falha em qualquer anexo reverte o documento inteiro (tudo ou nada)

Se `CreateFile` falhar para qualquer anexo (tamanho, storage), o handler chama `attachments.DeleteByDocument(docID)` + `docs.PermanentDelete(docID)` para o documento recém-criado nesta mesma requisição, e responde erro (não `201`). Não fica um documento parcialmente importado no PKD — o Notas só marca a nota como exportada/movida para a lixeira ao receber `201`.

## Consequências

- `internal/server/handlers_import.go`: `handleImport` ganha decodificação de `attachments`, chamada a `s.attachments.CreateFile` por item e rollback (`DeleteByDocument` + `PermanentDelete`) em caso de erro parcial.
- Nenhuma mudança em `AuthRequired`/`ImportTokenAuth` — a fronteira de autenticação do token de import continua restrita a `/api/import`.
- `editorPolicy` (sanitize.go) não precisou mudar — URLs geradas são relativas (`/api/attachments/{id}`), já cobertas por `AllowRelativeURLs(true)`.
