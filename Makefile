build-macos:
	mkdir -p build
	GOOS=darwin go build -o build/autobgp autobgp

build-linux:
	mkdir -p build
	GOOS=linux GOARCH=amd64 go build -o build/autobgp autobgp
