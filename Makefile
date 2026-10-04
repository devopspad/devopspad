.PHONY: dev preview build test integration

dev:
	wails dev

preview:
	npm run dev --prefix frontend

build:
	wails build

test:
	npm run build --prefix frontend
	go test -race -timeout 60s ./...
	go vet ./...
	npm run test:e2e --prefix frontend

integration:
	python3 tests/integration/run.py
