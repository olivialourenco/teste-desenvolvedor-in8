package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type Book struct {
	Title  string `json:"title"`
	Price  string `json:"price"`
	Rating string `json:"rating"`
}

type Crawler struct {
	client         *http.Client
	maxRetries     int
	initialBackoff time.Duration
	userAgent      string
	outputFile     string
	stateFile      string
}

type crawlState struct {
	NextPage string `json:"next_page"`
}

type HTTPStatusError struct {
	URL        string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("status %d for %s", e.StatusCode, e.URL)
}

// NewCrawler configura o cliente HTTP e as opções de retry e estado usadas pelo crawler.
func NewCrawler(outputFile, stateFile string) *Crawler {
	return &Crawler{
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
		maxRetries:     3,
		initialBackoff: 500 * time.Millisecond,
		userAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		outputFile:     outputFile,
		stateFile:      stateFile,
	}
}

// fetchPage realiza a requisição e parse do HTML, validando o status da resposta.
func (c *Crawler) fetchPage(ctx context.Context, fullURL string) (*html.Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{URL: fullURL, StatusCode: resp.StatusCode}
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear HTML: %w", err)
	}
	return doc, nil
}

// fetchPageWithRetry aplica tentativas com backoff exponencial para falhas temporárias.
func (c *Crawler) fetchPageWithRetry(ctx context.Context, fullURL string) (*html.Node, error) {
	var lastErr error
	backoff := c.initialBackoff

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		doc, err := c.fetchPage(ctx, fullURL)
		if err == nil {
			return doc, nil
		}

		lastErr = err
		if !isRetryable(err) {
			return nil, err
		}

		log.Printf("tentativa %d falhou para %s: %v", attempt+1, fullURL, err)
	}

	return nil, fmt.Errorf("não foi possível buscar %s após %d tentativas: %w", fullURL, c.maxRetries+1, lastErr)
}

func isRetryable(err error) bool {
	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= 500
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return false
}

// loadBooks recupera a lista de livros já gravada para permitir retomar a coleta.
func (c *Crawler) loadBooks() ([]Book, error) {
	data, err := os.ReadFile(c.outputFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var books []Book
	if err := json.Unmarshal(data, &books); err != nil {
		return nil, fmt.Errorf("falha ao ler saída existente: %w", err)
	}
	return books, nil
}

func (c *Crawler) saveJSON(filename string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	tmp := filename + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp, filename)
}

// saveState persiste o progresso atual do crawler para retomar no próximo ciclo.
func (c *Crawler) saveState(state crawlState) error {
	return c.saveJSON(c.stateFile, state)
}

// saveBooks atualiza o arquivo de saída com os livros coletados até o momento.
func (c *Crawler) saveBooks(books []Book) error {
	return c.saveJSON(c.outputFile, books)
}

// loadState carrega o estado do processo para retomar a coleta de onde parou.
func (c *Crawler) loadState() (crawlState, error) {
	var state crawlState
	data, err := os.ReadFile(c.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("falha ao ler estado de crawl: %w", err)
	}
	return state, nil
}

// clearState remove o arquivo de estado quando a coleta é concluída com sucesso.
func (c *Crawler) clearState() error {
	if err := os.Remove(c.stateFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Crawl coordena o loop de coleta, salva progresso e respeita intervalos entre páginas.
func (c *Crawler) Crawl(ctx context.Context, startURL string) ([]Book, error) {
	books, err := c.loadBooks()
	if err != nil {
		return nil, err
	}

	state, err := c.loadState()
	if err != nil {
		return books, err
	}

	currentURL := startURL
	if state.NextPage != "" {
		currentURL = state.NextPage
	}

	for currentURL != "" {
		log.Printf("coletando: %s", currentURL)

		pageCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		doc, err := c.fetchPageWithRetry(pageCtx, currentURL)
		cancel()
		if err != nil {
			return books, err
		}

		pageBooks := parseBooks(doc)
		books = append(books, pageBooks...)

		nextPage, err := resolveNextPage(doc, currentURL)
		if err != nil {
			return books, err
		}

		if err := c.saveBooks(books); err != nil {
			return books, err
		}
		if err := c.saveState(crawlState{NextPage: nextPage}); err != nil {
			return books, err
		}

		if nextPage == "" {
			break
		}

		select {
		case <-ctx.Done():
			return books, ctx.Err()
		default:
		}

		time.Sleep(600 * time.Millisecond)
		currentURL = nextPage
	}

	if err := c.clearState(); err != nil {
		return books, err
	}
	return books, nil
}

// parseBooks extrai a lista de livros a partir da árvore HTML parseada.
func parseBooks(doc *html.Node) []Book {
	var books []Book
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "article" && hasClass(n, "product_pod") {
			books = append(books, extractBook(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return books
}

// extractBook coleta título, preço e avaliação de um bloco individual de livro.
func extractBook(n *html.Node) Book {
	var title, price, rating string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p":
				if hasClass(n, "star-rating") {
					rating = getRating(n)
				}
				if hasClass(n, "price_color") && n.FirstChild != nil {
					price = strings.TrimSpace(n.FirstChild.Data)
				}
			case "a":
				if title == "" {
					title = getAttr(n, "title")
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return Book{Title: title, Price: price, Rating: rating}
}

func getRating(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, token := range strings.Fields(a.Val) {
				if token != "star-rating" {
					return token
				}
			}
		}
	}
	return ""
}

func resolveNextPage(doc *html.Node, currentURL string) (string, error) {
	var nextHref string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, "next") {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "a" {
					nextHref = getAttr(c, "href")
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if nextHref == "" {
		return "", nil
	}

	base, err := url.Parse(currentURL)
	if err != nil {
		return "", fmt.Errorf("URL inválida %s: %w", currentURL, err)
	}
	rel, err := url.Parse(nextHref)
	if err != nil {
		return "", fmt.Errorf("href inválido %s: %w", nextHref, err)
	}
	return base.ResolveReference(rel).String(), nil
}

func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, token := range strings.Fields(a.Val) {
				if token == class {
					return true
				}
			}
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func main() {
	ctx := context.Background()
	crawler := NewCrawler("books.json", "crawler_state.json")
	startURL := "http://books.toscrape.com/index.html"

	books, err := crawler.Crawl(ctx, startURL)
	if err != nil {
		log.Fatalf("crawler falhou: %v", err)
	}

	log.Printf("total de livros coletados: %d", len(books))
	for _, book := range books {
		fmt.Printf("Título: %-60s | Preço: %-10s | Avaliação: %s\n", book.Title, book.Price, book.Rating)
	}
}
