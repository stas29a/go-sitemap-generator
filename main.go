package main

import (
	"./sitemap"
	"flag"
)

func main() {
	parallel := flag.Int("parallel", 3, "number of parallel workers")
	xmlFilePath := flag.String("output-file", "/var/www/sitemap.xml", "output file path")
	maxDepth := flag.Int("max-depth", 3, "max depth of url navigation recursion")
	startUrl := flag.String("start-url", "https://habr.com", "start url to navigate")

	flag.Parse()

	sitemapGenerator := sitemap.NewSitemapGenerator(*parallel, *xmlFilePath, *maxDepth, *startUrl)
	sitemapGenerator.Run()
}
