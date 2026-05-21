FROM golang:1.24

WORKDIR /app
COPY . .
RUN go build -o verifiable-chain main.go
ENTRYPOINT ["/app/verifiable-chain"]
