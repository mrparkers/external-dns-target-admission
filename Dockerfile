FROM golang:1.13.6-alpine as builder

WORKDIR /usr/src/app/

COPY go.mod go.sum /usr/src/app/
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go test
RUN CGO_ENABLED=0 GOOS=linux go build -o external-dns-target-admission

FROM alpine:3.11.3

COPY --from=builder /usr/src/app/external-dns-target-admission .

ENTRYPOINT ["external-dns-target-admission"]
