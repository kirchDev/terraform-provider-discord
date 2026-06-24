BINARY := terraform-provider-discord

default: build

build:
	go build -o $(BINARY)

install:
	go install

tidy:
	go mod tidy

fmt:
	gofmt -s -w .

vet:
	go vet ./...

lint:
	golangci-lint run

generate:
	go generate ./...

# Generate docs/ from the provider schema (build + schema export + tfplugindocs).
docs:
	bash scripts/gen-docs.sh

test:
	go test ./... -timeout 120s

# Acceptance tests drive the provider through a full CRUD/import cycle against an
# in-memory mock of the Discord API — no token. Needs a TF binary (tofu/terraform);
# the harness auto-discovers tofu on PATH, or set TF_ACC_TERRAFORM_PATH.
testacc:
	TF_ACC=1 go test ./... -v -timeout 120m

.PHONY: default build install tidy fmt vet lint generate docs test testacc
