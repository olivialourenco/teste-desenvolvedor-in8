# Parte 1 - Desafio prático

Este projeto consiste em um web crawler desenvolvido para extrair informações de notebooks da marca **Lenovo** a partir do site de testes WebScraper.io. O objetivo é coletar dados específicos, ordená-los por preço e expô-los através de uma API RESTful em formato JSON.

##  Como Rodar o Projeto

1.  **Pré-requisitos**: Certifique-se de ter o Node.js instalado em sua máquina.
2.  **Navegação**: No terminal, acesse a pasta do projeto:
    ```bash
    cd parte-1
    ```
3.  **Instalação**: Instale as dependências necessárias (Express, Axios e Cheerio):
    ```bash
    npm install
    ```
4.  **Execução**: Inicie o servidor:
    ```bash
    node index.js
    ```
5.  **Acesso**: Abra o navegador ou utilize uma ferramenta como Postman/Insomnia para acessar a rota da API:
    `http://localhost:3000/laptops`

##  Decisões Técnicas

* **Node.js & Express**: Escolhidos pela eficiência no tratamento de requisições assíncronas e facilidade na criação de APIs RESTful.
* **Axios & Cheerio**: Utilizados para realizar as requisições HTTP e realizar o parsing do HTML. Essa escolha respeita a restrição de não utilizar ferramentas de automação de navegador (como Puppeteer ou Selenium).
* **Paginação Manual**: Como o catálogo de produtos está distribuído em várias páginas, implementei um loop que percorre as URLs de paginação para garantir a extração de todos os itens disponíveis no site.
* **Tratamento de Dados**: Durante o desenvolvimento, identifiquei inconsistências nos dados de origem (ex: produtos Lenovo com descrições de outras marcas). Optei por priorizar o título do produto para garantir que nenhum item da marca solicitada fosse ignorado pelo filtro.
* **Ordenação**: Os dados são processados e ordenados de forma crescente (do mais barato para o mais caro) antes de serem enviados na resposta da API.

##  Campos Extraídos

* **Nome do produto**
* **Preço**
* **Descrição**
* **Avaliação** (rating em estrelas)
* **Número de reviews**
* **Link para a página do produto**

##  Requisitos Atendidos

* Uso de Node.js.
* Sem automação de navegador.
* Exposição de dados via API RESTful em JSON.
* Ordenação por preço.
* Documentação clara das decisões técnicas.