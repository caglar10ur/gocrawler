all:
	go build crawl.go
format:
	gofmt -s -w -tabs=false -tabwidth=4 crawl.go
clean:
	rm -f crawl
