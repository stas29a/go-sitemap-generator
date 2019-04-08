# Sitemap generator

Simple sitemap generator as command line tool. <br>

### It can:<br>
* accept start url as argument<br>
* recursively navigate by site pages in parallel<br> 
* not use any external dependencies, only standard golang library<br>
* extract page urls only from `<a>` elements and take in account `<base>` element if declared
<br>

### Suggested program options:<br>
_--start-url=_             start url for parsing<br>
_--parallel=_  			number of parallel workers to navigate through site<br>
_--output-file=_			output file path<br>
_--max-depth=_ 			max depth of url navigation recursion<br>

