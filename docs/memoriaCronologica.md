# Implementação de um sistema de Memória Cronológica no PKD

Quero implementar um sistema de "memória cronológica" (MC) no PKD que irá criar um novo "bloco" de documentos indexados cronologicamente. A memória cronológica servirá para eu registrar eventos, como por exemplo, um almoço em família, a entrega de um produto, evento de saúde ou a visita de um amigo. Estes eventos serão indexados pela data e, opcionalmente, hora em que aconteceram, ou período do dia (manhã, tarde, noite ou madrugada).

A lista destes documentos da MC será mostrada, opcionalmente, no menu da esquerda (figura), após o bloco que lista os documentos "normais" e antes da entrada de "+ Novo Documento". A lista será representada por uma árvore colapsável, como a dos documentos "normais", onde o documento-raiz é o ANO da MC, seguido dos meses e dias.

![image-20260924054653310](C:\Users\EDalcin\AppData\Roaming\Typora\typora-user-images\image-20260924054653310.png)

Esta árvore é mostrada, por default, colapsada até sua raiz (ano) e com a opção de "mostrar/esconder", como são as TAGs.

Quero uma API específica para esta funcionalidade, onde ferramentas como o HERMES Agent pode criar uma MC automaticamente ou sob demanda, um relato meu solicitando para criar uma memoria cronológica. Esta API irá usar a mesma variável "PKD_IMPORT_TOKEN" já existente no docker do PKD, que será passada para o HERMES e qualquer outra ferramenta que necessite criar conteúdo no PKD.

Os documentos do PKD seguirão o mesmo princípio dos documentos normais, sendo embedados e pesquisáveis sintática e semanticamente, além de estarem disponíveis ao CHAT.



## Considere esta abordagem para nomear e identificar documentos de MC:

### ID público para documentos de memória cronológica

Emitir um identificador público para documentos que registram fatos e
eventos no tempo, legível por humanos e processável por máquinas,
gerável por qualquer cliente sem coordenação central.

#### Formato

[NAMESPACE-]AAAA[-MM[-DD[T<HH>[MM]]]]-SUFIXO6

Regex canônica:

^(?:[A-Z][A-Z0-9]{1,3}-)?[0-9]{4}(?:-[0-9]{2}(?:-[0-9]{2}(?:T[0-9]{2}(?:[0-9]{2})?)?)?)?-[0-9A-HJKMNP-TV-Z]{6}$

Exemplos:

MEM-1850-7QF3K9              só o ano é conhecido
MEM-2026-09-7QF3K9           ano e mês
MEM-2026-09-24-7QF3K9        dia
MEM-2026-09-24T14-7QF3K9     dia e hora (14h)
MEM-2026-09-24T1430-7QF3K9   dia, hora e minuto
MEM-2026-09-24T12-7QF3K9     dia, período "tarde" (bucket 12)

- NAMESPACE: opcional, 2 a 4 caracteres, estável. Agrupa antes de ordenar.
- Prefixo de data: precisão conhecida, componentes ausentes omitidos à
  direita. Nunca preencher com 00 ou XX.
- Hora: opcional, só quando DD existe. Hora local civil do evento.
  Mínimo 2 dígitos; 4 dígitos quando o minuto importa.
- SUFIXO6: 6 caracteres do alfabeto Crockford Base32
  (0123456789ABCDEFGHJKMNPQRSTVWXYZ — sem I, L, O, U), gerados por
  RNG criptográfico no momento da criação.

#### Buckets de período

Quando só o período é conhecido, gravar a hora de início do bucket no
mesmo slot da hora exata:

  T00  madrugada  00:00–05:59
  T06  manhã      06:00–11:59
  T12  tarde      12:00–17:59
  T18  noite      18:00–23:59

Os limites são convenção do projeto e devem ficar declarados junto do
formato. A ambiguidade entre "hora exata 06h" e "bucket manhã" é
resolvida pelo campo de precisão, não pelo ID.

#### Ordenação

Ordem lexicográfica = ordem cronológica, dentro de cada precisão e entre
precisões (grosseira antes da fina no mesmo período):
1850 < 1850-07 < 1850-07-12 < 1850-07-12T06 < 1850-07-12T1430.
A largura mínima de 2 dígitos na hora é o que preserva a monotonicidade.

#### Regras de emissão

1. Emitido uma vez, imutável. Título, data e hora corrigidos depois
   alteram o dado, nunca o identificador. Não renumerar jamais.
2. Congelado na criação: o prefixo é a projeção dos campos de data/hora
   avaliados no momento da emissão, não um valor derivado na leitura.
3. Chave de ordenação do acervo são os campos de data. O ID é projeção
   congelada deles, não a fonte da verdade.
4. Hora é hora local civil. UTC e fuso ficam no campo estruturado.
5. Unicidade garantida por restrição UNIQUE no banco. Colisão de sufixo:
   reprocessar a geração e tentar de novo, com limite de tentativas.
6. Normalização: entrada aceita em maiúsculas ou minúsculas; canônico é
   maiúsculo; Crockford decodifica I/L como 1 e O como 0.
7. ASCII puro, sem acento, sem til, sem espaço. Nunca conter
   : + ? / ~ . — precisa sobreviver intacto a URL, nome de arquivo,
   shell, Windows, S3, TEXT UNIQUE e ditado por telefone.

#### Validação na emissão

Rejeitar antes de emitir: mês fora de 1–12, dia fora do comprimento real
do mês (não basta 1–31: 2026-02-31 não pode virar ID), hora fora de
0–23, minuto fora de 0–59, hora sem dia, sufixo fora do alfabeto.

#### Legado

Documentos antigos cuja data associada foi preenchida por backfill a
partir da data de criação não devem receber ID ancorado no evento:
o prefixo seria data de registro disfarçada de data de fato. Emitir
apenas para documentos com data curada, ou tornar explícito que, nesse
conjunto, o prefixo é data de registro.

#### Não fazer

- Não usar UUIDv7 nem ULID como identificador do acervo: ordenam, mas
  são opacos ao humano.
- Não derivar o identificador do título nem de slug de título.
- Não embutir tipo semântico (fato, evento, decisão): classificação
  muda, vai para tag.
- Não usar sequência central vinda do banco: inviabiliza cliente offline.
- Não reaproveitar external_key, que é chave de idempotência por cliente.

#### Decisões ainda em aberto

1. Prefixo ancorado na data do evento (recomendado) ou na data de registro.
2. Sufixo aleatório puro (recomendado) ou slug congelado do título
   antes do sufixo, aceitando a fragilidade.
3. NAMESPACE fixo (MEM) ou ausente.