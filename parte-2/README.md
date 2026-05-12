# Parte 2 - Revisão de código

## Como Rodar o Projeto

1. Abra o terminal na pasta `parte-2`.
2. Garanta que o Go esteja instalado e configurado no seu ambiente.
3. Execute:

```bash
go run crawler.go
```

Isso irá rodar o crawler revisado e processar o site `books.toscrape.com`.

## Problemas encontrados

1. Uso de estado global (`results`) e saída só em memória
   - Impacto: se o processo cair no meio da coleta, todos os dados coletados são perdidos.

2. Falta de retry/backoff
   - Impacto: redes instáveis ou respostas 429/500 causam falha imediata e fim do crawler.

3. Falta de persistência incremental
   - Impacto: não há como retomar a coleta depois de uma queda ou erro, gerando retrabalho.

4. Construção de URL frágil
   - Impacto: a concatenação de strings pode quebrar em links relativos ou URLs com caminhos diferentes.

5. URL inicial incorreta
   - Impacto: o crawler começava em `http://books.toscrape.com/catalogue/index.html`, retornando 404 e abortando a coleta.

6. Parsing HTML ineficiente
   - Impacto: leitura completa do body em memória e conversão desnecessária aumentam uso de CPU/memória.

6. Match de classe incorreto
   - Impacto: `strings.Contains` pode retornar verdadeiro para classes parciais, tornando a extração instável.

## Correções aplicadas

- `Crawler` com cliente HTTP reutilizável e configurações centralizadas.
- `fetchPageWithRetry` para lidar com falhas transitórias, incluindo 429 e 5xx.
- `context.WithTimeout` por página para evitar bloqueios prolongados.
- URL inicial corrigida para `http://books.toscrape.com/index.html` para evitar 404 antes mesmo de iniciar a coleta.
- `resolveNextPage` usa `net/url` para montar URLs relativos corretamente.
- `saveBooks` e `saveState` gravam progressão incremental em `books.json` e `crawler_state.json`.
- `loadBooks` e `loadState` permitem retomar de onde parou.
- `hasClass` atualizado para validar classes como tokens separados.
- `html.Parse(resp.Body)` usado diretamente, eliminando cópia extra de conteúdo.

## Justificativa

Essas mudanças tornam o crawler mais estável em cenários reais:
- rede instável
- site lento
- erros temporários do servidor
- queda do processo durante a coleta

A persistência incremental é a principal melhoria para produção, porque garante que o trabalho não seja perdido e permite retomar automaticamente.

## Bibliotecas usadas

- `golang.org/x/net/html` (já utilizada)
- bibliotecas padrão do Go (`net/http`, `context`, `net/url`, `encoding/json`, etc.)

Nenhuma dependência externa adicional foi necessária.
