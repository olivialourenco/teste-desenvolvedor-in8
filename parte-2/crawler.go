package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
)

// Book representa um livro extraído do site
type Book struct {
	Title  string `json:"title"`
	Price  string `json:"price"`
	Rating string `json:"rating"`
}

// CrawlResult encapsula o resultado da coleta
type CrawlResult struct {
	Books      []Book    `json:"books"`
	TotalBooks int       `json:"total_books"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Duration   string    `json:"duration"`
}

// CrawlerConfig contém configurações do crawler
type CrawlerConfig struct {
	BaseURL         string
	StartPage       string
	MaxRetries      int
	InitialBackoff  time.Duration
	RequestTimeout  time.Duration
	RateLimitDelay  time.Duration
	OutputFile      string
	MaxBooksPerPage int
}

// Crawler executa a coleta de dados
type Crawler struct {
	config    CrawlerConfig
	client    *http.Client
	books     []Book
	startTime time.Time
}

// NewCrawler cria uma nova instância do crawler
func NewCrawler(config CrawlerConfig) *Crawler {
	return &Crawler{
		config: config,
		client: &http.Client{
			Timeout: config.RequestTimeout,
		},
		books:     make([]Book, 0),
		startTime: time.Now(),
	}
}

// fetchPage faz requisição HTTP com retry e backoff exponencial
func (c *Crawler) fetchPage(ctx context.Context, url string) (*html.Node, error) {
	var lastErr error

	for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		node, err := c.fetchPageOnce(ctx, url)
		if err == nil {
			return node, nil
		}

		lastErr = err

		// Não faz retry para certos erros
		if isNonRetryableError(err) {
			log.Printf("Erro não recuperável em %s: %v", url, err)
			return nil, err
		}

		if attempt < c.config.MaxRetries-1 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * c.config.InitialBackoff
			log.Printf("Tentativa %d falhou para %s. Aguardando %v antes de retry...", attempt+1, url, backoff)
			time.Sleep(backoff)
		}
	}

	return nil, fmt.Errorf("falha após %d tentativas: %w", c.config.MaxRetries, lastErr)
}

// fetchPageOnce faz uma única requisição
func (c *Crawler) fetchPageOnce(ctx context.Context, url string) (*html.Node, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	// Adiciona User-Agent para evitar bloqueios
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição HTTP: %w", err)
	}
	defer resp.Body.Close()

	// Valida status code
	if resp.StatusCode != http.StatusOK {
		// 429 e 503 são retentáveis
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			return nil, fmt.Errorf("rate limit/server unavailable: %d", resp.StatusCode)
		}
		// 404, 410, 403 não são retentáveis
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("erro HTTP não recuperável: %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("erro HTTP: %d", resp.StatusCode)
	}

	// Limita tamanho da resposta para evitar OOM
	limitedReader := io.LimitReader(resp.Body, 10*1024*1024) // 10MB max
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler body: %w", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("resposta vazia")
	}

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("erro ao fazer parse do HTML: %w", err)
	}

	return doc, nil
}

// isNonRetryableError determina se um erro não deve ser retentado
func isNonRetryableError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "não recuperável") ||
		strings.Contains(errStr, "resposta vazia") ||
		strings.Contains(errStr, "parse")
}

// parseBooks extrai livros da página
func (c *Crawler) parseBooks(doc *html.Node) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && hasClass(n, "product_pod") {
			if len(c.books) < c.config.MaxBooksPerPage {
				book := c.extractBook(n)
				// Valida se o livro tem dados importantes
				if book.Title != "" && book.Price != "" {
					c.books = append(c.books, book)
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
}

// extractBook extrai informações de um livro individual
func (c *Crawler) extractBook(n *html.Node) Book {
	var title, price, rating string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h3":
				// Extrai título do link dentro de h3
				if link := findElement(n, "a"); link != nil {
					title = getAttr(link, "title")
				}
			case "p":
				if hasClass(n, "star-rating") {
					rating = extractRating(n)
				}
				if hasClass(n, "price_color") {
					price = extractPrice(n)
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)

	return Book{
		Title:  strings.TrimSpace(title),
		Price:  strings.TrimSpace(price),
		Rating: strings.TrimSpace(rating),
	}
}

// extractRating extrai a classificação de estrelas
func extractRating(n *html.Node) string {
	classAttr := getAttr(n, "class")
	if classAttr == "" {
		return ""
	}
	parts := strings.Fields(classAttr)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// extractPrice extrai o preço do elemento
func extractPrice(n *html.Node) string {
	if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
		return n.FirstChild.Data
	}
	return ""
}

// findElement procura por um elemento específico
func findElement(n *html.Node, tag string) *html.Node {
	var result *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if result != nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == tag {
			result = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
			if result != nil {
				return
			}
		}
	}
	walk(n)
	return result
}

// getNextPage encontra o link da próxima página
func (c *Crawler) getNextPage(doc *html.Node) string {
	var next string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if next != "" {
			return
		}

		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, "next") {
			if a := findElement(n, "a"); a != nil {
				next = getAttr(a, "href")
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return next
}

// hasClass verifica se um elemento tem uma classe
func hasClass(n *html.Node, class string) bool {
	return strings.Contains(getAttr(n, "class"), class)
}

// getAttr obtém um atributo de um elemento
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// Run executa o crawler
func (c *Crawler) Run(ctx context.Context) error {
	baseURL := c.config.BaseURL
	page := c.config.StartPage

	pageCount := 0

	for page != "" {
		select {
		case <-ctx.Done():
			log.Println("Crawling cancelado.")
			return ctx.Err()
		default:
		}

		pageCount++
		url := baseURL + page
		log.Printf("Coletando página %d: %s", pageCount, page)

		doc, err := c.fetchPage(ctx, url)
		if err != nil {
			log.Printf("Erro ao buscar %s: %v", url, err)
			break
		}

		beforeCount := len(c.books)
		c.parseBooks(doc)
		log.Printf("  Livros encontrados nesta página: %d (total: %d)", len(c.books)-beforeCount, len(c.books))

		next := c.getNextPage(doc)
		page = next

		// Rate limiting - aguarda entre requisições
		if page != "" {
			time.Sleep(c.config.RateLimitDelay)
		}
	}

	return nil
}

// SaveResults salva os resultados em arquivo JSON
func (c *Crawler) SaveResults() error {
	result := CrawlResult{
		Books:      c.books,
		TotalBooks: len(c.books),
		StartTime:  c.startTime,
		EndTime:    time.Now(),
		Duration:   time.Since(c.startTime).String(),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar JSON: %w", err)
	}

	err = os.WriteFile(c.config.OutputFile, data, 0644)
	if err != nil {
		return fmt.Errorf("erro ao salvar arquivo: %w", err)
	}

	log.Printf("Resultados salvos em %s", c.config.OutputFile)
	return nil
}

// PrintResults imprime um resumo dos resultados
func (c *Crawler) PrintResults() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("RESUMO DA COLETA\n")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Total de livros coletados: %d\n", len(c.books))
	fmt.Printf("Tempo de execução: %v\n", time.Since(c.startTime))
	fmt.Println(strings.Repeat("=", 80))

	if len(c.books) > 0 {
		fmt.Println("\nPrimeiros 10 livros:")
		limit := len(c.books)
		if limit > 10 {
			limit = 10
		}
		for i := 0; i < limit; i++ {
			b := c.books[i]
			fmt.Printf("[%d] Título: %s | Preço: %s | Avaliação: %s\n",
				i+1, b.Title, b.Price, b.Rating)
		}
	}
}

func main() {
	// Configuração do crawler
	config := CrawlerConfig{
		BaseURL:         "http://books.toscrape.com/catalogue/",
		StartPage:       "page-1.html",
		MaxRetries:      3,
		InitialBackoff:  1 * time.Second,
		RequestTimeout:  10 * time.Second,
		RateLimitDelay:  500 * time.Millisecond,
		OutputFile:      "books_results.json",
		MaxBooksPerPage: 10000, // Limite de segurança
	}

	crawler := NewCrawler(config)

	// Configura context com timeout e signal handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Configura signal handler para graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Sinal de interrupção recebido. Finalizando...")
		cancel()
	}()

	// Executa o crawler
	log.Println("Iniciando coleta de livros...")
	if err := crawler.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("Erro durante execução: %v", err)
	}

	// Salva e exibe resultados
	crawler.PrintResults()

	if err := crawler.SaveResults(); err != nil {
		log.Printf("Erro ao salvar resultados: %v", err)
	}
}
