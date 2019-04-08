package sitemap

import (
	"log"
	"sync"
	"time"
	"net/http"
	"io/ioutil"
	"regexp"
	"net/url"
	"strings"
	"errors"
	"fmt"
	"os"
)

func NewSitemapGenerator(parallelWorkersCount int, outputFilePath string, maxDepth int, startUrl string) *SitemapGenerator{
		siteMapGenerator := &SitemapGenerator{
			parallelWorkersCount:parallelWorkersCount,
			outputFilePath:outputFilePath,
			maxDepth:maxDepth,
			startUrl:startUrl,
			httpTimeout: time.Second * 4,
			visitedUrl: make(map[string]bool),
			visitedUrlMutex: sync.RWMutex{},
		}

		return siteMapGenerator
}

type SitemapGenerator struct {
	parallelWorkersCount int
	outputFilePath string
	maxDepth int
	startUrl string
	httpTimeout time.Duration
	siteDomain string
	siteProtocol string
	visitedUrl map[string]bool //if site is too big we can use hash instead of string for less using memory
	visitedUrlMutex sync.RWMutex
}


func (sitemapGenerator *SitemapGenerator) Run() {
	log.Print("Starting building sitemap for " + sitemapGenerator.startUrl)
	parsedUrls := make(chan string, 2024)
	jobs := make(chan workerJob, 1024)

	var wg sync.WaitGroup

	for  i:=0; i < sitemapGenerator.parallelWorkersCount; i++ {
		wg.Add(1)
		go sitemapGenerator.worker(i, jobs, parsedUrls, &wg)
	}

	u, err := url.Parse(sitemapGenerator.startUrl)
	if err != nil {
		panic(err)
	}

	sitemapGenerator.siteDomain = u.Host
	sitemapGenerator.siteProtocol = u.Scheme

	newJob := workerJob{
		currentDepth: 0,
		url:sitemapGenerator.startUrl,
	}

	jobs<-newJob

	var wgSaver sync.WaitGroup

	wgSaver.Add(1)
	go sitemapGenerator.urlSaver(parsedUrls, &wgSaver)

	wg.Wait()
	log.Printf("Closing job channel")
	close(jobs)

	close(parsedUrls)
	wgSaver.Wait()
	log.Printf("Closing parsedUrl channel")
}




/**
Private
 */


type workerJob struct {
	url string
	currentDepth int
}

type workerContext struct {
	netClient *http.Client
	workerId int
	linkRegexp *regexp.Regexp
	baseRegexp *regexp.Regexp
}

func (sitemapGenerator *SitemapGenerator) worker(workerId int, channelWorkerJob chan workerJob, parsedUrls chan <- string, wg *sync.WaitGroup)  {
	var netClient = &http.Client{
		Timeout: sitemapGenerator.httpTimeout,
	}
	tries := 0
	maxTryCount := 2

	workerContextObj := workerContext{
		linkRegexp: regexp.MustCompile(`<a[^>]+\bhref=["']([^"']+)["']`),
		baseRegexp: regexp.MustCompile(`<base[^>]+\bhref=["']([^"']+)["']`),
		netClient: netClient,
	}

	for {
		select {
			case job,more := <-channelWorkerJob:
				if job.currentDepth > sitemapGenerator.maxDepth {
					log.Printf("worker(%d): max depth reached\n", workerId)
					continue
				}

				if more {

					if sitemapGenerator.isLinkVisited(job.url) {
						continue
					}

					tries = 0
					links := sitemapGenerator.handlePage(workerContextObj, job.url)
					sitemapGenerator.markLinkAsVisited(job.url)
					log.Printf("worker(%d): parse %s\n", workerId, job.url)

					for i:= range links {
						link := links[i]

						if sitemapGenerator.isLinkVisited(link) {
							continue
						}

						newJob := workerJob{
							currentDepth: job.currentDepth + 1,
							url:link,
						}


						go func() {
							parsedUrls <- link
						}()

						go func() {
							channelWorkerJob <- newJob
						}()
					}

				} else {
					wg.Done()
					log.Printf("worker(%d): done\n", workerId)
					return
				}
			case <-time.After(sitemapGenerator.httpTimeout):
				tries++

				if tries > maxTryCount {
					wg.Done()
					return
				}
		}
	}
}

func (sitemapGenerator *SitemapGenerator) handlePage(workerContextObj workerContext, url string) []string {
	response, err := workerContextObj.netClient.Get(url)

	if err != nil {
		log.Printf("worker(%d): can't get response %s\n", workerContextObj.workerId, err)
		return nil
	}

	buf, _ := ioutil.ReadAll(response.Body)
	strHtml := string(buf)
	links := sitemapGenerator.parseLinks(workerContextObj, strHtml, url)
	return links
}

func (sitemapGenerator *SitemapGenerator) parseLinks(workerContextObj workerContext, strHtml string, currentUrl string) []string {
	var baseTags = workerContextObj.baseRegexp.FindAllStringSubmatch(strHtml, -1)

	if len(baseTags) > 0 {
		currentUrl = baseTags[0][1]
	}

	var linkTags = workerContextObj.linkRegexp.FindAllStringSubmatch(strHtml, -1)
	var out []string
	for i := range linkTags {
		normalizedLink, err := sitemapGenerator.normalizeLink(linkTags[i][1], currentUrl)

		if err == nil {
			out = append(out, normalizedLink)
		}
	}

	return out
}

func (sitemapGenerator *SitemapGenerator) normalizeLink(link string, currentUrl string) (string,error) {
	if link[0] == '/' {
		return sitemapGenerator.siteProtocol + "://" + sitemapGenerator.siteDomain + link,nil
	} else if strings.Index(link, "http") == 0 {
		u, err := url.Parse(link)

		if err != nil {
			return "", err
		}

		if u.Host != sitemapGenerator.siteDomain {
			return "", errors.New("foreign domain, skip")
		}

		return link, nil
	} else {
		ind := strings.LastIndex(currentUrl, "/")
		parentLevelUrl := currentUrl[:ind]
		return parentLevelUrl + link, nil
	}
}

func (sitemapGenerator *SitemapGenerator) urlSaver(parsedUrls <-chan string, wg *sync.WaitGroup){
	f, err := os.Create(sitemapGenerator.outputFilePath)

	if err != nil {
		log.Println(err)
		return
	}

	fmt.Fprint(f, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	fmt.Fprint(f, "<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")

	strFmt := "\t<url>\n\t\t<loc>%s</loc>\n\t</url>\n"
	addedLinks := make(map[string]bool)

	for {
		parsedUrl, more := <-parsedUrls

		if more {
			if _, ok := addedLinks[parsedUrl]; ok {
				continue
			}

			fmt.Fprintf(f, strFmt, parsedUrl)
			addedLinks[parsedUrl] = true
			log.Printf("Added %s", parsedUrl)
		} else {
			fmt.Fprint(f, "</urlset>\n")
			err = f.Close()

			if err != nil {
				wg.Done()
				log.Println(err)
				return
			}

			wg.Done()
			return
		}
	}
}

func (sitemapGenerator *SitemapGenerator) isLinkVisited(link string) bool {
	sitemapGenerator.visitedUrlMutex.RLock()

	if _, ok := sitemapGenerator.visitedUrl[link]; ok {
		sitemapGenerator.visitedUrlMutex.RUnlock()
		return true
	} else {
		sitemapGenerator.visitedUrlMutex.RUnlock()
		return false
	}
}

func (sitemapGenerator *SitemapGenerator) markLinkAsVisited(link string)  {
	sitemapGenerator.visitedUrlMutex.Lock()
	sitemapGenerator.visitedUrl[link] = true
	sitemapGenerator.visitedUrlMutex.Unlock()
}