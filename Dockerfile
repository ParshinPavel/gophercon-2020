FROM golang:1.13.5-alpine3.10 as builder

RUN apk --update add git less openssh && \
    rm -rf /var/lib/apt/lists/* && \
    rm /var/cache/apk/*

WORKDIR /gophercon-2020
COPY go.mod go.sum ./

RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/gophercon-2020 main.go


FROM alpine:3.10

WORKDIR /usr/bin
COPY --from=builder /gophercon-2020/bin/gophercon-2020 ./
RUN chmod +x ./gophercon-2020

ENTRYPOINT ["/usr/bin/gophercon-2020"]
