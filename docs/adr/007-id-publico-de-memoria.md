# ADR-007: ID Público de Memória — congelado na emissão

**Status:** Aceito
**Data:** 2026-09-24
**Relacionado:** [docs/memoriaCronologica.md](../memoriaCronologica.md) (formato do ID)

Toda **Memória** recebe um ID público no formato
`MEM-AAAA[-MM[-DD[T<HH>[MM]]]]-SUFIXO6`, ancorado na **Data da Memória**
(não na data de criação) e **imutável**: corrigir a data depois altera os
campos, nunca o ID. Árvore, ordenação e busca usam só os campos de data; o ID é
uma projeção congelada deles no momento da emissão e não prova a data atual.

Por quê: os agentes (Hermes) são os principais criadores e podem guardar o ID
para corrigir ou complementar a Memória depois. Reemitir o ID quebraria essa
referência sem aviso. Correção de data é exceção, então o custo — um rótulo
desatualizado em casos raros — é menor que o de reemissão ou alias.

## Opções descartadas

- **Reemitir sem alias** — ID sempre mostra a data certa, mas invalida qualquer
  referência externa ao ID antigo (inclusive a do agente que criou a Memória).
- **Reemitir com alias** — mantém referências, mas exige tabela de aliases para
  um caso raro.
- **Bloquear a data após a criação** — força apagar e recriar para corrigir.

## Consequências

- **Período** no ID usa a hora de início: almoço → `T12`, lanche → `T16`,
  jantar → `T18`. Códigos com letras (`TAL`, `TJN`, `TLN`) violariam a regex
  canônica e quebrariam a ordem lexicográfica (`A` > `9` em ASCII). O nome do
  Período fica em campo estruturado.
- Precisão mínima é o ano (`MEM-1998-XXXXXX`); componentes desconhecidos são
  omitidos, nunca inventados.
- Links internos do PKD seguem usando o ID numérico do Documento; o ID público
  é rótulo de indexação.
