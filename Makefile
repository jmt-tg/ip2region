linux:
	GOOS=linux GOARCH=amd64 go build -o bin/linux-amd64/ip2region -ldflags "-s -w" .
	upx -9 bin/linux-amd64/ip2region
	echo "大小 `du -sh bin/linux-amd64/ip2region`"

docker:
	cd deploy && \
	docker build --platform linux/amd64 -t ip2region:latest .
