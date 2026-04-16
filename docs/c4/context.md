# C4 Level 1 — System Context: PKD

> **Versão**: v2 (003-pkm-refactor) · **Data**: 2026-04-16

## Descrição

PKD (Personal Knowledge Database) é um sistema PKM (Personal Knowledge Management) auto-hospedado de usuário único. O usuário armazena, conecta e recupera conhecimento através de documentos ricos, links bidirecionais e visualização em grafo.

## Diagrama

```mermaid
C4Context
    title Context Diagram — PKD Personal Knowledge Management System

    Person(user, "Usuário", "Único usuário autenticado. Acessa o PKD pelo navegador (desktop ou celular).")
    Person_Ext(public, "Visitante público", "Acessa documentos compartilhados via link público, sem autenticação.")

    System(pkd, "PKD", "Sistema PKM auto-hospedado. Armazena documentos, links bidirecionais, tags e anexos. Expõe API REST + SPA Svelte. Entregue como imagem Docker.")

    System_Ext(mobile_os, "SO Mobile (Android/iOS)", "Envia conteúdo ao PKD via menu 'Compartilhar' usando o PWA share_target.")
    System_Ext(ghcr, "GitHub Container Registry\nghcr.io/edalcin/pkd", "Hospeda a imagem Docker publicada pelo CI/CD a cada push em main.")
    System_Ext(og_sites, "Sites externos", "Consultados pelo servidor PKD para extração de metadados Open Graph quando uma URL é capturada.")

    Rel(user, pkd, "Usa", "HTTPS — autenticado via cookie de sessão")
    Rel(public, pkd, "Acessa documentos compartilhados", "HTTPS — sem autenticação")
    Rel(mobile_os, pkd, "Envia conteúdo compartilhado", "POST /api/capture via PWA share_target")
    Rel(pkd, og_sites, "Busca metadados Open Graph", "HTTP GET (best-effort, timeout 5s)")
    Rel(ghcr, pkd, "Fornece imagem Docker", "OCI pull pelo Docker runtime")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## Relacionamentos principais

| Relacionamento | Descrição |
|---|---|
| **Usuário → PKD** | Toda interação ocorre via SPA Svelte no navegador, autenticada pela senha mestra |
| **Mobile → PKD** | O SO envia conteúdo (link, texto, imagem) para `/api/capture` via PWA share_target |
| **Visitante → PKD** | Acessa `/public/{token}` — página estática sem JS, CSP restrita |
| **PKD → Sites externos** | Extração de `og:title` e `og:description` para documentos capturados com URL |
| **GHCR → Docker** | CI/CD publica nova imagem a cada push em `main`; sem deploy automático |

## Fora do escopo

- Multi-usuário ou controle de acesso por perfil
- Integração com armazenamento em nuvem (Google Drive, S3, Dropbox)
- Edição colaborativa em tempo real
- Sugestões de conexão via IA/LLM
- Sincronização offline bidirecional
