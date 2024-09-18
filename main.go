package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var visited = make(map[string]bool)
var finalURLs = make([]string, 0)
var mu sync.Mutex // Pour protéger les accès concurrents à visited et finalURLs

const maxURLs = 1000  // Limite du nombre d'URLs à trouver
const maxWorkers = 10 // Nombre maximum de "tabs" ou de travailleurs
const maxDepth = 5    // Profondeur maximale de crawl

var queue = make(chan Task, maxURLs) // File d'attente d'URLs avec profondeur

type Task struct {
	URL   string
	Depth int
}

// Worker function
func worker(id int, jobs <-chan Task, wg *sync.WaitGroup) {
	defer wg.Done()
	for task := range jobs {
		if task.Depth > maxDepth {
			continue
		}

		fmt.Printf("Worker %d: Processing URL: %s (Depth %d)\n", id, task.URL, task.Depth)
		fetch(task.URL, task.Depth)
		time.Sleep(3 * time.Second) // Attente pour éviter l'erreur 429
	}
}

// Fonction pour récupérer et traiter une URL
func fetch(urlStr string, depth int) {
	mu.Lock()
	if visited[urlStr] || len(finalURLs) >= maxURLs {
		mu.Unlock()
		return
	}
	visited[urlStr] = true // Marquer l'URL comme visitée
	mu.Unlock()

	// Récupérer l'URL
	res, err := http.Get(urlStr)
	if err != nil {
		log.Println(err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Printf("Error: Status code %d while fetching %s\n", res.StatusCode, urlStr)
		return
	}

	// Analyser la page
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Println(err)
		return
	}

	// Trouver tous les liens et les ajouter à la file d'attente
	doc.Find("a").Each(func(index int, item *goquery.Selection) {
		if len(finalURLs) >= maxURLs {
			return // Stop si on atteint la limite
		}

		link, exists := item.Attr("href")
		if exists {
			absoluteURL := resolveURL(urlStr, link)
			if strings.HasPrefix(absoluteURL, "http") && !strings.Contains(absoluteURL, "#") {
				mu.Lock()
				if !visited[absoluteURL] {
					finalURLs = append(finalURLs, absoluteURL) // Ajouter à la liste finale
					fmt.Println("URL found:", absoluteURL)
					// Ajouter l'URL et sa profondeur à la file d'attente
					queue <- Task{URL: absoluteURL, Depth: depth + 1}
				}
				mu.Unlock()
			}
		}
	})
}

// Fonction pour résoudre les URLs relatives en URLs absolues
func resolveURL(base, href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return baseURL.ResolveReference(u).String()
}

// Fonction principale
func main() {
	startURL := "https://www.kodoka.fr/index.php"

	var wg sync.WaitGroup

	// Lancer 5 travailleurs (goroutines)
	for i := 1; i <= maxWorkers; i++ {
		wg.Add(1)
		go worker(i, queue, &wg)
	}

	// Ajouter l'URL de départ à la file d'attente avec profondeur 0
	queue <- Task{URL: startURL, Depth: 0}

	// Fermer la file d'attente une fois que l'URL de départ a été ajoutée
	go func() {
		wg.Wait()
		close(queue)
	}()

	// Attendre que tous les travailleurs aient fini
	wg.Wait()

	fmt.Println("Crawling finished. Found", len(finalURLs), "URLs.")
	fmt.Println("Final URLs:")
}
