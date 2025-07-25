build:
	go build -o logdog cmd/main.go

cp:
	cp logdog ~/.local/bin/

install: build cp