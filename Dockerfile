FROM golang:1.14-alpine AS dependencies

RUN apk add --no-cache \
    git

WORKDIR /tmp/build

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN go build -o ./out/resizer .

FROM alpine

COPY --from=dependencies /tmp/build/out/resizer /app/resizer

EXPOSE 3000

CMD ["/app/resizer"]